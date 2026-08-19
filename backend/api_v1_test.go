package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAPIV1BearerLoginAuthorizesAndLogoutRevokes(t *testing.T) {
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	auth := &AuthConfig{Username: "lumir", PasswordHash: hashPassword("correct-password", []byte("lumic-default-salt-v1"))}
	store := &Store{posts: []Post{{ID: "post-1", Source: SourceWeibo, Author: "Author", Published: time.Now()}}}
	platforms := &BilibiliStore{config: BilibiliConfig{Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}, PixivSubscriptions: []SourceConfig{}}}
	mux := http.NewServeMux()
	registerAPIV1Routes(mux, sessions, auth, store, platforms)
	handler := authMiddleware(sessions, mux)

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"lumir","password":"correct-password"}`))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("v1 login failed: status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	var loginPayload struct {
		AccessToken string    `json:"accessToken"`
		TokenType   string    `json:"tokenType"`
		ExpiresIn   int       `json:"expiresIn"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginPayload.AccessToken == "" || loginPayload.TokenType != "Bearer" || loginPayload.ExpiresIn != int(sessionLifetime.Seconds()) || loginPayload.ExpiresAt.Before(time.Now().Add(23*time.Hour)) {
		t.Fatalf("unexpected login payload: %#v", loginPayload)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("protected v1 endpoint allowed an anonymous request: status=%d", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/posts?limit=1", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK || !strings.Contains(authorized.Body.String(), `"id":"post-1"`) {
		t.Fatalf("bearer token did not authorize v1 posts: status=%d body=%s", authorized.Code, authorized.Body.String())
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.Header.Set("Authorization", "bearer "+loginPayload.AccessToken)
	sessionResponse := httptest.NewRecorder()
	handler.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK || !strings.Contains(sessionResponse.Body.String(), `"authenticated":true`) {
		t.Fatalf("v1 session endpoint rejected bearer token: status=%d body=%s", sessionResponse.Code, sessionResponse.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusOK || sessions.valid(loginPayload.AccessToken) {
		t.Fatalf("v1 logout did not revoke token: status=%d body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}

	revokedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	revokedRequest.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	revokedResponse := httptest.NewRecorder()
	handler.ServeHTTP(revokedResponse, revokedRequest)
	if revokedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token remained authorized: status=%d", revokedResponse.Code)
	}
}

func TestAPIV1PostCursorPaginationIsStable(t *testing.T) {
	base := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store := &Store{posts: []Post{
		{ID: "post-a", Source: SourceWeibo, Author: "Alice", Caption: "Newest", Media: []string{"/flow/weibo/Alice/a.jpg"}, Videos: []PostVideo{{URL: "/flow/weibo/Alice/a.mp4", Poster: "/flow/weibo/Alice/a-poster.jpg"}}, TextFile: "/internal/post_contents.txt", Published: base.Add(3 * time.Hour)},
		{ID: "post-b", Source: SourcePixiv, Author: "Beta", Caption: "Second", Published: base.Add(2 * time.Hour)},
		{ID: "post-c", Source: SourceBilibili, Author: "Charlie", Caption: "Third", Published: base.Add(2 * time.Hour)},
		{ID: "post-d", Source: SourceWeibo, Author: "Delta", Caption: "Oldest", Published: base.Add(time.Hour)},
	}}
	handler := apiV1PostsHandler(store)

	firstResponse := httptest.NewRecorder()
	handler(firstResponse, httptest.NewRequest(http.MethodGet, "/api/v1/posts?limit=2&statsDate=2026-08-15&tzOffset=0", nil))
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first page failed: status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first apiV1PostPage
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "post-a" || first.Items[1].ID != "post-b" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if first.Stats == nil || first.Stats.All.Total != 4 || first.Stats.All.Today != 4 || first.Stats.All.Favorites != 0 {
		t.Fatalf("first page did not expose complete timeline statistics: %#v", first.Stats)
	}
	if first.Stats.BySource[SourceWeibo].Total != 2 || first.Stats.BySource[SourcePixiv].Total != 1 || first.Stats.BySource[SourceBilibili].Total != 1 {
		t.Fatalf("first page source statistics are incorrect: %#v", first.Stats.BySource)
	}
	if len(first.Items[0].PreviewMedia) != 1 || first.Items[0].PreviewMedia[0] != "/preview/weibo/Alice/a.jpg" {
		t.Fatalf("preview media was not exposed correctly: %#v", first.Items[0].PreviewMedia)
	}
	if len(first.Items[0].PreviewVideos) != 1 || first.Items[0].PreviewVideos[0].Poster != "/preview/weibo/Alice/a-poster.jpg" || first.Items[0].PreviewVideos[0].URL != "/flow/weibo/Alice/a.mp4" {
		t.Fatalf("preview video poster was not exposed correctly: %#v", first.Items[0].PreviewVideos)
	}
	if strings.Contains(firstResponse.Body.String(), "textFile") || strings.Contains(firstResponse.Body.String(), "post_contents") {
		t.Fatalf("v1 response exposed internal text archive path: %s", firstResponse.Body.String())
	}

	store.Lock()
	store.posts = append(store.posts, Post{ID: "post-new", Source: SourceWeibo, Author: "Inserted", Published: base.Add(4 * time.Hour)})
	store.Unlock()
	secondURL := "/api/v1/posts?limit=2&cursor=" + url.QueryEscape(first.NextCursor)
	secondResponse := httptest.NewRecorder()
	handler(secondResponse, httptest.NewRequest(http.MethodGet, secondURL, nil))
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second page failed: status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	var second apiV1PostPage
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(second.Items) != 2 || second.Items[0].ID != "post-c" || second.Items[1].ID != "post-d" || second.HasMore || second.NextCursor != "" {
		t.Fatalf("cursor page duplicated, skipped, or included a newly inserted leading post: %#v", second)
	}

	mismatchedResponse := httptest.NewRecorder()
	mismatchedURL := secondURL + "&source=weibo"
	handler(mismatchedResponse, httptest.NewRequest(http.MethodGet, mismatchedURL, nil))
	if mismatchedResponse.Code != http.StatusBadRequest {
		t.Fatalf("cursor was accepted with different filters: status=%d body=%s", mismatchedResponse.Code, mismatchedResponse.Body.String())
	}

	malformedResponse := httptest.NewRecorder()
	handler(malformedResponse, httptest.NewRequest(http.MethodGet, "/api/v1/posts?cursor=not-a-cursor", nil))
	if malformedResponse.Code != http.StatusBadRequest {
		t.Fatalf("malformed cursor was accepted: status=%d", malformedResponse.Code)
	}
}

func TestAPIV1PostFiltersOldestOrderAndLimitCap(t *testing.T) {
	base := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	store := &Store{posts: []Post{
		{ID: "one", Source: SourceWeibo, Author: "Alice", Caption: "Morning note", Tags: []string{"Daily"}, Liked: true, Published: base},
		{ID: "two", Source: SourceWeibo, Author: "Alice", Caption: "Evening note", Tags: []string{"Other"}, Liked: true, Published: base.Add(2 * time.Hour)},
		{ID: "three", Source: SourcePixiv, Author: "Alice", Caption: "Morning note", Tags: []string{"Daily"}, Liked: true, Published: base.Add(time.Hour)},
	}}
	handler := apiV1PostsHandler(store)
	response := httptest.NewRecorder()
	handler(response, httptest.NewRequest(http.MethodGet, "/api/v1/posts?source=weibo&liked=true&author=alice&tag=%23daily&q=morning&order=oldest&limit=999", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("filtered page failed: status=%d body=%s", response.Code, response.Body.String())
	}
	var page apiV1PostPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode filtered page: %v", err)
	}
	if page.Limit != apiV1MaxPageSize || len(page.Items) != 1 || page.Items[0].ID != "one" {
		t.Fatalf("unexpected filtered page: %#v", page)
	}
}

func TestAPIV1FeedsAreUnifiedAndHideStoragePaths(t *testing.T) {
	store := &Store{feeds: []SourceConfig{{ID: "legacy", Source: SourceTwitter, Name: "Legacy", Enabled: true, StoragePath: "C:/private/legacy"}}}
	platforms := &BilibiliStore{config: BilibiliConfig{
		Subscriptions:      []SourceConfig{{ID: "bili-1", Source: SourceBilibili, Name: "UP", Enabled: true, StoragePath: "/flow/bilibili/UP"}},
		WeiboSubscriptions: []SourceConfig{{ID: "weibo-1", Source: SourceWeibo, Name: "博主", Enabled: true, StoragePath: "/flow/weibo/博主"}},
		PixivSubscriptions: []SourceConfig{},
	}}
	response := httptest.NewRecorder()
	apiV1FeedsHandler(store, platforms)(response, httptest.NewRequest(http.MethodGet, "/api/v1/feeds", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("v1 feeds failed: status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []apiV1Feed `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode feeds: %v", err)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("expected unified feeds, got %#v", payload.Items)
	}
	if strings.Contains(response.Body.String(), "storagePath") || strings.Contains(response.Body.String(), "/flow/") || strings.Contains(response.Body.String(), "C:/private") {
		t.Fatalf("v1 feeds exposed an internal storage path: %s", response.Body.String())
	}
}
