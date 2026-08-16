package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadRemoteImageAndFlowPath(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.bilibili.com/" || !strings.Contains(r.Header.Get("Cookie"), "SESSDATA=test") {
			http.Error(w, "missing anti-hotlink headers", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-image"))
	}))
	defer server.Close()
	targetBase := filepath.Join(sourceStoragePath(SourceBilibili, "测试/UP"), "avatar")
	localPath, err := downloadRemoteImage(server.Client(), server.URL+"/avatar", targetBase, "https://www.bilibili.com/", "SESSDATA=test")
	if err != nil {
		t.Fatalf("download image: %v", err)
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("downloaded image missing: %v", err)
	}
	if got := flowPublicPath(SourceBilibili, "测试/UP", filepath.Base(localPath)); got != "/flow/bilibili/%E6%B5%8B%E8%AF%95_UP/avatar.png" {
		t.Fatalf("unexpected public path: %s", got)
	}
}

func TestInitializeFlowStorageCreatesPreviewRootAndRemovesLegacyCache(t *testing.T) {
	root := t.TempDir()
	configuredFlowRoot := filepath.Join(root, "flow")
	configuredPreviewRoot := filepath.Join(root, "previews")
	legacyPreviewRoot := filepath.Join(configuredFlowRoot, ".previews")
	if err := os.MkdirAll(legacyPreviewRoot, 0755); err != nil {
		t.Fatalf("create legacy preview directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyPreviewRoot, "stale.jpg"), []byte("stale"), 0600); err != nil {
		t.Fatalf("seed legacy preview: %v", err)
	}

	oldFlowRoot, oldPreviewRoot := flowRoot, previewRoot
	t.Cleanup(func() { flowRoot, previewRoot = oldFlowRoot, oldPreviewRoot })
	t.Setenv("LUMIC_FLOW_ROOT", configuredFlowRoot)
	t.Setenv("LUMIC_PREVIEW_ROOT", configuredPreviewRoot)

	if err := initializeFlowStorage(); err != nil {
		t.Fatalf("initialize flow storage: %v", err)
	}
	if _, err := os.Stat(configuredPreviewRoot); err != nil {
		t.Fatalf("preview root was not created: %v", err)
	}
	if _, err := os.Stat(legacyPreviewRoot); !os.IsNotExist(err) {
		t.Fatalf("legacy preview root still exists: %v", err)
	}
	for _, source := range []Source{SourceBilibili, SourcePixiv, SourceWeibo, SourceTwitter} {
		if _, err := os.Stat(filepath.Join(configuredFlowRoot, string(source))); err != nil {
			t.Fatalf("%s flow directory was not created: %v", source, err)
		}
	}
}

func TestMediaPreviewHandlerCompressesAndCleansCache(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot, oldPreviewRoot := flowRoot, previewRoot
	flowRoot, previewRoot = filepath.Join(root, "flow"), filepath.Join(root, "previews")
	t.Cleanup(func() { flowRoot, previewRoot = oldFlowRoot, oldPreviewRoot })
	root = flowRoot
	sourcePath := filepath.Join(root, "weibo", "author", "post.png")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatal(err)
	}
	original := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	draw.Draw(original, original.Bounds(), &image.Uniform{C: color.RGBA{R: 80, G: 140, B: 110, A: 255}}, image.Point{}, draw.Src)
	file, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, original); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/preview/weibo/author/post.png", nil)
	response := httptest.NewRecorder()
	mediaPreviewHandler(response, request)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "image/jpeg") {
		t.Fatalf("unexpected preview response: status=%d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if config.Width > mediaPreviewMaxDimension || config.Height > mediaPreviewMaxDimension {
		t.Fatalf("preview was not resized: %dx%d", config.Width, config.Height)
	}
	if config.Width != 900 || config.Height != 600 {
		t.Fatalf("unexpected preview dimensions: %dx%d", config.Width, config.Height)
	}
	previewPath, ok := mediaPreviewCachePath(root, sourcePath)
	if !ok {
		t.Fatal("preview path was rejected")
	}
	if _, err := os.Stat(previewPath); err != nil {
		t.Fatalf("preview cache missing: %v", err)
	}
	mobileRequest := httptest.NewRequest(http.MethodGet, "/preview/weibo/author/post.png?mobile=1", nil)
	mobileResponse := httptest.NewRecorder()
	mediaPreviewHandler(mobileResponse, mobileRequest)
	if mobileResponse.Code != http.StatusOK {
		t.Fatalf("unexpected mobile preview response: status=%d", mobileResponse.Code)
	}
	mobileConfig, _, err := image.DecodeConfig(bytes.NewReader(mobileResponse.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode mobile preview: %v", err)
	}
	if mobileConfig.Width != 720 || mobileConfig.Height != 480 {
		t.Fatalf("unexpected mobile preview dimensions: %dx%d", mobileConfig.Width, mobileConfig.Height)
	}
	mobilePreviewPath, ok := mediaPreviewMobileCachePath(root, sourcePath)
	if !ok {
		t.Fatal("mobile preview path was rejected")
	}
	if _, err := os.Stat(mobilePreviewPath); err != nil {
		t.Fatalf("mobile preview cache missing: %v", err)
	}
	qualityRequest := httptest.NewRequest(http.MethodGet, "/preview/weibo/author/post.png?level=4&device=mobile", nil)
	qualityResponse := httptest.NewRecorder()
	mediaPreviewHandler(qualityResponse, qualityRequest)
	qualityConfig, _, err := image.DecodeConfig(bytes.NewReader(qualityResponse.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode level 4 preview: %v", err)
	}
	if qualityConfig.Width != 480 || qualityConfig.Height != 320 {
		t.Fatalf("unexpected level 4 preview dimensions: %dx%d", qualityConfig.Width, qualityConfig.Height)
	}
	qualityPreviewPath, ok := mediaPreviewCachePathWithSuffix(root, sourcePath, ".q2.mobile.4.jpg")
	if !ok {
		t.Fatal("level 4 preview path was rejected")
	}
	originalRequest := httptest.NewRequest(http.MethodGet, "/preview/weibo/author/post.png?level=0&device=mobile", nil)
	originalResponse := httptest.NewRecorder()
	mediaPreviewHandler(originalResponse, originalRequest)
	originalConfig, format, err := image.DecodeConfig(bytes.NewReader(originalResponse.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode original preview: %v", err)
	}
	if format != "png" || originalConfig.Width != 1200 || originalConfig.Height != 800 {
		t.Fatalf("level 0 did not return the original: format=%s size=%dx%d", format, originalConfig.Width, originalConfig.Height)
	}

	if err := deletePostMedia([]string{"/flow/weibo/author/post.png"}); err != nil {
		t.Fatalf("delete media: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("original media still exists: %v", err)
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Fatalf("preview cache still exists: %v", err)
	}
	if _, err := os.Stat(mobilePreviewPath); !os.IsNotExist(err) {
		t.Fatalf("mobile preview cache still exists: %v", err)
	}
	if _, err := os.Stat(qualityPreviewPath); !os.IsNotExist(err) {
		t.Fatalf("level 4 preview cache still exists: %v", err)
	}
}

func TestProjectSettingsViewUsesPreviewDefaultsAndConfiguredLevels(t *testing.T) {
	view := projectSettingsView(BilibiliConfig{})
	if view.PreviewDesktopLevel != 2 || view.PreviewMobileLevel != 3 {
		t.Fatalf("unexpected preview defaults: desktop=%d mobile=%d", view.PreviewDesktopLevel, view.PreviewMobileLevel)
	}
	desktop, mobile := 4, 5
	view = projectSettingsView(BilibiliConfig{PreviewDesktopLevel: &desktop, PreviewMobileLevel: &mobile})
	if view.PreviewDesktopLevel != 4 || view.PreviewMobileLevel != 5 {
		t.Fatalf("configured preview levels were not preserved: desktop=%d mobile=%d", view.PreviewDesktopLevel, view.PreviewMobileLevel)
	}
}

func TestCleanupMediaPreviewCacheRemovesExpiredOrphanedAndLegacyFiles(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot, oldPreviewRoot := flowRoot, previewRoot
	flowRoot, previewRoot = filepath.Join(root, "flow"), filepath.Join(root, "previews")
	t.Cleanup(func() { flowRoot, previewRoot = oldFlowRoot, oldPreviewRoot })
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

	writeFile := func(filePath string, modified time.Time) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte("preview-cache"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filePath, modified, modified); err != nil {
			t.Fatal(err)
		}
	}

	keepSource := filepath.Join(flowRoot, "weibo", "author", "keep.png")
	staleSource := filepath.Join(flowRoot, "weibo", "author", "stale.png")
	writeFile(keepSource, now.Add(-48*time.Hour))
	writeFile(staleSource, now.Add(-60*24*time.Hour))
	keepPreview, _ := mediaPreviewCachePath(flowRoot, keepSource)
	stalePreview, _ := mediaPreviewCachePath(flowRoot, staleSource)
	keepMobilePreview, _ := mediaPreviewMobileCachePath(flowRoot, keepSource)
	staleMobilePreview, _ := mediaPreviewMobileCachePath(flowRoot, staleSource)
	orphanPreview := filepath.Join(previewRoot, "weibo", "missing", "orphan.png") + mediaPreviewCacheSuffix
	legacyPreview := strings.TrimSuffix(keepPreview, mediaPreviewCacheSuffix) + ".v3.jpg"
	oldTemporary := filepath.Join(previewRoot, "weibo", ".lumic-preview-old.tmp")
	recentTemporary := filepath.Join(previewRoot, "weibo", ".lumic-preview-recent.tmp")
	writeFile(keepPreview, now.Add(-7*24*time.Hour))
	writeFile(stalePreview, now.Add(-31*24*time.Hour))
	writeFile(keepMobilePreview, now.Add(-7*24*time.Hour))
	writeFile(staleMobilePreview, now.Add(-31*24*time.Hour))
	writeFile(orphanPreview, now.Add(-24*time.Hour))
	writeFile(legacyPreview, now.Add(-24*time.Hour))
	writeFile(oldTemporary, now.Add(-2*time.Hour))
	writeFile(recentTemporary, now.Add(-30*time.Minute))

	removed, reclaimed, err := cleanupMediaPreviewCache(now)
	if err != nil {
		t.Fatalf("cleanup preview cache: %v", err)
	}
	if removed != 5 || reclaimed <= 0 {
		t.Fatalf("unexpected cleanup result: removed=%d reclaimed=%d", removed, reclaimed)
	}
	for _, retained := range []string{keepPreview, keepMobilePreview, recentTemporary} {
		if _, err := os.Stat(retained); err != nil {
			t.Fatalf("expected retained cache %s: %v", retained, err)
		}
	}
	for _, deleted := range []string{stalePreview, staleMobilePreview, orphanPreview, legacyPreview, oldTemporary} {
		if _, err := os.Stat(deleted); !os.IsNotExist(err) {
			t.Fatalf("expected cache removal %s: %v", deleted, err)
		}
	}
}

func TestPostMediaBaseNameUsesAuthorDateAndImageSequence(t *testing.T) {
	published := time.Date(2026, time.August, 13, 20, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	single := Post{Author: "测试作者", Published: published, Media: []string{"one"}}
	if got := postMediaBaseName(single, 0); got != "测试作者-20260813" {
		t.Fatalf("unexpected single image name: %s", got)
	}
	multiple := Post{Author: "测试作者", Published: published, Media: []string{"one", "two", "three"}}
	for index, want := range []string{"测试作者-20260813·1", "测试作者-20260813·2", "测试作者-20260813·3"} {
		if got := postMediaBaseName(multiple, index); got != want {
			t.Fatalf("unexpected multiple image name at %d: got=%s want=%s", index, got, want)
		}
	}
}

func TestAvailableMediaTargetBaseAvoidsOverwrite(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "作者-20260813.jpg"), []byte("existing"), 0600); err != nil {
		t.Fatalf("seed existing image: %v", err)
	}
	if got := filepath.Base(availableMediaTargetBase(directory, "作者-20260813")); got != "作者-20260813-2" {
		t.Fatalf("unexpected collision name: %s", got)
	}
}

func TestBilibiliCaptionPreservesText(t *testing.T) {
	caption := firstNonEmptyRemoteText("\n第一行\n第二行\n")
	if caption != "第一行\n第二行" {
		t.Fatalf("caption lost line breaks: %q", caption)
	}
	if got := firstNonEmptyRemoteText("", "标题"); got != "标题" {
		t.Fatalf("caption fallback: %q", got)
	}
	if got := cleanRemoteText("第一段<br>第二段</p><p>第三段"); got != "第一段\n第二段\n第三段" {
		t.Fatalf("HTML line breaks were lost: %q", got)
	}
	if got := combineRemoteText("动态描述", "标题", "正文第一行<br>正文第二行", "动态描述"); got != "动态描述\n\n标题\n\n正文第一行\n正文第二行" {
		t.Fatalf("combined caption lost content: %q", got)
	}
}

func TestCleanRemoteTextPreservesUnicodeAndHTMLEmojiLabels(t *testing.T) {
	input := `今天很开心😄<img alt="[笑cry]" src="//h5.sinaimg.cn/emoji.gif">继续出发✨`
	if got := cleanRemoteText(input); got != "今天很开心😄[笑cry]继续出发✨" {
		t.Fatalf("emoji text was lost: %q", got)
	}
}

func TestBilibiliEmojiRemainsText(t *testing.T) {
	value := map[string]any{
		"type":      "RICH_TEXT_NODE_TYPE_EMOJI",
		"orig_text": "[doge]",
		"emoji": map[string]any{
			"icon_url": "//i0.hdslb.com/bfs/emote/doge.png",
		},
	}
	if got := bilibiliInlineText(value); got != "[doge]" {
		t.Fatalf("Bilibili emoji text was lost: %q", got)
	}
}

func TestBilibiliOpusPostExtractsArticleTextAndImages(t *testing.T) {
	state := map[string]any{"detail": map[string]any{
		"id_str": "979835568137437186",
		"modules": []any{
			map[string]any{"module_title": map[string]any{"text": "收藏专栏标题"}},
			map[string]any{"module_author": map[string]any{"name": "专栏作者", "face": "//i0.hdslb.com/bfs/face/avatar.jpg", "pub_ts": "1726974146"}},
			map[string]any{"module_content": map[string]any{"paragraphs": []any{
				map[string]any{"text": map[string]any{"nodes": []any{map[string]any{"word": map[string]any{"words": "正文第一段"}}}}},
				map[string]any{"pic": map[string]any{"pics": []any{map[string]any{"url": "https://i0.hdslb.com/bfs/new_dyn/content.jpg"}}}},
			}}},
			map[string]any{"module_top": map[string]any{"display": map[string]any{"album": map[string]any{"pics": []any{map[string]any{"url": "https://i0.hdslb.com/bfs/new_dyn/cover.jpg"}}}}}},
		},
	}}
	feed := SourceConfig{ID: bilibiliFavoriteOpusPrefix + "42", Source: SourceBilibili, Name: bilibiliFavoriteOpusName, Tags: []string{"B站收藏"}}
	post, err := bilibiliOpusPost("979835568137437186", state, feed)
	if err != nil {
		t.Fatalf("parse favorite opus: %v", err)
	}
	if post.ID != "bili-dynamic-979835568137437186" || post.Author != "专栏作者" || post.Avatar != "https://i0.hdslb.com/bfs/face/avatar.jpg" {
		t.Fatalf("unexpected opus identity: %#v", post)
	}
	if !strings.Contains(post.Caption, "收藏专栏标题") || !strings.Contains(post.Caption, "正文第一段") {
		t.Fatalf("opus text was incomplete: %q", post.Caption)
	}
	if len(post.Media) != 2 || !containsString(post.Media, "https://i0.hdslb.com/bfs/new_dyn/content.jpg") || !containsString(post.Media, "https://i0.hdslb.com/bfs/new_dyn/cover.jpg") {
		t.Fatalf("opus images were incomplete: %#v", post.Media)
	}
	if post.OriginalURL != "https://www.bilibili.com/opus/979835568137437186" || !containsString(post.FeedIDs, feed.ID) {
		t.Fatalf("unexpected opus source metadata: %#v", post)
	}
}

func TestAllowedBilibiliDynamicTypeIncludesSupportedContent(t *testing.T) {
	for _, dynamicType := range []string{"DYNAMIC_TYPE_WORD", "DYNAMIC_TYPE_DRAW", "DYNAMIC_TYPE_ARTICLE", "DYNAMIC_TYPE_OPUS", "DYNAMIC_TYPE_AV"} {
		if !allowedBilibiliDynamicType(dynamicType) {
			t.Fatalf("expected %s to be collected", dynamicType)
		}
	}
	for _, dynamicType := range []string{"DYNAMIC_TYPE_FORWARD", "DYNAMIC_TYPE_LIVE_RCMD"} {
		if allowedBilibiliDynamicType(dynamicType) {
			t.Fatalf("expected %s to remain filtered", dynamicType)
		}
	}
}

func TestBilibiliArticleContentTypeIsOptional(t *testing.T) {
	if bilibiliDynamicTypeEnabled("DYNAMIC_TYPE_ARTICLE", []string{"DRAW"}, false) {
		t.Fatal("article dynamic should be filtered when ARTICLE is not enabled")
	}
	if !bilibiliDynamicTypeEnabled("DYNAMIC_TYPE_ARTICLE", []string{"DRAW", "ARTICLE"}, false) {
		t.Fatal("article dynamic should be collected when ARTICLE is enabled")
	}
	for _, dynamicType := range []string{"DYNAMIC_TYPE_WORD", "DYNAMIC_TYPE_DRAW", "DYNAMIC_TYPE_OPUS"} {
		if !bilibiliDynamicTypeEnabled(dynamicType, []string{"DRAW"}, false) {
			t.Fatalf("expected %s to remain enabled without ARTICLE", dynamicType)
		}
	}
}

func TestBilibiliVideoDynamicRequiresOptIn(t *testing.T) {
	if bilibiliDynamicTypeEnabled("DYNAMIC_TYPE_AV", []string{"DRAW"}, false) {
		t.Fatal("video dynamic should be filtered without video opt-in")
	}
	if !bilibiliDynamicTypeEnabled("DYNAMIC_TYPE_AV", []string{"DRAW"}, true) {
		t.Fatal("video dynamic should be collected after video opt-in")
	}
}

func TestNormalizeBilibiliContentTypesPreservesLegacyArticles(t *testing.T) {
	legacy := normalizeBilibiliContentTypes(nil, true)
	if !containsString(legacy, "DRAW") || !containsString(legacy, "ARTICLE") {
		t.Fatalf("legacy content types were not preserved: %#v", legacy)
	}
	newFeed := normalizeBilibiliContentTypes(nil, false)
	if !containsString(newFeed, "DRAW") || containsString(newFeed, "ARTICLE") {
		t.Fatalf("new content types should default to DRAW only: %#v", newFeed)
	}
}

func TestBilibiliRichTextExtractsOpusCaption(t *testing.T) {
	raw := json.RawMessage(`{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":""},{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"非常好灵梦画了"}]}`)
	if got := bilibiliRichText(raw); got != "非常好灵梦画了" {
		t.Fatalf("opus caption was not extracted: %q", got)
	}
	plain := json.RawMessage(`"普通动态文字"`)
	if got := bilibiliRichText(plain); got != "普通动态文字" {
		t.Fatalf("plain caption was not extracted: %q", got)
	}
}

func TestBilibiliRichTextExtractsOrigTextNodes(t *testing.T) {
	raw := json.RawMessage(`{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_EMOJI","orig_text":"[灵魂出窍]"},{"type":"RICH_TEXT_NODE_TYPE_TEXT","orig_text":"非常好灵梦画了"}]}`)
	if got := bilibiliRichText(raw); got != "[灵魂出窍]非常好灵梦画了" {
		t.Fatalf("orig_text caption was not extracted in order: %q", got)
	}
}

func TestBilibiliRichTextExtractsNestedDrawCaption(t *testing.T) {
	raw := json.RawMessage(`{"major":{"draw":{"desc":{"rich_text_nodes":[{"text":"图文动态正文"}]}}}}`)
	if got := bilibiliRichText(raw); got != "图文动态正文" {
		t.Fatalf("nested draw caption was not extracted: %q", got)
	}
}

func TestBilibiliCaptionFromRawItemPreservesUnknownNestedText(t *testing.T) {
	raw := json.RawMessage(`{"id_str":"123","modules":{"module_dynamic":{"major":{"opus":{"summary":{"paragraphs":[{"nodes":[{"text":"列表原始 JSON 中的完整文案"}]}]}}}}}}`)
	if got := bilibiliCaptionFromRaw(raw); got != "列表原始 JSON 中的完整文案" {
		t.Fatalf("raw item caption was not extracted: %q", got)
	}
}

func TestBilibiliCaptionDecodesJSONStringContent(t *testing.T) {
	raw := json.RawMessage(`{"content":"{\"paragraphs\":[{\"nodes\":[{\"text\":\"full opus caption\"}]}]}"}`)
	if got := bilibiliRichText(raw); got != "full opus caption" {
		t.Fatalf("JSON string caption was not extracted: %q", got)
	}
}

func TestBilibiliDetailCaptionPrefersFullText(t *testing.T) {
	if got := mergeDetailedRemoteText("caption preview", "caption preview with the complete body"); got != "caption preview with the complete body" {
		t.Fatalf("detail caption did not replace preview: %q", got)
	}
	if !shouldFetchBilibiliDynamicDetail("DYNAMIC_TYPE_OPUS", "preview") || shouldFetchBilibiliDynamicDetail("DYNAMIC_TYPE_DRAW", "complete draw caption") {
		t.Fatal("unexpected Bilibili detail request decision")
	}
}

func TestCollectWeiboPostsKeepsStaticOriginalForLivePhoto(t *testing.T) {
	payload := map[string]any{
		"id":         "123",
		"created_at": "Fri Aug 14 12:00:00 +0800 2026",
		"text_raw":   "Live Photo 动态",
		"pic_video":  "picture-id:live-photo-fid",
		"user": map[string]any{
			"idstr":       "42",
			"screen_name": "作者",
			"avatar_hd":   "https://example.com/avatar.jpg",
		},
		"pic_infos": map[string]any{
			"picture-id": map[string]any{
				"type": "livephoto",
				"original": map[string]any{
					"url": "https://wx1.sinaimg.cn/large/picture.jpg",
				},
			},
		},
	}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "作者"}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Media) != 1 {
		t.Fatalf("live photo static image was not collected: %#v", posts)
	}
	if !strings.Contains(posts[0].Media[0], "/original/") {
		t.Fatalf("unexpected live photo static image: media=%q", posts[0].Media[0])
	}
}

func TestPixivBrowserCredentialsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax/user/12345" || r.URL.Query().Get("full") != "1" {
			http.Error(w, "unexpected Pixiv endpoint", http.StatusNotFound)
			return
		}
		for header, want := range map[string]string{
			"User-Agent":   "Mozilla/5.0 Test",
			"Baggage":      "sentry-environment=production",
			"Cookie":       "PHPSESSID=test_session",
			"Sentry-Trace": "trace-value",
			"X-CSRF-Token": "csrf-value",
		} {
			if got := r.Header.Get(header); got != want {
				t.Fatalf("unexpected %s header: %q", header, got)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":false,"message":"","body":{"userId":"12345","name":"Pixiv User","imageBig":"https://i.pximg.net/user-profile/img/avatar.jpg"}}`))
	}))
	defer server.Close()

	credentials, err := pixivBrowserCredentialsRequest(PixivCredentials{
		UserAgent:   "Mozilla/5.0 Test",
		Baggage:     "sentry-environment=production",
		Cookie:      "PHPSESSID=test_session",
		UserID:      "12345",
		SentryTrace: "trace-value",
		CSRFToken:   "csrf-value",
	}, "", server.URL)
	if err != nil {
		t.Fatalf("validate Pixiv browser credentials: %v", err)
	}
	if credentials.UserName != "Pixiv User" || credentials.UserID != "12345" || credentials.Avatar != "https://i.pximg.net/user-profile/img/avatar.jpg" {
		t.Fatalf("unexpected Pixiv account: %#v", credentials)
	}
}

