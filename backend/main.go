package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	xproxy "golang.org/x/net/proxy"
)

type Source string

const (
	SourceWeibo    Source = "weibo"
	SourcePixiv    Source = "pixiv"
	SourceBilibili Source = "bilibili"
)

type Post struct {
	ID        string    `json:"id"`
	Source    Source    `json:"source"`
	Author    string    `json:"author"`
	Avatar    string    `json:"avatar"`
	Caption   string    `json:"caption"`
	Tags      []string  `json:"tags"`
	Media     []string  `json:"media"`
	Published time.Time `json:"published"`
	Liked     bool      `json:"liked"`
}

type SourceConfig struct {
	ID              string    `json:"id"`
	Source          Source    `json:"source"`
	Name            string    `json:"name"`
	Handle          string    `json:"handle"`
	Avatar          string    `json:"avatar,omitempty"`
	Enabled         bool      `json:"enabled"`
	IncludePast     bool      `json:"includePast"`
	Schedule        string    `json:"schedule"`
	ContentTypes    []string  `json:"contentTypes,omitempty"`
	LastSyncedAt    time.Time `json:"lastSyncedAt"`
	LastSyncStatus  string    `json:"lastSyncStatus,omitempty"`
	LastSyncMessage string    `json:"lastSyncMessage,omitempty"`
	LastSyncCount   int       `json:"lastSyncCount,omitempty"`
	StoragePath     string    `json:"storagePath,omitempty"`
}

type BilibiliCredentials struct {
	Cookie          string `json:"cookie,omitempty"`
	SESSDATA        string `json:"SESSDATA"`
	BiliJCT         string `json:"bili_jct"`
	Buvid3          string `json:"buvid3"`
	DedeUserID      string `json:"DedeUserID"`
	AccessTimeValue string `json:"ac_time_value,omitempty"`
	Buvid4          string `json:"buvid4,omitempty"`
	DedeUserIDCKMd5 string `json:"DedeUserID__ckMd5,omitempty"`
}

type BilibiliAccount struct {
	Configured bool   `json:"configured"`
	UserID     string `json:"userId,omitempty"`
}

type PixivCredentials struct {
	RefreshToken string `json:"refreshToken,omitempty"`
	UserID       string `json:"userId,omitempty"`
	UserName     string `json:"userName,omitempty"`
}

type WeiboCredentials struct {
	Cookie   string `json:"cookie,omitempty"`
	UserID   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
}

type BilibiliQRSession struct {
	ID        string    `json:"id"`
	Key       string    `json:"-"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type WeiboQRSession struct {
	ID         string    `json:"id"`
	QRID       string    `json:"-"`
	Image      string    `json:"image,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Exchanging bool      `json:"-"`
}

type BilibiliConfig struct {
	Credentials        BilibiliCredentials `json:"credentials"`
	Pixiv              PixivCredentials    `json:"pixiv"`
	Weibo              WeiboCredentials    `json:"weibo"`
	Subscriptions      []SourceConfig      `json:"subscriptions"`
	WeiboSubscriptions []SourceConfig      `json:"weiboSubscriptions"`
	ProxyURL           string              `json:"proxyUrl,omitempty"`
}

type ProjectSettingsView struct {
	ProxyEnabled bool   `json:"proxyEnabled"`
	ProxyURL     string `json:"proxyUrl,omitempty"`
}

type BilibiliStore struct {
	sync.RWMutex
	config          BilibiliConfig
	key             []byte
	content         *Store
	bilibiliQR      map[string]BilibiliQRSession
	bilibiliClients map[string]*http.Client
	weiboQR         map[string]WeiboQRSession
	weiboClients    map[string]*http.Client
}

type BilibiliUser struct {
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Fans        int64  `json:"fans"`
	Description string `json:"description"`
}

type WeiboUser struct {
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Fans        int64  `json:"fans"`
	Description string `json:"description"`
}

type Store struct {
	sync.RWMutex
	posts []Post
	feeds []SourceConfig
	file  string
}

type ContentData struct {
	Posts []Post         `json:"posts"`
	Feeds []SourceConfig `json:"feeds"`
}

type SessionStore struct {
	sync.RWMutex
	tokens map[string]time.Time
}

type AuthConfig struct {
	sync.RWMutex
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

var authFile = "/data/auth.json"
var contentFile = "/data/content.json"
var bilibiliFile = "/data/bilibili.enc"
var secretFile = "/data/secret.key"
var flowRoot = "/flow"

func loadAuthConfig() *AuthConfig {
	if value := os.Getenv("LUMIC_AUTH_FILE"); value != "" {
		authFile = value
	}
	data, err := os.ReadFile(authFile)
	if err == nil {
		var config AuthConfig
		if json.Unmarshal(data, &config) == nil && config.Username != "" && config.PasswordHash != "" {
			return &config
		}
	}
	return &AuthConfig{Username: "Lumic", PasswordHash: hashPassword("Lumic", []byte("lumic-default-salt-v1"))}
}

func (a *AuthConfig) save() error {
	if err := os.MkdirAll(filepath.Dir(authFile), 0700); err != nil {
		return err
	}
	a.RLock()
	data, err := json.Marshal(struct {
		Username     string `json:"username"`
		PasswordHash string `json:"passwordHash"`
	}{a.Username, a.PasswordHash})
	a.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(authFile, data, 0600)
}

func loadOrCreateSecret() ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(secretFile), 0700); err != nil {
		return nil, err
	}
	if key, err := os.ReadFile(secretFile); err == nil && len(key) == 32 {
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(secretFile, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptJSON(key []byte, value any) ([]byte, error) {
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, []byte("lumic-bilibili-v1")), nil
}

func decryptJSON(key, encrypted []byte, value any) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(encrypted) < gcm.NonceSize() {
		return errors.New("invalid encrypted configuration")
	}
	nonce, ciphertext := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte("lumic-bilibili-v1"))
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, value)
}

func loadBilibiliStore() (*BilibiliStore, error) {
	if value := os.Getenv("LUMIC_BILIBILI_FILE"); value != "" {
		bilibiliFile = value
	}
	if value := os.Getenv("LUMIC_SECRET_FILE"); value != "" {
		secretFile = value
	}
	key, err := loadOrCreateSecret()
	if err != nil {
		return nil, err
	}
	store := &BilibiliStore{key: key, config: BilibiliConfig{Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}}, bilibiliQR: make(map[string]BilibiliQRSession), bilibiliClients: make(map[string]*http.Client), weiboQR: make(map[string]WeiboQRSession), weiboClients: make(map[string]*http.Client)}
	if data, err := os.ReadFile(bilibiliFile); err == nil {
		if err := decryptJSON(key, data, &store.config); err != nil {
			return nil, fmt.Errorf("decrypt bilibili configuration: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if store.config.Subscriptions == nil {
		store.config.Subscriptions = []SourceConfig{}
	}
	if store.config.WeiboSubscriptions == nil {
		store.config.WeiboSubscriptions = []SourceConfig{}
	}
	return store, nil
}

func initializeFlowStorage() error {
	if value := strings.TrimSpace(os.Getenv("LUMIC_FLOW_ROOT")); value != "" {
		flowRoot = value
	}
	for _, source := range []Source{SourceBilibili, SourcePixiv, SourceWeibo} {
		if err := os.MkdirAll(filepath.Join(flowRoot, string(source)), 0755); err != nil {
			return fmt.Errorf("create %s flow directory: %w", source, err)
		}
	}
	return nil
}

func safeFlowDirectoryName(name string) string {
	name = strings.TrimSpace(name)
	var builder strings.Builder
	for _, char := range name {
		if char < 32 || strings.ContainsRune(`<>:"/\\|?*`, char) {
			builder.WriteRune('_')
		} else {
			builder.WriteRune(char)
		}
	}
	name = strings.Trim(builder.String(), " .")
	if name == "" || name == "." || name == ".." {
		name = "unnamed"
	}
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true, "LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}
	if reserved[strings.ToUpper(name)] {
		name = "_" + name
	}
	return name
}

func sourceStoragePath(source Source, name string) string {
	return filepath.Join(flowRoot, string(source), safeFlowDirectoryName(name))
}

func flowPublicPath(source Source, author, name string) string {
	return "/flow/" + url.PathEscape(string(source)) + "/" + url.PathEscape(safeFlowDirectoryName(author)) + "/" + url.PathEscape(name)
}

func prepareSourceStorage(feed SourceConfig) (SourceConfig, error) {
	feed.StoragePath = sourceStoragePath(feed.Source, feed.Name)
	if err := os.MkdirAll(feed.StoragePath, 0755); err != nil {
		return feed, err
	}
	metadata, err := json.MarshalIndent(struct {
		Source       Source   `json:"source"`
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Handle       string   `json:"handle"`
		Avatar       string   `json:"avatar,omitempty"`
		ContentTypes []string `json:"contentTypes,omitempty"`
	}{feed.Source, feed.ID, feed.Name, feed.Handle, feed.Avatar, feed.ContentTypes}, "", "  ")
	if err != nil {
		return feed, err
	}
	temporary := filepath.Join(feed.StoragePath, ".source.json.tmp")
	if err := os.WriteFile(temporary, metadata, 0644); err != nil {
		return feed, err
	}
	if err := os.Rename(temporary, filepath.Join(feed.StoragePath, "source.json")); err != nil {
		_ = os.Remove(temporary)
		return feed, err
	}
	return feed, nil
}

func (b *BilibiliStore) reconcileFlowStorage() error {
	b.Lock()
	defer b.Unlock()
	lists := []*[]SourceConfig{&b.config.Subscriptions, &b.config.WeiboSubscriptions}
	for _, list := range lists {
		for index, feed := range *list {
			prepared, err := prepareSourceStorage(feed)
			if err != nil {
				return fmt.Errorf("prepare flow storage for %s: %w", feed.Name, err)
			}
			(*list)[index] = prepared
		}
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lumic-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err == nil {
		return nil
	}
	// Windows 不允许 Rename 覆盖已有文件；数据已完整同步到临时文件后再替换。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryName, path)
}

func (b *BilibiliStore) save() error {
	b.RLock()
	data, err := encryptJSON(b.key, b.config)
	b.RUnlock()
	if err != nil {
		return err
	}
	return atomicWriteFile(bilibiliFile, data, 0600)
}

func (s *SessionStore) create() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	s.Lock()
	s.tokens[token] = time.Now().Add(12 * time.Hour)
	s.Unlock()
	return token, nil
}

func (s *SessionStore) valid(token string) bool {
	if token == "" {
		return false
	}
	s.Lock()
	defer s.Unlock()
	expires, ok := s.tokens[token]
	if !ok || time.Now().After(expires) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func hashPassword(password string, salt []byte) string {
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(hash)
}

func passwordMatches(password, encoded string) bool {
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return false
	}
	salt := []byte("lumic-default-salt-v1")
	actual := hashPassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(encoded)) == 1
}

