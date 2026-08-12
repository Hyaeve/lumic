package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestBilibiliCaptionPreservesText(t *testing.T) {
	caption := firstNonEmptyRemoteText("\n第一行\n第二行\n")
	if caption != "第一行\n第二行" {
		t.Fatalf("caption lost line breaks: %q", caption)
	}
	if got := firstNonEmptyRemoteText("", "标题"); got != "标题" {
		t.Fatalf("caption fallback: %q", got)
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

func decodeTestResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return payload
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
				map[string]any{"user": map[string]any{"id": float64(3546637624412244), "screen_name": "测试博主", "profile_image_url": "https://example.com/avatar.jpg", "followers_count": float64(1234)}},
				map[string]any{"user": map[string]any{"id": json.Number("3546637624412244"), "name": "重复用户"}},
			},
		},
	}
	users := make([]WeiboUser, 0)
	collectWeiboUsers(payload, &users, make(map[string]bool))
	if len(users) != 1 {
		t.Fatalf("expected one unique user, got %#v", users)
	}
	if users[0].UserID != "3546637624412244" || users[0].Name != "测试博主" || users[0].Fans != 1234 {
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

func TestCollectWeiboPosts(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"list": []any{map[string]any{"id": "987", "text_raw": "测试动态", "text": "<b>测试动态</b>", "created_at": "Mon Aug 12 10:00:00 +0800 2024", "user": map[string]any{"screen_name": "原博主", "avatar_hd": "https://example.com/original.jpg"}}}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{Name: "备用名称", Avatar: "fallback"}, &posts, make(map[string]bool))
	if len(posts) != 1 || posts[0].Author != "原博主" || posts[0].Avatar != "https://example.com/original.jpg" || posts[0].Caption != "测试动态" {
		t.Fatalf("unexpected parsed posts: %#v", posts)
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
	if len(store.config.WeiboSubscriptions) != 1 || store.config.WeiboSubscriptions[0].ID != "weibo-12345" || store.config.WeiboSubscriptions[0].Avatar != "https://example.com/avatar.jpg" {
		t.Fatalf("unexpected subscriptions: %#v", store.config.WeiboSubscriptions)
	}
	if _, err := os.Stat(store.config.WeiboSubscriptions[0].StoragePath); err != nil {
		t.Fatalf("source storage was not created: %v", err)
	}

	feed := store.config.WeiboSubscriptions[0]
	feed.Enabled = false
	feed.Schedule = "每 12 小时"
	body, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/weibo/subscriptions", bytes.NewReader(body))
	updateResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK || store.config.WeiboSubscriptions[0].Enabled || store.config.WeiboSubscriptions[0].Schedule != "每 12 小时" {
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
