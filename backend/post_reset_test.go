package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func resetRequest(platform *BilibiliStore, id string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"id": id})
	platform.resetPostHandler(response, httptest.NewRequest(http.MethodPost, "/api/posts/reset", bytes.NewReader(body)))
	return response
}

func TestResetPostRearchivesAndPreservesMetadata(t *testing.T) {
	root := t.TempDir()
	oldRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldRoot })
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	mediaFailure := false
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mediaFailure {
			http.Error(w, "private-url-fixture", 403)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("new-original"))
	}))
	defer images.Close()
	fail, wrong := false, false
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/statuses/show" || r.URL.Query().Get("id") != "123" {
			t.Error("reset must only fetch one requested post")
		}
		if fail {
			http.Error(w, "private-secret", 429)
			return
		}
		id := "123"
		if wrong {
			id = "999"
		}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{
			"id": id, "text_raw": "fresh caption", "created_at": "Sun Nov 08 12:00:00 +0800 2026",
			"user": map[string]any{"id": "42", "screen_name": "author"},
			"pics": []any{map[string]any{"large": map[string]any{"url": images.URL + "/original.jpg"}}},
		}})
	})
	oldMedia := "/flow/old.jpg"
	if err := os.WriteFile(filepath.Join(root, "old.jpg"), []byte("old-original"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "author", Caption: "edited caption",
		FeedIDs: []string{"weibo-likes-42", "weibo-42"}, Tags: []string{"local-tag"}, Media: []string{oldMedia},
		Liked: true, FavoriteExplicit: true, Edited: true, Published: time.Now()}
	unrelated := Post{ID: "unrelated", Source: SourceWeibo, Author: "other", Caption: "unchanged"}
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{previous, unrelated}}
	for _, mode := range []string{"fetch", "wrong-id", "media", "success"} {
		fail, wrong, mediaFailure = mode == "fetch", mode == "wrong-id", mode == "media"
		platform.lastPostReset = time.Time{}
		response := resetRequest(platform, previous.ID)
		if mode != "success" {
			if response.Code != 502 || !postsEqual(platform.content.posts[0], previous) ||
				strings.Contains(response.Body.String(), "private") {
				t.Fatalf("%s reset damaged original or exposed remote failure: %d", mode, response.Code)
			}
			continue
		}
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		updated := platform.content.posts[0]
		if updated.Edited || updated.Caption != "fresh caption" || !updated.Liked || !updated.FavoriteExplicit ||
			!reflect.DeepEqual(updated.FeedIDs, previous.FeedIDs) || !containsString(updated.Tags, "local-tag") ||
			len(updated.Media) != 1 || updated.Media[0] == oldMedia {
			t.Fatal("reset must replace edited content but preserve local metadata")
		}
		if !postsEqual(platform.content.posts[1], unrelated) {
			t.Fatal("reset changed another post")
		}
		saved, err := loadStoreFile(platform.content.file)
		if err != nil || len(saved.posts) != 2 || saved.posts[0].Edited {
			t.Fatal("reset was not persisted", err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "old.jpg")); err != nil || string(data) != "old-original" {
		t.Fatal("reset must never delete old originals before success")
	}
}

