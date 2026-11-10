package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceLivePhotosOptionDefaults(t *testing.T) {
	on, off := true, false
	for _, scenario := range []struct {
		name   string
		source Source
		option *bool
		want   bool
	}{
		{"legacy-weibo", SourceWeibo, nil, true},
		{"enabled-weibo", SourceWeibo, &on, true},
		{"disabled-weibo", SourceWeibo, &off, false},
		{"unrelated-platform", SourceBilibili, &off, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			feed := SourceConfig{Source: scenario.source, IncludeLivePhotos: scenario.option}
			if got := sourceLivePhotosEnabled(feed); got != scenario.want {
				t.Fatalf("enabled=%v, want %v", got, scenario.want)
			}
		})
	}
}

func TestWeiboLivePhotosOptionPersistsAndLegacyPUTPreservesIt(t *testing.T) {
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	platform.config.WeiboSubscriptions = []SourceConfig{{ID: "weibo-42", Source: SourceWeibo, Name: "fixture", Schedule: "0 6 * * *", Enabled: true}}

	for _, scenario := range []struct {
		name, field string
		want        bool
	}{
		{"disable", `,"includeLivePhotos":false`, false},
		{"legacy-client-preserves-disabled", "", false},
		{"enable", `,"includeLivePhotos":true`, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			body := `{"id":"weibo-42","enabled":true,"schedule":"0 6 * * *"` + scenario.field + `}`
			response := httptest.NewRecorder()
			platform.weiboSubscriptionsHandler(response, httptest.NewRequest(http.MethodPut, "/api/weibo/subscriptions", strings.NewReader(body)))
			if response.Code != http.StatusOK {
				t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
			}
			feed := platform.config.WeiboSubscriptions[0]
			if feed.IncludeLivePhotos == nil || *feed.IncludeLivePhotos != scenario.want {
				t.Fatal("subscription option was not retained")
			}
			data, err := os.ReadFile(bilibiliFile)
			if err != nil || bytes.Contains(data, []byte("includeLivePhotos")) {
				t.Fatal("source configuration was not saved encrypted", err)
			}
			var restored BilibiliConfig
			if err := decryptJSON(platform.key, data, &restored); err != nil || len(restored.WeiboSubscriptions) != 1 || restored.WeiboSubscriptions[0].IncludeLivePhotos == nil || *restored.WeiboSubscriptions[0].IncludeLivePhotos != scenario.want {
				t.Fatal("option lost after encrypted reload", err)
			}
			prepared, err := prepareSourceStorage(feed)
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := os.ReadFile(filepath.Join(prepared.StoragePath, "source.json"))
			var source SourceConfig
			if err != nil || json.Unmarshal(metadata, &source) != nil || source.IncludeLivePhotos == nil || *source.IncludeLivePhotos != scenario.want {
				t.Fatal("source metadata lost the option", err)
			}
			projection := toAPIV1Feed(feed)
			if projection.IncludeLivePhotos == nil || *projection.IncludeLivePhotos != scenario.want {
				t.Fatal("API v1 projection lost the option")
			}
		})
	}
}

func TestWeiboLivePhotosOptionSkipsMotionAndCanBackfillLater(t *testing.T) {
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	images, motions := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jpg") {
			images++
			w.Header().Set("Content-Type", "image/jpeg")
			fmt.Fprint(w, "fixture-still")
			return
		}
		motions++
		w.Header().Set("Content-Type", "video/quicktime")
		fmt.Fprint(w, "fixture-motion")
	}))
	defer server.Close()
	platform.config.ProxyURL = server.URL
	off := false
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "fixture", Enabled: true, IncludeLivePhotos: &off}
	platform.config.WeiboSubscriptions = []SourceConfig{feed}
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json")}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		cards := []any{}
		if r.URL.Query().Get("page") == "1" {
			cards = append(cards, map[string]any{"id": "123", "created_at": "Thu Nov 05 12:00:00 +0800 2026", "text_raw": "fixture caption", "user": map[string]any{"idstr": "42", "screen_name": "fixture"}, "pic_ids": []any{"one"}, "pic_infos": map[string]any{"one": map[string]any{"type": "livephoto", "original": map[string]any{"url": server.URL + "/one.jpg"}, "video": "http://livephoto.us.sinaimg.cn/one.mov"}}})
		}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": cards}})
	})

	updated, err := platform.syncSource(feed, true)
	if err != nil || len(platform.content.posts) != 1 || images != 1 || motions != 0 {
		t.Fatalf("disabled option did not archive static-only: images=%d motions=%d err=%v", images, motions, err)
	}
	stored := platform.content.posts[0]
	if len(stored.LivePhotos) != 1 || len(toAPIV1Post(stored).LivePhotos) != 0 || len(withAvailablePostImages(stored).LivePhotos) != 0 || strings.Contains(updated.LastSyncWarning, "实况") {
		t.Fatal("static-only archive lost retry metadata, exposed motion, or warned about intentionally disabled motion")
	}
	if _, complete := reuseAvailablePostArchive(stored, stored); complete {
		t.Fatal("default archive check ignored missing motion")
	}
	if _, complete := reuseAvailablePostArchiveWithLivePhotos(stored, stored, false); !complete {
		t.Fatal("disabled option still treated missing motion as incomplete")
	}
	updated, err = platform.syncSource(updated, true)
	if err != nil || images != 1 || motions != 0 || !strings.Contains(updated.LastSyncMessage, "跳过 1 条") || strings.Contains(updated.LastSyncWarning, "实况") {
		t.Fatalf("disabled repeat history did not skip complete static archive: images=%d motions=%d message=%s err=%v", images, motions, updated.LastSyncMessage, err)
	}
	updated, err = platform.syncSource(updated, false)
	if err != nil || motions != 0 || strings.Contains(updated.LastSyncWarning, "实况") {
		t.Fatal("empty incremental pull warned about disabled motion", err)
	}

	on := true
	updated.IncludeLivePhotos = &on
	platform.config.WeiboSubscriptions[0].IncludeLivePhotos = &on
	updated, err = platform.syncSource(updated, true)
	if err != nil || images != 1 || motions != 1 || len(toAPIV1Post(platform.content.posts[0]).LivePhotos) != 1 {
		t.Fatalf("reenabled history did not fill only motion: images=%d motions=%d err=%v", images, motions, err)
	}
	motion := platform.content.posts[0].LivePhotos[0].Video
	updated.IncludeLivePhotos = &off
	platform.config.WeiboSubscriptions[0].IncludeLivePhotos = &off
	_, err = platform.syncSource(updated, true)
	if err != nil || images != 1 || motions != 1 || !livePhotoMotionAvailable(motion) || len(toAPIV1Post(platform.content.posts[0]).LivePhotos) != 1 {
		t.Fatal("disabling option removed or redownloaded an already archived live photo", err)
	}
}

