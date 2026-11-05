package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeiboLivePhotosRemainSingleImagesInSourceOrder(t *testing.T) {
	payload := map[string]any{
		"id": "123", "user": map[string]any{"idstr": "42", "screen_name": "fixture"},
		"pic_ids": []any{"b", "a", "c"}, "pic_video": "b:motion-b,missing:orphan",
		"pic_infos": map[string]any{
			"a": map[string]any{"type": "livephoto", "largest": map[string]any{"url": "https://wx1.sinaimg.cn/large/a.jpg"}, "video": "https://livephoto.us.sinaimg.cn/a.mov"},
			"b": map[string]any{"type": "livephoto", "original": map[string]any{"url": "https://wx1.sinaimg.cn/large/b.jpg"}},
			"c": map[string]any{"type": "pic", "original": map[string]any{"url": "https://wx1.sinaimg.cn/large/c.jpg"}},
		},
	}
	var posts []Post
	collectWeiboPosts(payload, SourceConfig{Source: SourceWeibo}, &posts, map[string]bool{})
	if len(posts) != 1 || len(posts[0].Media) != 3 || len(posts[0].LivePhotos) != 2 || len(posts[0].Videos) != 0 {
		t.Fatalf("live photos were lost or treated as extra videos: %+v", posts)
	}
	post := posts[0]
	for index, name := range []string{"b", "a", "c"} {
		if !strings.Contains(post.Media[index], "/original/"+name+".jpg") {
			t.Fatalf("wrong source image order: %v", post.Media)
		}
	}
	if post.LivePhotos[0].Image != post.Media[0] || post.LivePhotos[1].Image != post.Media[1] || !strings.Contains(post.LivePhotos[0].Video, "motion-b.mov") {
		t.Fatal("motion paired with the wrong image")
	}
	if len(filterSourcePosts(posts, SourceConfig{OnlyWithImages: true})) != 1 {
		t.Fatal("live photo incorrectly requires the regular-video subscription option")
	}
}

func TestLivePhotoArchiveAPILifecycleAndHistoryRepair(t *testing.T) {
	oldRoot, oldPreview := flowRoot, previewRoot
	flowRoot, previewRoot = t.TempDir(), t.TempDir()
	t.Cleanup(func() { flowRoot, previewRoot = oldRoot, oldPreview })
	images, videos := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".mov") {
			videos++
			w.Header().Set("Content-Type", "video/quicktime")
			_, _ = w.Write([]byte("fixture-video"))
		} else {
			images++
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fixture-image"))
		}
	}))
	defer server.Close()
	image := server.URL + "/one.jpg"
	post := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "fixture", FeedIDs: []string{"weibo-42"}, Media: []string{image}, LivePhotos: []PostLivePhoto{{Image: image, Video: "http://us.sinaimg.cn/one.mov", PictureID: "one"}}}
	feed := SourceConfig{ID: "weibo-42", Name: "fixture", Source: SourceWeibo}
	platform := &BilibiliStore{config: BilibiliConfig{ProxyURL: server.URL}}
	_, archived, err := platform.archiveSourceContent(feed, []Post{post})
	if err != nil || len(archived) != 1 || images != 1 || videos != 1 {
		t.Fatalf("failed to archive live photo: images=%d videos=%d err=%v", images, videos, err)
	}
	saved := archived[0]
	if len(saved.Media) != 1 || len(saved.Videos) != 0 || saved.LivePhotos[0].Image != saved.Media[0] || !localFlowFileAvailable(saved.LivePhotos[0].Video) {
		t.Fatal("archive lost the single-image live photo relationship")
	}
	store := &Store{file: filepath.Join(t.TempDir(), "content.json")}
	if _, err := store.mergePosts(archived); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(store.file)
	var restored ContentData
	if json.Unmarshal(data, &restored) != nil || len(restored.Posts[0].LivePhotos) != 1 {
		t.Fatal("live photo not persisted")
	}
	view := toAPIV1Post(restored.Posts[0])
	if len(view.Media) != 1 || len(view.PreviewMedia) != 1 || view.LivePhotos[0] != saved.LivePhotos[0] {
		t.Fatal("API lost live photo or added a separate media slot")
	}

	// Legacy archives have no verified picture ID; fetch a matched still and motion.
	legacy := saved
	legacy.LivePhotos = nil
	prepared, complete := reuseAvailablePostArchive(post, legacy)
	if complete || prepared.Media[0] != post.Media[0] || prepared.LivePhotos[0].Image != post.Media[0] {
		t.Fatal("history did not identify missing motion")
	}
	_, repaired, err := platform.archiveSourceContent(feed, []Post{prepared})
	if err != nil || images != 2 || videos != 2 {
		t.Fatalf("repair did not download a verified pair: %v", err)
	}
	prepared, complete = reuseAvailablePostArchive(post, repaired[0])
	if !complete || prepared.LivePhotos[0].Video != repaired[0].LivePhotos[0].Video {
		t.Fatal("repeat history did not reuse archived motion")
	}

	// Edited media order is image-bound; a later sync cannot undo the edit.
	response := editRequest(t, store, saved.ID, "edited", []string{saved.Media[0]}, nil)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if _, err := store.mergePosts(archived); err != nil || len(store.posts[0].LivePhotos) != 1 {
		t.Fatal("sync lost the edited live photo")
	}
	response = editRequest(t, store, saved.ID, "edited", []string{}, nil)
	if response.Code != 200 || len(store.posts[0].LivePhotos) != 0 {
		t.Fatal("removing an image retained active motion")
	}
	if _, err := store.mergePosts(archived); err != nil || len(store.posts[0].LivePhotos) != 0 {
		t.Fatal("sync revived removed live photo")
	}

	paths := postMediaPaths(saved)
	if !containsString(paths, saved.LivePhotos[0].Video) {
		t.Fatal("delete omitted motion file")
	}
	if err := deletePostMedia(paths); err != nil {
		t.Fatal(err)
	}
	if len(toAPIV1Post(saved).LivePhotos) != 0 {
		t.Fatal("deleted photo still exposed motion")
	}
}