func loginHandler(sessions *SessionStore, auth *AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		auth.RLock()
		username, storedHash := auth.Username, auth.PasswordHash
		auth.RUnlock()
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || subtle.ConstantTimeCompare([]byte(input.Username), []byte(username)) != 1 || !passwordMatches(input.Password, storedHash) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		token, err := sessions.create()
		if err != nil {
			http.Error(w, "unable to create session", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "lumic_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: os.Getenv("LUMIC_COOKIE_SECURE") == "true", MaxAge: 43200})
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func settingsHandler(auth *AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			auth.RLock()
			writeJSON(w, map[string]string{"username": auth.Username})
			auth.RUnlock()
			return
		}
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Username        string `json:"username"`
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || len(input.Username) < 3 || len(input.NewPassword) < 8 {
			http.Error(w, "invalid settings", http.StatusBadRequest)
			return
		}
		auth.RLock()
		matches := passwordMatches(input.CurrentPassword, auth.PasswordHash)
		auth.RUnlock()
		if !matches {
			http.Error(w, "invalid current password", http.StatusUnauthorized)
			return
		}
		auth.Lock()
		auth.Username = input.Username
		auth.PasswordHash = hashPassword(input.NewPassword, []byte("lumic-default-salt-v1"))
		auth.Unlock()
		if err := auth.save(); err != nil {
			http.Error(w, "unable to save settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func passwordHash() string {
	password := os.Getenv("LUMIC_PASSWORD")
	if password == "" {
		password = "Lumic"
	}
	return hashPassword(password, []byte("lumic-default-salt-v1"))
}

func authMiddleware(sessions *SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" || r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("lumic_session")
		if err != nil || !sessions.valid(cookie.Value) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func demoStore() *Store {
	now := time.Now()
	return &Store{
		posts: []Post{
			{ID: "wb-001", Source: SourceWeibo, Author: "林间拾光", Avatar: "https://i.pravatar.cc/96?img=47", Caption: "把黄昏收藏进今天的相册里。风经过树梢，带来一点夏天的回声。", Tags: []string{"日常", "摄影"}, Published: now.Add(-42 * time.Minute), Liked: true},
			{ID: "px-002", Source: SourcePixiv, Author: "Aoi Sora", Avatar: "https://i.pravatar.cc/96?img=32", Caption: "新作｜雨后的玻璃温室，想画出潮湿空气里柔软的光。", Tags: []string{"原创", "插画", "光影"}, Published: now.Add(-2 * time.Hour), Media: []string{"https://images.unsplash.com/photo-1518709268805-4e9042af9f23?w=900&q=80"}},
			{ID: "bl-003", Source: SourceBilibili, Author: "慢慢生活研究所", Avatar: "https://i.pravatar.cc/96?img=12", Caption: "一期关于城市散步的记录，和你分享最近发现的三家小店。", Tags: []string{"VLOG", "城市漫游"}, Published: now.Add(-5 * time.Hour)},
			{ID: "wb-004", Source: SourceWeibo, Author: "山茶花开时", Avatar: "https://i.pravatar.cc/96?img=5", Caption: "今日份蓝色时刻。愿我们都能留住一些不被打扰的浪漫。", Tags: []string{"随手拍", "生活"}, Published: now.Add(-8 * time.Hour), Liked: true},
			{ID: "px-005", Source: SourcePixiv, Author: "Mori", Avatar: "https://i.pravatar.cc/96?img=20", Caption: "夏日习作 #sketch #summer", Tags: []string{"sketch", "summer"}, Published: now.Add(-24 * time.Hour)},
		},
		feeds: []SourceConfig{
			{ID: "feed-1", Source: SourceWeibo, Name: "我的点赞", Handle: "个人账号", Enabled: true, IncludePast: false, Schedule: "每 6 小时", LastSyncedAt: now.Add(-42 * time.Minute)},
			{ID: "feed-2", Source: SourceWeibo, Name: "建筑可视化研究所", Handle: "@arch_visual", Enabled: true, Schedule: "每天 09:00", LastSyncedAt: now.Add(-3 * time.Hour)},
			{ID: "feed-3", Source: SourcePixiv, Name: "Aoi Sora", Handle: "@aoisora", Enabled: true, Schedule: "每 12 小时", LastSyncedAt: now.Add(-2 * time.Hour)},
			{ID: "feed-4", Source: SourceBilibili, Name: "慢慢生活研究所", Handle: "UID 184729", Enabled: false, Schedule: "每天 20:00", LastSyncedAt: now.Add(-24 * time.Hour)},
		},
	}
}

func loadStore() (*Store, error) {
	if value := strings.TrimSpace(os.Getenv("LUMIC_CONTENT_FILE")); value != "" {
		contentFile = value
	}
	return loadStoreFile(contentFile)
}

func loadStoreFile(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var content ContentData
		if err := json.Unmarshal(data, &content); err != nil {
			return nil, fmt.Errorf("decode content data: %w", err)
		}
		if content.Posts == nil {
			content.Posts = []Post{}
		}
		if content.Feeds == nil {
			content.Feeds = []SourceConfig{}
		}
		return &Store{posts: content.Posts, feeds: content.Feeds, file: path}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	store := demoStore()
	store.file = path
	store.Lock()
	err = store.saveLocked()
	store.Unlock()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) saveLocked() error {
	if s.file == "" {
		return nil
	}
	data, err := json.Marshal(ContentData{Posts: s.posts, Feeds: s.feeds})
	if err != nil {
		return err
	}
	return atomicWriteFile(s.file, data, 0600)
}

func (s *Store) mergePosts(incoming []Post) (int, error) {
	if len(incoming) == 0 {
		return 0, nil
	}
	s.Lock()
	defer s.Unlock()
	indexes := make(map[string]int, len(s.posts)+len(incoming))
	for index, post := range s.posts {
		indexes[post.ID] = index
	}
	previous := append([]Post(nil), s.posts...)
	added, changed := 0, false
	for _, post := range incoming {
		if post.ID == "" {
			continue
		}
		if index, exists := indexes[post.ID]; exists {
			post.Liked = s.posts[index].Liked
			if !postsEqual(s.posts[index], post) {
				s.posts[index] = post
				changed = true
			}
			continue
		}
		indexes[post.ID] = len(s.posts)
		s.posts = append(s.posts, post)
		added++
		changed = true
	}
	if !changed {
		return 0, nil
	}
	sort.SliceStable(s.posts, func(i, j int) bool { return s.posts[i].Published.After(s.posts[j].Published) })
	if err := s.saveLocked(); err != nil {
		s.posts = previous
		return 0, err
	}
	return added, nil
}

func postsEqual(left, right Post) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func deletePostMedia(media []string) error {
	root, err := filepath.Abs(flowRoot)
	if err != nil {
		return err
	}
	for _, raw := range media {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		parsed, parseErr := url.Parse(value)
		if parseErr == nil && parsed.Scheme != "" && parsed.Scheme != "file" {
			continue
		}
		candidate := value
		if parseErr == nil && parsed.Scheme == "file" {
			candidate = parsed.Path
		}
		if strings.HasPrefix(filepath.ToSlash(candidate), "/flow/") {
			candidate = filepath.Join(flowRoot, strings.TrimPrefix(filepath.ToSlash(candidate), "/flow/"))
		}
		absolute, err := filepath.Abs(filepath.FromSlash(candidate))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(absolute); err == nil {
			if info.IsDir() {
				continue
			}
			if err := os.Remove(absolute); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *Store) postsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			writeAPIError(w, http.StatusBadRequest, "动态 ID 不能为空")
			return
		}
		s.Lock()
		for index, post := range s.posts {
			if post.ID == id {
				if err := deletePostMedia(post.Media); err != nil {
					s.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法删除动态关联媒体文件")
					return
				}
				previous := s.posts
				s.posts = append(append([]Post(nil), s.posts[:index]...), s.posts[index+1:]...)
				if err := s.saveLocked(); err != nil {
					s.posts = previous
					s.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化动态删除")
					return
				}
				s.Unlock()
				writeJSON(w, map[string]string{"status": "deleted", "message": "动态及关联媒体文件已永久删除"})
				return
			}
		}
		s.Unlock()
		writeAPIError(w, http.StatusNotFound, "动态不存在或已删除")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.RLock()
	result := append([]Post(nil), s.posts...)
	s.RUnlock()
	if source := r.URL.Query().Get("source"); source != "" && source != "all" {
		filtered := result[:0]
		for _, post := range result {
			if string(post.Source) == source {
				filtered = append(filtered, post)
			}
		}
		result = filtered
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Published.After(result[j].Published) })
	writeJSON(w, result)
}

func (s *Store) feedsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		s.Lock()
		for index, feed := range s.feeds {
			if feed.ID == id {
				previous := s.feeds
				s.feeds = append(append([]SourceConfig(nil), s.feeds[:index]...), s.feeds[index+1:]...)
				if err := s.saveLocked(); err != nil {
					s.feeds = previous
					s.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化订阅删除")
					return
				}
				s.Unlock()
				writeJSON(w, map[string]string{"status": "deleted", "message": "订阅已永久删除，已采集文件予以保留"})
				return
			}
		}
		s.Unlock()
		writeAPIError(w, http.StatusNotFound, "来源不存在")
		return
	}
	if r.Method == http.MethodPost && r.URL.Query().Get("action") == "sync" {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		s.Lock()
		for index := range s.feeds {
			if s.feeds[index].ID == id {
				s.feeds[index].LastSyncedAt = time.Now()
				feed := s.feeds[index]
				if err := s.saveLocked(); err != nil {
					s.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法保存同步状态")
					return
				}
				s.Unlock()
				writeJSON(w, map[string]any{"status": "completed", "message": "来源状态已更新；该来源暂无在线采集器", "source": feed})
				return
			}
		}
		s.Unlock()
		writeAPIError(w, http.StatusNotFound, "来源不存在")
		return
	}
	if r.Method == http.MethodPost {
		var feed SourceConfig
		if err := json.NewDecoder(r.Body).Decode(&feed); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		feed.ID = "feed-" + time.Now().Format("150405")
		feed.Enabled = true
		s.Lock()
		s.feeds = append(s.feeds, feed)
		if err := s.saveLocked(); err != nil {
			s.feeds = s.feeds[:len(s.feeds)-1]
			s.Unlock()
			writeAPIError(w, http.StatusInternalServerError, "无法保存新来源")
			return
		}
		s.Unlock()
		writeJSON(w, feed)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.RLock()
	result := append([]SourceConfig(nil), s.feeds...)
	s.RUnlock()
	writeJSON(w, result)
}

func cleanCookieValue(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, "\"'")
}

func cookieValue(rawCookie, name string) string {
	for _, part := range strings.Split(rawCookie, ";") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) == 2 && strings.EqualFold(strings.TrimSpace(pair[0]), name) {
			return cleanCookieValue(pair[1])
		}
	}
	return ""
}

