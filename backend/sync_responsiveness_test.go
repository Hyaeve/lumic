package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSyncDownloadDoesNotBlockConfigurationOrFeedReads(t *testing.T) {
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	requested, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requested)
		<-release
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "fixture-image")
	}))
	defer func() { unblock(); server.Close() }()
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "fixture", Enabled: true, Schedule: "0 6 * * *"}
	platform.config.WeiboSubscriptions = []SourceConfig{feed}
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json")}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		cards := []any{map[string]any{"id": "123", "text_raw": "fixture", "created_at": "Thu Nov 05 12:00:00 +0800 2026", "user": map[string]any{"idstr": "42", "screen_name": "fixture"}, "pics": []any{map[string]any{"pid": "p1", "large": map[string]any{"url": server.URL + "/one.jpg"}}}}}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": cards}})
	})
	finished := make(chan error, 1)
	go func() { _, err := platform.syncSource(feed, false); finished <- err }()
	select {
	case <-requested:
	case <-time.After(3 * time.Second):
		t.Fatal("archive did not reach the blocked download")
	}
	saved := make(chan error, 1)
	go func() {
		platform.Lock()
		platform.config.WeiboSubscriptions[0].Schedule = "1 6 * * *"
		platform.Unlock()
		saved <- platform.save()
	}()
	select {
	case err := <-saved:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("configuration writer was blocked by a network download")
	}
	read := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		platform.weiboSubscriptionsHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/subscriptions", nil))
		read <- response.Code
	}()
	select {
	case code := <-read:
		if code != http.StatusOK {
			t.Errorf("feed read status=%d", code)
		}
	case <-time.After(time.Second):
		t.Error("feed read was blocked behind a waiting configuration writer")
	}
	unblock()
	select {
	case err := <-finished:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sync did not finish after releasing the image")
	}
	data, err := os.ReadFile(bilibiliFile)
	var restored BilibiliConfig
	if err != nil || decryptJSON(platform.key, data, &restored) != nil || len(restored.WeiboSubscriptions) != 1 || restored.WeiboSubscriptions[0].Schedule != "1 6 * * *" {
		t.Fatal("sync overwrote an independent settings update", err)
	}
}

func TestConcurrentPlatformSavesRetainAllSubscriptionChanges(t *testing.T) {
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	for index := 0; index < 12; index++ {
		platform.config.WeiboSubscriptions = append(platform.config.WeiboSubscriptions, SourceConfig{ID: fmt.Sprint(index), Source: SourceWeibo})
	}
	var workers sync.WaitGroup
	errors := make(chan error, 12)
	for index := 0; index < 12; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			platform.Lock()
			platform.config.WeiboSubscriptions[index].Tags = []string{fmt.Sprint(index)}
			platform.Unlock()
			errors <- platform.save()
		}(index)
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	data, err := os.ReadFile(bilibiliFile)
	var restored BilibiliConfig
	if err != nil || decryptJSON(platform.key, data, &restored) != nil || len(restored.WeiboSubscriptions) != 12 {
		t.Fatal("cannot restore saved config", err)
	}
	for index, feed := range restored.WeiboSubscriptions {
		if len(feed.Tags) != 1 || feed.Tags[0] != fmt.Sprint(index) {
			t.Fatal("concurrent save lost a source update")
		}
	}
}

func TestCollectionReconcileKeepsExistingReadSnapshots(t *testing.T) {
	feed := SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo, Name: weiboLikesName}
	image := flowPublicPath(SourceWeibo, weiboLikesName, "one.jpg")
	video := flowPublicPath(SourceWeibo, weiboLikesName, "one.mp4")
	motion := flowPublicPath(SourceWeibo, weiboLikesName, "one.mov")
	post := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "fixture", FeedIDs: []string{feed.ID}, Media: []string{image}, Videos: []PostVideo{{URL: video}}, LivePhotos: []PostLivePhoto{{Image: image, Video: motion}}}
	createPostImageFixtures(t, []Post{{Media: []string{image, video, motion}}})
	store := &Store{posts: []Post{post}}
	author := SourceConfig{ID: "weibo-99", Source: SourceWeibo, Name: "fixture"}
	if err := store.reconcileCollectionMedia(feed, []SourceConfig{feed, author}); err != nil {
		t.Fatal(err)
	}
	if post.Media[0] != image || post.Videos[0].URL != video || post.LivePhotos[0].Image != image || post.LivePhotos[0].Video != motion {
		t.Fatal("collection movement mutated a snapshot held by another reader")
	}
	updated := store.posts[0]
	if updated.Media[0] == image || updated.Videos[0].URL == video || updated.LivePhotos[0].Video == motion || updated.LivePhotos[0].Image != updated.Media[0] {
		t.Fatal("collection archive did not move the local media and live-photo pair")
	}
}
