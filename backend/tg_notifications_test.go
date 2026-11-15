package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func telegramNotificationFixture(t *testing.T, names ...string) {
	t.Helper()
	previous := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = previous })
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(flowRoot, name), []byte("image-fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTelegramNotificationSnapshotContainsOnlyLatestAndTotals(t *testing.T) {
	telegramNotificationFixture(t, "one.jpg", "two.jpg", "three.jpg")
	now := time.Now()
	posts := []Post{
		{ID: "older", Author: "older author", Caption: "older text", Published: now.Add(-time.Hour), Media: []string{"/flow/one.jpg", "/flow/two.jpg", "/flow/one.jpg", "https://not-an-archived-image.invalid/photo.jpg", "/flow/missing.jpg"}},
		{ID: "latest", Author: "latest author", Caption: "latest text", Published: now, Media: []string{"/flow/three.jpg"}},
		{ID: "older", Author: "duplicate", Caption: "duplicate ignored"},
	}
	notification, ok := telegramSyncNotification(SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "source", StoragePath: "private-local-path"}, posts)
	if !ok || notification.Added != 2 || notification.Images != 3 || len(notification.Posts) != 1 || notification.Posts[0].ID != "latest" || notification.Feed.StoragePath != "" {
		t.Fatalf("unexpected notification snapshot: %+v", notification)
	}
	posts[1].Media[0] = "/flow/one.jpg"
	posts[1].Caption = "changed later"
	if notification.Posts[0].Caption != "latest text" || !reflect.DeepEqual(notification.Posts[0].Media, []string{"/flow/three.jpg"}) {
		t.Fatal("notification retained caller-owned media or mutable body")
	}
	text := telegramSyncText(notification)
	if !strings.Contains(text, "latest text") || strings.Contains(text, "older text") || !strings.HasSuffix(text, "\n\n\n本次更新：新增 2 条动态，3 张图片。") || strings.Contains(text, "private-local-path") {
		t.Fatal("latest post or separated aggregate result missing from notification")
	}
	if _, ok := telegramSyncNotification(SourceConfig{}, nil); ok {
		t.Fatal("zero new posts must not produce a notification")
	}
}

func TestTelegramNotificationDeliveryBatchesOnlyLatestPhotos(t *testing.T) {
	images := make([]string, 22)
	for index := range images {
		images[index] = "/flow/fixture.jpg"
	}
	notification := tgSyncNotification{Feed: SourceConfig{Source: SourcePixiv, Name: "source"}, Added: 7, Images: 50,
		Posts: []Post{{ID: "latest", Caption: "body", Media: images}}}
	var sizes []int
	var captions []string
	err := deliverTelegramSyncNotification(context.Background(), notification, func(batch []string, caption string) error {
		sizes = append(sizes, len(batch))
		captions = append(captions, caption)
		return nil
	}, func(string) error { t.Fatal("short caption should accompany the final image batch"); return nil })
	if err != nil || !reflect.DeepEqual(sizes, []int{10, 10, 2}) || captions[0] != "" || captions[1] != "" || captions[2] != telegramSyncText(notification) {
		t.Fatalf("photos were not sent in Telegram-compatible batches: %v %v", sizes, err)
	}
}