func normalizeBilibiliCredentials(credentials BilibiliCredentials) BilibiliCredentials {
	credentials.Cookie = strings.TrimSpace(credentials.Cookie)
	credentials.SESSDATA = cleanCookieValue(credentials.SESSDATA)
	credentials.BiliJCT = cleanCookieValue(credentials.BiliJCT)
	credentials.Buvid3 = cleanCookieValue(credentials.Buvid3)
	credentials.DedeUserID = cleanCookieValue(credentials.DedeUserID)
	credentials.AccessTimeValue = cleanCookieValue(credentials.AccessTimeValue)
	credentials.Buvid4 = cleanCookieValue(credentials.Buvid4)
	credentials.DedeUserIDCKMd5 = cleanCookieValue(credentials.DedeUserIDCKMd5)
	if credentials.Cookie != "" {
		credentials.SESSDATA = cookieValue(credentials.Cookie, "SESSDATA")
		credentials.BiliJCT = cookieValue(credentials.Cookie, "bili_jct")
		credentials.Buvid3 = cookieValue(credentials.Cookie, "buvid3")
		credentials.DedeUserID = cookieValue(credentials.Cookie, "DedeUserID")
		credentials.AccessTimeValue = cookieValue(credentials.Cookie, "ac_time_value")
		credentials.Buvid4 = cookieValue(credentials.Cookie, "buvid4")
		credentials.DedeUserIDCKMd5 = cookieValue(credentials.Cookie, "DedeUserID__ckMd5")
	}
	return credentials
}

func bilibiliCookie(credentials BilibiliCredentials) string {
	if strings.TrimSpace(credentials.Cookie) != "" {
		return strings.TrimSpace(credentials.Cookie)
	}
	values := [][2]string{{"SESSDATA", credentials.SESSDATA}, {"bili_jct", credentials.BiliJCT}, {"buvid3", credentials.Buvid3}, {"DedeUserID", credentials.DedeUserID}, {"ac_time_value", credentials.AccessTimeValue}, {"buvid4", credentials.Buvid4}, {"DedeUserID__ckMd5", credentials.DedeUserIDCKMd5}}
	parts := make([]string, 0, len(values))
	for _, item := range values {
		if value := cleanCookieValue(item[1]); value != "" {
			parts = append(parts, item[0]+"="+value)
		}
	}
	return strings.Join(parts, "; ")
}

func validateProxyURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("代理地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h" {
		return nil, errors.New("代理仅支持 http、https、socks5 或 socks5h")
	}
	return parsed, nil
}

func externalHTTPClient(proxyURL string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	if strings.TrimSpace(proxyURL) != "" {
		parsed, err := validateProxyURL(proxyURL)
		if err != nil {
			return nil, err
		}
		if parsed.Scheme == "socks5" || parsed.Scheme == "socks5h" {
			var auth *xproxy.Auth
			if parsed.User != nil {
				password, _ := parsed.User.Password()
				auth = &xproxy.Auth{User: parsed.User.Username(), Password: password}
			}
			dialer, err := xproxy.SOCKS5("tcp", parsed.Host, auth, xproxy.Direct)
			if err != nil {
				return nil, err
			}
			transport.Proxy = nil
			transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		} else {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}, nil
}

func maskedProxyURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "••••••")
	}
	return parsed.String()
}

func bilibiliRequest(endpoint string, credentials BilibiliCredentials, proxyURL string, target any) error {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Origin", "https://www.bilibili.com")
	request.Header.Set("Referer", "https://www.bilibili.com/")
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.Header.Set("Cookie", bilibiliCookie(credentials))
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bilibili returned status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func verifyBilibiliCredentials(credentials BilibiliCredentials, proxyURL string) error {
	credentials = normalizeBilibiliCredentials(credentials)
	if credentials.SESSDATA == "" || credentials.DedeUserID == "" {
		return errors.New("完整 Cookie 中缺少 SESSDATA 或 DedeUserID")
	}
	if _, err := strconv.ParseInt(credentials.DedeUserID, 10, 64); err != nil {
		return errors.New("invalid DedeUserID")
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			IsLogin bool  `json:"isLogin"`
			Mid     int64 `json:"mid"`
		} `json:"data"`
	}
	if err := bilibiliRequest("https://api.bilibili.com/x/web-interface/nav", credentials, proxyURL, &payload); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return errors.New("连接 B 站超时，请检查项目代理或容器网络")
		}
		if proxyURL != "" {
			return errors.New("无法通过项目代理连接 B 站，请检查代理地址、认证信息及容器可达性")
		}
		return errors.New("无法直连 B 站，请在设置中配置项目代理后重试")
	}
	if payload.Code != 0 {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = "未知原因"
		}
		if payload.Code == -101 {
			return fmt.Errorf("B 站未识别登录态（代码 -101：%s）。请改用浏览器 Network 请求头中的完整 Cookie，并确保容器与浏览器使用相同出口 IP", message)
		}
		return fmt.Errorf("B 站接口拒绝验证（代码 %d：%s）", payload.Code, message)
	}
	if !payload.Data.IsLogin {
		return errors.New("SESSDATA 登录态已失效或 Cookie 不完整")
	}
	if strconv.FormatInt(payload.Data.Mid, 10) != credentials.DedeUserID {
		return fmt.Errorf("DedeUserID 不匹配，当前 Cookie 对应 UID %d", payload.Data.Mid)
	}
	return nil
}

func (b *BilibiliStore) accountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		account := BilibiliAccount{Configured: b.config.Credentials.SESSDATA != "", UserID: b.config.Credentials.DedeUserID}
		b.RUnlock()
		writeJSON(w, account)
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var credentials BilibiliCredentials
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&credentials) != nil {
		http.Error(w, "invalid credentials", http.StatusBadRequest)
		return
	}
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	credentials = normalizeBilibiliCredentials(credentials)
	if err := verifyBilibiliCredentials(credentials, proxyURL); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.Lock()
	b.config.Credentials = credentials
	b.Unlock()
	if err := b.save(); err != nil {
		http.Error(w, "unable to save bilibili account", http.StatusInternalServerError)
		return
	}
	writeJSON(w, BilibiliAccount{Configured: true, UserID: credentials.DedeUserID})
}

func plainBilibiliText(value string) string {
	return strings.TrimSpace(strings.NewReplacer("<em class=\"keyword\">", "", "</em>", "", "<em>", "").Replace(value))
}

func (b *BilibiliStore) searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if len([]rune(keyword)) < 1 || len([]rune(keyword)) > 40 {
		http.Error(w, "invalid keyword", http.StatusBadRequest)
		return
	}
	b.RLock()
	credentials := b.config.Credentials
	b.RUnlock()
	if credentials.SESSDATA == "" {
		http.Error(w, "bilibili account is not configured", http.StatusPreconditionFailed)
		return
	}
	endpoint := "https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&page=1&page_size=20&keyword=" + url.QueryEscape(keyword)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				Mid   json.Number `json:"mid"`
				Uname string      `json:"uname"`
				Upic  string      `json:"upic"`
				Fans  int64       `json:"fans"`
				Usign string      `json:"usign"`
			} `json:"result"`
		} `json:"data"`
	}
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil || payload.Code != 0 {
		http.Error(w, "bilibili search is temporarily unavailable", http.StatusBadGateway)
		return
	}
	users := make([]BilibiliUser, 0, len(payload.Data.Result))
	for _, result := range payload.Data.Result {
		avatar := result.Upic
		if strings.HasPrefix(avatar, "//") {
			avatar = "https:" + avatar
		}
		users = append(users, BilibiliUser{UserID: result.Mid.String(), Name: plainBilibiliText(result.Uname), Avatar: avatar, Fans: result.Fans, Description: result.Usign})
	}
	writeJSON(w, users)
}

