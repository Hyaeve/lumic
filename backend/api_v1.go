package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sessionLifetime      = 24 * time.Hour
	apiV1DefaultPageSize = 30
	apiV1MaxPageSize     = 100
)

type apiV1Post struct {
	ID            string      `json:"id"`
	Source        Source      `json:"source"`
	FeedIDs       []string    `json:"feedIds,omitempty"`
	Author        string      `json:"author"`
	Avatar        string      `json:"avatar"`
	Caption       string      `json:"caption"`
	Tags          []string    `json:"tags"`
	Media         []string    `json:"media"`
	PreviewMedia  []string    `json:"previewMedia"`
	Videos        []PostVideo `json:"videos,omitempty"`
	PreviewVideos []PostVideo `json:"previewVideos,omitempty"`
	OriginalURL   string      `json:"originalUrl,omitempty"`
	Published     time.Time   `json:"published"`
	Liked         bool        `json:"liked"`
}

type apiV1Feed struct {
	ID              string    `json:"id"`
	Source          Source    `json:"source"`
	Name            string    `json:"name"`
	Handle          string    `json:"handle"`
	Avatar          string    `json:"avatar,omitempty"`
	Enabled         bool      `json:"enabled"`
	Schedule        string    `json:"schedule"`
	ContentTypes    []string  `json:"contentTypes,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	OnlyWithImages  bool      `json:"onlyWithImages,omitempty"`
	IncludeVideos   bool      `json:"includeVideos,omitempty"`
	IncludeKeywords []string  `json:"includeKeywords,omitempty"`
	ExcludeKeywords []string  `json:"excludeKeywords,omitempty"`
	LastSyncedAt    time.Time `json:"lastSyncedAt"`
	LastSyncStatus  string    `json:"lastSyncStatus,omitempty"`
	LastSyncMessage string    `json:"lastSyncMessage,omitempty"`
	LastSyncCount   int       `json:"lastSyncCount,omitempty"`
}

type apiV1PostPage struct {
	Items      []apiV1Post `json:"items"`
	NextCursor string      `json:"nextCursor,omitempty"`
	HasMore    bool        `json:"hasMore"`
	Limit      int         `json:"limit"`
}

type apiV1PostCursor struct {
	Version    int       `json:"version"`
	Published  time.Time `json:"published"`
	ID         string    `json:"id"`
	Order      string    `json:"order"`
	FilterHash string    `json:"filterHash"`
}

type apiV1PostQuery struct {
	Limit      int
	Order      string
	Source     string
	Liked      *bool
	Author     string
	Tag        string
	Search     string
	Cursor     *apiV1PostCursor
	FilterHash string
}

func registerAPIV1Routes(mux *http.ServeMux, sessions *SessionStore, auth *AuthConfig, store *Store, platforms *BilibiliStore) {
	mux.HandleFunc("/api/v1", apiV1InfoHandler)
	mux.HandleFunc("/api/v1/health", apiV1HealthHandler)
	mux.HandleFunc("/api/v1/auth/login", apiV1LoginHandler(sessions, auth))
	mux.HandleFunc("/api/v1/auth/session", apiV1SessionHandler(sessions))
	mux.HandleFunc("/api/v1/auth/logout", apiV1LogoutHandler(sessions))
	mux.HandleFunc("/api/v1/posts", apiV1PostsHandler(store))
	mux.HandleFunc("/api/v1/feeds", apiV1FeedsHandler(store, platforms))
	mux.HandleFunc("/api/v1/sync", platforms.syncHandler)

	// Versioned aliases keep the Android client off the browser-only route namespace.
	mux.HandleFunc("/api/v1/bilibili/account", platforms.accountHandler)
	mux.HandleFunc("/api/v1/bilibili/qr", platforms.bilibiliQRHandler)
	mux.HandleFunc("/api/v1/bilibili/search", platforms.searchHandler)
	mux.HandleFunc("/api/v1/bilibili/subscriptions", platforms.subscriptionsHandler)
	mux.HandleFunc("/api/v1/pixiv/account", platforms.pixivHandler)
	mux.HandleFunc("/api/v1/pixiv/subscriptions", platforms.pixivSubscriptionsHandler)
	mux.HandleFunc("/api/v1/weibo/account", platforms.weiboAccountHandler)
	mux.HandleFunc("/api/v1/weibo/qr", platforms.weiboQRHandler)
	mux.HandleFunc("/api/v1/weibo/search", platforms.weiboSearchHandler)
	mux.HandleFunc("/api/v1/weibo/likes", platforms.weiboLikesHandler)
	mux.HandleFunc("/api/v1/weibo/subscriptions", platforms.weiboSubscriptionsHandler)
	mux.HandleFunc("/api/v1/project/settings", platforms.projectSettingsHandler)
	mux.HandleFunc("/api/v1/account/settings", settingsHandler(auth))
	mux.HandleFunc("/api/v1/configuration/backup", configurationBackupHandler(auth, platforms))
}

func apiV1InfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"service":           "Lumic",
		"apiVersion":        "v1",
		"authentication":    "Bearer",
		"sessionTTLSeconds": int(sessionLifetime.Seconds()),
		"defaultPageSize":   apiV1DefaultPageSize,
		"maxPageSize":       apiV1MaxPageSize,
	})
}

func apiV1HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"status": "ok", "apiVersion": "v1"})
}

func apiV1LoginHandler(sessions *SessionStore, auth *AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || !credentialsMatch(auth, input.Username, input.Password) {
			writeAPIError(w, http.StatusUnauthorized, "账号或密码错误")
			return
		}
		token, err := sessions.create()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法创建客户端会话")
			return
		}
		expiresAt, _ := sessions.expiration(token)
		writeJSON(w, map[string]any{
			"accessToken": token,
			"tokenType":   "Bearer",
			"expiresIn":   int(sessionLifetime.Seconds()),
			"expiresAt":   expiresAt.UTC(),
		})
	}
}

func apiV1SessionHandler(sessions *SessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		token := requestSessionToken(r)
		if token == "" || !sessions.valid(token) {
			writeJSON(w, map[string]any{"authenticated": false})
			return
		}
		expiresAt, _ := sessions.expiration(token)
		writeJSON(w, map[string]any{"authenticated": true, "expiresAt": expiresAt.UTC()})
	}
}

func apiV1LogoutHandler(sessions *SessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if token := requestSessionToken(r); token != "" {
			sessions.revoke(token)
		}
		clearSessionCookie(w)
		writeJSON(w, map[string]string{"status": "logged_out"})
	}
}

func credentialsMatch(auth *AuthConfig, username, password string) bool {
	auth.RLock()
	storedUsername, storedHash := auth.Username, auth.PasswordHash
	auth.RUnlock()
	return subtle.ConstantTimeCompare([]byte(username), []byte(storedUsername)) == 1 && passwordMatches(password, storedHash)
}

func bearerToken(r *http.Request) string {
	parts := strings.Fields(strings.TrimSpace(r.Header.Get("Authorization")))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func requestSessionToken(r *http.Request) string {
	if token := bearerToken(r); token != "" {
		return token
	}
	if cookie, err := r.Cookie("lumic_session"); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func isPublicAPIPath(path string) bool {
	switch path {
	case "/api/login", "/api/session", "/api/health", "/api/v1", "/api/v1/health", "/api/v1/auth/login", "/api/v1/auth/session":
		return true
	default:
		return false
	}
}

func (s *SessionStore) expiration(token string) (time.Time, bool) {
	s.RLock()
	expiresAt, ok := s.tokens[token]
	s.RUnlock()
	return expiresAt, ok
}

func apiV1PostsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			store.postsHandler(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		query, err := parseAPIV1PostQuery(r)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		store.RLock()
		posts := append([]Post(nil), store.posts...)
		store.RUnlock()
		posts = filterAndSortAPIV1Posts(posts, query)
		if query.Cursor != nil {
			remaining := posts[:0]
			for _, post := range posts {
				if apiV1PostAfterCursor(post, *query.Cursor, query.Order) {
					remaining = append(remaining, post)
				}
			}
			posts = remaining
		}
		hasMore := len(posts) > query.Limit
		if len(posts) > query.Limit {
			posts = posts[:query.Limit]
		}
		items := make([]apiV1Post, 0, len(posts))
		for _, post := range posts {
			items = append(items, toAPIV1Post(post))
		}
		page := apiV1PostPage{Items: items, HasMore: hasMore, Limit: query.Limit}
		if hasMore && len(posts) > 0 {
			last := posts[len(posts)-1]
			page.NextCursor, err = encodeAPIV1PostCursor(apiV1PostCursor{Version: 1, Published: last.Published, ID: last.ID, Order: query.Order, FilterHash: query.FilterHash})
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "无法生成下一页游标")
				return
			}
		}
		writeJSON(w, page)
	}
}

func parseAPIV1PostQuery(r *http.Request) (apiV1PostQuery, error) {
	values := r.URL.Query()
	query := apiV1PostQuery{
		Limit:  apiV1DefaultPageSize,
		Order:  strings.ToLower(strings.TrimSpace(values.Get("order"))),
		Source: strings.ToLower(strings.TrimSpace(values.Get("source"))),
		Author: strings.TrimSpace(values.Get("author")),
		Tag:    strings.TrimSpace(values.Get("tag")),
		Search: strings.TrimSpace(values.Get("q")),
	}
	if query.Order == "" {
		query.Order = "newest"
	}
	if query.Order != "newest" && query.Order != "oldest" {
		return query, &apiV1QueryError{"order 仅支持 newest 或 oldest"}
	}
	if query.Source == "" {
		query.Source = "all"
	}
	if query.Source != "all" && !validSourcesForAPI[Source(query.Source)] {
		return query, &apiV1QueryError{"source 不受支持"}
	}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 {
			return query, &apiV1QueryError{"limit 必须是正整数"}
		}
		if limit > apiV1MaxPageSize {
			limit = apiV1MaxPageSize
		}
		query.Limit = limit
	}
	if rawLiked := strings.TrimSpace(values.Get("liked")); rawLiked != "" {
		liked, err := strconv.ParseBool(rawLiked)
		if err != nil {
			return query, &apiV1QueryError{"liked 必须是 true 或 false"}
		}
		query.Liked = &liked
	}
	query.FilterHash = apiV1PostFilterHash(query)
	if rawCursor := strings.TrimSpace(values.Get("cursor")); rawCursor != "" {
		cursor, err := decodeAPIV1PostCursor(rawCursor)
		if err != nil || cursor.Version != 1 || strings.TrimSpace(cursor.ID) == "" || cursor.Order != query.Order || cursor.FilterHash != query.FilterHash {
			return query, &apiV1QueryError{"cursor 无效或与当前筛选条件不匹配"}
		}
		query.Cursor = &cursor
	}
	return query, nil
}

type apiV1QueryError struct{ message string }

func (e *apiV1QueryError) Error() string { return e.message }

var validSourcesForAPI = map[Source]bool{
	SourceBilibili: true,
	SourceWeibo:    true,
	SourcePixiv:    true,
	SourceTwitter:  true,
}

func apiV1PostFilterHash(query apiV1PostQuery) string {
	liked := ""
	if query.Liked != nil {
		liked = strconv.FormatBool(*query.Liked)
	}
	canonical := strings.Join([]string{
		query.Order,
		query.Source,
		liked,
		strings.ToLower(query.Author),
		strings.ToLower(query.Tag),
		strings.ToLower(query.Search),
	}, "\x1f")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:12])
}

func filterAndSortAPIV1Posts(posts []Post, query apiV1PostQuery) []Post {
	searchTerms := strings.Fields(strings.ToLower(query.Search))
	filtered := posts[:0]
	for _, post := range posts {
		if query.Source != "all" && string(post.Source) != query.Source {
			continue
		}
		if query.Liked != nil && post.Liked != *query.Liked {
			continue
		}
		if query.Author != "" && !strings.EqualFold(post.Author, query.Author) {
			continue
		}
		if query.Tag != "" && !containsEqualFold(post.Tags, strings.TrimPrefix(query.Tag, "#")) {
			continue
		}
		if len(searchTerms) > 0 {
			values := append([]string{post.Caption, post.Author}, post.Tags...)
			matchedAll := true
			for _, term := range searchTerms {
				matchedTerm := false
				for _, value := range values {
					if strings.Contains(strings.ToLower(value), term) {
						matchedTerm = true
						break
					}
				}
				if !matchedTerm {
					matchedAll = false
					break
				}
			}
			if !matchedAll {
				continue
			}
		}
		filtered = append(filtered, post)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Published.Equal(filtered[j].Published) {
			return filtered[i].ID < filtered[j].ID
		}
		if query.Order == "oldest" {
			return filtered[i].Published.Before(filtered[j].Published)
		}
		return filtered[i].Published.After(filtered[j].Published)
	})
	return filtered
}

func apiV1PostAfterCursor(post Post, cursor apiV1PostCursor, order string) bool {
	if post.Published.Equal(cursor.Published) {
		return post.ID > cursor.ID
	}
	if order == "oldest" {
		return post.Published.After(cursor.Published)
	}
	return post.Published.Before(cursor.Published)
}

func encodeAPIV1PostCursor(cursor apiV1PostCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeAPIV1PostCursor(value string) (apiV1PostCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return apiV1PostCursor{}, err
	}
	var cursor apiV1PostCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return apiV1PostCursor{}, err
	}
	return cursor, nil
}

func containsEqualFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func toAPIV1Post(post Post) apiV1Post {
	previews := make([]string, 0, len(post.Media))
	for _, media := range post.Media {
		previews = append(previews, apiV1PreviewPath(media))
	}
	previewVideos := make([]PostVideo, 0, len(post.Videos))
	for _, video := range post.Videos {
		previewVideos = append(previewVideos, PostVideo{URL: video.URL, Poster: apiV1PreviewPath(video.Poster)})
	}
	return apiV1Post{
		ID:            post.ID,
		Source:        post.Source,
		FeedIDs:       append([]string(nil), post.FeedIDs...),
		Author:        post.Author,
		Avatar:        post.Avatar,
		Caption:       post.Caption,
		Tags:          append([]string(nil), post.Tags...),
		Media:         append([]string(nil), post.Media...),
		PreviewMedia:  previews,
		Videos:        append([]PostVideo(nil), post.Videos...),
		PreviewVideos: previewVideos,
		OriginalURL:   post.OriginalURL,
		Published:     post.Published,
		Liked:         post.Liked,
	}
}

func apiV1PreviewPath(media string) string {
	if !strings.HasPrefix(media, "/flow/") {
		return media
	}
	return "/preview/" + strings.TrimPrefix(strings.SplitN(media, "?", 2)[0], "/flow/")
}

func apiV1FeedsHandler(store *Store, platforms *BilibiliStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.RLock()
		feeds := append([]SourceConfig(nil), store.feeds...)
		store.RUnlock()
		feeds = append(feeds, platforms.allSourceConfigs()...)
		unique := make(map[string]SourceConfig, len(feeds))
		for _, feed := range feeds {
			if strings.TrimSpace(feed.ID) != "" {
				unique[feed.ID] = feed
			}
		}
		result := make([]apiV1Feed, 0, len(unique))
		for _, feed := range unique {
			result = append(result, toAPIV1Feed(feed))
		}
		sort.Slice(result, func(i, j int) bool {
			if result[i].Source == result[j].Source {
				return result[i].Name < result[j].Name
			}
			return result[i].Source < result[j].Source
		})
		writeJSON(w, map[string]any{"items": result})
	}
}

func toAPIV1Feed(feed SourceConfig) apiV1Feed {
	return apiV1Feed{
		ID:              feed.ID,
		Source:          feed.Source,
		Name:            feed.Name,
		Handle:          feed.Handle,
		Avatar:          feed.Avatar,
		Enabled:         feed.Enabled,
		Schedule:        feed.Schedule,
		ContentTypes:    append([]string(nil), feed.ContentTypes...),
		Tags:            append([]string(nil), feed.Tags...),
		OnlyWithImages:  feed.OnlyWithImages,
		IncludeVideos:   feed.IncludeVideos,
		IncludeKeywords: append([]string(nil), feed.IncludeKeywords...),
		ExcludeKeywords: append([]string(nil), feed.ExcludeKeywords...),
		LastSyncedAt:    feed.LastSyncedAt,
		LastSyncStatus:  feed.LastSyncStatus,
		LastSyncMessage: feed.LastSyncMessage,
		LastSyncCount:   feed.LastSyncCount,
	}
}
