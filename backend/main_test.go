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

func TestCollectWeiboUsers(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"cards": []any{
				map[string]any{"user": map[string]any{"id": "42", "screen_name": "测试博主", "profile_image_url": "https://example.com/avatar.jpg", "followers_count": float64(1234)}},
				map[string]any{"user": map[string]any{"id": "42", "name": "重复用户"}},
			},
		},
	}
	users := make([]WeiboUser, 0)
	collectWeiboUsers(payload, &users, make(map[string]bool))
	if len(users) != 1 {
		t.Fatalf("expected one unique user, got %#v", users)
	}
	if users[0].UserID != "42" || users[0].Name != "测试博主" || users[0].Fans != 1234 {
		t.Fatalf("unexpected parsed user: %#v", users[0])
	}
}

func TestPostDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{{ID: "post-1", Source: SourceWeibo, Author: "测试作者"}, {ID: "post-2", Source: SourcePixiv, Author: "保留作者"}}, feeds: []SourceConfig{}, file: path}
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
	if status := decodeTestResponse(t, syncResponse)["status"]; status != "started" {
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
	if status := decodeTestResponse(t, syncResponse)["status"]; status != "started" {
		t.Fatalf("unexpected sync status: %v", status)
	}
	if store.config.Subscriptions[0].LastSyncedAt.IsZero() {
		t.Fatal("sync did not update LastSyncedAt")
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

	createRequest := httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions", strings.NewReader(`{"userId":"12345","name":"测试博主","includePast":true,"schedule":"每 6 小时"}`))
	createResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	if len(store.config.WeiboSubscriptions) != 1 || store.config.WeiboSubscriptions[0].ID != "weibo-12345" {
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
