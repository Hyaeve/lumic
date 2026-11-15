package main

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The proxy only accepts fixture hosts; no provider request can escape the test.
func authorWindowProxy(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || (r.Host != "api.bilibili.com:443" && r.Host != "www.pixiv.net:443" && r.Host != "api.twitterapi.io:443") {
			t.Errorf("unexpected fixture target: %s %s", r.Method, r.Host)
			http.Error(w, "fixture target denied", 400)
			return
		}
		target, err := net.Dial("tcp", server.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			target.Close()
			t.Error(err)
			return
		}
		_, _ = connection.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { defer connection.Close(); defer target.Close(); _, _ = io.Copy(target, connection) }()
		_, _ = io.Copy(connection, target)
	}))
	previous := http.DefaultTransport
	transport := previous.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = previous
		transport.CloseIdleConnections()
		proxy.Close()
		server.Close()
	})
	return proxy.URL
}

func TestAuthorWindowBilibiliTraversesInitialPagesAndRejectsIncompleteCursor(t *testing.T) {
	now := time.Now()
	var mode, calls atomic.Int32
	item := func(id string, published time.Time) map[string]any {
		return map[string]any{"id_str": id, "type": "DYNAMIC_TYPE_WORD", "modules": map[string]any{
			"module_author":  map[string]any{"name": "fixture", "mid": 42, "pub_ts": published.Unix()},
			"module_dynamic": map[string]any{"desc": map[string]string{"text": "fixture caption"}},
		}}
	}
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Host != "api.bilibili.com" || r.URL.Path != "/x/polymer/web-dynamic/v1/feed/space" || r.URL.Query().Get("host_mid") != "42" {
			t.Error("wrong Bilibili author route")
		}
		offset := r.URL.Query().Get("offset")
		var items []any
		next := "next"
		switch offset {
		case "":
			items = []any{item("100", now.Add(-100*time.Hour)), item("101", now.Add(-time.Hour))}
		case "next":
			items, next = []any{item("102", now.Add(-48*time.Hour))}, "older"
		case "older":
			items, next = []any{item("103", now.Add(-96*time.Hour))}, "end"
		default:
			t.Error("fetched beyond initial 72h window")
		}
		if mode.Load() == 1 {
			items, next = []any{item("101", now.Add(-time.Hour))}, ""
		}
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": items, "offset": next, "has_more": true}})
	})
	b := &BilibiliStore{config: BilibiliConfig{ProxyURL: proxy, Credentials: BilibiliCredentials{SESSDATA: "fixture"}}}
	feed := SourceConfig{ID: "bili-42", Source: SourceBilibili, ContentTypes: []string{"WORD"}, AuthorSync: &authorSyncWindow{SubscribedAt: now}}
	posts, err := b.fetchBilibiliPosts(feed, false)
	if err != nil || calls.Load() != 3 || len(posts) != 2 || posts[0].ID != "bili-dynamic-101" || posts[1].ID != "bili-dynamic-102" {
		t.Fatalf("initial window stopped at pinned old post or first page: %v, %d calls, %+v", err, calls.Load(), posts)
	}
	calls.Store(0)
	legacy := feed
	legacy.AuthorSync = nil
	if posts, err := b.fetchBilibiliPosts(legacy, false); err != nil || calls.Load() != 1 || len(posts) != 2 {
		t.Fatalf("legacy first-page behavior changed: %v, %d calls", err, calls.Load())
	}
	mode.Store(1)
	if _, err := b.fetchBilibiliPosts(feed, false); err == nil {
		t.Fatal("incomplete Bilibili pagination was reported as a complete author window")
	}
}

func TestAuthorWindowPixivIsNotTruncatedToTwentyFourAndRejectsPartialDetails(t *testing.T) {
	now := time.Now()
	var requests, fail atomic.Int32
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "www.pixiv.net" {
			t.Error("wrong Pixiv host")
		}
		if r.URL.Path == "/ajax/user/42/profile/all" {
			works := make(map[string]any)
			for id := 100; id <= 125; id++ {
				works[strconv.Itoa(id)] = nil
			}
			writeJSON(w, map[string]any{"error": false, "body": map[string]any{"illusts": works}})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/ajax/illust/")
		requests.Add(1)
		if fail.Load() != 0 {
			http.Error(w, "fixture unavailable", 403)
			return
		}
		published := now.Add(-time.Hour)
		if id == "100" {
			published = now.Add(-96 * time.Hour)
		}
		writeJSON(w, map[string]any{"error": false, "body": map[string]any{
			"id": id, "userId": "42", "userName": "fixture", "createDate": published.Format(time.RFC3339),
			"pageCount": 1, "urls": map[string]string{"original": "https://i.pximg.net/" + id + ".jpg"},
		}})
	})
	b := &BilibiliStore{config: BilibiliConfig{ProxyURL: proxy, Pixiv: PixivCredentials{Cookie: "fixture", UserID: "99"}}}
	feed := SourceConfig{ID: "pixiv-42", Source: SourcePixiv, AuthorSync: &authorSyncWindow{SubscribedAt: now}}
	posts, err := b.fetchPixivPosts(feed, false)
	if err != nil || len(posts) != 25 || requests.Load() != 26 || posts[24].ID != "pixiv-illust-101" {
		t.Fatalf("Pixiv author window was truncated or failed its time boundary: %v, %d posts, %d details", err, len(posts), requests.Load())
	}
	fail.Store(1)
	if _, err := b.fetchPixivPosts(feed, false); err == nil {
		t.Fatal("Pixiv incomplete details must not be considered a successful window")
	}
}

