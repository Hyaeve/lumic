package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func authorSubscriptionRequest(t *testing.T, b *BilibiliStore, method, path, body string, status int) authorSubscriptionResponse {
	t.Helper()
	response := httptest.NewRecorder()
	b.authorSubscriptionHandler(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%s: want %d no-store, got %d %s", method, status, response.Code, response.Body.String())
	}
	var view authorSubscriptionResponse
	if status < 400 && json.Unmarshal(response.Body.Bytes(), &view) != nil {
		t.Fatalf("%s returned invalid JSON: %s", method, response.Body.String())
	}
	return view
}

func TestAuthorSubscriptionWeeklyLocalClock(t *testing.T) {
	clicked := time.Date(2026, time.November, 16, 4, 30, 0, 0, time.UTC)
	schedule, err := weeklyAuthorSchedule(clicked, "Asia/Shanghai")
	if err != nil || schedule != "0 12 * * 1" {
		t.Fatalf("unexpected local cron %q: %v", schedule, err)
	}
	feed := SourceConfig{Schedule: schedule, ScheduleTimeZone: "Asia/Shanghai"}
	next := time.Date(2026, time.November, 23, 4, 0, 0, 0, time.UTC)
	if !sourceScheduleMatches(feed, next) || sourceScheduleMatches(feed, next.Add(time.Hour)) || sourceScheduleMatches(feed, clicked) {
		t.Fatal("stored local schedule did not match the intended weekly UTC window")
	}
	schedule, err = weeklyAuthorSchedule(time.Date(2026, time.November, 16, 2, 40, 0, 0, time.UTC), "America/New_York")
	if err != nil || schedule != "0 21 * * 0" {
		t.Fatalf("local day rollover did not affect weekday: %q, %v", schedule, err)
	}
	if _, err := weeklyAuthorSchedule(clicked, "bad/timezone"); err == nil {
		t.Fatal("invalid time zone accepted")
	}
	for _, target := range []authorSubscriptionTarget{
		{Source: SourceBilibili, ID: "123", Name: "author"},
		{Source: SourceWeibo, ID: "123", Name: "author"},
		{Source: SourcePixiv, ID: "123", Name: "author"},
		{Source: SourceTwitter, ID: "author_123", Name: "author"},
	} {
		feed := newAuthorSubscription(target, clicked, schedule, "America/New_York")
		if !feed.Enabled || !feed.OnlyWithImages || feed.IncludeVideos || feed.StartDate != "" ||
			sourceAuthorIDFromFeedID(feed.Source, feed.ID) != target.ID || !sourceIncrementalBoundary(feed).Equal(clicked.Add(-72*time.Hour)) || !sourceLivePhotosEnabled(feed) {
			t.Fatalf("author subscription did not preserve platform defaults and initial window: %+v", feed)
		}
		if target.Source == SourceBilibili && (len(feed.ContentTypes) != 1 || feed.ContentTypes[0] != "DRAW") {
			t.Fatal("Bilibili default draw option changed")
		}
	}
}