func TestTelegramNotificationLongTextAndFailedPhotoFallback(t *testing.T) {
	notification := tgSyncNotification{Feed: SourceConfig{Source: SourceWeibo, Name: "source"}, Added: 2, Images: 1,
		Posts: []Post{{ID: "latest", Caption: strings.Repeat("记录\U0001F31F", 1600), Media: []string{"/flow/fixture.jpg"}}}}
	var sent []string
	photos := 0
	err := deliverTelegramSyncNotification(context.Background(), notification, func(images []string, caption string) error {
		photos++
		if caption != "" || len(images) != 1 {
			t.Fatal("overlong text must not be sent as a photo caption")
		}
		return nil
	}, func(text string) error {
		if !utf8.ValidString(text) || telegramTextUnits(text) > 4096 {
			t.Fatal("message split violated Telegram bounds or Unicode")
		}
		sent = append(sent, text)
		return nil
	})
	if err != nil || photos != 1 || len(sent) < 2 || strings.Join(sent, "") != telegramSyncText(notification) || !strings.Contains(sent[len(sent)-1], "\n\n\n本次更新：") {
		t.Fatalf("long text delivery was incomplete: %v", err)
	}
	sent = nil
	failure := errors.New("safe fixture photo failure")
	err = deliverTelegramSyncNotification(context.Background(), notification, func([]string, string) error { return failure }, func(text string) error {
		sent = append(sent, text)
		return nil
	})
	if !errors.Is(err, failure) || !strings.HasPrefix(strings.Join(sent, ""), "图片暂未发送成功。") || !strings.HasSuffix(strings.Join(sent, ""), "\n\n\n本次更新：新增 2 条动态，1 张图片。") {
		t.Fatal("failed image must still deliver the text result without swallowing its error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := deliverTelegramSyncNotification(ctx, notification, func([]string, string) error { called = true; return nil }, func(string) error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatal("canceled bot generation sent a notification")
	}
}

func TestTelegramMergeDeltaUsesCommittedUniqueIDs(t *testing.T) {
	telegramNotificationFixture(t)
	store := &Store{file: filepath.Join(t.TempDir(), "content.json"), posts: []Post{{ID: "existing", Caption: "old"}}}
	posts, err := store.mergePostsWithAdded([]Post{
		{ID: "existing", Caption: "updated"}, {ID: "new", Caption: "first"}, {ID: "new", Caption: "final"},
	})
	if err != nil || len(posts) != 1 || posts[0].ID != "new" || posts[0].Caption != "final" {
		t.Fatalf("delta included repaired/duplicate posts or stale duplicate body: %+v %v", posts, err)
	}
	var additions atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			added, err := store.mergePostsWithAdded([]Post{{ID: "raced", Caption: "same"}})
			if err != nil {
				t.Error(err)
			}
			additions.Add(int32(len(added)))
		}()
	}
	workers.Wait()
	if additions.Load() != 1 {
		t.Fatalf("concurrent source merges double-reported the same post: %d", additions.Load())
	}
	store.file = t.TempDir()
	if err := os.WriteFile(filepath.Join(store.file, "keep"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if added, err := store.mergePostsWithAdded([]Post{{ID: "unsaved"}}); err == nil || len(added) != 0 {
		t.Fatal("failed persistence reported a new archived post")
	}
	for _, post := range store.posts {
		if post.ID == "unsaved" {
			t.Fatal("failed persistence retained uncommitted content")
		}
	}
}

func TestTelegramNotificationQueueIsBoundedAndUsesMigratedOriginals(t *testing.T) {
	telegramNotificationFixture(t, "migrated.jpg")
	post := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "author", Media: []string{"/flow/old-location.jpg"}}
	bot := &tgBot{config: TGBotConfig{Enabled: true, Notifications: true, ChatID: "42"}, notifications: make(chan tgSyncNotification, 1)}
	b := &BilibiliStore{content: &Store{posts: []Post{{ID: post.ID, Source: post.Source, Author: post.Author, Media: []string{"/flow/migrated.jpg"}}}}, tgRuntime: bot}
	feed := SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo, Name: "我的点赞"}
	b.queueTelegramNotification(feed, []Post{post})
	select {
	case received := <-bot.notifications:
		if received.Images != 1 || !reflect.DeepEqual(received.Posts[0].Media, []string{"/flow/migrated.jpg"}) {
			t.Fatal("notification used a stale collection media path")
		}
	default:
		t.Fatal("enabled notification did not enter the queue")
	}
	bot.config.Notifications = false
	b.queueTelegramNotification(feed, []Post{post})
	if len(bot.notifications) != 0 {
		t.Fatal("disabled notifications still queued data")
	}
	bot.config.Notifications = true
	b.queueTelegramNotification(feed, nil)
	if len(bot.notifications) != 0 {
		t.Fatal("zero additions were queued")
	}
	b.queueTelegramNotification(feed, []Post{post})
	finished := make(chan struct{})
	go func() { b.queueTelegramNotification(feed, []Post{post}); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("full Telegram queue blocked synchronization")
	}
	if len(bot.notifications) != 1 {
		t.Fatal("notification queue exceeded its configured capacity")
	}
}

func TestTelegramNotificationSyncHookOnlyReportsNewSuccessfulPosts(t *testing.T) {
	telegramNotificationFixture(t)
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	b.content = &Store{file: filepath.Join(t.TempDir(), "posts.json")}
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "author", Enabled: true}
	b.config.WeiboSubscriptions = []SourceConfig{feed}
	bot := &tgBot{config: TGBotConfig{Enabled: true, Notifications: true, ChatID: "42"}, notifications: make(chan tgSyncNotification, 2)}
	b.tgRuntime = bot
	var failed atomic.Bool
	var remoteID atomic.Int32
	remoteID.Store(123)
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if failed.Load() {
			http.Error(w, "fixture denied", http.StatusTooManyRequests)
			return
		}
		if page := r.URL.Query().Get("page"); page != "" && page != "1" {
			writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": []any{}}})
			return
		}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": []any{map[string]any{"mblog": map[string]any{
			"id": fmt.Sprint(remoteID.Load()), "text_raw": "newly archived body", "created_at": "Sun Nov 15 12:00:00 +0800 2026", "user": map[string]any{"idstr": "42", "screen_name": "author"},
		}}}}})
	})
	updated, err := b.syncSource(feed, false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case notification := <-bot.notifications:
		if notification.Added != 1 || notification.Images != 0 || len(notification.Posts) != 1 || notification.Posts[0].ID != "weibo-status-123" {
			t.Fatalf("synchronization queued wrong delta: %+v", notification)
		}
	default:
		t.Fatal("successful sync did not queue newly archived content")
	}
	if _, err := b.syncSource(updated, false); err != nil || len(bot.notifications) != 0 {
		t.Fatalf("zero-new sync generated a notification: %v", err)
	}
	failed.Store(true)
	if _, err := b.syncSource(updated, false); err == nil || len(bot.notifications) != 0 {
		t.Fatal("failed upstream sync generated a notification")
	}
	failed.Store(false)
	remoteID.Store(124)
	previousConfigPath := bilibiliFile
	bilibiliFile = t.TempDir()
	t.Cleanup(func() { bilibiliFile = previousConfigPath })
	if err := os.WriteFile(filepath.Join(bilibiliFile, "keep"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.syncSource(updated, true); err == nil || len(bot.notifications) != 0 {
		t.Fatal("failed configuration save generated a success notification")
	}
}