func TestResetPostGuardsAndConcurrentChanges(t *testing.T) {
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	previous := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "author", Caption: "original"}
	store := &Store{posts: []Post{previous}}
	platform := &BilibiliStore{content: store}
	for _, id := range []string{"", "missing"} {
		want := 404
		if id == "" {
			want = 400
		}
		if got := resetRequest(platform, id).Code; got != want {
			t.Fatalf("id guard: %d", got)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/posts/reset", strings.NewReader(`{"id":"weibo-status-123"}`))
	anonymous := httptest.NewRecorder()
	authMiddleware(newSessionStore(nil), http.HandlerFunc(platform.resetPostHandler)).ServeHTTP(anonymous, request)
	if anonymous.Code != 401 {
		t.Fatal("reset must require authentication")
	}
	platform.resetMu.Lock()
	response := resetRequest(platform, previous.ID)
	platform.resetMu.Unlock()
	if response.Code != 409 {
		t.Fatal("overlapping reset must be rejected")
	}
	changed := previous
	changed.Caption = "new remote"
	store.posts[0].Caption = "concurrent edit"
	if _, err := store.replaceResetPost(previous, changed); err == nil || store.posts[0].Caption != "concurrent edit" {
		t.Fatal("reset overwrote a concurrent edit")
	}
	store.posts = nil
	if _, err := store.replaceResetPost(previous, changed); err == nil || len(store.posts) != 0 {
		t.Fatal("reset revived a deleted post")
	}
	store.posts = []Post{previous}
	store.file = t.TempDir()
	if err := os.WriteFile(filepath.Join(store.file, "keep"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.replaceResetPost(previous, changed); err == nil || store.posts[0].Caption != previous.Caption || store.posts[0].Edited {
		t.Fatal("failed persistence must roll back the post")
	}
}

func TestResetSinglePostPlatformEndpoints(t *testing.T) {
	payloads := map[string]any{
		"bilibili": map[string]any{"code": 0, "data": map[string]any{"item": map[string]any{
			"id_str": "123", "type": "DYNAMIC_TYPE_WORD", "modules": map[string]any{
				"module_author":  map[string]any{"name": "author", "pub_ts": 1794100800},
				"module_dynamic": map[string]any{"desc": map[string]any{"text": "fresh bili"}},
			},
		}}},
		"pixiv": map[string]any{"error": false, "body": map[string]any{"id": "123", "title": "fresh pixiv", "userName": "author", "pageCount": 1,
			"createDate": "2026-11-08T12:00:00+08:00", "urls": map[string]any{"original": "https://i.pximg.net/original.jpg"}}},
		"twitter": map[string]any{"tweets": []any{map[string]any{"id": "123", "text": "fresh tweet", "author": map[string]any{"name": "author", "userName": "author"}}}},
	}
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.Host {
		case "api.bilibili.com":
			if r.URL.Path != "/x/polymer/web-dynamic/v1/detail" || r.URL.Query().Get("id") != "123" {
				t.Error("wrong Bilibili detail route")
			}
			writeJSON(w, payloads["bilibili"])
		case "www.pixiv.net":
			if r.URL.Path != "/ajax/illust/123" {
				t.Error("wrong Pixiv detail route")
			}
			writeJSON(w, payloads["pixiv"])
		case "api.twitterapi.io":
			if r.URL.Path != "/twitter/tweets" || r.URL.Query().Get("tweet_ids") != "123" {
				t.Error("wrong Twitter detail route")
			}
			writeJSON(w, payloads["twitter"])
		default:
			t.Error("unexpected host")
		}
	}))
	defer server.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", 400)
			return
		}
		switch r.Host {
		case "api.bilibili.com:443", "www.pixiv.net:443", "api.twitterapi.io:443":
		default:
			t.Error("unexpected proxy target")
			http.Error(w, "denied", 403)
			return
		}
		target, err := net.Dial("tcp", server.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			target.Close()
			t.Error(err)
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { defer conn.Close(); defer target.Close(); _, _ = io.Copy(target, conn) }()
		_, _ = io.Copy(conn, target)
	}))
	defer proxy.Close()
	previousTransport := http.DefaultTransport
	transport := previousTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previousTransport; transport.CloseIdleConnections() })
	platform := &BilibiliStore{config: BilibiliConfig{
		ProxyURL:    proxy.URL,
		Credentials: BilibiliCredentials{SESSDATA: "fixture"}, Pixiv: PixivCredentials{Cookie: "fixture"},
		Twitter: TwitterCredentials{APIKey: "fixture"},
	}}
	feed := SourceConfig{Name: "author", ContentTypes: []string{"DRAW", "WORD", "ARTICLE"}, IncludeVideos: true}
	for _, test := range []struct {
		source Source
		id     string
	}{
		{SourceBilibili, "bili-dynamic-123"}, {SourcePixiv, "pixiv-illust-123"}, {SourceTwitter, "twitter-tweet-123"},
	} {
		feed.Source = test.source
		post, err := platform.fetchResetPost(Post{ID: test.id, Source: test.source}, feed)
		if err != nil || post.ID != test.id || post.Source != test.source || !strings.HasPrefix(post.Caption, "fresh") {
			t.Fatalf("detail reset for %s: %v", test.source, err)
		}
	}
	if calls != 3 {
		t.Fatalf("single-post reset must not fetch an entire timeline: %d requests", calls)
	}
}