func TestPixivBookmarksSourceUsesDefaultTag(t *testing.T) {
	oldFlowRoot, oldBilibiliFile := flowRoot, bilibiliFile
	flowRoot = t.TempDir()
	bilibiliFile = filepath.Join(t.TempDir(), "platforms.enc")
	t.Cleanup(func() {
		flowRoot = oldFlowRoot
		bilibiliFile = oldBilibiliFile
	})
	store := &BilibiliStore{
		key: make([]byte, 32),
		config: BilibiliConfig{
			Pixiv:              PixivCredentials{Cookie: "PHPSESSID=test", UserID: "12345", UserName: "收藏账号", Avatar: "https://example.com/avatar.jpg"},
			PixivSubscriptions: []SourceConfig{},
		},
	}
	body := bytes.NewBufferString(`{"bookmarks":true,"includePast":true,"tags":[]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/pixiv/subscriptions", body)
	recorder := httptest.NewRecorder()
	store.pixivSubscriptionsHandler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("add Pixiv bookmarks source: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var feed SourceConfig
	if err := json.NewDecoder(recorder.Body).Decode(&feed); err != nil {
		t.Fatalf("decode Pixiv bookmarks source: %v", err)
	}
	if feed.ID != "pixiv-bookmarks-12345" || feed.Name != "P站收藏" || !containsString(feed.Tags, "P站收藏") {
		t.Fatalf("unexpected Pixiv bookmarks source: %#v", feed)
	}
	encodedFeed, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedFeed), "includePast") {
		t.Fatalf("legacy includePast setting leaked into the saved source: %s", encodedFeed)
	}
	if _, err := os.Stat(filepath.Join(feed.StoragePath, "source.json")); err != nil {
		t.Fatalf("Pixiv bookmarks metadata missing: %v", err)
	}
}

func TestBilibiliFavoriteOpusSourceUsesConnectedAccount(t *testing.T) {
	oldFlowRoot, oldBilibiliFile := flowRoot, bilibiliFile
	flowRoot = t.TempDir()
	bilibiliFile = filepath.Join(t.TempDir(), "platforms.enc")
	t.Cleanup(func() {
		flowRoot = oldFlowRoot
		bilibiliFile = oldBilibiliFile
	})
	store := &BilibiliStore{
		key: make([]byte, 32),
		config: BilibiliConfig{
			Credentials:   BilibiliCredentials{SESSDATA: "test", DedeUserID: "42", UserName: "收藏账号", Avatar: "https://example.com/avatar.jpg"},
			Subscriptions: []SourceConfig{},
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/bilibili/subscriptions?type=favorite-opus", nil)
	response := httptest.NewRecorder()
	store.subscriptionsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("add Bilibili favorite opus source: status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.config.Subscriptions) != 1 {
		t.Fatalf("favorite opus source was not added: %#v", store.config.Subscriptions)
	}
	feed := store.config.Subscriptions[0]
	if feed.ID != bilibiliFavoriteOpusPrefix+"42" || feed.Name != bilibiliFavoriteOpusName || feed.StoragePath != sourceStoragePath(SourceBilibili, bilibiliFavoriteOpusName) {
		t.Fatalf("unexpected favorite opus source: %#v", feed)
	}
	if feed.OnlyWithImages || !containsString(feed.Tags, "B站收藏") {
		t.Fatalf("unexpected favorite opus defaults: %#v", feed)
	}
	if _, err := os.Stat(filepath.Join(feed.StoragePath, "source.json")); err != nil {
		t.Fatalf("favorite opus source metadata missing: %v", err)
	}
}

func TestBilibiliImageDownloadDelayIsThrottled(t *testing.T) {
	old := os.Getenv("LUMIC_IMAGE_DOWNLOAD_DELAY_MS")
	_ = os.Unsetenv("LUMIC_IMAGE_DOWNLOAD_DELAY_MS")
	t.Cleanup(func() { _ = os.Setenv("LUMIC_IMAGE_DOWNLOAD_DELAY_MS", old) })
	if delay := imageDownloadDelay(SourceBilibili, 0); delay < 1500*time.Millisecond {
		t.Fatalf("Bilibili delay too short: %s", delay)
	}
	if delay := imageDownloadDelay(SourceWeibo, 0); delay != 180*time.Millisecond {
		t.Fatalf("unexpected non-Bilibili delay: %s", delay)
	}
}

func TestCollectedWeiboPostIncludesOriginalURL(t *testing.T) {
	payload := map[string]any{"mblog": map[string]any{"id": "AbCd", "text_raw": "正文", "created_at": "Mon Aug 12 10:00:00 +0800 2024", "user": map[string]any{"idstr": "12345", "screen_name": "博主"}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || posts[0].OriginalURL != "https://weibo.com/12345/AbCd" {
		t.Fatalf("unexpected original URL: %#v", posts)
	}
}

func TestCollectWeiboPostsIncludesPlayableVideo(t *testing.T) {
	payload := map[string]any{"mblog": map[string]any{
		"id":         "video-post",
		"text_raw":   "video caption",
		"created_at": "Mon Aug 12 10:00:00 +0800 2024",
		"user":       map[string]any{"idstr": "12345", "screen_name": "author"},
		"page_info": map[string]any{"media_info": map[string]any{
			"mp4_1080p_mp4": "https://f.video.weibocdn.com/video.mp4",
			"poster_url":    "https://wx1.sinaimg.cn/large/poster.jpg",
		}},
	}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Videos) != 1 {
		t.Fatalf("Weibo video was not collected: %#v", posts)
	}
	if posts[0].Videos[0].URL != "https://f.video.weibocdn.com/video.mp4" || posts[0].Videos[0].Poster != "https://wx1.sinaimg.cn/large/poster.jpg" {
		t.Fatalf("unexpected Weibo video metadata: %#v", posts[0].Videos[0])
	}
}

func TestCollectWeiboPostsFindsNestedSiblingVideoPoster(t *testing.T) {
	payload := map[string]any{"mblog": map[string]any{
		"id":         "nested-video-post",
		"text_raw":   "nested video caption",
		"created_at": "Mon Aug 12 10:00:00 +0800 2024",
		"user":       map[string]any{"idstr": "12345", "screen_name": "author", "cover_image": "https://image.example/profile-cover.jpg"},
		"page_info": map[string]any{
			"page_pic": map[string]any{"pic_big": "https://wx1.sinaimg.cn/large/nested-poster.jpg"},
			"media_info": map[string]any{"playback_list": []any{map[string]any{
				"play_info": map[string]any{"stream_url_hd": "https://f.video.weibocdn.com/nested-video.mp4"},
			}}},
		},
	}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Videos) != 1 {
		t.Fatalf("nested Weibo video was not collected: %#v", posts)
	}
	video := posts[0].Videos[0]
	if video.URL != "https://f.video.weibocdn.com/nested-video.mp4" || video.Poster != "https://wx1.sinaimg.cn/large/nested-poster.jpg" {
		t.Fatalf("unexpected nested Weibo video metadata: %#v", video)
	}
}

func TestNormalizePostVideosKeepsHighestQualityAndAvailablePoster(t *testing.T) {
	videos := normalizePostVideos([]PostVideo{
		{URL: "https://video.example/post-720.mp4", Poster: "https://image.example/poster.jpg"},
		{URL: "https://video.example/post-1080.mp4"},
		{URL: "https://video.example/post-360.mp4"},
	})
	if len(videos) != 1 {
		t.Fatalf("expected one normalized video, got %#v", videos)
	}
	if videos[0].URL != "https://video.example/post-1080.mp4" || videos[0].Poster != "https://image.example/poster.jpg" {
		t.Fatalf("unexpected normalized video: %#v", videos[0])
	}
}

func TestNormalizeStoredPostVideosRemovesDuplicateLocalFiles(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	directory := filepath.Join(root, "weibo", "author")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"post-1080.mp4", "post-720.mp4", "post-360.mp4", "poster.jpg"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := &Store{posts: []Post{{ID: "video-post", Source: SourceWeibo, Author: "author", Videos: []PostVideo{
		{URL: "/flow/weibo/author/post-720.mp4", Poster: "/flow/weibo/author/poster.jpg"},
		{URL: "/flow/weibo/author/post-1080.mp4"},
		{URL: "/flow/weibo/author/post-360.mp4"},
	}}}}
	changed, err := store.normalizeStoredPostVideos()
	if err != nil || !changed {
		t.Fatalf("normalize stored videos: changed=%v err=%v", changed, err)
	}
	if len(store.posts[0].Videos) != 1 || store.posts[0].Videos[0].URL != "/flow/weibo/author/post-1080.mp4" || store.posts[0].Videos[0].Poster != "/flow/weibo/author/poster.jpg" {
		t.Fatalf("unexpected stored video: %#v", store.posts[0].Videos)
	}
	for _, name := range []string{"post-720.mp4", "post-360.mp4"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatalf("duplicate video was not removed: %s err=%v", name, err)
		}
	}
	for _, name := range []string{"post-1080.mp4", "poster.jpg"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("retained video asset missing: %s err=%v", name, err)
		}
	}
}

func TestFilterSourcePosts(t *testing.T) {
	posts := []Post{
		{ID: "keep", Caption: "今天分享一张插画", Media: []string{"image.jpg"}},
		{ID: "blocked", Caption: "插画抽奖活动", Media: []string{"image.jpg"}},
		{ID: "text", Caption: "插画文字记录"},
		{ID: "miss", Caption: "普通摄影", Media: []string{"image.jpg"}},
	}
	feed := SourceConfig{OnlyWithImages: true, IncludeKeywords: []string{"插画", "绘画"}, ExcludeKeywords: []string{"抽奖"}}
	filtered := filterSourcePosts(posts, feed)
	if len(filtered) != 1 || filtered[0].ID != "keep" {
		t.Fatalf("unexpected filtered posts: %#v", filtered)
	}
}

func TestFilterSourcePostsSupportsImagesAndOptInVideos(t *testing.T) {
	posts := []Post{
		{ID: "image", Media: []string{"image.jpg"}},
		{ID: "video", Videos: []PostVideo{{URL: "video.mp4"}}},
		{ID: "text", Caption: "text only"},
	}

	imagesOnly := filterSourcePosts(posts, SourceConfig{OnlyWithImages: true})
	if len(imagesOnly) != 1 || imagesOnly[0].ID != "image" {
		t.Fatalf("image-only filtering returned %#v", imagesOnly)
	}

	imagesAndVideos := filterSourcePosts(posts, SourceConfig{OnlyWithImages: true, IncludeVideos: true})
	if len(imagesAndVideos) != 2 || imagesAndVideos[0].ID != "image" || imagesAndVideos[1].ID != "video" {
		t.Fatalf("image-and-video filtering returned %#v", imagesAndVideos)
	}

	allWithoutVideos := filterSourcePosts(posts, SourceConfig{})
	if len(allWithoutVideos) != 2 || allWithoutVideos[0].ID != "image" || allWithoutVideos[1].ID != "text" {
		t.Fatalf("video opt-out filtering returned %#v", allWithoutVideos)
	}
}

func TestFilterSourcePostsHonorsInclusiveStartDate(t *testing.T) {
	boundary := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	posts := []Post{
		{ID: "older", Published: boundary.Add(-time.Second)},
		{ID: "boundary", Published: boundary},
		{ID: "newer", Published: boundary.Add(12 * time.Hour)},
	}
	filtered := filterSourcePosts(posts, SourceConfig{StartDate: "2026-08-01"})
	if len(filtered) != 2 || filtered[0].ID != "boundary" || filtered[1].ID != "newer" {
		t.Fatalf("start-date filtering returned %#v", filtered)
	}
	if unbounded := filterSourcePosts(posts, SourceConfig{}); len(unbounded) != len(posts) {
		t.Fatalf("empty start date unexpectedly filtered posts: %#v", unbounded)
	}
}

func TestNormalizeSourceStartDate(t *testing.T) {
	for _, value := range []string{"", "2026-08-01", " 2026-08-01 "} {
		normalized, err := normalizeSourceStartDate(value)
		if err != nil {
			t.Fatalf("normalize %q: %v", value, err)
		}
		if value != "" && normalized != "2026-08-01" {
			t.Fatalf("unexpected normalized date for %q: %q", value, normalized)
		}
	}
	for _, value := range []string{"2026/08/01", "2026-02-30", "08-01-2026"} {
		if _, err := normalizeSourceStartDate(value); err == nil {
			t.Fatalf("invalid date was accepted: %q", value)
		}
	}
}

func TestCronScheduleMatchingAndMigration(t *testing.T) {
	at := time.Date(2026, 8, 13, 6, 0, 0, 0, time.Local)
	for _, expression := range []string{"0 6 * * *", "0 */6 * * *", "0 6 * * 4"} {
		if !validCron(expression) || !cronMatches(expression, at) {
			t.Fatalf("cron should match: %s", expression)
		}
	}
	if cronMatches("30 6 * * *", at) {
		t.Fatal("cron matched the wrong minute")
	}
	if normalizeSchedule("每 6 小时") != "0 */6 * * *" || normalizeSchedule("") != "0 6 * * *" {
		t.Fatal("legacy schedule migration failed")
	}
}

func TestMergePostsUpdatesArchivedMedia(t *testing.T) {
	store := &Store{posts: []Post{{ID: "post-1", Source: SourceBilibili, Author: "UP", Media: []string{"https://remote/image.jpg"}, Liked: true}}}
	added, err := store.mergePosts([]Post{{ID: "post-1", Source: SourceBilibili, Author: "UP", Media: []string{"/flow/bilibili/UP/post-1.jpg"}}})
	if err != nil || added != 0 {
		t.Fatalf("merge existing post: added=%d err=%v", added, err)
	}
	if store.posts[0].Media[0] != "/flow/bilibili/UP/post-1.jpg" || !store.posts[0].Liked {
		t.Fatalf("existing post was not updated correctly: %#v", store.posts[0])
	}
}

func TestWeiboLikesSourceDoesNotBecomeFavorite(t *testing.T) {
	store := &Store{posts: []Post{
		{ID: "legacy-like", Source: SourceWeibo, Liked: true},
		{ID: "manual-favorite", Source: SourceWeibo, Liked: true, FavoriteExplicit: true},
	}}
	incoming := []Post{
		{ID: "legacy-like", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42"}},
		{ID: "manual-favorite", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42"}},
	}
	if _, err := store.mergePosts(incoming); err != nil {
		t.Fatalf("merge weibo likes source: %v", err)
	}
	if store.posts[0].Liked || !containsString(store.posts[0].FeedIDs, "weibo-likes-42") {
		t.Fatalf("imported weibo like was treated as an app favorite: %#v", store.posts[0])
	}
	if !store.posts[1].Liked || !store.posts[1].FavoriteExplicit {
		t.Fatalf("explicit app favorite was not preserved: %#v", store.posts[1])
	}
}

func TestSetSourceTagsAppliesToEverySourcePost(t *testing.T) {
	store := &Store{posts: []Post{
		{ID: "like-a", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42"}, Author: "作者甲", Tags: []string{"旧标签"}},
		{ID: "like-b", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42", "weibo-7"}, Author: "作者乙", Tags: []string{"旧标签"}},
		{ID: "legacy-bili", Source: SourceBilibili, Author: "UP主", Tags: []string{"旧标签"}},
	}}
	likesFeed := SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo, Name: "我的点赞", Tags: []string{"点赞来源"}}
	weiboFeed := SourceConfig{ID: "weibo-7", Source: SourceWeibo, Name: "作者乙", Tags: []string{"关注作者"}}
	biliFeed := SourceConfig{ID: "bili-8", Source: SourceBilibili, Name: "UP主", Tags: []string{"绘画"}}
	feeds := []SourceConfig{likesFeed, weiboFeed, biliFeed}
	if err := store.setSourceTags(likesFeed, feeds); err != nil {
		t.Fatalf("apply likes source tags: %v", err)
	}
	if !stringSlicesEqual(store.posts[0].Tags, []string{"点赞来源"}) || !stringSlicesEqual(store.posts[1].Tags, []string{"点赞来源", "关注作者"}) {
		t.Fatalf("source tags were not applied to all source posts: %#v", store.posts)
	}
	if err := store.setSourceTags(biliFeed, feeds); err != nil {
		t.Fatalf("apply legacy source tags: %v", err)
	}
	if !containsString(store.posts[2].FeedIDs, "bili-8") || !stringSlicesEqual(store.posts[2].Tags, []string{"绘画"}) {
		t.Fatalf("legacy author membership was not backfilled: %#v", store.posts[2])
	}
}

func TestSetSourceTagsMatchesAuthorEvenWhenPostAlreadyHasAnotherFeed(t *testing.T) {
	store := &Store{posts: []Post{
		{ID: "same-author", Source: SourceBilibili, Author: "UP主", FeedIDs: []string{"bili-old"}, Tags: []string{"旧"}},
		{ID: "different-author", Source: SourceBilibili, Author: "另一个UP主", FeedIDs: []string{"bili-old"}, Tags: []string{"保留"}},
	}}
	feed := SourceConfig{ID: "bili-new", Source: SourceBilibili, Name: "UP主", Tags: []string{"新标签"}}
	if err := store.setSourceTags(feed, []SourceConfig{feed, {ID: "bili-old", Source: SourceBilibili, Name: "旧来源", Tags: []string{"旧来源标签"}}}); err != nil {
		t.Fatalf("apply source tags: %v", err)
	}
	if !containsString(store.posts[0].FeedIDs, "bili-new") || !stringSlicesEqual(store.posts[0].Tags, []string{"旧来源标签", "新标签"}) {
		t.Fatalf("matching author was not tagged: %#v", store.posts[0])
	}
	if containsString(store.posts[1].FeedIDs, "bili-new") || !stringSlicesEqual(store.posts[1].Tags, []string{"保留"}) {
		t.Fatalf("different author was unexpectedly tagged: %#v", store.posts[1])
	}
}

func TestPostsAfterUsesStrictIncrementalBoundary(t *testing.T) {
	boundary := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	posts := []Post{
		{ID: "new", Published: boundary.Add(time.Second)},
		{ID: "same", Published: boundary},
		{ID: "old", Published: boundary.Add(-time.Second)},
	}
	filtered := postsAfter(posts, boundary)
	if len(filtered) != 1 || filtered[0].ID != "new" {
		t.Fatalf("incremental sync kept the wrong posts: %#v", filtered)
	}
	all := postsAfter(posts, time.Time{})
	if len(all) != len(posts) {
		t.Fatalf("full sync boundary unexpectedly filtered posts: %#v", all)
	}
}

func TestPostsAfterOrCaptionRepairReusesArchivedMedia(t *testing.T) {
	boundary := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	store := &Store{posts: []Post{{
		ID:        "bili-dynamic-1",
		Source:    SourceBilibili,
		Caption:   "",
		Avatar:    "/flow/bilibili/author/avatar.jpg",
		Media:     []string{"/flow/bilibili/author/post.jpg"},
		Published: boundary.Add(-24 * time.Hour),
		Liked:     true,
	}}}
	incoming := []Post{{
		ID:        "bili-dynamic-1",
		Source:    SourceBilibili,
		Caption:   "非常好灵梦画了",
		Avatar:    "https://example.com/avatar.jpg",
		Media:     []string{"https://example.com/post.jpg"},
		Published: boundary.Add(-24 * time.Hour),
	}}

	filtered := store.postsAfterOrCaptionRepair(incoming, boundary)
	if len(filtered) != 1 {
		t.Fatalf("caption repair was filtered out: %#v", filtered)
	}
	if filtered[0].Caption != "非常好灵梦画了" || filtered[0].Media[0] != "/flow/bilibili/author/post.jpg" || filtered[0].Avatar != "/flow/bilibili/author/avatar.jpg" || !filtered[0].Liked {
		t.Fatalf("caption repair did not preserve archived state: %#v", filtered[0])
	}
}

func TestPostsAfterOrSourceMembershipMigratesExistingWeiboLikes(t *testing.T) {
	boundary := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	store := &Store{posts: []Post{{ID: "weibo-status-1", Source: SourceWeibo, Media: []string{"/flow/weibo/我的点赞/post.jpg"}, Published: boundary.Add(-time.Hour), Liked: true}}}
	incoming := []Post{{ID: "weibo-status-1", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42"}, Media: []string{"https://example.com/post.jpg"}, Published: boundary.Add(-time.Hour)}}
	filtered := store.postsAfterOrSourceMembership(incoming, boundary, "weibo-likes-42")
	if len(filtered) != 1 || filtered[0].Media[0] != "/flow/weibo/我的点赞/post.jpg" || !containsString(filtered[0].FeedIDs, "weibo-likes-42") {
		t.Fatalf("existing weibo likes membership was not migrated: %#v", filtered)
	}
}

func TestPostsAfterOrSourceMembershipKeepsUnknownOldCollectionPost(t *testing.T) {
	boundary := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	store := &Store{}
	incoming := []Post{{ID: "old-new-favorite", Source: SourcePixiv, FeedIDs: []string{"pixiv-bookmarks-7"}, Published: boundary.Add(-30 * 24 * time.Hour)}}
	filtered := store.postsAfterOrSourceMembership(incoming, boundary, "pixiv-bookmarks-7")
	if len(filtered) != 1 || filtered[0].ID != "old-new-favorite" {
		t.Fatalf("newly collected old post was discarded: %#v", filtered)
	}
}

func TestHistoryBackfillSkipsCompleteArchivedPosts(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	feed := SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo, Name: weiboLikesName, Tags: []string{"liked"}}
	directory := sourceStoragePath(feed.Source, feed.Name)
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(directory, "existing.jpg")
	textPath := filepath.Join(directory, "post_contents.txt")
	if err := os.WriteFile(imagePath, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(textPath, []byte("caption"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{posts: []Post{{
		ID:       "existing",
		Source:   SourceWeibo,
		FeedIDs:  []string{feed.ID},
		Author:   "author",
		Caption:  "caption",
		Tags:     []string{"liked"},
		Media:    []string{flowPublicPath(feed.Source, feed.Name, filepath.Base(imagePath))},
		TextFile: flowPublicPath(feed.Source, feed.Name, filepath.Base(textPath)),
	}}}
	incoming := []Post{
		{ID: "existing", Source: SourceWeibo, FeedIDs: []string{feed.ID}, Author: "author", Caption: "caption", Tags: []string{"liked"}, Media: []string{"https://example.com/existing.jpg"}},
		{ID: "missing", Source: SourceWeibo, FeedIDs: []string{feed.ID}, Author: "author", Caption: "new", Tags: []string{"liked"}, Media: []string{"https://example.com/missing.jpg"}},
	}
	pending, skipped := store.postsForHistoryBackfill(incoming, feed)
	if skipped != 1 || len(pending) != 1 || pending[0].ID != "missing" {
		t.Fatalf("history backfill did not skip the complete archive: pending=%#v skipped=%d", pending, skipped)
	}
}

func TestHistoryBackfillReusesExistingFilesAndRepairsMissingOnes(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	feed := SourceConfig{ID: "pixiv-bookmarks-7", Source: SourcePixiv, Name: pixivBookmarksName}
	directory := sourceStoragePath(SourcePixiv, "artist")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(directory, "first.jpg")
	if err := os.WriteFile(existingPath, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{posts: []Post{{
		ID:      "pixiv-illust-1",
		Source:  SourcePixiv,
		FeedIDs: []string{"pixiv-artist-7"},
		Author:  "artist",
		Media:   []string{flowPublicPath(SourcePixiv, "artist", filepath.Base(existingPath)), "/flow/pixiv/artist/missing.jpg"},
	}}}
	incoming := []Post{{
		ID:      "pixiv-illust-1",
		Source:  SourcePixiv,
		FeedIDs: []string{feed.ID},
		Author:  "artist",
		Media:   []string{"https://example.com/first.jpg", "https://example.com/second.jpg"},
	}}
	pending, skipped := store.postsForHistoryBackfill(incoming, feed)
	if skipped != 0 || len(pending) != 1 {
		t.Fatalf("incomplete archive was skipped: pending=%#v skipped=%d", pending, skipped)
	}
	if pending[0].Media[0] != flowPublicPath(SourcePixiv, "artist", filepath.Base(existingPath)) || pending[0].Media[1] != "https://example.com/second.jpg" {
		t.Fatalf("available archive files were not reused selectively: %#v", pending[0].Media)
	}
	if !containsString(pending[0].FeedIDs, feed.ID) {
		t.Fatalf("collection membership was not added: %#v", pending[0].FeedIDs)
	}
}

func TestHistoryBackfillHonorsSourceFiltersBeforePlanning(t *testing.T) {
	feed := SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo, OnlyWithImages: true, IncludeVideos: true, IncludeKeywords: []string{"keep"}, ExcludeKeywords: []string{"blocked"}}
	incoming := []Post{
		{ID: "image", Caption: "keep image", Media: []string{"image.jpg"}},
		{ID: "video", Caption: "keep video", Videos: []PostVideo{{URL: "video.mp4"}}},
		{ID: "text", Caption: "keep text"},
		{ID: "blocked", Caption: "keep blocked", Media: []string{"image.jpg"}},
	}
	filtered := filterSourcePosts(incoming, feed)
	pending, skipped := (&Store{}).postsForHistoryBackfill(filtered, feed)
	if skipped != 0 || len(pending) != 2 || pending[0].ID != "image" || pending[1].ID != "video" {
		t.Fatalf("history backfill ignored source filters: %#v", pending)
	}
}

func TestServeFrontendFallsBackForRoutesButNotMissingAssets(t *testing.T) {
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "public", "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "index.html"), []byte("spa-index"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "assets", "app.js"), []byte("app-code"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })

	for _, route := range []string{"/liked", "/favorites", "/source/bilibili", "/settings/platforms", "/author/bilibili/%E6%B5%8B%E8%AF%95"} {
		response := httptest.NewRecorder()
		serveFrontend(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "spa-index") {
			t.Fatalf("route %s did not receive SPA index: status=%d body=%q", route, response.Code, response.Body.String())
		}
	}
	assetResponse := httptest.NewRecorder()
	serveFrontend(assetResponse, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if assetResponse.Code != http.StatusNotFound {
		t.Fatalf("missing asset status=%d", assetResponse.Code)
	}
}

func TestLoginWeiboWithPasswordExchangesSession(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	oldPreloginEndpoint := weiboPreloginEndpoint
	oldEndpoint := weiboLoginEndpoint
	oldValidator := validateWeiboLoginSession
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prelogin" {
			if r.Method != http.MethodGet || r.URL.Query().Get("su") != encodeWeiboUsername("account") {
				t.Fatalf("unexpected prelogin request: %s %s", r.Method, r.URL.String())
			}
			fmt.Fprintf(w, `sinaSSOController.preloginCallBack({"servertime":123456,"nonce":"nonce","pubkey":"%x","rsakv":"rsa-version","showpin":0})`, privateKey.N)
			return
		}
		if r.URL.Path != "/login" || r.Method != http.MethodPost {
			t.Fatalf("unexpected login request: %s %s", r.Method, r.URL.String())
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		ciphertext, err := hex.DecodeString(r.Form.Get("sp"))
		if err != nil {
			t.Fatalf("password was not hex encoded: %v", err)
		}
		plain, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, ciphertext)
		if err != nil || string(plain) != "123456\tnonce\nsecret" || r.Form.Get("pwencode") != "rsa2" || r.Form.Get("su") != encodeWeiboUsername("account") {
			t.Fatalf("credentials were not SSO encoded correctly: plain=%q form=%#v err=%v", plain, r.Form, err)
		}
		http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "session", Path: "/"})
		writeJSON(w, map[string]any{"retcode": 0, "uid": "42"})
	}))
	defer server.Close()
	weiboPreloginEndpoint = server.URL + "/prelogin"
	weiboLoginEndpoint = server.URL + "/login"
	validateWeiboLoginSession = func(cookie, userID, proxyURL string) (WeiboCredentials, error) {
		if !strings.Contains(cookie, "SUB=session") || userID != "42" {
			t.Fatalf("unexpected session: cookie=%q userID=%q", cookie, userID)
		}
		return WeiboCredentials{Cookie: cookie, UserID: userID, UserName: "测试账号"}, nil
	}
	t.Cleanup(func() {
		weiboPreloginEndpoint = oldPreloginEndpoint
		weiboLoginEndpoint = oldEndpoint
		validateWeiboLoginSession = oldValidator
	})

	credentials, err := loginWeiboWithPassword("account", "secret", "")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if credentials.UserID != "42" || credentials.UserName != "测试账号" {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
}

func TestWeiboAccountHandlerRequiresCompletePasswordLogin(t *testing.T) {
	store := &BilibiliStore{config: BilibiliConfig{}, key: make([]byte, 32)}
	request := httptest.NewRequest(http.MethodPut, "/api/weibo/account", strings.NewReader(`{"username":"account"}`))
	response := httptest.NewRecorder()
	store.weiboAccountHandler(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "账号和密码均不能为空") {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWeiboAccountHandlerReturnsValidatedAvatar(t *testing.T) {
	tempDir := t.TempDir()
	oldFile, oldValidator := bilibiliFile, validateWeiboLoginSession
	bilibiliFile = filepath.Join(tempDir, "platform.enc")
	validateWeiboLoginSession = func(cookie, userID, proxyURL string) (WeiboCredentials, error) {
		return WeiboCredentials{Cookie: "SUB=session", UserID: "42", UserName: "登录账号", Avatar: "https://example.com/avatar.jpg"}, nil
	}
	t.Cleanup(func() {
		bilibiliFile = oldFile
		validateWeiboLoginSession = oldValidator
	})
	store := &BilibiliStore{config: BilibiliConfig{}, key: make([]byte, 32)}
	request := httptest.NewRequest(http.MethodPut, "/api/weibo/account", strings.NewReader(`{"cookie":"SUB=session","userId":"42"}`))
	response := httptest.NewRecorder()
	store.weiboAccountHandler(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"avatar":"https://example.com/avatar.jpg"`) {
		t.Fatalf("validated account avatar was not returned: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCollectionFeedsUseConnectedAccountAvatars(t *testing.T) {
	config := BilibiliConfig{
		Credentials: BilibiliCredentials{DedeUserID: "11", Avatar: "//i.example.com/bili.jpg"},
		Weibo:       WeiboCredentials{UserID: "22", UserName: "微博账号", Avatar: "https://i.example.com/weibo.jpg"},
		Pixiv:       PixivCredentials{UserID: "33", Avatar: "https://i.example.com/pixiv.jpg"},
	}
	feeds := []SourceConfig{
		{ID: bilibiliFavoriteOpusPrefix + "11", Source: SourceBilibili, Name: bilibiliFavoriteOpusName, Avatar: "https://stale.example.com/bili.png"},
		{ID: "weibo-likes-22", Source: SourceWeibo, Name: weiboLikesName, Avatar: "https://stale.example.com/weibo.png"},
		{ID: "pixiv-bookmarks-33", Source: SourcePixiv, Name: pixivBookmarksName, Avatar: "https://stale.example.com/pixiv.png"},
		{ID: "weibo-44", Source: SourceWeibo, Name: "普通订阅", Avatar: "https://i.example.com/author.jpg"},
	}
	applyCollectionAccountProfiles(feeds, config)
	if feeds[0].Avatar != "https://i.example.com/bili.jpg" {
		t.Fatalf("Bilibili collection avatar was not hydrated: %#v", feeds[0])
	}
	if feeds[1].Avatar != config.Weibo.Avatar || feeds[1].Handle != config.Weibo.UserName {
		t.Fatalf("Weibo likes account profile was not hydrated: %#v", feeds[1])
	}
	if feeds[2].Avatar != config.Pixiv.Avatar {
		t.Fatalf("Pixiv bookmarks avatar was not hydrated: %#v", feeds[2])
	}
	if feeds[3].Avatar != "https://i.example.com/author.jpg" {
		t.Fatalf("regular source avatar changed unexpectedly: %#v", feeds[3])
	}
	local := collectionFeedWithAccountProfile(SourceConfig{ID: "weibo-likes-22", Source: SourceWeibo, Name: weiboLikesName, Avatar: "/flow/weibo/%E6%88%91%E7%9A%84%E7%82%B9%E8%B5%9E/avatar.jpg"}, config)
	if !strings.HasPrefix(local.Avatar, "/flow/") {
		t.Fatalf("cached collection avatar should remain preferred: %#v", local)
	}
}

func TestWeiboAccountHandlerRefreshesMissingProfile(t *testing.T) {
	tempDir := t.TempDir()
	oldFile, oldFetcher := bilibiliFile, fetchWeiboAccountProfile
	bilibiliFile = filepath.Join(tempDir, "platform.enc")
	fetchWeiboAccountProfile = func(userID string, credentials WeiboCredentials, proxyURL string) (WeiboUser, error) {
		return WeiboUser{UserID: userID, Name: "当前微博账号", Avatar: "https://example.com/current.jpg"}, nil
	}
	t.Cleanup(func() {
		bilibiliFile = oldFile
		fetchWeiboAccountProfile = oldFetcher
	})
	store := &BilibiliStore{config: BilibiliConfig{Weibo: WeiboCredentials{Cookie: "SUB=session", UserID: "42"}}, key: make([]byte, 32)}
	response := httptest.NewRecorder()
	store.weiboAccountHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/account", nil))
	if response.Code != http.StatusOK || store.config.Weibo.UserName != "当前微博账号" || store.config.Weibo.Avatar != "https://example.com/current.jpg" {
		t.Fatalf("missing weibo profile was not refreshed: status=%d account=%#v body=%s", response.Code, store.config.Weibo, response.Body.String())
	}
}

func TestLoginWeiboWithPasswordExplainsPlatformSystemError(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	oldPreloginEndpoint, oldEndpoint := weiboPreloginEndpoint, weiboLoginEndpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prelogin" {
			fmt.Fprintf(w, `sinaSSOController.preloginCallBack({"servertime":123456,"nonce":"nonce","pubkey":"%x","rsakv":"rsa-version","showpin":0})`, privateKey.N)
			return
		}
		writeJSON(w, map[string]any{"retcode": 50000000, "reason": "系统错误，请稍后再试"})
	}))
	defer server.Close()
	weiboPreloginEndpoint, weiboLoginEndpoint = server.URL+"/prelogin", server.URL+"/login"
	t.Cleanup(func() { weiboPreloginEndpoint, weiboLoginEndpoint = oldPreloginEndpoint, oldEndpoint })

	_, err = loginWeiboWithPassword("account", "secret", "")
	if err == nil || !strings.Contains(err.Error(), "扫码或 Cookie 登录") {
		t.Fatalf("unexpected error guidance: %v", err)
	}
}

func TestPostsHandlerPersistsLikedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{{ID: "post-1", Author: "作者"}}, feeds: []SourceConfig{}, file: path}
	request := httptest.NewRequest(http.MethodPatch, "/api/posts?id=post-1", strings.NewReader(`{"liked":true}`))
	recorder := httptest.NewRecorder()
	store.postsHandler(recorder, request)
	if recorder.Code != http.StatusOK || !store.posts[0].Liked || !store.posts[0].FavoriteExplicit {
		t.Fatalf("like request failed: status=%d post=%#v body=%s", recorder.Code, store.posts[0], recorder.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted content: %v", err)
	}
	var content ContentData
	if err := json.Unmarshal(data, &content); err != nil || len(content.Posts) != 1 || !content.Posts[0].Liked || !content.Posts[0].FavoriteExplicit {
		t.Fatalf("liked state was not persisted: err=%v content=%#v", err, content)
	}

	missingRequest := httptest.NewRequest(http.MethodPatch, "/api/posts?id=missing", strings.NewReader(`{"liked":true}`))
	missingRecorder := httptest.NewRecorder()
	store.postsHandler(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing post returned status %d", missingRecorder.Code)
	}
}

func TestPostsHandlerBatchRemovesFavorites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{
		{ID: "one", Liked: true, FavoriteExplicit: true},
		{ID: "two", Liked: true, FavoriteExplicit: true},
		{ID: "three", Liked: true, FavoriteExplicit: true},
	}, feeds: []SourceConfig{}, file: path}
	request := httptest.NewRequest(http.MethodPatch, "/api/posts", strings.NewReader(`{"ids":["one","three"],"liked":false}`))
	response := httptest.NewRecorder()
	store.postsHandler(response, request)
	if response.Code != http.StatusOK || store.posts[0].Liked || !store.posts[1].Liked || store.posts[2].Liked {
		t.Fatalf("batch unfavorite failed: status=%d posts=%#v body=%s", response.Code, store.posts, response.Body.String())
	}
}