func (b *BilibiliStore) subscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig{}, b.config.Subscriptions...)
		b.RUnlock()
		writeJSON(w, result)
		return
	}
	if r.Method == http.MethodDelete {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.Lock()
		for index, feed := range b.config.Subscriptions {
			if feed.ID == id {
				previous := b.config.Subscriptions
				b.config.Subscriptions = append(append([]SourceConfig(nil), b.config.Subscriptions[:index]...), b.config.Subscriptions[index+1:]...)
				b.Unlock()
				if err := b.save(); err != nil {
					b.Lock()
					b.config.Subscriptions = previous
					b.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化订阅删除")
					return
				}
				writeJSON(w, map[string]string{"status": "deleted", "message": "订阅已永久删除，内容目录和已采集文件予以保留"})
				return
			}
		}
		b.Unlock()
		writeAPIError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	if r.Method == http.MethodPost && r.URL.Query().Get("action") == "sync" {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.RLock()
		var feed SourceConfig
		for _, candidate := range b.config.Subscriptions {
			if candidate.ID == id {
				feed = candidate
				break
			}
		}
		b.RUnlock()
		if feed.ID == "" {
			writeAPIError(w, http.StatusNotFound, "订阅不存在")
			return
		}
		updated, err := b.syncSource(feed)
		if err != nil {
			writeJSON(w, map[string]any{"status": "failed", "message": err.Error(), "source": updated})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "message": updated.LastSyncMessage, "source": updated})
		return
	}
	if r.Method == http.MethodPut {
		var input SourceConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || !strings.HasPrefix(input.ID, "bili-") {
			writeAPIError(w, http.StatusBadRequest, "来源设置无效")
			return
		}
		allowedSchedules := map[string]bool{"每 1 小时": true, "每 6 小时": true, "每 12 小时": true, "每天 20:00": true}
		if !allowedSchedules[input.Schedule] {
			writeAPIError(w, http.StatusBadRequest, "执行计划无效")
			return
		}
		b.Lock()
		found := false
		for index := range b.config.Subscriptions {
			if b.config.Subscriptions[index].ID == input.ID {
				b.config.Subscriptions[index].Enabled = input.Enabled
				b.config.Subscriptions[index].IncludePast = input.IncludePast
				b.config.Subscriptions[index].Schedule = input.Schedule
				b.config.Subscriptions[index].ContentTypes = []string{"DRAW", "ARTICLE"}
				input = b.config.Subscriptions[index]
				found = true
				break
			}
		}
		b.Unlock()
		if !found {
			writeAPIError(w, http.StatusNotFound, "来源不存在")
			return
		}
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存来源设置")
			return
		}
		writeJSON(w, input)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		UserID      string `json:"userId"`
		Name        string `json:"name"`
		Avatar      string `json:"avatar"`
		IncludePast bool   `json:"includePast"`
		Schedule    string `json:"schedule"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Name) == "" {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if input.Schedule == "" {
		input.Schedule = "每 6 小时"
	}
	userID := strings.TrimSpace(input.UserID)
	if _, err := strconv.ParseUint(userID, 10, 64); err != nil {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	feed := SourceConfig{ID: "bili-" + userID, Source: SourceBilibili, Name: strings.TrimSpace(input.Name), Handle: "UID " + userID, Avatar: strings.TrimSpace(input.Avatar), Enabled: true, IncludePast: input.IncludePast, Schedule: input.Schedule, ContentTypes: []string{"DRAW", "ARTICLE"}}
	var storageErr error
	feed, storageErr = prepareSourceStorage(feed)
	if storageErr != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法创建 UP 主内容目录")
		return
	}
	b.Lock()
	for _, existing := range b.config.Subscriptions {
		if existing.ID == feed.ID {
			b.Unlock()
			http.Error(w, "already subscribed", http.StatusConflict)
			return
		}
	}
	b.config.Subscriptions = append(b.config.Subscriptions, feed)
	b.Unlock()
	if err := b.save(); err != nil {
		http.Error(w, "unable to save subscription", http.StatusInternalServerError)
		return
	}
	writeJSON(w, feed)
}

func (b *BilibiliStore) projectSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		proxyURL := b.config.ProxyURL
		b.RUnlock()
		writeJSON(w, ProjectSettingsView{ProxyEnabled: proxyURL != "", ProxyURL: maskedProxyURL(proxyURL)})
		return
	}
	var input struct {
		ProxyURL string `json:"proxyUrl"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "代理设置格式无效")
		return
	}
	input.ProxyURL = strings.TrimSpace(input.ProxyURL)
	if input.ProxyURL != "" {
		if _, err := validateProxyURL(input.ProxyURL); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if r.Method == http.MethodPost {
		client, err := externalHTTPClient(input.ProxyURL)
		if err == nil {
			request, requestErr := http.NewRequest(http.MethodGet, "https://www.pixiv.net/robots.txt", nil)
			if requestErr == nil {
				request.Header.Set("User-Agent", "Lumic/1.0")
				response, requestErr := client.Do(request)
				if requestErr == nil {
					response.Body.Close()
					if response.StatusCode < 500 {
						writeJSON(w, map[string]string{"status": "ok", "message": "代理可以访问 pixiv"})
						return
					}
				}
			}
		}
		writeAPIError(w, http.StatusBadGateway, "代理测试失败，请检查地址、认证信息和 Docker 容器网络")
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.Lock()
	b.config.ProxyURL = input.ProxyURL
	b.Unlock()
	if err := b.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存项目代理")
		return
	}
	writeJSON(w, ProjectSettingsView{ProxyEnabled: input.ProxyURL != "", ProxyURL: maskedProxyURL(input.ProxyURL)})
}

// Bilibili sync accepts only image-text and article cards; video and forwarded-video cards are excluded.
func allowedBilibiliDynamicType(dynamicType string) bool {
	return dynamicType == "DYNAMIC_TYPE_DRAW" || dynamicType == "DYNAMIC_TYPE_ARTICLE"
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func normalizeRemoteImage(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	return value
}

func cleanRemoteText(value string) string {
	return strings.TrimSpace(html.UnescapeString(htmlTagPattern.ReplaceAllString(value, "")))
}

func firstNonEmptyRemoteText(values ...string) string {
	for _, value := range values {
		value = cleanRemoteText(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func mediaFileExtension(remoteURL, contentType string) string {
	extension := strings.ToLower(filepath.Ext(path.Base(strings.Split(remoteURL, "?")[0])))
	if matched, _ := regexp.MatchString(`^\.(jpe?g|png|gif|webp|avif)$`, extension); matched {
		return extension
	}
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])
	switch contentType {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	default:
		return ".jpg"
	}
}

func downloadRemoteImage(client *http.Client, remoteURL, targetBase, referer, cookie string) (string, error) {
	remoteURL = normalizeRemoteImage(remoteURL)
	if remoteURL == "" || strings.HasPrefix(remoteURL, "/flow/") {
		return remoteURL, nil
	}
	request, err := http.NewRequest(http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return "", fmt.Errorf("unexpected content type %s", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("empty image")
	}
	target := targetBase + mediaFileExtension(remoteURL, contentType)
	if err := atomicWriteFile(target, data, 0644); err != nil {
		return "", err
	}
	return target, nil
}

func (b *BilibiliStore) archiveSourceContent(feed SourceConfig, posts []Post) (SourceConfig, []Post, error) {
	prepared, err := prepareSourceStorage(feed)
	if err != nil {
		return feed, posts, err
	}
	client, err := externalHTTPClient(b.config.ProxyURL)
	if err != nil {
		return feed, posts, err
	}
	referer, cookie := "https://www.bilibili.com/", bilibiliCookie(b.config.Credentials)
	if feed.Source == SourceWeibo {
		referer, cookie = "https://weibo.com/", b.config.Weibo.Cookie
	}
	if prepared.Avatar != "" && !strings.HasPrefix(prepared.Avatar, "/flow/") {
		avatarPath, downloadErr := downloadRemoteImage(client, prepared.Avatar, filepath.Join(prepared.StoragePath, "avatar"), referer, cookie)
		if downloadErr == nil {
			prepared.Avatar = flowPublicPath(prepared.Source, prepared.Name, filepath.Base(avatarPath))
		}
	}
	for postIndex := range posts {
		posts[postIndex].Avatar = prepared.Avatar
		localMedia := make([]string, 0, len(posts[postIndex].Media))
		for mediaIndex, remoteURL := range posts[postIndex].Media {
			if strings.HasPrefix(remoteURL, "/flow/") {
				localMedia = append(localMedia, remoteURL)
				continue
			}
			base := filepath.Join(prepared.StoragePath, safeFlowDirectoryName(posts[postIndex].ID)+"-"+strconv.Itoa(mediaIndex+1))
			localPath, downloadErr := downloadRemoteImage(client, remoteURL, base, referer, cookie)
			if downloadErr != nil {
				return feed, posts, fmt.Errorf("下载动态 %s 的第 %d 张图片失败: %w", posts[postIndex].ID, mediaIndex+1, downloadErr)
			}
			localMedia = append(localMedia, flowPublicPath(prepared.Source, prepared.Name, filepath.Base(localPath)))
		}
		posts[postIndex].Media = localMedia
	}
	prepared, err = prepareSourceStorage(prepared)
	return prepared, posts, err
}

func parseRemoteTimestamp(raw json.RawMessage) int64 {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" || value == "null" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func (b *BilibiliStore) findBilibiliUser(name string) (BilibiliUser, error) {
	b.RLock()
	credentials, proxyURL := b.config.Credentials, b.config.ProxyURL
	b.RUnlock()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				Mid   json.Number `json:"mid"`
				Uname string      `json:"uname"`
				Upic  string      `json:"upic"`
				Fans  int64       `json:"fans"`
				Usign string      `json:"usign"`
			} `json:"result"`
		} `json:"data"`
	}
	endpoint := "https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&page=1&page_size=20&keyword=" + url.QueryEscape(name)
	if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil || payload.Code != 0 {
		return BilibiliUser{}, errors.New("无法校正 UP 主资料")
	}
	for _, result := range payload.Data.Result {
		if plainBilibiliText(result.Uname) == name {
			return BilibiliUser{UserID: result.Mid.String(), Name: name, Avatar: normalizeRemoteImage(result.Upic), Fans: result.Fans, Description: result.Usign}, nil
		}
	}
	return BilibiliUser{}, errors.New("无法按昵称找到原 UP 主，请删除后重新订阅")
}

