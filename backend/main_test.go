package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func TestRegularFeedSyncAndDelete(t *testing.T) {
	store := &Store{feeds: []SourceConfig{{ID: "feed-demo", Source: SourceBilibili, Name: "演示来源", Enabled: true}}}

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
}
