package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLivePhoto403KeepsPostsWarnsAndCanRepair(t *testing.T) {
	var output bytes.Buffer
	previousLog := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousLog) })
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	deny := true
	images, motions := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jpg") {
			images++
			w.Header().Set("Content-Type", "image/jpeg")
			fmt.Fprint(w, "fixture-image")
			return
		}
		motions++
		if r.Header.Get("Cookie") != "" {
			t.Error("CDN received account cookie")
		}
		if deny {
			w.WriteHeader(403)
			fmt.Fprint(w, "private-response-fixture")
			return
		}
		w.Header().Set("Content-Type", "video/quicktime")
		fmt.Fprint(w, "fixture-motion")
	}))
	defer server.Close()
	platform.config.ProxyURL = server.URL
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "private-author-fixture", Enabled: true}
	platform.config.WeiboSubscriptions = []SourceConfig{feed}
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json")}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		posts := []any{}
		if r.URL.Query().Get("page") == "1" {
			for index := 1; index <= 2; index++ {
				id := fmt.Sprint(index)
				posts = append(posts, map[string]any{"id": id, "created_at": "Thu Nov 05 12:00:00 +0800 2026", "text_raw": "private-caption-fixture", "user": map[string]any{"idstr": "42", "screen_name": "private-author-fixture"}, "pic_ids": []any{id}, "pic_infos": map[string]any{id: map[string]any{"type": "livephoto", "original": map[string]any{"url": server.URL + "/" + id + ".jpg"}, "video": "http://livephoto.us.sinaimg.cn/" + id + ".mov?token=private-token-fixture"}}})
			}
		}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": posts}})
	})
	updated, err := platform.syncSource(feed, true)
	if err != nil || len(platform.content.posts) != 2 || images != 2 || motions != 1 {
		t.Fatalf("403 aborted the archive or retried restricted motion: posts=%d images=%d motion=%d err=%v", len(platform.content.posts), images, motions, err)
	}
	if !strings.Contains(updated.LastSyncWarning, "2 张实况照片") || !strings.Contains(updated.LastSyncWarning, "HTTP 403") || updated.LastSyncStatus != "success" {
		t.Fatal("partial archive warning missing")
	}
	if strings.Contains(updated.LastSyncMessage, "历史动态获取完成") {
		t.Fatal("partial archive was reported as complete")
	}
	for _, post := range platform.content.posts {
		view := toAPIV1Post(post)
		legacy := withAvailablePostImages(post)
		if len(view.Media) != 1 || len(view.LivePhotos) != 0 || len(legacy.LivePhotos) != 0 || len(post.LivePhotos) != 1 {
			t.Fatal("unarchived remote motion leaked to viewer or retry metadata lost")
		}
		_, complete := reuseAvailablePostArchive(post, post)
		if complete {
			t.Fatal("partial motion was considered completely archived")
		}
	}
	stored, err := os.ReadFile(platform.content.file)
	var restored ContentData
	if err != nil || json.Unmarshal(stored, &restored) != nil || len(restored.Posts[0].LivePhotos) != 1 {
		t.Fatal("retry metadata was not persisted")
	}
	for _, expected := range []string{"event=sync_start", "event=sync_fetch", "event=sync_filter", "event=archive_progress", "event=media_download_failed", "http=403", "livephoto.us.sinaimg.cn", "event=sync_archive", "event=sync_store", "event=sync_end", "status=partial"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("missing container diagnostic %s", expected)
		}
	}
	for _, secret := range []string{"private-token-fixture", "private-caption-fixture", "private-author-fixture", "private-response-fixture", "token=", "/1.mov"} {
		if strings.Contains(output.String(), secret) || strings.Contains(updated.LastSyncWarning, secret) {
			t.Fatal("container log/warning disclosed private content")
		}
	}

	deny = false
	// An incremental pull with no newer posts must not clear pending-media status.
	updated, err = platform.syncSource(updated, false)
	if err != nil || !strings.Contains(updated.LastSyncWarning, "仍有 2 张实况照片") || motions != 1 {
		t.Fatal("empty incremental sync cleared unresolved media warning or retried automatically")
	}
	// Simulate a normal later user-triggered history pull, not a retry after 403.
	updated, err = platform.syncSource(updated, true)
	if err != nil || strings.Contains(updated.LastSyncWarning, "实况") || strings.Contains(updated.LastSyncWarning, "HTTP 403") || !strings.Contains(updated.LastSyncWarning, "免登录") || images != 2 || motions != 3 {
		t.Fatalf("later history repair did not reuse JPEG and clear warning: images=%d motions=%d err=%v", images, motions, err)
	}
	for _, post := range platform.content.posts {
		if len(toAPIV1Post(post).LivePhotos) != 1 {
			t.Fatal("repaired motion not exposed")
		}
	}
}