func (b *BilibiliStore) fetchBilibiliPosts(feed SourceConfig) ([]Post, error) {
	userID := strings.TrimSpace(strings.TrimPrefix(feed.ID, "bili-"))
	b.RLock()
	credentials, proxyURL := b.config.Credentials, b.config.ProxyURL
	b.RUnlock()
	if credentials.SESSDATA == "" {
		return nil, errors.New("B 站账号未连接")
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items []struct {
				ID      string `json:"id_str"`
				Type    string `json:"type"`
				Modules struct {
					Author struct {
						Name  string          `json:"name"`
						Face  string          `json:"face"`
						PubTs json.RawMessage `json:"pub_ts"`
					} `json:"module_author"`
					Dynamic struct {
						Desc *struct {
							Text string `json:"text"`
						} `json:"desc"`
						Major *struct {
							Draw *struct {
								Items []struct {
									Src string `json:"src"`
								} `json:"items"`
							} `json:"draw"`
							Article *struct {
								Title   string   `json:"title"`
								Desc    string   `json:"desc"`
								Content string   `json:"content"`
								Covers  []string `json:"covers"`
							} `json:"article"`
							Opus *struct {
								Title   string `json:"title"`
								Summary string `json:"summary"`
								Content string `json:"content"`
								Pics    []struct {
									URL string `json:"url"`
								} `json:"pics"`
							} `json:"opus"`
						} `json:"major"`
					} `json:"module_dynamic"`
				} `json:"modules"`
			} `json:"items"`
		} `json:"data"`
	}
	endpoint := "https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/space?host_mid=" + url.QueryEscape(userID)
	if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil {
		return nil, fmt.Errorf("B 站请求失败: %w", err)
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("B 站接口拒绝拉取: %s", payload.Message)
	}
	posts := make([]Post, 0, len(payload.Data.Items))
	for _, item := range payload.Data.Items {
		if item.ID == "" || (item.Type != "DYNAMIC_TYPE_DRAW" && item.Type != "DYNAMIC_TYPE_ARTICLE") {
			continue
		}
		caption := ""
		if item.Modules.Dynamic.Desc != nil {
			caption = cleanRemoteText(item.Modules.Dynamic.Desc.Text)
		}
		media := make([]string, 0)
		if major := item.Modules.Dynamic.Major; major != nil {
			if major.Draw != nil {
				for _, image := range major.Draw.Items {
					media = append(media, normalizeRemoteImage(image.Src))
				}
			}
			if major.Article != nil {
				for _, image := range major.Article.Covers {
					media = append(media, normalizeRemoteImage(image))
				}
			}
			if major.Opus != nil {
				for _, image := range major.Opus.Pics {
					media = append(media, normalizeRemoteImage(image.URL))
				}
			}
			if caption == "" && major.Article != nil {
				caption = firstNonEmptyRemoteText(major.Article.Content, major.Article.Desc, major.Article.Title)
			}
			if caption == "" && major.Opus != nil {
				caption = firstNonEmptyRemoteText(major.Opus.Content, major.Opus.Summary, major.Opus.Title)
			}
		}
		publishedAt := parseRemoteTimestamp(item.Modules.Author.PubTs)
		published := time.Unix(publishedAt, 0)
		if publishedAt == 0 {
			published = time.Now()
		}
		name, avatar := item.Modules.Author.Name, normalizeRemoteImage(item.Modules.Author.Face)
		if name == "" {
			name = feed.Name
		}
		if avatar == "" {
			avatar = feed.Avatar
		}
		posts = append(posts, Post{ID: "bili-dynamic-" + item.ID, Source: SourceBilibili, Author: name, Avatar: avatar, Caption: caption, Tags: []string{}, Media: media, Published: published})
	}
	return posts, nil
}

func collectWeiboPosts(value any, feed SourceConfig, posts *[]Post, seen map[string]bool) {
	if object, ok := value.(map[string]any); ok {
		mblog := object
		if nested, exists := object["mblog"].(map[string]any); exists {
			mblog = nested
		}
		user, hasUser := mblog["user"].(map[string]any)
		id := jsonValueString(mblog["id"])
		if hasUser && id != "" && id != "<nil>" && !seen[id] {
			seen[id] = true
			name := jsonValueString(user["screen_name"])
			avatar := normalizeRemoteImage(jsonValueString(user["avatar_hd"]))
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(user["avatar_large"]))
			}
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(user["profile_image_url"]))
			}
			if name == "" {
				name = feed.Name
			}
			if avatar == "" {
				avatar = feed.Avatar
			}
			published, parseErr := time.Parse("Mon Jan 02 15:04:05 -0700 2006", fmt.Sprint(mblog["created_at"]))
			if parseErr != nil {
				published = time.Now()
			}
			media := make([]string, 0)
			if pictures, ok := mblog["pics"].([]any); ok {
				for _, rawPicture := range pictures {
					picture, _ := rawPicture.(map[string]any)
					large, _ := picture["large"].(map[string]any)
					image := normalizeRemoteImage(fmt.Sprint(large["url"]))
					if image != "" && image != "<nil>" {
						media = append(media, image)
					}
				}
			}
			if picInfos, ok := mblog["pic_infos"].(map[string]any); ok {
				for _, rawPicture := range picInfos {
					picture, _ := rawPicture.(map[string]any)
					large, _ := picture["large"].(map[string]any)
					image := normalizeRemoteImage(jsonValueString(large["url"]))
					if image == "" {
						original, _ := picture["original"].(map[string]any)
						image = normalizeRemoteImage(jsonValueString(original["url"]))
					}
					if image != "" {
						media = append(media, image)
					}
				}
			}
			caption := jsonValueString(mblog["text_raw"])
			if caption == "" {
				caption = cleanRemoteText(jsonValueString(mblog["text"]))
			}
			*posts = append(*posts, Post{ID: "weibo-status-" + id, Source: SourceWeibo, Author: name, Avatar: avatar, Caption: caption, Tags: []string{}, Media: media, Published: published})
		}
		for _, child := range object {
			collectWeiboPosts(child, feed, posts, seen)
		}
	} else if list, ok := value.([]any); ok {
		for _, child := range list {
			collectWeiboPosts(child, feed, posts, seen)
		}
	}
}

func (b *BilibiliStore) fetchWeiboPosts(feed SourceConfig) ([]Post, error) {
	userID := strings.TrimSpace(strings.TrimPrefix(feed.ID, "weibo-"))
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" {
		return nil, errors.New("微博账号未连接")
	}
	endpoints := []string{
		"https://weibo.com/ajax/statuses/mymblog?uid=" + url.QueryEscape(userID) + "&page=1&feature=0",
		"https://weibo.com/ajax/statuses/mymblog?uid=" + url.QueryEscape(userID) + "&page=1&feature=0&profile_ftype=1",
		"https://m.weibo.cn/api/container/getIndex?type=uid&value=" + url.QueryEscape(userID) + "&containerid=" + url.QueryEscape("107603"+userID),
	}
	var lastErr error
	for _, endpoint := range endpoints {
		payload, _, err := weiboRawRequest(endpoint, credentials, proxyURL)
		if err != nil {
			lastErr = err
			continue
		}
		posts := make([]Post, 0)
		collectWeiboPosts(payload, feed, &posts, make(map[string]bool))
		if len(posts) > 0 {
			return posts, nil
		}
		lastErr = errors.New("接口未返回该博主的动态")
	}
	return nil, fmt.Errorf("微博拉取失败：%w", lastErr)
}

func (b *BilibiliStore) syncSource(feed SourceConfig) (SourceConfig, error) {
	originalID := feed.ID
	if feed.Source == SourceBilibili {
		userID := strings.TrimPrefix(feed.ID, "bili-")
		if _, err := strconv.ParseUint(userID, 10, 64); err != nil || len(userID) > 15 || strings.TrimSpace(feed.Avatar) == "" {
			if repaired, repairErr := b.findBilibiliUser(feed.Name); repairErr == nil {
				feed.ID = "bili-" + repaired.UserID
				feed.Handle = "UID " + repaired.UserID
				feed.Avatar = repaired.Avatar
			}
		}
	}
	var posts []Post
	var err error
	switch feed.Source {
	case SourceBilibili:
		posts, err = b.fetchBilibiliPosts(feed)
	case SourceWeibo:
		posts, err = b.fetchWeiboPosts(feed)
	default:
		err = errors.New("该来源尚未配置采集器")
	}
	added := 0
	if err == nil {
		if len(posts) > 0 {
			if strings.TrimSpace(posts[0].Author) != "" {
				feed.Name = posts[0].Author
			}
			if strings.TrimSpace(posts[0].Avatar) != "" {
				feed.Avatar = posts[0].Avatar
			}
		}
		b.RLock()
		feed, posts, err = b.archiveSourceContent(feed, posts)
		b.RUnlock()
		if err == nil && b.content == nil {
			err = errors.New("动态存储未初始化")
		} else if err == nil {
			added, err = b.content.mergePosts(posts)
		}
	}
	now := time.Now()
	b.Lock()
	lists := []*[]SourceConfig{&b.config.Subscriptions, &b.config.WeiboSubscriptions}
	for _, list := range lists {
		for index := range *list {
			if (*list)[index].ID != feed.ID && (*list)[index].ID != originalID {
				continue
			}
			(*list)[index].ID = feed.ID
			(*list)[index].Name = feed.Name
			(*list)[index].Handle = feed.Handle
			if feed.Avatar != "" {
				(*list)[index].Avatar = feed.Avatar
			}
			(*list)[index].LastSyncedAt = now
			(*list)[index].LastSyncCount = added
			if err != nil {
				(*list)[index].LastSyncStatus = "failed"
				(*list)[index].LastSyncMessage = err.Error()
			} else {
				(*list)[index].LastSyncStatus = "success"
				(*list)[index].LastSyncMessage = fmt.Sprintf("拉取完成，新增 %d 条动态", added)
			}
			feed = (*list)[index]
		}
	}
	b.Unlock()
	if saveErr := b.save(); saveErr != nil && err == nil {
		err = saveErr
	}
	return feed, err
}

