package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func resetWeiboPayload() map[string]any {
	return map[string]any{"id": "123", "text_raw": "fresh caption", "created_at": "Sun Nov 08 12:00:00 +0800 2026",
		"user": map[string]any{"id": "42", "screen_name": "author"}}
}

func TestResetWeiboUsesWorkingDesktopSession(t *testing.T) {
	previousRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = previousRoot })
	platform := newWeiboAuthTestStore(t, WeiboCredentials{Cookie: "SUB=desktop-fixture", CookieScoped: true, UserID: "99"})
	previous := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "author", Caption: "old caption", FeedIDs: []string{"weibo-likes-99"}, Liked: true}
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{previous}}
	var calls []string
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Host+r.URL.Path)
		if r.Host == "m.weibo.cn" {
			writeJSON(w, map[string]any{"ok": -100})
			return
		}
		if r.Host != "weibo.com" || r.Header.Get("Cookie") != "SUB=desktop-fixture" {
			t.Error("desktop reset must use the same scoped session as likes")
		}
		switch r.URL.Path {
		case "/ajax/statuses/likelist":
			writeJSON(w, map[string]any{"ok": 1, "statuses": []any{resetWeiboPayload()}})
		case "/ajax/statuses/show":
			if r.URL.Query().Get("id") != "123" {
				t.Error("reset fetched the wrong status")
			}
			writeJSON(w, resetWeiboPayload())
		default:
			t.Error("unexpected desktop route")
		}
	})
	liked, err := platform.fetchWeiboLikedPosts(SourceConfig{ID: "weibo-likes-99", Source: SourceWeibo}, false)
	if err != nil || len(liked) != 1 {
		t.Fatal("desktop likes fixture did not succeed", err)
	}
	response := resetRequest(platform, previous.ID)
	if response.Code != http.StatusOK || platform.content.posts[0].Caption != "fresh caption" || !platform.content.posts[0].Liked {
		t.Fatalf("working desktop session must reset without a mobile login: %d %s", response.Code, response.Body.String())
	}
	if len(calls) != 2 || calls[1] != "weibo.com/ajax/statuses/show" {
		t.Fatalf("reset should fetch only the desktop detail: %v", calls)
	}
}

func TestResetWeiboDetailCompatibilityAndCookieIsolation(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		credentials WeiboCredentials
		status      int
		wantCalls   int
	}{
		{"missing-route", WeiboCredentials{Cookie: "SUB=desktop", MobileCookie: "SUB=mobile", CookieScoped: true, UserID: "99"}, 404, 2},
		{"gone-route", WeiboCredentials{Cookie: "SUB=desktop", MobileCookie: "SUB=mobile", CookieScoped: true, UserID: "99"}, 410, 2},
		{"expired-desktop", WeiboCredentials{Cookie: "SUB=desktop", CookieScoped: true, UserID: "99"}, 401, 2},
		{"mobile-only", WeiboCredentials{MobileCookie: "SUB=mobile", CookieScoped: true, UserID: "99"}, 0, 1},
		{"public-only", WeiboCredentials{}, 0, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			platform := newWeiboAuthTestStore(t, scenario.credentials)
			calls := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Host == "weibo.com" {
					if r.URL.Path != "/ajax/statuses/show" || r.Header.Get("Cookie") != "SUB=desktop" {
						t.Error("wrong desktop route or cookie")
					}
					w.WriteHeader(scenario.status)
					return
				}
				if r.Host != "m.weibo.cn" || r.URL.Path != "/statuses/show" || r.URL.Query().Get("id") != "123" || r.Header.Get("Cookie") != scenario.credentials.MobileCookie {
					t.Error("mobile detail must use only its own cookie or public access")
				}
				writeJSON(w, map[string]any{"ok": 1, "data": resetWeiboPayload()})
			})
			post, err := platform.fetchResetPost(Post{ID: "weibo-status-123", Source: SourceWeibo}, SourceConfig{Source: SourceWeibo})
			if err != nil || post.Caption != "fresh caption" || calls != scenario.wantCalls {
				t.Fatalf("detail compatibility failed: post=%s calls=%d err=%v", post.ID, calls, err)
			}
		})
	}
}

func TestResetWeiboDesktopRejectsDoNotFallback(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		status     int
	}{
		{"forbidden", `{}`, 403},
		{"limited", `{}`, 429},
		{"captcha", `{"ok":0,"msg":"安全验证"}`, 200},
		{"upstream", `{"ok":0,"msg":"unavailable"}`, 200},
		{"wrong-id", `{"id":"999","user":{"id":"42"},"text_raw":"wrong"}`, 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			platform := newWeiboAuthTestStore(t, WeiboCredentials{Cookie: "SUB=desktop", CookieScoped: true, UserID: "99"})
			previous := Post{ID: "weibo-status-123", Source: SourceWeibo, Caption: "old caption"}
			platform.content = &Store{posts: []Post{previous}}
			calls := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Host != "weibo.com" || r.URL.Path != "/ajax/statuses/show" {
					t.Error("explicit refusal or mismatched detail must not switch endpoints")
				}
				w.WriteHeader(scenario.status)
				w.Write([]byte(scenario.body))
			})
			response := resetRequest(platform, previous.ID)
			if response.Code != 502 || calls != 1 || !postsEqual(platform.content.posts[0], previous) || strings.Contains(response.Body.String(), "SUB=") {
				t.Fatalf("refusal protection failed: status=%d calls=%d", response.Code, calls)
			}
		})
	}
}

func TestResetWeiboUsesRenewedCookieForArchive(t *testing.T) {
	previousRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = previousRoot })
	platform := newWeiboAuthTestStore(t, WeiboCredentials{Cookie: "SUB=old", MobileCookie: "SUB=mobile", CookieScoped: true, UserID: "99"})
	previous := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "author", Caption: "old caption"}
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{previous}}
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "SUB=renewed" {
			http.Error(w, "stale session fixture", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("new-original"))
	}))
	defer images.Close()
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "weibo.com" || r.URL.Path != "/ajax/statuses/show" || r.Header.Get("Cookie") != "SUB=old" {
			t.Error("unexpected detail request")
		}
		http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "renewed", Path: "/"})
		payload := resetWeiboPayload()
		payload["pics"] = []any{map[string]any{"large": map[string]any{"url": images.URL + "/original.jpg"}}}
		writeJSON(w, payload)
	})
	response := resetRequest(platform, previous.ID)
	if response.Code != 200 || platform.config.Weibo.Cookie != "SUB=renewed" || platform.config.Weibo.MobileCookie != "SUB=mobile" || len(platform.content.posts[0].Media) != 1 {
		t.Fatalf("reset must archive with the renewed desktop session: %d %s", response.Code, response.Body.String())
	}
}