func TestAuthorWindowTwitterTraversesPinnedPageAndPreservesFiveSecondGate(t *testing.T) {
	now := time.Now()
	var mu sync.Mutex
	var times []time.Time
	tweet := func(id string, published time.Time) map[string]any {
		return map[string]any{"id": id, "text": "fixture", "createdAt": published.Format(time.RFC3339), "author": map[string]string{"userName": "author42", "name": "fixture"}}
	}
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		if r.Host != "api.twitterapi.io" || r.URL.Path != "/twitter/user/last_tweets" || r.URL.Query().Get("userName") != "author42" {
			t.Error("wrong Twitter author route")
		}
		items := []any{tweet("100", now.Add(-100*time.Hour)), tweet("101", now.Add(-time.Hour))}
		next := "older"
		if r.URL.Query().Get("cursor") == "older" {
			items, next = []any{tweet("102", now.Add(-96*time.Hour))}, "never"
		}
		writeJSON(w, map[string]any{"tweets": items, "has_next_page": true, "next_cursor": next})
	})
	twitterRequestGate.Lock()
	previous := twitterRequestGate.last
	twitterRequestGate.last = time.Time{}
	twitterRequestGate.Unlock()
	t.Cleanup(func() { twitterRequestGate.Lock(); twitterRequestGate.last = previous; twitterRequestGate.Unlock() })
	b := &BilibiliStore{config: BilibiliConfig{ProxyURL: proxy, Twitter: TwitterCredentials{APIKey: "fixture", UserName: "owner"}}}
	feed := SourceConfig{ID: "twitter-author-author42", Source: SourceTwitter, AuthorSync: &authorSyncWindow{SubscribedAt: now}}
	posts, err := b.fetchTwitterPosts(feed, false)
	if err != nil || len(postsAfter(posts, sourceIncrementalBoundary(feed))) != 1 {
		t.Fatalf("Twitter bounded author window failed: %v, %+v", err, posts)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(times) != 2 || times[1].Sub(times[0]) < 4900*time.Millisecond {
		t.Fatalf("Twitter page traversal bypassed five-second request gate: %+v", times)
	}
}

func TestAuthorWindowConfigurationPersistsAndFailedSaveRollsBack(t *testing.T) {
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("configuration test must not launch an upstream pull")
		http.Error(w, "fixture denied", 403)
	})
	oldFile := bilibiliFile
	bilibiliFile = t.TempDir()
	if err := os.WriteFile(filepath.Join(bilibiliFile, "keep"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	target := authorSubscriptionTarget{Source: SourceWeibo, ID: "42", Name: "fixture"}
	at := time.Date(2026, time.November, 16, 4, 30, 0, 0, time.UTC)
	if _, status := b.changeAuthorSubscription(target, false, at, "0 12 * * 1", "Asia/Shanghai"); status != 500 || len(b.allSourceConfigs()) != 0 || len(b.syncTasks.views(nil)) != 0 {
		t.Fatalf("failed save leaked subscription or a background task: %d", status)
	}
	bilibiliFile = oldFile
	release := fillSourceSyncTestPlatform(t, b, SourceWeibo)
	t.Cleanup(release)
	view, status := b.changeAuthorSubscription(target, false, at, "0 12 * * 1", "Asia/Shanghai")
	if status != 202 || view.Task == nil {
		t.Fatalf("valid persisted subscription failed: %d %+v", status, view)
	}
	data, err := os.ReadFile(bilibiliFile)
	var restored BilibiliConfig
	if err != nil || decryptJSON(b.key, data, &restored) != nil || len(restored.WeiboSubscriptions) != 1 {
		t.Fatal("encrypted author subscription could not be restored")
	}
	feed := restored.WeiboSubscriptions[0]
	if feed.Schedule != "0 12 * * 1" || feed.ScheduleTimeZone != "Asia/Shanghai" || feed.AuthorSync == nil || !feed.AuthorSync.SubscribedAt.Equal(at) || !sourceIncrementalBoundary(feed).Equal(at.Add(-72*time.Hour)) {
		t.Fatalf("timezone or initial cursor lost on restore: %+v", feed)
	}
	queued := findSourceSyncTestTask(t, b, feed.ID)
	if _, status := b.changeAuthorSubscription(target, true, at, "", ""); status != 200 {
		t.Fatal("queued fixture subscription was not removed")
	}
	release()
	if result := waitQueueTestTask(t, queued); result.err == nil {
		t.Fatal("removed fixture subscription unexpectedly reached upstream")
	}
}
