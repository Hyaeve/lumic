package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fillSourceSyncTestPlatform(t *testing.T, b *BilibiliStore, source Source) func() {
	t.Helper()
	_, limit := syncPlatform(source)
	tasks := make([]*syncTask, 0, limit)
	releases := make([]func(), 0, limit)
	for i := 0; i < limit; i++ {
		gate, release := queueTestGate(t)
		task, _ := b.syncTasks.submit(SourceConfig{ID: fmt.Sprintf("busy-%d", i), Source: source}, false, false, func() syncTaskResult {
			<-gate
			return syncTaskResult{}
		})
		tasks = append(tasks, task)
		releases = append(releases, release)
	}
	return func() {
		for _, release := range releases {
			release()
		}
		for _, task := range tasks {
			waitQueueTestTask(t, task)
		}
	}
}

func sourceSyncTestTasks(t *testing.T, b *BilibiliStore, ids string) []syncTaskView {
	t.Helper()
	response := httptest.NewRecorder()
	b.syncStatusHandler(response, httptest.NewRequest(http.MethodGet, "/api/sync/status?ids="+ids, nil))
	var payload struct {
		Tasks []syncTaskView `json:"tasks"`
	}
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload.Tasks == nil {
		t.Fatalf("invalid task snapshot: status=%d body=%s", response.Code, response.Body.String())
	}
	return payload.Tasks
}

func findSourceSyncTestTask(t *testing.T, b *BilibiliStore, sourceID string) *syncTask {
	t.Helper()
	b.syncTasks.mu.Lock()
	defer b.syncTasks.mu.Unlock()
	for _, task := range b.syncTasks.active {
		if task.key.id == sourceID {
			return task
		}
	}
	t.Fatalf("no active task for %s", sourceID)
	return nil
}

func clearSourceSyncTestFeeds(b *BilibiliStore) {
	b.Lock()
	b.config.Subscriptions, b.config.WeiboSubscriptions, b.config.PixivSubscriptions, b.config.TwitterSubscriptions = nil, nil, nil, nil
	b.Unlock()
}