func (b *BilibiliStore) syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.RLock()
	feeds := append(append([]SourceConfig{}, b.config.Subscriptions...), b.config.WeiboSubscriptions...)
	b.RUnlock()
	results := make([]SourceConfig, 0, len(feeds))
	for _, feed := range feeds {
		if !feed.Enabled {
			continue
		}
		updated, _ := b.syncSource(feed)
		results = append(results, updated)
	}
	writeJSON(w, map[string]any{"status": "completed", "message": fmt.Sprintf("已完成 %d 个来源的拉取", len(results)), "sources": results})
}

func pixivTokenRequest(refreshToken, proxyURL string) (PixivCredentials, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return PixivCredentials{}, errors.New("refresh_token 不能为空")
	}
	clientID, clientSecret := strings.TrimSpace(os.Getenv("LUMIC_PIXIV_CLIENT_ID")), strings.TrimSpace(os.Getenv("LUMIC_PIXIV_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		return PixivCredentials{}, errors.New("服务端未配置 LUMIC_PIXIV_CLIENT_ID 和 LUMIC_PIXIV_CLIENT_SECRET")
	}
	form := url.Values{"client_id": {clientID}, "client_secret": {clientSecret}, "grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	request, err := http.NewRequest(http.MethodPost, "https://oauth.secure.pixiv.net/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return PixivCredentials{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "PixivIOSApp/7.13.0")
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return PixivCredentials{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return PixivCredentials{}, err
	}
	defer response.Body.Close()
	var payload struct {
		Error string `json:"error"`
		User  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return PixivCredentials{}, err
	}
	if response.StatusCode != http.StatusOK || payload.Error != "" {
		return PixivCredentials{}, errors.New("Pixiv refresh_token 无效或已过期")
	}
	return PixivCredentials{RefreshToken: refreshToken, UserID: payload.User.ID, UserName: payload.User.Name}, nil
}

func (b *BilibiliStore) pixivHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	proxyURL, current := b.config.ProxyURL, b.config.Pixiv
	b.RUnlock()
	if r.Method == http.MethodGet {
		current.RefreshToken = ""
		writeJSON(w, map[string]any{"configured": current.UserID != "", "userId": current.UserID, "userName": current.UserName})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "Pixiv 凭证格式无效")
		return
	}
	credentials, err := pixivTokenRequest(strings.TrimSpace(input.RefreshToken), proxyURL)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.Lock()
	b.config.Pixiv = credentials
	b.Unlock()
	if err := b.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存 Pixiv 凭证")
		return
	}
	writeJSON(w, map[string]any{"configured": true, "userId": credentials.UserID, "userName": credentials.UserName})
}

func browserClient(proxyURL string) (*http.Client, error) {
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client.Jar = jar
	return client, nil
}

func setBilibiliAPIHeaders(request *http.Request) {
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Referer", "https://www.bilibili.com/")
	request.Header.Set("Origin", "https://www.bilibili.com")
	request.Header.Set("Sec-CH-UA", `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`)
	request.Header.Set("Sec-CH-UA-Mobile", "?0")
	request.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
}

func randomSessionID() string {
	idBytes := make([]byte, 24)
	_, _ = rand.Read(idBytes)
	return base64.RawURLEncoding.EncodeToString(idBytes)
}

func cookiesFromResponse(response *http.Response) BilibiliCredentials {
	credentials := BilibiliCredentials{}
	for _, cookie := range response.Cookies() {
		switch cookie.Name {
		case "SESSDATA":
			credentials.SESSDATA = cookie.Value
		case "bili_jct":
			credentials.BiliJCT = cookie.Value
		case "DedeUserID":
			credentials.DedeUserID = cookie.Value
		case "DedeUserID__ckMd5":
			credentials.DedeUserIDCKMd5 = cookie.Value
		case "buvid3":
			credentials.Buvid3 = cookie.Value
		case "buvid4":
			credentials.Buvid4 = cookie.Value
		}
	}
	return credentials
}

func mergeBilibiliCookies(credentials BilibiliCredentials, client *http.Client) BilibiliCredentials {
	for _, rawURL := range []string{"https://www.bilibili.com/", "https://passport.bilibili.com/", "https://api.bilibili.com/"} {
		parsed, _ := url.Parse(rawURL)
		for _, cookie := range client.Jar.Cookies(parsed) {
			switch cookie.Name {
			case "SESSDATA":
				credentials.SESSDATA = cookie.Value
			case "bili_jct":
				credentials.BiliJCT = cookie.Value
			case "DedeUserID":
				credentials.DedeUserID = cookie.Value
			case "DedeUserID__ckMd5":
				credentials.DedeUserIDCKMd5 = cookie.Value
			case "buvid3":
				credentials.Buvid3 = cookie.Value
			case "buvid4":
				credentials.Buvid4 = cookie.Value
			}
		}
	}
	return credentials
}

func (b *BilibiliStore) bilibiliQRHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	if r.Method == http.MethodPost {
		client, err := browserClient(proxyURL)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, err.Error())
			return
		}
		homeRequest, _ := http.NewRequest(http.MethodGet, "https://www.bilibili.com/", nil)
		homeRequest.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
		if response, requestErr := client.Do(homeRequest); requestErr == nil {
			response.Body.Close()
		}
		request, _ := http.NewRequest(http.MethodGet, "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil)
		setBilibiliAPIHeaders(request)
		response, err := client.Do(request)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "无法连接 B 站二维码服务")
			return
		}
		defer response.Body.Close()
		var payload struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				URL string `json:"url"`
				Key string `json:"qrcode_key"`
			} `json:"data"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil || payload.Code != 0 || payload.Data.URL == "" || payload.Data.Key == "" {
			writeAPIError(w, http.StatusBadGateway, "B 站二维码接口响应异常："+payload.Message)
			return
		}
		now := time.Now()
		session := BilibiliQRSession{ID: randomSessionID(), Key: payload.Data.Key, URL: payload.Data.URL, CreatedAt: now, ExpiresAt: now.Add(3 * time.Minute)}
		b.Lock()
		b.bilibiliQR[session.ID] = session
		b.bilibiliClients[session.ID] = client
		b.Unlock()
		writeJSON(w, session)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	b.RLock()
	session, ok := b.bilibiliQR[id]
	client := b.bilibiliClients[id]
	b.RUnlock()
	if !ok || client == nil {
		writeAPIError(w, http.StatusNotFound, "B 站扫码会话不存在")
		return
	}
	if time.Now().After(session.ExpiresAt) {
		b.Lock()
		delete(b.bilibiliQR, id)
		delete(b.bilibiliClients, id)
		b.Unlock()
		writeAPIError(w, http.StatusGone, "B 站二维码已过期")
		return
	}
	endpoint := "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key=" + url.QueryEscape(session.Key)
	request, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	setBilibiliAPIHeaders(request)
	response, err := client.Do(request)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "无法查询 B 站扫码状态")
		return
	}
	defer response.Body.Close()
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Code         int    `json:"code"`
			Message      string `json:"message"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil || payload.Code != 0 {
		writeAPIError(w, http.StatusBadGateway, "B 站扫码状态响应异常："+payload.Message)
		return
	}
	switch payload.Data.Code {
	case 86101:
		writeJSON(w, map[string]string{"status": "waiting", "message": "等待扫码"})
		return
	case 86090:
		writeJSON(w, map[string]string{"status": "scanned", "message": "已扫码，请在手机上确认"})
		return
	case 86038:
		writeAPIError(w, http.StatusGone, "B 站二维码已过期")
		return
	case 0:
		credentials := mergeBilibiliCookies(cookiesFromResponse(response), client)
		credentials.AccessTimeValue = payload.Data.RefreshToken
		if credentials.SESSDATA == "" || credentials.BiliJCT == "" || credentials.DedeUserID == "" {
			writeAPIError(w, http.StatusBadGateway, "B 站登录成功但响应缺少必要 Cookie")
			return
		}
		if credentials.Buvid3 == "" {
			var spi struct {
				Code int `json:"code"`
				Data struct {
					B3 string `json:"b_3"`
					B4 string `json:"b_4"`
				} `json:"data"`
			}
			if err := bilibiliRequest("https://api.bilibili.com/x/frontend/finger/spi", credentials, proxyURL, &spi); err == nil {
				credentials.Buvid3, credentials.Buvid4 = spi.Data.B3, spi.Data.B4
			}
		}
		b.Lock()
		b.config.Credentials = credentials
		delete(b.bilibiliQR, id)
		delete(b.bilibiliClients, id)
		b.Unlock()
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存 B 站登录状态")
			return
		}
		writeJSON(w, map[string]string{"status": "connected", "userId": credentials.DedeUserID})
		return
	default:
		writeAPIError(w, http.StatusBadGateway, "B 站扫码失败："+payload.Data.Message)
		return
	}
}

func weiboClient(proxyURL string) (*http.Client, error) {
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client.Jar = jar
	return client, nil
}

func weiboRawRequest(endpoint string, credentials WeiboCredentials, proxyURL string) (map[string]any, int, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Cookie", credentials.Cookie)
	request.Header.Set("Referer", "https://weibo.com/")
	request.Header.Set("Origin", "https://weibo.com")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	for _, cookie := range strings.Split(credentials.Cookie, ";") {
		parts := strings.SplitN(strings.TrimSpace(cookie), "=", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "XSRF-TOKEN") {
			request.Header.Set("X-XSRF-TOKEN", parts[1])
		}
	}
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, fmt.Errorf("微博接口返回 HTTP %d，请检查登录状态或代理出口", response.StatusCode)
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		if bytes.Contains(bytes.ToLower(body), []byte("<html")) || bytes.Contains(bytes.ToLower(body), []byte("<!doctype")) {
			return nil, response.StatusCode, errors.New("微博返回了网页拦截页，请重新扫码登录或更换代理出口")
		}
		return nil, response.StatusCode, errors.New("微博接口返回格式异常，请重新扫码登录")
	}
	return payload, response.StatusCode, nil
}

