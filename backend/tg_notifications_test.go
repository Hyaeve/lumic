package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestTelegramNotificationDeliveryUsesSinglePreviewWithCaption(t *testing.T) {
	images := make([]string, 22)
	for index := range images {
		images[index] = fmt.Sprintf("/flow/fixture-%d.jpg", index)
	}
	notification := tgSyncNotification{Feed: SourceConfig{Source: SourcePixiv, Name: "source"}, Added: 7, Images: 50,
		Posts: []Post{{ID: "latest", Caption: "body", Media: images}}}
	var sizes []int
	var captions []string
	err := deliverTelegramSyncNotification(context.Background(), notification, func(batch []string, caption string) error {
		if !reflect.DeepEqual(batch, images[:1]) {
			t.Fatal("notification did not use only the latest post's first image")
		}
		sizes = append(sizes, len(batch))
		captions = append(captions, caption)
		return nil
	}, func(string) error { t.Fatal("caption must stay in the same message as the preview"); return nil })
	if err != nil || !reflect.DeepEqual(sizes, []int{1}) || len(captions) != 1 || captions[0] != telegramSyncText(notification) {
		t.Fatalf("notification was not exactly one captioned photo: %v %v", sizes, err)
	}
}

func TestTelegramNotificationLongTextAndFailedPhotoFallback(t *testing.T) {
	notification := tgSyncNotification{Feed: SourceConfig{Source: SourceWeibo, Name: "source"}, Added: 2, Images: 1,
		Posts: []Post{{ID: "latest", Caption: strings.Repeat("记录\U0001F31F", 1600), Media: []string{"/flow/fixture.jpg"}}}}
	var sent []string
	photos := 0
	err := deliverTelegramSyncNotification(context.Background(), notification, func(images []string, caption string) error {
		photos++
		if !utf8.ValidString(caption) || telegramTextUnits(caption) > 1024 || len(images) != 1 || !strings.Contains(caption, "…") || !strings.HasSuffix(caption, "\n\n\n本次更新：新增 2 条动态，1 张图片。") {
			t.Fatal("caption truncation broke Unicode, bounds, or the complete totals")
		}
		return nil
	}, func(text string) error {
		sent = append(sent, text)
		return nil
	})
	if err != nil || photos != 1 || len(sent) != 0 {
		t.Fatalf("long text was incorrectly split into separate messages: %v", err)
	}
	sent = nil
	failure := errors.New("safe fixture photo failure")
	err = deliverTelegramSyncNotification(context.Background(), notification, func([]string, string) error { return failure }, func(text string) error {
		sent = append(sent, text)
		return nil
	})
	if !errors.Is(err, failure) || len(sent) != 1 || !utf8.ValidString(sent[0]) || telegramTextUnits(sent[0]) > 4096 || !strings.HasPrefix(sent[0], "图片暂未发送成功。") || !strings.HasSuffix(sent[0], "\n\n\n本次更新：新增 2 条动态，1 张图片。") {
		t.Fatal("failed image must still deliver the text result without swallowing its error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := deliverTelegramSyncNotification(ctx, notification, func([]string, string) error { called = true; return nil }, func(string) error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatal("canceled bot generation sent a notification")
	}
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, tgPhotoDeliveryUncertainError{err: errors.New("unknown upload result")}} {
		if err := deliverTelegramSyncNotification(context.Background(), notification, func([]string, string) error { return failure }, func(string) error { t.Error("ambiguous or canceled photo send produced an extra message"); return nil }); !errors.Is(err, failure) {
			t.Fatal("uncertain result was swallowed", err)
		}
	}
	notification.Posts[0].Media = nil
	sent = nil
	err = deliverTelegramSyncNotification(context.Background(), notification, func([]string, string) error { t.Error("text-only update attempted a photo"); return nil }, func(text string) error { sent = append(sent, text); return nil })
	if err != nil || len(sent) != 1 || !utf8.ValidString(sent[0]) || telegramTextUnits(sent[0]) > 4096 || !strings.HasSuffix(sent[0], "\n\n\n本次更新：新增 2 条动态，1 张图片。") {
		t.Fatal("text-only update did not stay in a single bounded message", err)
	}
}