func TestPostsHandlerDeletesMultipleAndAuthorPosts(t *testing.T) {
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(t.TempDir(), "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	authorDirectory := sourceStoragePath(SourceWeibo, "甲")
	if err := os.MkdirAll(authorDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorDirectory, "media.jpg"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{
		{ID: "one", Source: SourceWeibo, Author: "甲"},
		{ID: "two", Source: SourceWeibo, Author: "乙"},
		{ID: "three", Source: SourceWeibo, Author: "甲"},
		{ID: "four", Source: SourceBilibili, Author: "甲"},
	}, feeds: []SourceConfig{}, file: path}
	request := httptest.NewRequest(http.MethodDelete, "/api/posts", strings.NewReader(`{"ids":["two"]}`))
	response := httptest.NewRecorder()
	store.postsHandler(response, request)
	if response.Code != http.StatusOK || len(store.posts) != 3 {
		t.Fatalf("batch delete failed: status=%d posts=%#v body=%s", response.Code, store.posts, response.Body.String())
	}
	authorRequest := httptest.NewRequest(http.MethodDelete, "/api/posts", strings.NewReader(`{"source":"weibo","author":"甲"}`))
	authorResponse := httptest.NewRecorder()
	store.postsHandler(authorResponse, authorRequest)
	if authorResponse.Code != http.StatusOK || len(store.posts) != 1 || store.posts[0].ID != "four" {
		t.Fatalf("author delete failed: status=%d posts=%#v body=%s", authorResponse.Code, store.posts, authorResponse.Body.String())
	}
	if _, err := os.Stat(authorDirectory); !os.IsNotExist(err) {
		t.Fatalf("author content directory was not deleted: %v", err)
	}
}

func TestAuthorDeleteRemovesResidualDirectoryWithoutPosts(t *testing.T) {
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(t.TempDir(), "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	authorDirectory := sourceStoragePath(SourcePixiv, "Residual Artist")
	if err := os.MkdirAll(authorDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorDirectory, "orphan.jpg"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	store := &Store{posts: []Post{}, feeds: []SourceConfig{}, file: filepath.Join(t.TempDir(), "content.json")}
	request := httptest.NewRequest(http.MethodDelete, "/api/posts", strings.NewReader(`{"source":"pixiv","author":"Residual Artist"}`))
	response := httptest.NewRecorder()
	store.postsHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("residual directory delete failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(authorDirectory); !os.IsNotExist(err) {
		t.Fatalf("residual author directory was not deleted: %v", err)
	}
}

func decodeTestResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return payload
}

func TestNormalizeWeiboCookieAndRequestHeaders(t *testing.T) {
	cookie := normalizeWeiboCookie("Cookie: SUB=first; XSRF-TOKEN=token; sub=duplicate; empty\nignored")
	if cookie != "SUB=first; XSRF-TOKEN=token" {
		t.Fatalf("unexpected normalized cookie: %q", cookie)
	}
	if got := weiboCookieValue(cookie, "xsrf-token"); got != "token" {
		t.Fatalf("unexpected cookie value: %q", got)
	}
	mobileRequest := httptest.NewRequest(http.MethodGet, "https://m.weibo.cn/api/container/getIndex", nil)
	weiboRequestHeaders(mobileRequest, cookie)
	if mobileRequest.Header.Get("Origin") != "https://m.weibo.cn" || mobileRequest.Header.Get("Referer") != "https://m.weibo.cn/" {
		t.Fatalf("unexpected mobile headers: origin=%q referer=%q", mobileRequest.Header.Get("Origin"), mobileRequest.Header.Get("Referer"))
	}
	if mobileRequest.Header.Get("X-XSRF-TOKEN") != "token" {
		t.Fatalf("missing XSRF header: %q", mobileRequest.Header.Get("X-XSRF-TOKEN"))
	}
	webRequest := httptest.NewRequest(http.MethodGet, "https://weibo.com/ajax/profile/info", nil)
	weiboRequestHeaders(webRequest, cookie)
	if webRequest.Header.Get("Origin") != "https://weibo.com" || webRequest.Header.Get("Referer") != "https://weibo.com/" {
		t.Fatalf("unexpected web headers: origin=%q referer=%q", webRequest.Header.Get("Origin"), webRequest.Header.Get("Referer"))
	}
}

func TestWeiboJSONPAndCrossDomainURL(t *testing.T) {
	body := []byte(`STK_123({"retcode":20000000,"data":{"alt":"ticket"}})`)
	var payload struct {
		RetCode int `json:"retcode"`
		Data    struct {
			Alt string `json:"alt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(unwrapJSONP(body), &payload); err != nil {
		t.Fatalf("decode JSONP: %v", err)
	}
	if payload.RetCode != 20000000 || payload.Data.Alt != "ticket" {
		t.Fatalf("unexpected JSONP payload: %#v", payload)
	}
	if got := weiboCrossDomainURL("https://example.com/cross?a=1", 0); got != "https://example.com/cross?a=1&action=login" {
		t.Fatalf("unexpected first cross-domain URL: %s", got)
	}
	if got := weiboCrossDomainURL("https://example.com/cross", 0); got != "https://example.com/cross?action=login" {
		t.Fatalf("unexpected URL without query: %s", got)
	}
	if got := weiboCrossDomainURL("https://example.com/other?a=1", 1); got != "https://example.com/other?a=1" {
		t.Fatalf("non-first URL changed: %s", got)
	}
}

func TestRemoteValueParsing(t *testing.T) {
	if got := jsonValueString(float64(3546637624412244)); got != "3546637624412244" {
		t.Fatalf("unexpected float UID: %s", got)
	}
	if got := jsonValueString(json.Number("3546637624412244")); got != "3546637624412244" {
		t.Fatalf("unexpected JSON number UID: %s", got)
	}
	if got := parseRemoteTimestamp(json.RawMessage(`"1723456800"`)); got != 1723456800 {
		t.Fatalf("unexpected string timestamp: %d", got)
	}
}

func TestCollectWeiboUsers(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"cards": []any{
				map[string]any{"user": map[string]any{"idstr": "3546637624412244", "screen_name": "测试博主", "avatar": "//example.com/avatar.jpg", "followers_count": float64(1234)}},
				map[string]any{"user": map[string]any{"id": json.Number("3546637624412244"), "name": "重复用户"}},
			},
		},
	}
	users := make([]WeiboUser, 0)
	collectWeiboUsers(payload, &users, make(map[string]bool))
	if len(users) != 1 {
		t.Fatalf("expected one unique user, got %#v", users)
	}
	if users[0].UserID != "3546637624412244" || users[0].Name != "测试博主" || users[0].Fans != 1234 || users[0].Avatar != "https://example.com/avatar.jpg" {
		t.Fatalf("unexpected parsed user: %#v", users[0])
	}
}

func TestPostDelete(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	localMedia := filepath.Join(root, "weibo", "post-1.jpg")
	outsideMedia := filepath.Join(filepath.Dir(root), "must-remain.jpg")
	if err := os.MkdirAll(filepath.Dir(localMedia), 0755); err != nil {
		t.Fatalf("create media directory: %v", err)
	}
	if err := os.WriteFile(localMedia, []byte("local"), 0600); err != nil {
		t.Fatalf("create local media: %v", err)
	}
	if err := os.WriteFile(outsideMedia, []byte("outside"), 0600); err != nil {
		t.Fatalf("create outside media: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outsideMedia) })
	path := filepath.Join(root, "content.json")
	store := &Store{posts: []Post{{ID: "post-1", Source: SourceWeibo, Author: "测试作者", Media: []string{"/flow/weibo/post-1.jpg", "https://example.com/image.jpg", outsideMedia}}, {ID: "post-2", Source: SourcePixiv, Author: "保留作者"}}, feeds: []SourceConfig{}, file: path}
	store.Lock()
	if err := store.saveLocked(); err != nil {
		t.Fatalf("seed content store: %v", err)
	}
	store.Unlock()

	request := httptest.NewRequest(http.MethodDelete, "/api/posts?id=post-1", nil)
	response := httptest.NewRecorder()
	store.postsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.posts) != 1 || store.posts[0].ID != "post-2" {
		t.Fatalf("unexpected posts after delete: %#v", store.posts)
	}
	if _, err := os.Stat(localMedia); !os.IsNotExist(err) {
		t.Fatalf("local media was not deleted, stat error=%v", err)
	}
	if _, err := os.Stat(outsideMedia); err != nil {
		t.Fatalf("outside media was unexpectedly removed: %v", err)
	}

	reloaded, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("reload content store: %v", err)
	}
	if len(reloaded.posts) != 1 || reloaded.posts[0].ID != "post-2" {
		t.Fatalf("deleted post returned after reload: %#v", reloaded.posts)
	}

	missingRequest := httptest.NewRequest(http.MethodDelete, "/api/posts?id=post-1", nil)
	missingResponse := httptest.NewRecorder()
	store.postsHandler(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
}

func TestPostDeleteHandlesEncodedFlowURLWithQuery(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	localMedia := filepath.Join(root, "bilibili", "测试作者", "dynamic-1.jpg")
	if err := os.MkdirAll(filepath.Dir(localMedia), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localMedia, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{posts: []Post{{ID: "post-encoded", Source: SourceBilibili, Author: "测试作者", Media: []string{"/flow/bilibili/%E6%B5%8B%E8%AF%95%E4%BD%9C%E8%80%85/dynamic-1.jpg?v=1"}}}, feeds: []SourceConfig{}, file: filepath.Join(root, "content.json")}
	response := httptest.NewRecorder()
	store.postsHandler(response, httptest.NewRequest(http.MethodDelete, "/api/posts?id=post-encoded", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(localMedia); !os.IsNotExist(err) {
		t.Fatalf("encoded local media was not deleted: %v", err)
	}
}

func TestPostDeleteRemovesVideoAndPoster(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })

	directory := filepath.Join(root, "weibo", "author")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	videoPath := filepath.Join(directory, "post-video.mp4")
	posterPath := filepath.Join(directory, "post-video-poster.jpg")
	if err := os.WriteFile(videoPath, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(posterPath, []byte("poster"), 0600); err != nil {
		t.Fatal(err)
	}

	store := &Store{
		posts: []Post{{
			ID:     "video-post",
			Source: SourceWeibo,
			Author: "author",
			Videos: []PostVideo{{URL: "/flow/weibo/author/post-video.mp4", Poster: "/flow/weibo/author/post-video-poster.jpg"}},
		}},
		file: filepath.Join(root, "content.json"),
	}
	if deleted, err := store.deletePosts([]string{"video-post"}, "", ""); err != nil || deleted != 1 {
		t.Fatalf("delete video post: deleted=%d err=%v", deleted, err)
	}
	for _, mediaPath := range []string{videoPath, posterPath} {
		if _, err := os.Stat(mediaPath); !os.IsNotExist(err) {
			t.Fatalf("video media was not deleted: %s stat=%v", mediaPath, err)
		}
	}
}

func TestAuthorTextArchiveCombinesPostsAndPreservesEmoji(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	store := &Store{posts: []Post{
		{ID: "later", Source: SourceWeibo, Author: "拾光/作者", Caption: "今天很开心 😄 [doge]\n第二行", Published: time.Date(2026, time.August, 13, 9, 30, 0, 0, time.Local)},
		{ID: "earlier", Source: SourceWeibo, Author: "拾光/作者", Caption: "更早的一条", Published: time.Date(2026, time.August, 12, 8, 0, 0, 0, time.Local)},
	}}

	changed, err := store.reconcileTextArchives()
	if err != nil {
		t.Fatalf("reconcile author text archive: %v", err)
	}
	if !changed {
		t.Fatal("post text paths were not updated")
	}
	expectedPublicPath := flowPublicPath(SourceWeibo, "拾光/作者", "post_contents.txt")
	if store.posts[0].TextFile != expectedPublicPath || store.posts[1].TextFile != expectedPublicPath {
		t.Fatalf("posts do not share one author archive: %#v", store.posts)
	}
	data, err := os.ReadFile(filepath.Join(sourceStoragePath(SourceWeibo, "拾光/作者"), "post_contents.txt"))
	if err != nil {
		t.Fatalf("read archived text: %v", err)
	}
	want := "[2026-08-12 08:00:00] 拾光/作者\n更早的一条\n\n[2026-08-13 09:30:00] 拾光/作者\n今天很开心 😄 [doge]\n第二行\n"
	if got := string(data); got != want {
		t.Fatalf("archived text changed: got %q want %q", got, want)
	}
}

func TestCollectionTextArchivesUseSourceFolderInsteadOfAuthorFolders(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	store := &Store{posts: []Post{
		{ID: "weibo-a", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42"}, Author: "微博作者甲", Caption: "点赞正文甲", Published: time.Date(2026, time.August, 12, 8, 0, 0, 0, time.Local)},
		{ID: "weibo-b", Source: SourceWeibo, FeedIDs: []string{"weibo-likes-42"}, Author: "微博作者乙", Caption: "点赞正文乙", Published: time.Date(2026, time.August, 13, 8, 0, 0, 0, time.Local)},
		{ID: "pixiv-a", Source: SourcePixiv, FeedIDs: []string{"pixiv-bookmarks-7"}, Author: "画师甲", Caption: "收藏作品甲", Published: time.Date(2026, time.August, 12, 9, 0, 0, 0, time.Local)},
		{ID: "pixiv-b", Source: SourcePixiv, FeedIDs: []string{"pixiv-bookmarks-7"}, Author: "画师乙", Caption: "收藏作品乙", Published: time.Date(2026, time.August, 13, 9, 0, 0, 0, time.Local)},
		{ID: "direct", Source: SourcePixiv, FeedIDs: []string{"pixiv-8"}, Author: "普通画师", Caption: "普通作品", Published: time.Date(2026, time.August, 14, 9, 0, 0, 0, time.Local)},
	}}
	if _, err := store.reconcileTextArchives(); err != nil {
		t.Fatalf("reconcile collection text archives: %v", err)
	}
	weiboPath := flowPublicPath(SourceWeibo, weiboLikesName, "post_contents.txt")
	pixivPath := flowPublicPath(SourcePixiv, pixivBookmarksName, "post_contents.txt")
	if store.posts[0].TextFile != weiboPath || store.posts[1].TextFile != weiboPath {
		t.Fatalf("Weibo likes were not combined in source folder: %#v", store.posts[:2])
	}
	if store.posts[2].TextFile != pixivPath || store.posts[3].TextFile != pixivPath {
		t.Fatalf("Pixiv bookmarks were not combined in source folder: %#v", store.posts[2:4])
	}
	if store.posts[4].TextFile != flowPublicPath(SourcePixiv, "普通画师", "post_contents.txt") {
		t.Fatalf("regular artist archive path changed unexpectedly: %#v", store.posts[4])
	}
	weiboContents, err := os.ReadFile(filepath.Join(sourceStoragePath(SourceWeibo, weiboLikesName), "post_contents.txt"))
	if err != nil || !bytes.Contains(weiboContents, []byte("微博作者甲")) || !bytes.Contains(weiboContents, []byte("微博作者乙")) {
		t.Fatalf("Weibo likes archive is incomplete: data=%q err=%v", weiboContents, err)
	}
	pixivContents, err := os.ReadFile(filepath.Join(sourceStoragePath(SourcePixiv, pixivBookmarksName), "post_contents.txt"))
	if err != nil || !bytes.Contains(pixivContents, []byte("画师甲")) || !bytes.Contains(pixivContents, []byte("画师乙")) {
		t.Fatalf("Pixiv bookmarks archive is incomplete: data=%q err=%v", pixivContents, err)
	}
}

func TestCollectionTextMigrationKeepsAuthorArchiveUsedByRegularFeed(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	author := "同时订阅的画师"
	authorPath := filepath.Join(sourceStoragePath(SourcePixiv, author), "post_contents.txt")
	if err := os.MkdirAll(filepath.Dir(authorPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authorPath, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	legacyPublicPath := flowPublicPath(SourcePixiv, author, "post_contents.txt")
	store := &Store{posts: []Post{
		{ID: "bookmarked", Source: SourcePixiv, FeedIDs: []string{"pixiv-bookmarks-7", "pixiv-9"}, Author: author, Caption: "收藏中的作品", TextFile: legacyPublicPath, Published: time.Date(2026, time.August, 12, 8, 0, 0, 0, time.Local)},
		{ID: "regular", Source: SourcePixiv, FeedIDs: []string{"pixiv-9"}, Author: author, Caption: "普通订阅作品", TextFile: legacyPublicPath, Published: time.Date(2026, time.August, 13, 8, 0, 0, 0, time.Local)},
	}}
	if _, err := store.reconcileTextArchives(); err != nil {
		t.Fatalf("migrate shared text archive: %v", err)
	}
	if store.posts[0].TextFile != flowPublicPath(SourcePixiv, pixivBookmarksName, "post_contents.txt") || store.posts[1].TextFile != legacyPublicPath {
		t.Fatalf("unexpected split archive paths: %#v", store.posts)
	}
	authorContents, err := os.ReadFile(authorPath)
	if err != nil || bytes.Contains(authorContents, []byte("收藏中的作品")) || !bytes.Contains(authorContents, []byte("普通订阅作品")) {
		t.Fatalf("regular author archive was removed or mixed: data=%q err=%v", authorContents, err)
	}
}

func TestReconcileCollectionMediaMovesExistingFilesIntoSourceFolder(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot, oldPreviewRoot := flowRoot, previewRoot
	flowRoot, previewRoot = filepath.Join(root, "flow"), filepath.Join(root, "previews")
	t.Cleanup(func() { flowRoot, previewRoot = oldFlowRoot, oldPreviewRoot })
	tests := []struct {
		name     string
		source   Source
		feedID   string
		feedName string
		author   string
		fileName string
	}{
		{name: "weibo likes", source: SourceWeibo, feedID: "weibo-likes-42", feedName: weiboLikesName, author: "微博作者", fileName: "weibo.jpg"},
		{name: "pixiv bookmarks", source: SourcePixiv, feedID: "pixiv-bookmarks-7", feedName: pixivBookmarksName, author: "Pixiv Artist", fileName: "pixiv.png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldPath := filepath.Join(sourceStoragePath(test.source, test.author), test.fileName)
			if err := os.MkdirAll(filepath.Dir(oldPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(oldPath, []byte(test.name), 0600); err != nil {
				t.Fatal(err)
			}
			store := &Store{
				posts: []Post{{ID: test.name, Source: test.source, FeedIDs: []string{test.feedID}, Author: test.author, Media: []string{flowPublicPath(test.source, test.author, test.fileName)}}},
				file:  filepath.Join(root, strings.ReplaceAll(test.name, " ", "-")+".json"),
			}
			feed := SourceConfig{ID: test.feedID, Source: test.source, Name: test.feedName}
			if err := store.reconcileCollectionMedia(feed); err != nil {
				t.Fatalf("move collection media: %v", err)
			}
			expectedPublicPath := flowPublicPath(test.source, test.feedName, test.fileName)
			if store.posts[0].Media[0] != expectedPublicPath {
				t.Fatalf("unexpected migrated media path: %q", store.posts[0].Media[0])
			}
			if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
				t.Fatalf("old author media was not moved: %v", err)
			}
			if _, err := os.Stat(filepath.Join(sourceStoragePath(test.source, test.feedName), test.fileName)); err != nil {
				t.Fatalf("collection media missing from source folder: %v", err)
			}
		})
	}
}

func TestMergePostsRebuildsSingleAuthorTextArchive(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	published := time.Date(2026, time.August, 13, 10, 0, 0, 0, time.Local)
	store := &Store{posts: []Post{{ID: "first", Source: SourceBilibili, Author: "同日作者", Caption: "第一条", Published: published}}, file: filepath.Join(root, "content.json")}
	if _, err := store.reconcileTextArchives(); err != nil {
		t.Fatal(err)
	}
	_, err := store.mergePosts([]Post{
		{ID: "first", Source: SourceBilibili, Author: "同日作者", Caption: "更新后的第一条", Published: published},
		{ID: "second", Source: SourceBilibili, Author: "同日作者", Caption: "第二条", Published: published.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("merge posts: %v", err)
	}
	if store.posts[0].TextFile == "" || store.posts[0].TextFile != store.posts[1].TextFile {
		t.Fatalf("merged posts do not share one archive: %#v", store.posts)
	}
	data, err := os.ReadFile(filepath.Join(sourceStoragePath(SourceBilibili, "同日作者"), "post_contents.txt"))
	if err != nil || !bytes.Contains(data, []byte("更新后的第一条")) || !bytes.Contains(data, []byte("第二条")) {
		t.Fatalf("updated author archive mismatch: data=%q err=%v", data, err)
	}
}

func TestLoadStoreFileMergesLegacyTextArchivesAndDeleteRebuildsIt(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(root, "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	path := filepath.Join(root, "content.json")
	authorDirectory := sourceStoragePath(SourcePixiv, "历史画师")
	if err := os.MkdirAll(authorDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	legacyFirst := filepath.Join(authorDirectory, "历史画师-20260812.txt")
	legacySecond := filepath.Join(authorDirectory, "历史画师-20260813.txt")
	if err := os.WriteFile(legacyFirst, []byte("旧文件一"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacySecond, []byte("旧文件二"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := ContentData{Posts: []Post{
		{ID: "legacy-first", Source: SourcePixiv, Author: "历史画师", Caption: "旧动态正文 🌙", Published: time.Date(2026, time.August, 12, 8, 0, 0, 0, time.Local), TextFile: flowPublicPath(SourcePixiv, "历史画师", filepath.Base(legacyFirst))},
		{ID: "legacy-second", Source: SourcePixiv, Author: "历史画师", Caption: "另一条正文", Published: time.Date(2026, time.August, 13, 8, 0, 0, 0, time.Local), TextFile: flowPublicPath(SourcePixiv, "历史画师", filepath.Base(legacySecond))},
	}, Feeds: []SourceConfig{}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if len(store.posts) != 2 || store.posts[0].TextFile == "" || store.posts[0].TextFile != store.posts[1].TextFile {
		t.Fatalf("text archive path was not backfilled: %#v", store.posts)
	}
	textPath, ok := localFlowArchivePath(store.posts[0].TextFile)
	if !ok {
		t.Fatalf("invalid archived text path: %q", store.posts[0].TextFile)
	}
	if archived, readErr := os.ReadFile(textPath); readErr != nil || !bytes.Contains(archived, []byte("旧动态正文 🌙")) || !bytes.Contains(archived, []byte("另一条正文")) {
		t.Fatalf("backfilled text mismatch: data=%q err=%v", archived, readErr)
	}
	if _, err := os.Stat(legacyFirst); !os.IsNotExist(err) {
		t.Fatalf("first per-post archive was not removed: %v", err)
	}
	if _, err := os.Stat(legacySecond); !os.IsNotExist(err) {
		t.Fatalf("second per-post archive was not removed: %v", err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(persisted, []byte(`"textFile"`)) {
		t.Fatalf("text archive path was not persisted: data=%s err=%v", persisted, err)
	}
	if deleted, deleteErr := store.deletePosts([]string{"legacy-first"}, "", ""); deleteErr != nil || deleted != 1 {
		t.Fatalf("delete post: deleted=%d err=%v", deleted, deleteErr)
	}
	remaining, err := os.ReadFile(textPath)
	if err != nil || bytes.Contains(remaining, []byte("旧动态正文 🌙")) || !bytes.Contains(remaining, []byte("另一条正文")) {
		t.Fatalf("author archive was not rebuilt after deleting one post: data=%q err=%v", remaining, err)
	}
	if deleted, deleteErr := store.deletePosts([]string{"legacy-second"}, "", ""); deleteErr != nil || deleted != 1 {
		t.Fatalf("delete final post: deleted=%d err=%v", deleted, deleteErr)
	}
	if _, err := os.Stat(textPath); !os.IsNotExist(err) {
		t.Fatalf("empty author archive was not deleted: %v", err)
	}
}

func TestRegularFeedSyncAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{}, feeds: []SourceConfig{{ID: "feed-demo", Source: SourceBilibili, Name: "演示来源", Enabled: true}}, file: path}
	store.Lock()
	if err := store.saveLocked(); err != nil {
		t.Fatalf("seed content store: %v", err)
	}
	store.Unlock()

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/feeds?action=sync&id=feed-demo", nil)
	syncResponse := httptest.NewRecorder()
	store.feedsHandler(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncResponse.Code, syncResponse.Body.String())
	}
	if status := decodeTestResponse(t, syncResponse)["status"]; status != "completed" {
		t.Fatalf("unexpected sync status: %v", status)
	}
	if store.feeds[0].LastSyncedAt.IsZero() {
		t.Fatal("sync did not update LastSyncedAt")
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/feeds?id=feed-demo", nil)
	deleteResponse := httptest.NewRecorder()
	store.feedsHandler(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if len(store.feeds) != 0 {
		t.Fatalf("feed was not deleted: %#v", store.feeds)
	}
	reloaded, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("reload content store: %v", err)
	}
	if len(reloaded.feeds) != 0 {
		t.Fatalf("deleted feed returned after reload: %#v", reloaded.feeds)
	}
}

func TestBilibiliSubscriptionKeepsLargeUserIDAsString(t *testing.T) {
	oldFile, oldFlowRoot := bilibiliFile, flowRoot
	tempDir := t.TempDir()
	bilibiliFile = filepath.Join(tempDir, "platform.enc")
	flowRoot = filepath.Join(tempDir, "flow")
	t.Cleanup(func() { bilibiliFile, flowRoot = oldFile, oldFlowRoot })
	store := &BilibiliStore{key: make([]byte, 32), config: BilibiliConfig{}, bilibiliQR: make(map[string]BilibiliQRSession), bilibiliClients: make(map[string]*http.Client), weiboQR: make(map[string]WeiboQRSession), weiboClients: make(map[string]*http.Client)}
	request := httptest.NewRequest(http.MethodPost, "/api/bilibili/subscriptions", strings.NewReader(`{"userId":"3546637624412244","name":"测试 UP 主","avatar":"https://example.com/up.jpg"}`))
	response := httptest.NewRecorder()
	store.subscriptionsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.config.Subscriptions) != 1 || store.config.Subscriptions[0].ID != "bili-3546637624412244" || store.config.Subscriptions[0].Avatar != "https://example.com/up.jpg" {
		t.Fatalf("large UID lost precision: %#v", store.config.Subscriptions)
	}
}

func TestBilibiliSubscriptionSettingsPersistStartDate(t *testing.T) {
	oldFile := bilibiliFile
	bilibiliFile = filepath.Join(t.TempDir(), "platform.enc")
	t.Cleanup(func() { bilibiliFile = oldFile })
	store := &BilibiliStore{
		key:    make([]byte, 32),
		config: BilibiliConfig{Subscriptions: []SourceConfig{{ID: "bili-123", Source: SourceBilibili, Name: "author", Enabled: true, Schedule: "0 6 * * *"}}},
	}

	request := httptest.NewRequest(http.MethodPut, "/api/bilibili/subscriptions", strings.NewReader(`{"id":"bili-123","enabled":true,"schedule":"0 6 * * *","startDate":"2026-08-01","contentTypes":["DRAW"]}`))
	response := httptest.NewRecorder()
	store.subscriptionsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save start date: status=%d body=%s", response.Code, response.Body.String())
	}
	if got := store.config.Subscriptions[0].StartDate; got != "2026-08-01" {
		t.Fatalf("start date was not persisted: %q", got)
	}

	invalidRequest := httptest.NewRequest(http.MethodPut, "/api/bilibili/subscriptions", strings.NewReader(`{"id":"bili-123","enabled":true,"schedule":"0 6 * * *","startDate":"2026-02-30"}`))
	invalidResponse := httptest.NewRecorder()
	store.subscriptionsHandler(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid start date status=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}
	if got := store.config.Subscriptions[0].StartDate; got != "2026-08-01" {
		t.Fatalf("invalid update changed the stored start date: %q", got)
	}
}

func TestCollectWeiboPosts(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"list": []any{map[string]any{"id": "987", "text_raw": "测试动态", "text": "<b>测试动态</b>", "created_at": "Mon Aug 12 10:00:00 +0800 2024", "user": map[string]any{"screen_name": "原博主", "avatar_hd": "https://example.com/original.jpg"}}}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{Name: "备用名称", Avatar: "fallback"}, &posts, make(map[string]bool))
	if len(posts) != 1 || posts[0].Author != "原博主" || posts[0].Avatar != "https://example.com/original.jpg" || posts[0].Caption != "测试动态" {
		t.Fatalf("unexpected parsed posts: %#v", posts)
	}
}

func TestCollectWeiboPostsPrefersOriginalImages(t *testing.T) {
	payload := map[string]any{"list": []any{map[string]any{
		"id": "original-image-test", "created_at": "Mon Aug 12 10:00:00 +0800 2024",
		"user": map[string]any{"screen_name": "author", "idstr": "123"},
		"pic_infos": map[string]any{"one": map[string]any{
			"large":    map[string]any{"url": "https://wx1.sinaimg.cn/large/example.jpg"},
			"original": map[string]any{"url": "https://wx1.sinaimg.cn/original/example.jpg"},
		}},
	}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Media) != 1 || posts[0].Media[0] != "https://wx1.sinaimg.cn/original/example.jpg" {
		t.Fatalf("original image was not preferred: %#v", posts)
	}
}

func TestCollectWeiboPostsUpgradesLargeImageURL(t *testing.T) {
	payload := map[string]any{"list": []any{map[string]any{
		"id": "large-image-test", "created_at": "Mon Aug 12 10:00:00 +0800 2024",
		"user": map[string]any{"screen_name": "author", "idstr": "123"},
		"pics": []any{map[string]any{"large": map[string]any{"url": "https://wx2.sinaimg.cn/mw690/example.png"}}},
	}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Media) != 1 || posts[0].Media[0] != "https://wx2.sinaimg.cn/original/example.png" {
		t.Fatalf("large image URL was not upgraded: %#v", posts)
	}
}

func TestBilibiliSubscriptionSyncAndDelete(t *testing.T) {
	oldFile := bilibiliFile
	bilibiliFile = filepath.Join(t.TempDir(), "platform.enc")
	t.Cleanup(func() { bilibiliFile = oldFile })

	store := &BilibiliStore{
		key:             make([]byte, 32),
		config:          BilibiliConfig{Subscriptions: []SourceConfig{{ID: "bili-123", Source: SourceBilibili, Name: "测试 UP 主", Enabled: true}}},
		bilibiliQR:      make(map[string]BilibiliQRSession),
		bilibiliClients: make(map[string]*http.Client),
		weiboQR:         make(map[string]WeiboQRSession),
		weiboClients:    make(map[string]*http.Client),
	}

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/bilibili/subscriptions?action=sync&id=bili-123", nil)
	syncResponse := httptest.NewRecorder()
	store.subscriptionsHandler(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncResponse.Code, syncResponse.Body.String())
	}
	if status := decodeTestResponse(t, syncResponse)["status"]; status != "failed" {
		t.Fatalf("unexpected sync status: %v", status)
	}
	if store.config.Subscriptions[0].LastSyncedAt.IsZero() || store.config.Subscriptions[0].LastSyncStatus != "failed" {
		t.Fatalf("sync failure status was not persisted: %#v", store.config.Subscriptions[0])
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/bilibili/subscriptions?id=bili-123", nil)
	deleteResponse := httptest.NewRecorder()
	store.subscriptionsHandler(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if len(store.config.Subscriptions) != 0 {
		t.Fatalf("subscription was not deleted: %#v", store.config.Subscriptions)
	}
	persisted, err := os.ReadFile(bilibiliFile)
	if err != nil {
		t.Fatalf("read persisted platform store: %v", err)
	}
	var reloaded BilibiliConfig
	if err := decryptJSON(store.key, persisted, &reloaded); err != nil {
		t.Fatalf("decrypt persisted platform store: %v", err)
	}
	if len(reloaded.Subscriptions) != 0 {
		t.Fatalf("deleted subscription returned after reload: %#v", reloaded.Subscriptions)
	}
}

func TestWeiboSubscriptionLifecycle(t *testing.T) {
	oldFile, oldFlowRoot := bilibiliFile, flowRoot
	tempDir := t.TempDir()
	bilibiliFile = filepath.Join(tempDir, "platform.enc")
	flowRoot = filepath.Join(tempDir, "flow")
	t.Cleanup(func() { bilibiliFile, flowRoot = oldFile, oldFlowRoot })

	store := &BilibiliStore{
		key:             make([]byte, 32),
		config:          BilibiliConfig{Weibo: WeiboCredentials{Cookie: "SUB=test", UserID: "42"}, Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}},
		bilibiliQR:      make(map[string]BilibiliQRSession),
		bilibiliClients: make(map[string]*http.Client),
		weiboQR:         make(map[string]WeiboQRSession),
		weiboClients:    make(map[string]*http.Client),
	}

	createRequest := httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions", strings.NewReader(`{"userId":"12345","name":"测试博主","avatar":"https://example.com/avatar.jpg","includePast":true,"schedule":"每 6 小时"}`))
	createResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	if len(store.config.WeiboSubscriptions) != 1 || store.config.WeiboSubscriptions[0].ID != "weibo-12345" || store.config.WeiboSubscriptions[0].Avatar != "https://example.com/avatar.jpg" || !store.config.WeiboSubscriptions[0].OnlyWithImages {
		t.Fatalf("unexpected subscriptions: %#v", store.config.WeiboSubscriptions)
	}
	if _, err := os.Stat(store.config.WeiboSubscriptions[0].StoragePath); err != nil {
		t.Fatalf("source storage was not created: %v", err)
	}

	feed := store.config.WeiboSubscriptions[0]
	feed.Enabled = false
	feed.IncludeVideos = true
	feed.Schedule = "每 12 小时"
	body, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/weibo/subscriptions", bytes.NewReader(body))
	updateResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK || store.config.WeiboSubscriptions[0].Enabled || !store.config.WeiboSubscriptions[0].IncludeVideos || store.config.WeiboSubscriptions[0].Schedule != "0 */12 * * *" {
		t.Fatalf("update failed status=%d subscriptions=%#v", updateResponse.Code, store.config.WeiboSubscriptions)
	}

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions?action=sync&id=weibo-12345", nil)
	syncResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK || store.config.WeiboSubscriptions[0].LastSyncedAt.IsZero() {
		t.Fatalf("sync failed status=%d subscriptions=%#v", syncResponse.Code, store.config.WeiboSubscriptions)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/weibo/subscriptions?id=weibo-12345", nil)
	deleteResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 0 {
		t.Fatalf("delete failed status=%d subscriptions=%#v", deleteResponse.Code, store.config.WeiboSubscriptions)
	}
	persisted, err := os.ReadFile(bilibiliFile)
	if err != nil {
		t.Fatalf("read persisted platform store: %v", err)
	}
	var reloaded BilibiliConfig
	if err := decryptJSON(store.key, persisted, &reloaded); err != nil {
		t.Fatalf("decrypt persisted platform store: %v", err)
	}
	if len(reloaded.WeiboSubscriptions) != 0 {
		t.Fatalf("deleted weibo subscription returned after reload: %#v", reloaded.WeiboSubscriptions)
	}
}

func TestWeiboLikesSubscriptionCanBeAddedAndDeleted(t *testing.T) {
	tempDir := t.TempDir()
	oldFile, oldFlowRoot := bilibiliFile, flowRoot
	bilibiliFile, flowRoot = filepath.Join(tempDir, "platform.enc"), filepath.Join(tempDir, "flow")
	t.Cleanup(func() { bilibiliFile, flowRoot = oldFile, oldFlowRoot })
	store := &BilibiliStore{config: BilibiliConfig{Weibo: WeiboCredentials{Cookie: "SUB=test", UserID: "42", UserName: "账号", Avatar: "https://example.com/me.jpg"}, WeiboSubscriptions: []SourceConfig{}}, key: make([]byte, 32)}
	create := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(create, httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions?type=likes", nil))
	if create.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 1 || store.config.WeiboSubscriptions[0].ID != "weibo-likes-42" || !store.config.WeiboSubscriptions[0].Enabled || !store.config.WeiboSubscriptions[0].OnlyWithImages {
		t.Fatalf("create likes source failed: status=%d feeds=%#v body=%s", create.Code, store.config.WeiboSubscriptions, create.Body.String())
	}
	remove := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(remove, httptest.NewRequest(http.MethodDelete, "/api/weibo/subscriptions?id=weibo-likes-42", nil))
	if remove.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 0 {
		t.Fatalf("delete likes source failed: status=%d feeds=%#v", remove.Code, store.config.WeiboSubscriptions)
	}
}

func TestReconcileMovesWeiboLikesIntoCanonicalDirectory(t *testing.T) {
	tempDir := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(tempDir, "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })

	oldName := "错误作者目录"
	oldDirectory := sourceStoragePath(SourceWeibo, oldName)
	if err := os.MkdirAll(oldDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDirectory, "image.jpg"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	content := &Store{file: filepath.Join(tempDir, "content.json"), posts: []Post{{ID: "liked-post", Source: SourceWeibo, Media: []string{flowPublicPath(SourceWeibo, oldName, "image.jpg")}, Liked: true}}}
	store := &BilibiliStore{
		content: content,
		config: BilibiliConfig{WeiboSubscriptions: []SourceConfig{{
			ID: "weibo-likes-42", Source: SourceWeibo, Name: oldName, StoragePath: oldDirectory,
		}}},
	}

	if err := store.reconcileFlowStorage(); err != nil {
		t.Fatalf("reconcile likes storage: %v", err)
	}
	expectedDirectory := sourceStoragePath(SourceWeibo, "我的点赞")
	if store.config.WeiboSubscriptions[0].Name != "我的点赞" || store.config.WeiboSubscriptions[0].StoragePath != expectedDirectory {
		t.Fatalf("likes source was not canonicalized: %#v", store.config.WeiboSubscriptions[0])
	}
	if _, err := os.Stat(filepath.Join(expectedDirectory, "image.jpg")); err != nil {
		t.Fatalf("liked image was not moved: %v", err)
	}
	expectedMedia := flowPublicPath(SourceWeibo, "我的点赞", "image.jpg")
	if content.posts[0].Media[0] != expectedMedia {
		t.Fatalf("stored media path was not rewritten: %q", content.posts[0].Media[0])
	}
	if content.posts[0].Liked || !containsString(content.posts[0].FeedIDs, "weibo-likes-42") {
		t.Fatalf("weibo likes post was not separated from app favorites: %#v", content.posts[0])
	}
}

func TestLoadStoreFileInitializesEmptyContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("initialize content store: %v", err)
	}
	if len(store.posts) != 0 || len(store.feeds) != 0 {
		t.Fatalf("new content store must be empty: posts=%#v feeds=%#v", store.posts, store.feeds)
	}
	reloaded, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("reload content store: %v", err)
	}
	if len(reloaded.posts) != 0 || len(reloaded.feeds) != 0 {
		t.Fatalf("persisted empty store loaded demo content: posts=%#v feeds=%#v", reloaded.posts, reloaded.feeds)
	}
}

func TestLoadStoreFileRemovesLegacyEmojiImagesButKeepsCaptionText(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(root, "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	emojiPath := filepath.Join(flowRoot, "weibo", "author", "emoji-old.png")
	if err := os.MkdirAll(filepath.Dir(emojiPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(emojiPath, []byte("legacy emoji"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "content.json")
	legacy := ContentData{Posts: []Post{{
		ID:      "legacy-emoji",
		Source:  SourceWeibo,
		Author:  "author",
		Caption: "今天很开心😄[笑cry]",
		Emojis:  []PostEmoji{{Text: "[笑cry]", URL: "/flow/weibo/author/emoji-old.png"}},
	}}, Feeds: []SourceConfig{}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("load legacy content: %v", err)
	}
	if len(store.posts) != 1 || store.posts[0].Caption != "今天很开心😄[笑cry]" || len(store.posts[0].Emojis) != 0 {
		t.Fatalf("legacy emoji migration changed text or kept metadata: %#v", store.posts)
	}
	if _, err := os.Stat(emojiPath); !os.IsNotExist(err) {
		t.Fatalf("legacy emoji image was not removed: %v", err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte(`"emojis"`)) {
		t.Fatalf("legacy emoji metadata was persisted: %s", persisted)
	}
}

func TestConfigurationBackupExportAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	oldAuthFile, oldBilibiliFile := authFile, bilibiliFile
	authFile, bilibiliFile = filepath.Join(tempDir, "auth.json"), filepath.Join(tempDir, "platform.enc")
	t.Cleanup(func() { authFile, bilibiliFile = oldAuthFile, oldBilibiliFile })
	auth := &AuthConfig{Username: "backup-user", PasswordHash: "backup-hash"}
	platforms := &BilibiliStore{key: make([]byte, 32), config: BilibiliConfig{ProxyURL: "http://127.0.0.1:7890", WeiboSubscriptions: []SourceConfig{{ID: "weibo-42", Source: SourceWeibo, Name: "作者"}}, Subscriptions: []SourceConfig{}}}
	handler := configurationBackupHandler(auth, platforms)
	exportResponse := httptest.NewRecorder()
	handler(exportResponse, httptest.NewRequest(http.MethodGet, "/api/configuration/backup", nil))
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export failed: status=%d body=%s", exportResponse.Code, exportResponse.Body.String())
	}
	var backup ConfigurationBackup
	if err := json.Unmarshal(exportResponse.Body.Bytes(), &backup); err != nil || backup.Version != 1 || backup.Auth.Username != "backup-user" || len(backup.Platforms.WeiboSubscriptions) != 1 {
		t.Fatalf("unexpected backup: err=%v backup=%#v", err, backup)
	}
	auth.Username, auth.PasswordHash = "changed", "changed-hash"
	platforms.config = BilibiliConfig{Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}}
	restoreResponse := httptest.NewRecorder()
	handler(restoreResponse, httptest.NewRequest(http.MethodPut, "/api/configuration/backup", bytes.NewReader(exportResponse.Body.Bytes())))
	if restoreResponse.Code != http.StatusOK || auth.Username != "backup-user" || platforms.config.ProxyURL != "http://127.0.0.1:7890" || len(platforms.config.WeiboSubscriptions) != 1 {
		t.Fatalf("restore failed: status=%d auth=%#v platforms=%#v body=%s", restoreResponse.Code, auth, platforms.config, restoreResponse.Body.String())
	}
}

func TestSessionIsInvalidatedAcrossStoreRestart(t *testing.T) {
	first := &SessionStore{tokens: make(map[string]time.Time)}
	token, err := first.create()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !first.valid(token) {
		t.Fatal("new session was not valid in its original store")
	}
	restarted := &SessionStore{tokens: make(map[string]time.Time)}
	if restarted.valid(token) {
		t.Fatal("session remained valid after store restart")
	}
}

func TestSessionHandlerRejectsCookieAfterRestart(t *testing.T) {
	token, err := (&SessionStore{tokens: make(map[string]time.Time)}).create()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	restarted := &SessionStore{tokens: make(map[string]time.Time)}
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
	response := httptest.NewRecorder()
	sessionHandler(restarted)(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authenticated":false`) {
		t.Fatalf("restarted session was accepted: status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "lumic_session" || cookies[0].MaxAge != -1 {
		t.Fatalf("expired session cookie was not cleared: %#v", cookies)
	}
}

func TestLoginCookieExpiresAfterTwentyFourHours(t *testing.T) {
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	auth := &AuthConfig{Username: "lumic", PasswordHash: hashPassword("correct-password", []byte("lumic-default-salt-v1"))}
	request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"lumic","password":"correct-password"}`))
	response := httptest.NewRecorder()
	loginHandler(sessions, auth)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("login failed: status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != 24*60*60 {
		t.Fatalf("unexpected login cookie lifetime: %#v", cookies)
	}
}

func TestLogoutRevokesSessionAndClearsCookie(t *testing.T) {
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	token, err := sessions.create()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	request.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
	response := httptest.NewRecorder()
	logoutHandler(sessions)(response, request)

	if response.Code != http.StatusOK || sessions.valid(token) {
		t.Fatalf("logout did not revoke session: status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("logout did not clear cookie: %#v", cookies)
	}
}