func TestSourceSyncAsyncSubmissionAndStatusRoutes(t *testing.T) {
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	var requests atomic.Int32
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "fixture"}
	b.config.WeiboSubscriptions = []SourceConfig{feed}
	release := fillSourceSyncTestPlatform(t, b, SourceWeibo)
	mux := http.NewServeMux()
	registerAPIV1Routes(mux, nil, nil, nil, b)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sync/tasks", strings.NewReader(`{"sourceId":"weibo-42","full":false}`)))
	var first syncTaskView
	if response.Code != http.StatusAccepted || response.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(response.Body.Bytes(), &first) != nil || first.ID == "" || first.SourceID != feed.ID || first.Status != syncTaskQueued || first.Full {
		t.Fatalf("submission did not return queued 202: %d %s", response.Code, response.Body.String())
	}
	duplicate := httptest.NewRecorder()
	b.syncTasksHandler(duplicate, httptest.NewRequest(http.MethodPost, "/api/sync/tasks", strings.NewReader(`{"sourceId":"weibo-42"}`)))
	var second syncTaskView
	if duplicate.Code != http.StatusAccepted || json.Unmarshal(duplicate.Body.Bytes(), &second) != nil || second.ID != first.ID {
		t.Fatalf("same-mode submission was not deduplicated: %d %s", duplicate.Code, duplicate.Body.String())
	}
	conflict := httptest.NewRecorder()
	b.syncTasksHandler(conflict, httptest.NewRequest(http.MethodPost, "/api/sync/tasks", strings.NewReader(`{"sourceId":"weibo-42","full":true}`)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("mode conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	status := httptest.NewRecorder()
	mux.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/sync/status", nil))
	if status.Code != http.StatusOK || status.Header().Get("Cache-Control") != "no-store" || !strings.Contains(status.Body.String(), first.ID) {
		t.Fatalf("V1 status route omitted active task: %d %s", status.Code, status.Body.String())
	}
	task := findSourceSyncTestTask(t, b, feed.ID)
	clearSourceSyncTestFeeds(b)
	release()
	result := waitQueueTestTask(t, task)
	if result.err == nil || !strings.Contains(result.err.Error(), "订阅已删除") || requests.Load() != 0 {
		t.Fatalf("deleted queued source was still executed: %+v", result)
	}
	if active := sourceSyncTestTasks(t, b, ""); len(active) != 0 {
		t.Fatalf("terminal tasks leaked into unrequested snapshot: %#v", active)
	}
	terminal := sourceSyncTestTasks(t, b, first.ID)
	if len(terminal) != 1 || terminal[0].Status != syncTaskFailed || terminal[0].Message != result.err.Error() {
		t.Fatalf("requested terminal task was not returned: %#v", terminal)
	}
}

func TestSourceSyncTaskEndpointsRejectInvalidRequests(t *testing.T) {
	b := &BilibiliStore{}
	for _, scenario := range []struct {
		name, method, path, body, allow string
		handler                         http.HandlerFunc
		want                            int
	}{
		{"submit-method", http.MethodGet, "/api/sync/tasks", "", "POST", b.syncTasksHandler, http.StatusMethodNotAllowed},
		{"status-method", http.MethodPost, "/api/sync/status", "", "GET", b.syncStatusHandler, http.StatusMethodNotAllowed},
		{"submit-bad-json", http.MethodPost, "/api/sync/tasks", "{", "", b.syncTasksHandler, http.StatusBadRequest},
		{"submit-empty-id", http.MethodPost, "/api/sync/tasks", `{"sourceId":"  "}`, "", b.syncTasksHandler, http.StatusBadRequest},
		{"submit-unknown-id", http.MethodPost, "/api/sync/tasks", `{"sourceId":"missing"}`, "", b.syncTasksHandler, http.StatusNotFound},
		{"submit-body-limit", http.MethodPost, "/api/sync/tasks", `{"sourceId":"` + strings.Repeat("x", 5000) + `"}`, "", b.syncTasksHandler, http.StatusBadRequest},
		{"status-id-limit", http.MethodGet, "/api/sync/status?ids=" + strings.Repeat("x", (16<<10)+1), "", "", b.syncStatusHandler, http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			scenario.handler(response, httptest.NewRequest(scenario.method, scenario.path, strings.NewReader(scenario.body)))
			if response.Code != scenario.want || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Allow") != scenario.allow {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestSourceSyncLegacyPlatformRoutesUseQueueAndCanceledWaitKeepsTask(t *testing.T) {
	for _, scenario := range []struct {
		source Source
		id     string
	}{
		{SourceBilibili, "bili-42"}, {SourcePixiv, "pixiv-42"}, {SourceWeibo, "weibo-42"}, {SourceTwitter, "twitter-author-fixture"},
	} {
		t.Run(string(scenario.source), func(t *testing.T) {
			b := newWeiboAuthTestStore(t, WeiboCredentials{})
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("deleted queued subscription made an unexpected platform request")
				w.WriteHeader(http.StatusInternalServerError)
			})
			feed := SourceConfig{ID: scenario.id, Source: scenario.source, Name: "fixture"}
			var handler http.HandlerFunc
			switch scenario.source {
			case SourceBilibili:
				b.config.Subscriptions, handler = []SourceConfig{feed}, b.subscriptionsHandler
			case SourcePixiv:
				b.config.PixivSubscriptions, handler = []SourceConfig{feed}, b.pixivSubscriptionsHandler
			case SourceWeibo:
				b.config.WeiboSubscriptions, handler = []SourceConfig{feed}, b.weiboSubscriptionsHandler
			case SourceTwitter:
				b.config.TwitterSubscriptions, handler = []SourceConfig{feed}, b.twitterSubscriptionsHandler
			}
			release := fillSourceSyncTestPlatform(t, b, scenario.source)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			response := httptest.NewRecorder()
			handler(response, httptest.NewRequest(http.MethodPost, "/api/"+string(scenario.source)+"/subscriptions?action=sync&id="+feed.ID, nil).WithContext(ctx))
			task := findSourceSyncTestTask(t, b, feed.ID)
			requireQueueTestStatus(t, &b.syncTasks, task, syncTaskQueued)
			if task.full || task.legacy {
				t.Fatal("normal subscription task has the wrong mode")
			}
			conflict := httptest.NewRecorder()
			handler(conflict, httptest.NewRequest(http.MethodPost, "/api/"+string(scenario.source)+"/subscriptions?action=resync&id="+feed.ID, nil).WithContext(ctx))
			if conflict.Code != http.StatusConflict {
				t.Fatalf("legacy route did not reject incompatible task: %d %s", conflict.Code, conflict.Body.String())
			}
			clearSourceSyncTestFeeds(b)
			release()
			if result := waitQueueTestTask(t, task); result.err == nil || !strings.Contains(result.err.Error(), "订阅已删除") {
				t.Fatalf("canceling HTTP wait canceled the queued job or deleted source ran: %+v", result)
			}
		})
	}
}

func TestSourceSyncLegacyWeiboLikesUsesSameSourceQueue(t *testing.T) {
	b := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "42", Cookie: "SUB=fixture"})
	b.config.WeiboSubscriptions = []SourceConfig{{ID: "weibo-likes-42", Source: SourceWeibo, Name: weiboLikesName}}
	release := fillSourceSyncTestPlatform(t, b, SourceWeibo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.weiboLikesHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/weibo/likes", nil).WithContext(ctx))
	task := findSourceSyncTestTask(t, b, "weibo-likes-42")
	requireQueueTestStatus(t, &b.syncTasks, task, syncTaskQueued)
	if !task.legacy {
		t.Fatal("legacy likes task lost its response mode")
	}
	conflict := httptest.NewRecorder()
	b.syncTasksHandler(conflict, httptest.NewRequest(http.MethodPost, "/api/sync/tasks", strings.NewReader(`{"sourceId":"weibo-likes-42"}`)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("likes and subscription handlers did not share deduplication: %d %s", conflict.Code, conflict.Body.String())
	}
	b.Lock()
	b.config.Weibo.UserID = "99"
	b.Unlock()
	release()
	result := waitQueueTestTask(t, task)
	if result.err == nil || result.httpStatus != http.StatusConflict || !strings.Contains(result.err.Error(), "账号已更换") {
		t.Fatalf("queued likes used the old account: %+v", result)
	}
}

func TestSourceSyncQueuedTaskUsesLatestSubscriptionSettings(t *testing.T) {
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "fixture", Enabled: true, OnlyWithImages: true, IncludeKeywords: []string{"old"}}
	b.config.WeiboSubscriptions = []SourceConfig{feed}
	b.content = &Store{file: filepath.Join(t.TempDir(), "posts.json")}
	var requests atomic.Int32
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"ok":1,"data":{"cards":[{"mblog":{"id":"123","text":"fresh caption","created_at":"Wed Nov 11 12:00:00 +0800 2026","user":{"idstr":"42","screen_name":"fixture"}}}]}}`)
	})
	release := fillSourceSyncTestPlatform(t, b, SourceWeibo)
	task, _ := b.enqueueSourceSync(feed, false)
	requireQueueTestStatus(t, &b.syncTasks, task, syncTaskQueued)
	off := false
	b.Lock()
	b.config.WeiboSubscriptions[0].OnlyWithImages = false
	b.config.WeiboSubscriptions[0].IncludeKeywords = []string{"fresh"}
	b.config.WeiboSubscriptions[0].IncludeLivePhotos = &off
	b.config.WeiboSubscriptions[0].Tags = []string{"updated"}
	b.Unlock()
	release()
	result := waitQueueTestTask(t, task)
	if result.err != nil || requests.Load() != 1 || len(b.content.posts) != 1 || b.content.posts[0].Caption != "fresh caption" || !containsString(b.content.posts[0].Tags, "updated") || result.source.OnlyWithImages || result.source.IncludeLivePhotos == nil || *result.source.IncludeLivePhotos {
		t.Fatalf("queued task used stale filters/configuration: requests=%d posts=%v result=%+v", requests.Load(), b.content.posts, result)
	}
	status := sourceSyncTestTasks(t, b, task.id)
	if len(status) != 1 || status[0].Status != syncTaskCompleted || status[0].Message != result.source.LastSyncMessage {
		t.Fatalf("completed sync missing from requested snapshot: %#v", status)
	}
}

func TestSourceSyncBatchAndSchedulerQueueWithoutBlocking(t *testing.T) {
	for _, mode := range []string{"batch", "scheduled"} {
		t.Run(mode, func(t *testing.T) {
			b := newWeiboAuthTestStore(t, WeiboCredentials{})
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("deleted queued source made an unexpected platform request")
				w.WriteHeader(http.StatusInternalServerError)
			})
			b.config.WeiboSubscriptions = []SourceConfig{
				{ID: "weibo-42", Source: SourceWeibo, Name: "one", Enabled: true, Schedule: "0 6 * * *"},
				{ID: "weibo-43", Source: SourceWeibo, Name: "two", Enabled: true, Schedule: "0 6 * * *"},
				{ID: "weibo-44", Source: SourceWeibo, Name: "disabled", Enabled: false, Schedule: "0 6 * * *"},
			}
			release := fillSourceSyncTestPlatform(t, b, SourceWeibo)
			if mode == "batch" {
				response := httptest.NewRecorder()
				b.syncHandler(response, httptest.NewRequest(http.MethodPost, "/api/sync", nil))
				if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"started"`) {
					t.Fatalf("batch sync did not return immediately: %d %s", response.Code, response.Body.String())
				}
				duplicate := httptest.NewRecorder()
				b.syncHandler(duplicate, httptest.NewRequest(http.MethodPost, "/api/sync", nil))
				if !strings.Contains(duplicate.Body.String(), `"status":"running"`) {
					t.Fatalf("duplicate batch started again: %s", duplicate.Body.String())
				}
			} else {
				b.runScheduledSync(time.Date(2026, 11, 11, 6, 0, 0, 0, time.Local))
			}
			deadline := time.Now().Add(2 * time.Second)
			for len(b.syncTasks.views(nil)) != 4 {
				if time.Now().After(deadline) {
					t.Fatal("batch/scheduler did not queue both enabled sources")
				}
				time.Sleep(time.Millisecond)
			}
			first := findSourceSyncTestTask(t, b, "weibo-42")
			second := findSourceSyncTestTask(t, b, "weibo-43")
			requireQueueTestStatus(t, &b.syncTasks, first, syncTaskQueued)
			requireQueueTestStatus(t, &b.syncTasks, second, syncTaskQueued)
			b.runScheduledSync(time.Date(2026, 11, 11, 6, 0, 0, 0, time.Local))
			if got := len(b.syncTasks.views(nil)); got != 4 {
				t.Fatalf("scheduler/batch duplicated tasks or included disabled source: %d tasks", got)
			}
			clearSourceSyncTestFeeds(b)
			release()
			waitQueueTestTask(t, first)
			waitQueueTestTask(t, second)
			for {
				b.RLock()
				running := b.syncInProgress
				b.RUnlock()
				if !running {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("batch completion flag was not cleared")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}