func weiboRequest(endpoint string, credentials WeiboCredentials, proxyURL string, target any) error {
	payload, _, err := weiboRawRequest(endpoint, credentials, proxyURL)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func collectWeiboUsers(value any, users *[]WeiboUser, seen map[string]bool) {
	if object, ok := value.(map[string]any); ok {
		if nested, exists := object["user"].(map[string]any); exists {
			collectWeiboUsers(nested, users, seen)
		}
		id := jsonValueString(object["id"])
		name := strings.TrimSpace(fmt.Sprint(object["screen_name"]))
		if name == "<nil>" || name == "" {
			name = strings.TrimSpace(jsonValueString(object["name"]))
		}
		if id != "" && id != "<nil>" && name != "" && name != "<nil>" && !seen[id] {
			avatar := normalizeRemoteImage(jsonValueString(object["avatar_hd"]))
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(object["avatar_large"]))
			}
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(object["profile_image_url"]))
			}
			fans := int64(0)
			if raw, ok := object["followers_count"]; ok {
				fans, _ = strconv.ParseInt(jsonValueString(raw), 10, 64)
			}
			description := strings.TrimSpace(fmt.Sprint(object["description"]))
			if description == "<nil>" {
				description = ""
			}
			seen[id] = true
			*users = append(*users, WeiboUser{UserID: id, Name: name, Avatar: avatar, Fans: fans, Description: description})
		}
		for _, child := range object {
			collectWeiboUsers(child, users, seen)
		}
		return
	}
	if list, ok := value.([]any); ok {
		for _, child := range list {
			collectWeiboUsers(child, users, seen)
		}
	}
}

func (b *BilibiliStore) weiboSearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if len([]rune(keyword)) < 1 || len([]rune(keyword)) > 40 {
		writeAPIError(w, http.StatusBadRequest, "搜索关键词长度应为 1 到 40 个字符")
		return
	}
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" {
		writeAPIError(w, http.StatusPreconditionFailed, "请先连接微博账号")
		return
	}
	endpoints := []string{
		"https://weibo.com/ajax/side/search?q=" + url.QueryEscape(keyword),
		"https://m.weibo.cn/api/container/getIndex?containerid=" + url.QueryEscape("100103type=3&q="+keyword) + "&page_type=searchall",
	}
	users := make([]WeiboUser, 0)
	seen := make(map[string]bool)
	statuses := make([]string, 0, len(endpoints))
	lastMessage := ""
	for _, endpoint := range endpoints {
		payload, status, err := weiboRawRequest(endpoint, credentials, proxyURL)
		if err != nil {
			statuses = append(statuses, strconv.Itoa(status))
			lastMessage = err.Error()
			continue
		}
		collectWeiboUsers(payload, &users, seen)
		if len(users) > 0 {
			break
		}
	}
	if len(users) == 0 {
		message := "微博搜索暂时不可用（接口状态：" + strings.Join(statuses, "/") + "）。请重新扫码登录或检查代理出口"
		if lastMessage != "" {
			message += "；" + lastMessage
		}
		writeAPIError(w, http.StatusBadGateway, message)
		return
	}
	client, clientErr := externalHTTPClient(proxyURL)
	if clientErr == nil {
		for index := range users {
			if users[index].Avatar == "" {
				continue
			}
			storage := sourceStoragePath(SourceWeibo, users[index].Name)
			if err := os.MkdirAll(storage, 0755); err != nil {
				continue
			}
			avatarPath, err := downloadRemoteImage(client, users[index].Avatar, filepath.Join(storage, "avatar"), "https://weibo.com/", credentials.Cookie)
			if err == nil {
				users[index].Avatar = flowPublicPath(SourceWeibo, users[index].Name, filepath.Base(avatarPath))
			}
		}
	}
	writeJSON(w, users)
}