func TestLivePhotoMigrationAndMissingMotionFallback(t *testing.T) {
	post := Post{ID: "weibo-status-123", Source: SourceWeibo, Author: "fixture", FeedIDs: []string{"weibo-likes-42"}, Media: []string{flowPublicPath(SourceWeibo, "fixture", "one.jpg")}}
	createPostImageFixtures(t, []Post{post})
	video := flowPublicPath(SourceWeibo, "fixture", "one.mov")
	path, _ := localFlowArchivePath(video)
	if err := os.WriteFile(path, []byte("fixture-video"), 0600); err != nil {
		t.Fatal(err)
	}
	post.LivePhotos = []PostLivePhoto{{Image: post.Media[0], Video: video}}
	store := &Store{posts: []Post{post}}
	feed := SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo, Name: weiboLikesName}
	if err := store.reconcileCollectionMedia(feed, []SourceConfig{feed}); err != nil {
		t.Fatal(err)
	}
	moved := store.posts[0]
	moved.Media = append([]string(nil), moved.Media...)
	moved.LivePhotos = append([]PostLivePhoto(nil), moved.LivePhotos...)
	if moved.LivePhotos[0].Image != moved.Media[0] || !localFlowFileAvailable(moved.LivePhotos[0].Video) || moved.LivePhotos[0].Video == video {
		t.Fatal("collection migration broke live photo")
	}
	if err := store.rewriteMediaPrefix("/flow/weibo/", "/flow/renamed/"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(store.posts[0].LivePhotos[0].Image, "/flow/renamed/") || !strings.HasPrefix(store.posts[0].LivePhotos[0].Video, "/flow/renamed/") {
		t.Fatal("prefix rewrite lost live photo references")
	}
	// Use the pre-rename paths; rewrite changes metadata, not the filesystem.
	path, _ = localFlowArchivePath(moved.LivePhotos[0].Video)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	view := toAPIV1Post(moved)
	if len(view.Media) != 1 || len(view.LivePhotos) != 0 || len(moved.LivePhotos) != 1 {
		t.Fatal("missing motion should retain static image without mutating stored post")
	}
}

func TestWeiboLivePhotoRejectsInvalidFIDs(t *testing.T) {
	for _, fid := range []string{"../secret", "https://example.test/x", "a/b", "a:443", "a?token=private"} {
		if weiboLivePhotoVideo(nil, fid) != "" {
			t.Fatalf("unsafe fid accepted: %q", fid)
		}
	}
}

func TestWeiboLivePhotoPreservesValidatedDispatch(t *testing.T) {
	for _, scenario := range []struct{ source, want string }{
		{"https://video.weibo.com/media/play?livephoto=%2F%2Flivephoto.us.sinaimg.cn%2Fone.mov", "https://video.weibo.com/media/play?livephoto=%2F%2Flivephoto.us.sinaimg.cn%2Fone.mov"},
		{"https://video.weibo.com/media/play?livephoto=https%3A%2F%2Fevil.test%2Fone.mov", ""},
		{"https://video.weibo.com/media/play?livephoto=javascript%3Aalert%281%29", ""},
		{"https://video.weibo.com/media/play", ""},
	} {
		if got := weiboLivePhotoVideo(map[string]any{"video": scenario.source}, ""); got != scenario.want {
			t.Fatalf("incorrect motion wrapper result: %q", got)
		}
	}
}

func TestLivePhotoReuseUsesPictureIDAfterSourceReorder(t *testing.T) {
	stored := Post{Media: []string{"/flow/fixture/a.jpg", "/flow/fixture/b.jpg"}, LivePhotos: []PostLivePhoto{{Image: "/flow/fixture/a.jpg", Video: "/flow/fixture/a.mov", PictureID: "a"}, {Image: "/flow/fixture/b.jpg", Video: "/flow/fixture/b.mov", PictureID: "b"}}}
	createPostImageFixtures(t, []Post{stored})
	for _, photo := range stored.LivePhotos {
		path, _ := localFlowArchivePath(photo.Video)
		if err := os.WriteFile(path, []byte("fixture-video"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	incoming := Post{Media: []string{"https://example.test/b.jpg", "https://example.test/a.jpg"}, LivePhotos: []PostLivePhoto{{Image: "https://example.test/b.jpg", Video: "https://example.test/b.mov", PictureID: "b"}, {Image: "https://example.test/a.jpg", Video: "https://example.test/a.mov", PictureID: "a"}}}
	prepared, complete := reuseAvailablePostArchive(incoming, stored)
	if !complete || prepared.LivePhotos[0].Image != stored.Media[1] || prepared.LivePhotos[0].Video != stored.LivePhotos[1].Video {
		t.Fatal("source reorder mismatched a live photo")
	}
	if incoming.Media[0] != "https://example.test/b.jpg" {
		t.Fatal("archive reuse mutated incoming media")
	}
}

func TestLivePhotoPairSurvivesReorderAndDropsRemovedImage(t *testing.T) {
	post := Post{Media: []string{"b", "a"}, LivePhotos: []PostLivePhoto{{Image: "a", Video: "a.mov"}, {Image: "b", Video: "b.mov"}, {Image: "removed", Video: "removed.mov"}}}
	photos := normalizePostLivePhotos(post)
	if len(photos) != 2 || photos[0].Image != "b" || photos[1].Image != "a" {
		t.Fatal("live photos use unstable positional indexes")
	}
}