func TestTelegramNotificationUploadsOneCompressedPhotoWithCaption(t *testing.T) {
	telegramNotificationFixture(t)
	original := tgDetailedImageFixture(t, 1100, 800)
	if err := os.WriteFile(filepath.Join(flowRoot, "original.png"), original, 0600); err != nil {
		t.Fatal(err)
	}
	var photos, text, groups atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			photos.Add(1)
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			reader, info, err := r.FormFile("preview")
			if err != nil {
				t.Error("compressed preview was not attached", err)
				return
			}
			data, _ := io.ReadAll(reader)
			reader.Close()
			config, format, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil || format != "jpeg" || info.Filename != "preview.jpg" || bytes.Equal(data, original) || len(data) >= len(original) || config.Width != 720 || config.Height >= 800 {
				t.Error("notification uploaded an original instead of a smaller JPEG preview", err, format, len(data), len(original), config)
			}
			caption := r.FormValue("caption")
			if r.FormValue("chat_id") != "42" || r.FormValue("photo") != "attach://preview" || len(r.MultipartForm.File) != 1 || !strings.Contains(caption, "body") || !strings.HasSuffix(caption, "\n\n\n本次更新：新增 3 条动态，17 张图片。") {
				t.Error("photo and update text did not share one message with aggregate totals")
			}
			writeJSON(w, map[string]any{"ok": true, "result": map[string]int{"message_id": 7}})
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			text.Add(1)
			writeJSON(w, map[string]any{"ok": true, "result": map[string]int{"message_id": 8}})
		default:
			groups.Add(1)
			http.Error(w, "unexpected notification API", 400)
		}
	}))
	t.Cleanup(server.Close)
	bot := &tgBot{config: TGBotConfig{Token: tgTestToken, ChatID: "42", Notifications: true}, apiBase: server.URL, client: server.Client()}
	notification := tgSyncNotification{Feed: SourceConfig{Source: SourceWeibo, Name: "source"}, Added: 3, Images: 17, Posts: []Post{{Caption: "body", Media: []string{"/flow/original.png", "/flow/must-not-read.png"}}}}
	if err := bot.sendSyncNotification(context.Background(), notification); err != nil || photos.Load() != 1 || text.Load() != 0 || groups.Load() != 0 {
		t.Fatal("notification did not send exactly one captioned preview", err, photos.Load(), text.Load(), groups.Load())
	}
	if after, err := os.ReadFile(filepath.Join(flowRoot, "original.png")); err != nil || !bytes.Equal(after, original) {
		t.Fatal("generating a notification altered the archived original", err)
	}
}

func TestTelegramNotificationSafeFallbackAndUncertainUpload(t *testing.T) {
	telegramNotificationFixture(t)
	if err := os.WriteFile(filepath.Join(flowRoot, "photo.png"), tgImageFixture(t), 0600); err != nil {
		t.Fatal(err)
	}
	var photos, texts atomic.Int32
	var photoStatus atomic.Int32
	photoStatus.Store(http.StatusBadRequest)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			photos.Add(1)
			status := photoStatus.Load()
			w.WriteHeader(int(status))
			fmt.Fprintf(w, `{"ok":false,"error_code":%d}`, status)
			return
		}
		texts.Add(1)
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		if !strings.HasPrefix(fmt.Sprint(input["text"]), "图片暂未发送成功。") {
			t.Error("text fallback omitted photo failure context")
		}
		writeJSON(w, map[string]any{"ok": true, "result": map[string]int{"message_id": 8}})
	}))
	t.Cleanup(server.Close)
	bot := &tgBot{config: TGBotConfig{Token: tgTestToken, ChatID: "42", Notifications: true}, apiBase: server.URL, client: server.Client()}
	notification := tgSyncNotification{Added: 1, Images: 1, Posts: []Post{{Caption: "body", Media: []string{"/flow/photo.png"}}}}
	if err := bot.sendSyncNotification(context.Background(), notification); err == nil || photos.Load() != 1 || texts.Load() != 1 {
		t.Fatal("explicitly rejected photo did not fall back once", err, photos.Load(), texts.Load())
	}
	notification.Posts[0].Media[0] = "/flow/missing.png"
	if err := bot.sendSyncNotification(context.Background(), notification); err == nil || photos.Load() != 1 || texts.Load() != 2 {
		t.Fatal("missing local photo attempted an upload or duplicated fallback", err)
	}
	notification.Posts[0].Media[0] = "/flow/photo.png"
	var outcome tgPhotoDeliveryUncertainError
	for _, status := range []int32{http.StatusRequestTimeout, http.StatusBadGateway} {
		photoStatus.Store(status)
		if err := bot.sendSyncNotification(context.Background(), notification); !errors.As(err, &outcome) || texts.Load() != 2 {
			t.Fatal("uncertain HTTP response sent a duplicate text notification", status, err)
		}
	}
	if photos.Load() != 3 {
		t.Fatal("uncertain HTTP response retried the upload")
	}
	var uncertain atomic.Int32
	bot.client = &http.Client{Transport: tgTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		uncertain.Add(1)
		return nil, errors.New("fixture connection lost after upload")
	})}
	if err := bot.sendSyncNotification(context.Background(), notification); !errors.As(err, &outcome) || uncertain.Load() != 1 || texts.Load() != 2 {
		t.Fatal("ambiguous upload was retried or followed by a duplicate notification", err)
	}
	bot.client = &http.Client{Transport: tgTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		uncertain.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := bot.sendSyncNotification(ctx, notification); !errors.Is(err, context.DeadlineExceeded) || uncertain.Load() != 2 || texts.Load() != 2 {
		t.Fatal("canceled upload retried or sent a text fallback", err)
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