func TestWeiboLivePhotosOptionGatesDirectArchive(t *testing.T) {
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "fixture-still")
	}))
	defer server.Close()
	off := false
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "fixture", IncludeLivePhotos: &off}
	image := server.URL + "/one.jpg"
	post := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "fixture", Media: []string{image}, LivePhotos: []PostLivePhoto{{Image: image, Video: "http://livephoto.us.sinaimg.cn/one.mov", PictureID: "one"}}}
	platform := &BilibiliStore{config: BilibiliConfig{ProxyURL: server.URL}}
	prepared, posts, err := platform.archiveSourceContent(feed, []Post{post})
	if err != nil || requests != 1 || len(posts) != 1 || !localFlowFileAvailable(posts[0].Media[0]) || len(posts[0].LivePhotos) != 1 || posts[0].LivePhotos[0].Video != post.LivePhotos[0].Video || prepared.LastSyncWarning != "" {
		t.Fatalf("direct archive bypassed disabled option: requests=%d warning=%s err=%v", requests, prepared.LastSyncWarning, err)
	}
}

func TestWeiboLivePhotosOptionAppliesToPostReset(t *testing.T) {
	platform := newWeiboAuthTestStore(t, WeiboCredentials{})
	off := false
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "fixture", IncludeLivePhotos: &off}
	platform.config.WeiboSubscriptions = []SourceConfig{feed}
	previous := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "fixture", Caption: "old caption", FeedIDs: []string{feed.ID}, Media: []string{flowPublicPath(SourceWeibo, "fixture", "old.jpg")}}
	previous.LivePhotos = []PostLivePhoto{{Image: previous.Media[0], Video: "http://livephoto.us.sinaimg.cn/one.mov", PictureID: "one"}}
	createPostImageFixtures(t, []Post{previous})
	platform.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{previous}}
	images, motions, details := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jpg") {
			images++
			w.Header().Set("Content-Type", "image/jpeg")
			fmt.Fprint(w, "reset-still")
			return
		}
		motions++
		w.Header().Set("Content-Type", "video/quicktime")
		fmt.Fprint(w, "unexpected-motion")
	}))
	defer server.Close()
	platform.config.ProxyURL = server.URL
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		details++
		if r.Host != "m.weibo.cn" || r.URL.Path != "/statuses/show" || r.URL.Query().Get("id") != "123" {
			t.Error("reset fetched an unexpected platform route")
		}
		payload := resetWeiboPayload()
		payload["user"] = map[string]any{"idstr": "42", "screen_name": "fixture"}
		payload["pic_ids"] = []any{"one"}
		payload["pic_infos"] = map[string]any{"one": map[string]any{"type": "livephoto", "original": map[string]any{"url": server.URL + "/one.jpg"}, "video": "http://livephoto.us.sinaimg.cn/one.mov"}}
		writeJSON(w, map[string]any{"ok": 1, "data": payload})
	})
	response := resetRequest(platform, previous.ID)
	if response.Code != http.StatusOK || details != 1 || images != 1 || motions != 0 {
		t.Fatalf("disabled reset did not archive static-only: status=%d details=%d images=%d motions=%d body=%s", response.Code, details, images, motions, response.Body.String())
	}
	var resetBody struct {
		Post Post `json:"post"`
	}
	if json.Unmarshal(response.Body.Bytes(), &resetBody) != nil || len(resetBody.Post.Media) != 1 || len(resetBody.Post.LivePhotos) != 0 {
		t.Fatal("reset response exposed unarchived live-photo motion")
	}
	updated := platform.content.posts[0]
	if updated.Caption != "fresh caption" || len(updated.Media) != 1 || !localFlowFileAvailable(updated.Media[0]) || len(updated.LivePhotos) != 1 || len(toAPIV1Post(updated).LivePhotos) != 0 || len(withAvailablePostImages(updated).LivePhotos) != 0 {
		t.Fatal("reset lost static content/retry metadata or exposed unarchived motion")
	}
	if !localFlowFileAvailable(previous.Media[0]) {
		t.Fatal("reset deleted the previous static archive")
	}
	postsResponse := httptest.NewRecorder()
	apiV1PostsHandler(platform.content)(postsResponse, httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil))
	var body struct {
		Items []apiV1Post `json:"items"`
	}
	if postsResponse.Code != http.StatusOK || json.Unmarshal(postsResponse.Body.Bytes(), &body) != nil || len(body.Items) != 1 || len(body.Items[0].Media) != 1 || len(body.Items[0].LivePhotos) != 0 {
		t.Fatalf("posts API did not expose static-only reset content: status=%d body=%s", postsResponse.Code, postsResponse.Body.String())
	}
}
