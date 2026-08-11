package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
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
	ID           string    `json:"id"`
	Source       Source    `json:"source"`
	Name         string    `json:"name"`
	Handle       string    `json:"handle"`
	Enabled      bool      `json:"enabled"`
	IncludePast  bool      `json:"includePast"`
	Schedule     string    `json:"schedule"`
	ContentTypes []string  `json:"contentTypes,omitempty"`
	LastSyncedAt time.Time `json:"lastSyncedAt"`
}

type BilibiliCredentials struct {
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

type WeiboQRSession struct {
	ID        string    `json:"id"`
	QRID      string    `json:"-"`
	Image     string    `json:"image,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type BilibiliConfig struct {
	Credentials   BilibiliCredentials `json:"credentials"`
	Pixiv         PixivCredentials    `json:"pixiv"`
	Weibo         WeiboCredentials    `json:"weibo"`
	Subscriptions []SourceConfig      `json:"subscriptions"`
	ProxyURL      string              `json:"proxyUrl,omitempty"`
}

type ProjectSettingsView struct {
	ProxyEnabled bool   `json:"proxyEnabled"`
	ProxyURL     string `json:"proxyUrl,omitempty"`
}

type BilibiliStore struct {
	sync.RWMutex
	config       BilibiliConfig
	key          []byte
	weiboQR      map[string]WeiboQRSession
	weiboClients map[string]*http.Client
}

type BilibiliUser struct {
	UserID      int64  `json:"userId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Fans        int64  `json:"fans"`
	Description string `json:"description"`
}

type Store struct {
	sync.RWMutex
	posts []Post
	feeds []SourceConfig
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
var bilibiliFile = "/data/bilibili.enc"
var secretFile = "/data/secret.key"

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
	store := &BilibiliStore{key: key, config: BilibiliConfig{Subscriptions: []SourceConfig{}}, weiboQR: make(map[string]WeiboQRSession), weiboClients: make(map[string]*http.Client)}
	if data, err := os.ReadFile(bilibiliFile); err == nil {
		if err := decryptJSON(key, data, &store.config); err != nil {
			return nil, fmt.Errorf("decrypt bilibili configuration: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return store, nil
}

func (b *BilibiliStore) save() error {
	b.RLock()
	data, err := encryptJSON(b.key, b.config)
	b.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(bilibiliFile), 0700); err != nil {
		return err
	}
	return os.WriteFile(bilibiliFile, data, 0600)
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

func (s *Store) postsHandler(w http.ResponseWriter, r *http.Request) {
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

func bilibiliCookie(credentials BilibiliCredentials) string {
	values := [][2]string{{"SESSDATA", credentials.SESSDATA}, {"bili_jct", credentials.BiliJCT}, {"buvid3", credentials.Buvid3}, {"DedeUserID", credentials.DedeUserID}, {"ac_time_value", credentials.AccessTimeValue}, {"buvid4", credentials.Buvid4}, {"DedeUserID__ckMd5", credentials.DedeUserIDCKMd5}}
	parts := make([]string, 0, len(values))
	for _, item := range values {
		if item[1] != "" {
			parts = append(parts, item[0]+"="+item[1])
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
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124 Safari/537.36")
	request.Header.Set("Referer", "https://www.bilibili.com/")
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
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target)
}

func verifyBilibiliCredentials(credentials BilibiliCredentials, proxyURL string) error {
	if credentials.SESSDATA == "" || credentials.BiliJCT == "" || credentials.Buvid3 == "" || credentials.DedeUserID == "" {
		return errors.New("missing required credentials")
	}
	if _, err := strconv.ParseInt(credentials.DedeUserID, 10, 64); err != nil {
		return errors.New("invalid DedeUserID")
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
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
		return fmt.Errorf("B 站接口拒绝验证（代码 %d）", payload.Code)
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
				Mid   int64  `json:"mid"`
				Uname string `json:"uname"`
				Upic  string `json:"upic"`
				Fans  int64  `json:"fans"`
				Usign string `json:"usign"`
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
		users = append(users, BilibiliUser{UserID: result.Mid, Name: plainBilibiliText(result.Uname), Avatar: avatar, Fans: result.Fans, Description: result.Usign})
	}
	writeJSON(w, users)
}

func (b *BilibiliStore) subscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig(nil), b.config.Subscriptions...)
		b.RUnlock()
		writeJSON(w, result)
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
		UserID      int64  `json:"userId"`
		Name        string `json:"name"`
		IncludePast bool   `json:"includePast"`
		Schedule    string `json:"schedule"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || input.UserID <= 0 || strings.TrimSpace(input.Name) == "" {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if input.Schedule == "" {
		input.Schedule = "每 6 小时"
	}
	feed := SourceConfig{ID: fmt.Sprintf("bili-%d", input.UserID), Source: SourceBilibili, Name: strings.TrimSpace(input.Name), Handle: fmt.Sprintf("UID %d", input.UserID), Enabled: true, IncludePast: input.IncludePast, Schedule: input.Schedule, ContentTypes: []string{"DRAW", "ARTICLE"}}
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

func syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"status": "started", "message": "同步任务已加入队列"})
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
		checkURL := "https://passport.weibo.com/sso/v2/qrcode/check?entry=miniblog&qrid=" + url.QueryEscape(session.QRID)
		request, _ := http.NewRequest(http.MethodGet, checkURL, nil)
		request.Header.Set("User-Agent", "Mozilla/5.0")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法查询微博扫码状态")
			return
		}
		defer response.Body.Close()
		var payload struct {
			RetCode int    `json:"retcode"`
			Msg     string `json:"msg"`
			Data    struct {
				Alt string `json:"alt"`
			} `json:"data"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil {
			writeAPIError(w, http.StatusBadGateway, "微博扫码状态响应异常")
			return
		}
		if payload.RetCode != 20000000 {
			writeJSON(w, map[string]any{"status": "waiting", "message": payload.Msg})
			return
		}
		loginURL := "https://login.sina.com.cn/sso/login.php?entry=miniblog&returntype=TEXT&crossdomain=1&cdult=3&domain=weibo.com&alt=" + url.QueryEscape(payload.Data.Alt)
		loginRequest, _ := http.NewRequest(http.MethodGet, loginURL, nil)
		loginRequest.Header.Set("User-Agent", "Mozilla/5.0")
		loginResponse, loginErr := client.Do(loginRequest)
		if loginErr != nil {
			writeAPIError(w, http.StatusBadGateway, "微博登录票据交换失败")
			return
		}
		defer loginResponse.Body.Close()
		var loginPayload struct {
			UID         string   `json:"uid"`
			Nick        string   `json:"nick"`
			CrossDomain []string `json:"crossDomainUrlList"`
		}
		if json.NewDecoder(io.LimitReader(loginResponse.Body, 2<<20)).Decode(&loginPayload) != nil || loginPayload.UID == "" {
			writeAPIError(w, http.StatusBadGateway, "微博登录响应异常")
			return
		}
		for _, crossURL := range loginPayload.CrossDomain {
			crossRequest, e := http.NewRequest(http.MethodGet, crossURL, nil)
			if e == nil {
				if crossResponse, e := client.Do(crossRequest); e == nil {
					crossResponse.Body.Close()
				}
			}
		}
		cookieURL, _ := url.Parse("https://weibo.com/")
		cookieParts := []string{}
		for _, cookie := range client.Jar.Cookies(cookieURL) {
			cookieParts = append(cookieParts, cookie.Name+"="+cookie.Value)
		}
		if len(cookieParts) == 0 {
			writeAPIError(w, http.StatusBadGateway, "微博未返回登录 Cookie")
			return
		}
		credentials := WeiboCredentials{Cookie: strings.Join(cookieParts, "; "), UserID: loginPayload.UID, UserName: loginPayload.Nick}
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
	request, _ := http.NewRequest(http.MethodGet, "https://passport.weibo.com/sso/v2/qrcode/image?entry=miniblog&size=180", nil)
	request.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := client.Do(request)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "无法连接微博扫码服务")
		return
	}
	defer response.Body.Close()
	var payload struct {
		RetCode int `json:"retcode"`
		Data    struct {
			QRID  string `json:"qrid"`
			Image string `json:"image"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil || payload.Data.QRID == "" || payload.Data.Image == "" {
		writeAPIError(w, http.StatusBadGateway, "微博扫码接口响应异常")
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
	store := demoStore()
	auth := loadAuthConfig()
	bilibili, err := loadBilibiliStore()
	if err != nil {
		log.Fatal("unable to load Bilibili configuration: ", err)
	}
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", loginHandler(sessions, auth))
	mux.HandleFunc("/api/settings", settingsHandler(auth))
	mux.HandleFunc("/api/posts", store.postsHandler)
	mux.HandleFunc("/api/feeds", store.feedsHandler)
	mux.HandleFunc("/api/sync", syncHandler)
	mux.HandleFunc("/api/bilibili/account", bilibili.accountHandler)
	mux.HandleFunc("/api/pixiv/account", bilibili.pixivHandler)
	mux.HandleFunc("/api/weibo/qr", bilibili.weiboQRHandler)
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
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, "public/index.html")
			return
		}
		http.ServeFile(w, r, "public"+r.URL.Path)
	})
	log.Println("Lumic API listening on :5500")
	log.Fatal(http.ListenAndServe(":5500", handler))
}