func TestAuthorSubscriptionEnqueueCancelAndPreserveArchives(t *testing.T) {
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	post := Post{ID: "weibo-status-100", Source: SourceWeibo, Author: "account", AuthorID: "42", Caption: "keep this"}
	b.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{post}}
	b.config.WeiboSubscriptions = []SourceConfig{
		{ID: "weibo-likes-42", Source: SourceWeibo, Name: "account"},
		{ID: "weibo-43", Source: SourceWeibo, Name: "account"},
	}
	release := fillSourceSyncTestPlatform(t, b, SourceWeibo)
	t.Cleanup(release)
	path := "/api/author-subscription?source=weibo&author=account&postId=" + post.ID
	body := `{"source":"weibo","author":"account","postId":"weibo-status-100","timeZone":"Asia/Shanghai"}`
	if view := authorSubscriptionRequest(t, b, "GET", path, "", 200); !view.Available || view.Subscribed {
		t.Fatalf("unrelated same-name and collection sources must not match: %+v", view)
	}
	view := authorSubscriptionRequest(t, b, "POST", path, body, 202)
	if !view.Subscribed || view.Feed == nil || view.Feed.ID != "weibo-42" || view.Feed.ScheduleTimeZone != "Asia/Shanghai" || view.Task == nil || view.Task.Status != syncTaskQueued {
		t.Fatalf("subscription did not enqueue a background task: %+v", view)
	}
	task := findSourceSyncTestTask(t, b, "weibo-42")
	if again := authorSubscriptionRequest(t, b, "POST", path, body, 200); !again.Subscribed || again.Feed.ID != view.Feed.ID || again.Task != nil || again.Feed.Schedule != view.Feed.Schedule {
		t.Fatalf("duplicate subscription didn't preserve the existing settings: %+v", again)
	}
	if status := authorSubscriptionRequest(t, b, "GET", path, "", 200); !status.Subscribed || status.Feed.ID != "weibo-42" {
		t.Fatalf("exact author's subscription is not detected: %+v", status)
	}
	removed := authorSubscriptionRequest(t, b, "DELETE", path, "", 200)
	if !removed.Available || removed.Subscribed || len(b.allSourceConfigs()) != 2 || !postsEqual(b.content.posts[0], post) {
		t.Fatalf("unsubscribe changed unrelated configuration or archive: %+v", removed)
	}
	authorSubscriptionRequest(t, b, "POST", path, body, 409)
	release()
	if result := waitQueueTestTask(t, task); result.err == nil || !strings.Contains(result.err.Error(), "订阅已删除") {
		t.Fatalf("deleted queued subscription still launched: %+v", result)
	}
	if status := authorSubscriptionRequest(t, b, "GET", path, "", 200); status.Subscribed || !status.Available {
		t.Fatalf("retained archived author cannot be subscribed again: %+v", status)
	}
	for _, verb := range []string{"GET", "POST", "DELETE"} {
		response := httptest.NewRecorder()
		authMiddleware(newSessionStore(nil), http.HandlerFunc(b.authorSubscriptionHandler)).ServeHTTP(response, httptest.NewRequest(verb, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s was not rejected", verb)
		}
	}
}

func TestAuthorSubscriptionCollectionRequiresMatchingIdentity(t *testing.T) {
	legacy := SourceConfig{ID: "pixiv-43", Source: SourcePixiv, Name: "old-name"}
	quick := SourceConfig{ID: "pixiv-42", Source: SourcePixiv, Name: "current-name", AuthorSync: &authorSyncWindow{SubscribedAt: time.Now()}}
	post := Post{Source: SourcePixiv, Author: "old-name", AuthorID: "42", FeedIDs: []string{"pixiv-bookmarks-99"}}
	if got, ok := collectionAuthorSubscription(post, []SourceConfig{legacy, quick}); !ok || got.ID != quick.ID {
		t.Fatal("exact author must win even when its display name changed")
	}
	post.Author = quick.Name
	for _, unattributedID := range []string{"101", ""} {
		post.AuthorID = unattributedID
		if _, ok := collectionAuthorSubscription(post, []SourceConfig{quick}); ok {
			t.Fatal("same display name cannot send unrelated posts to a quick subscription")
		}
	}
	post.Author = legacy.Name
	if got, ok := collectionAuthorSubscription(post, []SourceConfig{legacy}); !ok || got.ID != legacy.ID {
		t.Fatal("preexisting legacy collection archive behavior changed")
	}
}