func (b *BilibiliStore) weiboSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig{}, b.config.WeiboSubscriptions...)
		b.RUnlock()
		writeJSON(w, result)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if r.Method == http.MethodDelete {
		b.Lock()
		for index, feed := range b.config.WeiboSubscriptions {
			if feed.ID == id {
				previous := b.config.WeiboSubscriptions
				b.config.WeiboSubscriptions = append(append([]SourceConfig(nil), previous[:index]...), previous[index+1:]...)
				b.Unlock()
				if err := b.save(); err != nil {
					b.Lock()
					b.config.WeiboSubscriptions = previous
					b.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化微博订阅删除")
					return
				}
				writeJSON(w, map[string]string{"status": "deleted", "message": "微博博主订阅已永久删除，已采集文件予以保留"})
				return
			}
		}
		b.Unlock()
		writeAPIError(w, http.StatusNotFound, "微博订阅不存在")
		return
	}
	if r.Method == http.MethodPost && r.URL.Query().Get("action") == "sync" {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.RLock()
		var feed SourceConfig
		for _, candidate := range b.config.WeiboSubscriptions {
			if candidate.ID == id {
				feed = candidate
				break
			}
		}
		b.RUnlock()
		if feed.ID == "" {
			writeAPIError(w, http.StatusNotFound, "微博订阅不存在")
			return
		}
		updated, err := b.syncSource(feed)
		if err != nil {
			writeJSON(w, map[string]any{"status": "failed", "message": err.Error(), "source": updated})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "message": updated.LastSyncMessage, "source": updated})
		return
	}
	if r.Method == http.MethodPut {
		var input SourceConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || !strings.HasPrefix(input.ID, "weibo-") {
			writeAPIError(w, http.StatusBadRequest, "来源设置无效")
			return
		}
		allowedSchedules := map[string]bool{"每 1 小时": true, "每 6 小时": true, "每 12 小时": true, "每天 20:00": true}
		if !allowedSchedules[input.Schedule] {
			writeAPIError(w, http.StatusBadRequest, "执行计划无效")
			return
		}
		b.Lock()
		found := false
		for index := range b.config.WeiboSubscriptions {
			if b.config.WeiboSubscriptions[index].ID == input.ID {
				b.config.WeiboSubscriptions[index].Enabled = input.Enabled
				b.config.WeiboSubscriptions[index].IncludePast = input.IncludePast
				b.config.WeiboSubscriptions[index].Schedule = input.Schedule
				input = b.config.WeiboSubscriptions[index]
				found = true
				break
			}
		}
		b.Unlock()
		if !found {
			writeAPIError(w, http.StatusNotFound, "微博来源不存在")
			return
		}
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存微博来源设置")
			return
		}
		writeJSON(w, input)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		UserID      string `json:"userId"`
		Name        string `json:"name"`
		Avatar      string `json:"avatar"`
		IncludePast bool   `json:"includePast"`
		Schedule    string `json:"schedule"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Name) == "" {
		writeAPIError(w, http.StatusBadRequest, "微博订阅信息无效")
		return
	}
	b.RLock()
	configured := b.config.Weibo.Cookie != ""
	b.RUnlock()
	if !configured {
		writeAPIError(w, http.StatusPreconditionFailed, "请先连接微博账号")
		return
	}
	if input.Schedule == "" {
		input.Schedule = "每 6 小时"
	}
	feed := SourceConfig{ID: "weibo-" + strings.TrimSpace(input.UserID), Source: SourceWeibo, Name: strings.TrimSpace(input.Name), Handle: "UID " + strings.TrimSpace(input.UserID), Avatar: strings.TrimSpace(input.Avatar), Enabled: true, IncludePast: input.IncludePast, Schedule: input.Schedule}
	var err error
	if feed, err = prepareSourceStorage(feed); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法创建微博博主内容目录")
		return
	}
	b.Lock()
	for _, existing := range b.config.WeiboSubscriptions {
		if existing.ID == feed.ID {
			b.Unlock()
			writeAPIError(w, http.StatusConflict, "已经订阅该微博博主")
			return
		}
	}
	b.config.WeiboSubscriptions = append(b.config.WeiboSubscriptions, feed)
	b.Unlock()
	if err := b.save(); err != nil {
		b.Lock()
		b.config.WeiboSubscriptions = b.config.WeiboSubscriptions[:len(b.config.WeiboSubscriptions)-1]
		b.Unlock()
		writeAPIError(w, http.StatusInternalServerError, "无法保存微博订阅")
		return
	}
	writeJSON(w, feed)
}

func jsonValueString(value any) string {
	if value == nil {
		return ""
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	if strings.ContainsAny(text, ".eE") {
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			return strconv.FormatFloat(number, 'f', -1, 64)
		}
	}
	return text
}

func jsonScalarString(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return strings.Trim(value, "\"")
}

func unwrapJSONP(body []byte) []byte {
	value := strings.TrimSpace(string(body))
	start, end := strings.IndexByte(value, '{'), strings.LastIndexByte(value, '}')
	if start >= 0 && end >= start {
		return []byte(value[start : end+1])
	}
	return body
}

func weiboCrossDomainURL(rawURL string, index int) string {
	if index != 0 {
		return rawURL
	}
	separator := "&"
	if !strings.Contains(rawURL, "?") {
		separator = "?"
	}
	return rawURL + separator + "action=login"
}

func responseSummary(body []byte) string {
	value := strings.TrimSpace(string(body))
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func (b *BilibiliStore) weiboAccountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.RLock()
	credentials := b.config.Weibo
	b.RUnlock()
	writeJSON(w, map[string]any{"configured": credentials.Cookie != "", "userId": credentials.UserID, "userName": credentials.UserName})
}

func (b *BilibiliStore) weiboQRHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	if r.Method == http.MethodGet {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.RLock()
		if id == "" {
			account := b.config.Weibo
			b.RUnlock()
			writeJSON(w, map[string]any{"configured": account.UserID != "", "userId": account.UserID, "userName": account.UserName})
			return
		}
		session, ok := b.weiboQR[id]
		client := b.weiboClients[id]
		b.RUnlock()
		if client == nil {
			writeAPIError(w, http.StatusNotFound, "扫码会话已失效，请重新获取二维码")
			return
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, "扫码会话不存在，请重新获取二维码")
			return
		}
		if time.Now().After(session.ExpiresAt) {
			b.Lock()
			delete(b.weiboQR, id)
			delete(b.weiboClients, id)
			b.Unlock()
			writeAPIError(w, http.StatusGone, "二维码已过期")
			return
		}
		checkQuery := url.Values{
			"entry":    {"weibo"},
			"qrid":     {session.QRID},
			"callback": {"STK_" + strconv.FormatInt(time.Now().UnixNano()/100000, 10)},
		}
		checkURL := "https://login.sina.com.cn/sso/qrcode/check?" + checkQuery.Encode()
		request, _ := http.NewRequest(http.MethodGet, checkURL, nil)
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
		request.Header.Set("Accept", "*/*")
		request.Header.Set("Referer", "https://mail.sina.com.cn/")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法查询微博扫码状态")
			return
		}
		defer response.Body.Close()
		checkBody, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if readErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法读取微博扫码状态")
			return
		}
		var payload struct {
			RetCode int    `json:"retcode"`
			Msg     string `json:"msg"`
			Data    struct {
				Alt string `json:"alt"`
			} `json:"data"`
		}
		if json.Unmarshal(unwrapJSONP(checkBody), &payload) != nil {
			writeAPIError(w, http.StatusBadGateway, "微博扫码状态响应异常："+responseSummary(checkBody))
			return
		}
		if payload.RetCode != 20000000 {
			writeJSON(w, map[string]any{"status": "waiting", "message": payload.Msg})
			return
		}
		b.Lock()
		current, exists := b.weiboQR[id]
		if !exists || current.Exchanging {
			b.Unlock()
			writeJSON(w, map[string]any{"status": "confirming", "message": "正在确认微博登录"})
			return
		}
		current.Exchanging = true
		b.weiboQR[id] = current
		b.Unlock()
		defer func() {
			b.Lock()
			if active, exists := b.weiboQR[id]; exists {
				active.Exchanging = false
				b.weiboQR[id] = active
			}
			b.Unlock()
		}()
		loginQuery := url.Values{
			"entry":       {"weibo"},
			"returntype":  {"TEXT"},
			"crossdomain": {"1"},
			"cdult":       {"3"},
			"domain":      {"weibo.com"},
			"alt":         {payload.Data.Alt},
			"savestate":   {"30"},
			"callback":    {"STK_" + strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		loginURL := "http://login.sina.com.cn/sso/login.php?" + loginQuery.Encode()
		loginRequest, _ := http.NewRequest(http.MethodGet, loginURL, nil)
		loginRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
		loginRequest.Header.Set("Accept", "*/*")
		loginRequest.Header.Set("Referer", "https://mail.sina.com.cn/")
		loginResponse, loginErr := client.Do(loginRequest)
		if loginErr != nil {
			writeAPIError(w, http.StatusBadGateway, "微博登录票据交换失败："+loginErr.Error())
			return
		}
		defer loginResponse.Body.Close()
		loginBody, readErr := io.ReadAll(io.LimitReader(loginResponse.Body, 2<<20))
		if readErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法读取微博登录响应")
			return
		}
		var loginPayload struct {
			RetCode     json.RawMessage `json:"retcode"`
			UID         json.RawMessage `json:"uid"`
			Nick        string          `json:"nick"`
			Reason      string          `json:"reason"`
			CrossDomain []string        `json:"crossDomainUrlList"`
		}
		if err := json.Unmarshal(unwrapJSONP(loginBody), &loginPayload); err != nil {
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("微博登录响应无法解析（HTTP %d）：%s", loginResponse.StatusCode, responseSummary(loginBody)))
			return
		}
		loginUID, loginRetCode := jsonScalarString(loginPayload.UID), jsonScalarString(loginPayload.RetCode)
		if loginResponse.StatusCode != http.StatusOK || (loginRetCode != "" && loginRetCode != "0") || loginUID == "" {
			detail := loginPayload.Reason
			if detail == "" {
				detail = responseSummary(loginBody)
			}
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("微博登录响应异常（HTTP %d，retcode %s）：%s", loginResponse.StatusCode, loginRetCode, detail))
			return
		}
		for index, crossURL := range loginPayload.CrossDomain {
			crossURL = weiboCrossDomainURL(crossURL, index)
			crossRequest, e := http.NewRequest(http.MethodGet, crossURL, nil)
			if e == nil {
				crossRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
				crossRequest.Header.Set("Referer", "https://mail.sina.com.cn/")
				if crossResponse, e := client.Do(crossRequest); e == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(crossResponse.Body, 1<<20))
					crossResponse.Body.Close()
				}
			}
		}
		homeRequest, _ := http.NewRequest(http.MethodGet, "https://weibo.com/", nil)
		homeRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
		if homeResponse, homeErr := client.Do(homeRequest); homeErr == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(homeResponse.Body, 1<<20))
			homeResponse.Body.Close()
		}
		cookieParts, cookieNames := []string{}, map[string]bool{}
		for _, rawURL := range []string{"https://weibo.com/", "https://www.weibo.com/", "https://m.weibo.cn/", "https://login.sina.com.cn/", "https://passport.weibo.com/"} {
			cookieURL, _ := url.Parse(rawURL)
			for _, cookie := range client.Jar.Cookies(cookieURL) {
				if !cookieNames[cookie.Name] {
					cookieNames[cookie.Name] = true
					cookieParts = append(cookieParts, cookie.Name+"="+cookie.Value)
				}
			}
		}
		if len(cookieParts) == 0 {
			writeAPIError(w, http.StatusBadGateway, "微博未返回登录 Cookie")
			return
		}
		credentials := WeiboCredentials{Cookie: strings.Join(cookieParts, "; "), UserID: loginUID, UserName: loginPayload.Nick}
		b.Lock()
		b.config.Weibo = credentials
		delete(b.weiboQR, id)
		delete(b.weiboClients, id)
		b.Unlock()
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存微博登录状态")
			return
		}
		writeJSON(w, map[string]any{"status": "connected", "userId": credentials.UserID, "userName": credentials.UserName})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client, err := weiboClient(proxyURL)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	imageQuery := url.Values{
		"entry":    {"weibo"},
		"size":     {"180"},
		"callback": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	request, _ := http.NewRequest(http.MethodGet, "https://login.sina.com.cn/sso/qrcode/image?"+imageQuery.Encode(), nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Referer", "https://mail.sina.com.cn/")
	response, err := client.Do(request)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "无法连接微博扫码服务")
		return
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if readErr != nil {
		writeAPIError(w, http.StatusBadGateway, "无法读取微博扫码响应")
		return
	}
	var payload struct {
		RetCode int `json:"retcode"`
		Data    struct {
			QRID  string `json:"qrid"`
			Image string `json:"image"`
		} `json:"data"`
	}
	if err := json.Unmarshal(unwrapJSONP(body), &payload); err != nil || response.StatusCode != http.StatusOK || payload.RetCode != 20000000 || payload.Data.QRID == "" || payload.Data.Image == "" {
		summary := strings.TrimSpace(string(body))
		if len(summary) > 180 {
			summary = summary[:180]
		}
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("微博扫码接口响应异常（HTTP %d）：%s", response.StatusCode, summary))
		return
	}
	now := time.Now()
	idBytes := make([]byte, 24)
	_, _ = rand.Read(idBytes)
	session := WeiboQRSession{ID: base64.RawURLEncoding.EncodeToString(idBytes), QRID: payload.Data.QRID, Image: payload.Data.Image, CreatedAt: now, ExpiresAt: now.Add(3 * time.Minute)}
	b.Lock()
	b.weiboQR[session.ID] = session
	b.weiboClients[session.ID] = client
	b.Unlock()
	writeJSON(w, session)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(value)
}

func main() {
	if err := initializeFlowStorage(); err != nil {
		log.Fatal("unable to initialize flow storage: ", err)
	}
	store, err := loadStore()
	if err != nil {
		log.Fatal("unable to load content data: ", err)
	}
	auth := loadAuthConfig()
	bilibili, err := loadBilibiliStore()
	if err != nil {
		log.Fatal("unable to load Bilibili configuration: ", err)
	}
	bilibili.content = store
	if err := bilibili.reconcileFlowStorage(); err != nil {
		log.Fatal("unable to prepare source flow storage: ", err)
	}
	if err := bilibili.save(); err != nil {
		log.Fatal("unable to persist source storage paths: ", err)
	}
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", loginHandler(sessions, auth))
	mux.HandleFunc("/api/settings", settingsHandler(auth))
	mux.HandleFunc("/api/posts", store.postsHandler)
	mux.HandleFunc("/api/feeds", store.feedsHandler)
	mux.HandleFunc("/api/sync", bilibili.syncHandler)
	mux.HandleFunc("/api/bilibili/account", bilibili.accountHandler)
	mux.HandleFunc("/api/bilibili/qr", bilibili.bilibiliQRHandler)
	mux.HandleFunc("/api/pixiv/account", bilibili.pixivHandler)
	mux.HandleFunc("/api/weibo/account", bilibili.weiboAccountHandler)
	mux.HandleFunc("/api/weibo/qr", bilibili.weiboQRHandler)
	mux.HandleFunc("/api/weibo/search", bilibili.weiboSearchHandler)
	mux.HandleFunc("/api/weibo/subscriptions", bilibili.weiboSubscriptionsHandler)
	mux.HandleFunc("/api/bilibili/search", bilibili.searchHandler)
	mux.HandleFunc("/api/bilibili/subscriptions", bilibili.subscriptionsHandler)
	mux.HandleFunc("/api/project/settings", bilibili.projectSettingsHandler)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, map[string]string{"status": "ok"}) })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; script-src 'self'; connect-src 'self'")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			authMiddleware(sessions, mux).ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/flow/") {
			http.StripPrefix("/flow/", http.FileServer(http.Dir(flowRoot))).ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, "public/index.html")
			return
		}
		http.ServeFile(w, r, "public"+r.URL.Path)
	})
	log.Println("Lumic API listening on :5500")
	log.Fatal(http.ListenAndServe(":5500", handler))
}
