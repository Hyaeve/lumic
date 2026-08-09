package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
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
	LastSyncedAt time.Time `json:"lastSyncedAt"`
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

func syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"status": "started", "message": "同步任务已加入队列"})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(value)
}

func main() {
	store := demoStore()
	auth := loadAuthConfig()
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", loginHandler(sessions, auth))
	mux.HandleFunc("/api/settings", settingsHandler(auth))
	mux.HandleFunc("/api/posts", store.postsHandler)
	mux.HandleFunc("/api/feeds", store.feedsHandler)
	mux.HandleFunc("/api/sync", syncHandler)
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