func TestAuthorSubscriptionThreeDayWindowAndIncrementalCursor(t *testing.T) {
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	if err := os.WriteFile(filepath.Join(flowRoot, "fixture.jpg"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	b.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{{ID: "weibo-status-100", Source: SourceWeibo, Author: "account", AuthorID: "42"}}}
	now := time.Now()
	var phase, pages atomic.Int32
	weiboPost := func(id string, published time.Time) map[string]any {
		return map[string]any{"mblog": map[string]any{
			"id": id, "text_raw": "illustration", "created_at": published.Format("Mon Jan 02 15:04:05 -0700 2006"),
			"user": map[string]string{"idstr": "42", "screen_name": "account"},
			"pics": []map[string]any{{"large": map[string]string{"url": "/flow/fixture.jpg"}}},
		}}
	}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("containerid") != "10760342" {
			t.Error("quick subscription requested a different author's timeline")
		}
		pages.Add(1)
		var posts []map[string]any
		switch phase.Load() {
		case 0:
			switch r.URL.Query().Get("page") {
			case "1":
				posts = []map[string]any{weiboPost("199", now.Add(-96*time.Hour)), weiboPost("201", now.Add(-time.Hour))}
			case "2":
				posts = []map[string]any{weiboPost("202", now.Add(-48*time.Hour))}
			case "3":
				posts = []map[string]any{weiboPost("198", now.Add(-90*time.Hour))}
			default:
				t.Error("initial run fetched beyond the last three days")
			}
		case 1:
			http.Error(w, "fixture failure", http.StatusTooManyRequests)
			return
		case 2:
			switch r.URL.Query().Get("page") {
			case "1":
				posts = []map[string]any{weiboPost("203", now.Add(time.Hour)), weiboPost("204", now.Add(-2*time.Hour))}
			case "2":
				posts = []map[string]any{weiboPost("205", now.Add(-3*time.Hour))}
			default:
				t.Error("incremental sync fetched well before the subscription cursor")
			}
		}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": posts}})
	})
	query := url.Values{"source": {"weibo"}, "author": {"account"}, "postId": {"weibo-status-100"}}
	path := "/api/author-subscription?" + query.Encode()
	view := authorSubscriptionRequest(t, b, "POST", path, `{"source":"weibo","author":"account","postId":"weibo-status-100","timeZone":"Asia/Shanghai"}`, 202)
	if view.Feed == nil || view.Task == nil {
		t.Fatal("quick subscribe response missing source or task")
	}
	b.syncTasks.mu.Lock()
	startup := b.syncTasks.byID[view.Task.ID]
	b.syncTasks.mu.Unlock()
	finished := waitQueueTestTask(t, startup)
	if finished.err != nil || pages.Load() != 3 {
		t.Fatalf("initial pull didn't use the correct boundary across pages: %+v, pages %d", finished, pages.Load())
	}
	feed, exists := b.findSyncSource("weibo-42", SourceWeibo)
	if !exists || feed.AuthorSync == nil || !feed.AuthorSync.PulledThrough.After(now) || feed.StartDate != "" {
		t.Fatalf("bad startup cursor or a permanent history date filter: %+v", feed)
	}
	cursor := feed.AuthorSync.PulledThrough
	phase.Store(1)
	if after, err := b.syncSource(feed, false); err == nil || !after.AuthorSync.PulledThrough.Equal(cursor) || after.LastSyncStatus != "failed" {
		t.Fatalf("failed pull advanced its cursor: %+v, %v", after, err)
	}
	phase.Store(2)
	pages.Store(0)
	feed, _ = b.findSyncSource(feed.ID, feed.Source)
	if after, err := b.syncSource(feed, false); err != nil || !after.AuthorSync.PulledThrough.After(cursor) || pages.Load() != 2 {
		t.Fatalf("incremental pull failed to advance from its last success: %+v, %v, pages %d", after, err, pages.Load())
	}
	saved, err := loadStoreFile(b.content.file)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, post := range saved.posts {
		seen[post.ID] = true
	}
	if len(seen) != 4 || !seen["weibo-status-100"] || !seen["weibo-status-201"] || !seen["weibo-status-202"] || !seen["weibo-status-203"] {
		t.Fatalf("unexpected archived posts: %v", seen)
	}
}
