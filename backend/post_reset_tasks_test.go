package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitPostResetTask(t *testing.T, q *postResetTasks, id string) postResetTaskView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range q.views() {
			if view.ID == id && view.Status != syncTaskRunning {
				return view
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("reset task did not finish")
	return postResetTaskView{}
}

func TestPostResetTaskDedupProgressAndHistory(t *testing.T) {
	var queue postResetTasks
	gate := make(chan struct{})
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(gate) }) })
	var calls atomic.Int32
	first, accepted := queue.submit([]string{"one", "two"}, func(id string) error {
		calls.Add(1)
		<-gate
		if id == "two" {
			return errors.New("private-cookie=secret upstream body")
		}
		return nil
	})
	if !accepted || first.Status != syncTaskRunning || first.Total != 2 || first.Completed != 0 {
		t.Fatalf("invalid initial view: %+v", first)
	}
	duplicate, accepted := queue.submit([]string{"two", "one"}, func(string) error { panic("duplicate must not run") })
	if !accepted || duplicate.ID != first.ID {
		t.Fatal("same ID set did not reuse its active task")
	}
	if _, accepted := queue.submit([]string{"other"}, func(string) error { return nil }); accepted {
		t.Fatal("different active batch was accepted")
	}
	first.PostIDs[0] = "mutated"
	if queue.views()[0].PostIDs[0] != "one" {
		t.Fatal("caller can mutate queued IDs")
	}
	release.Do(func() { close(gate) })
	terminal := waitPostResetTask(t, &queue, first.ID)
	if terminal.Status != syncTaskCompleted || terminal.Completed != 2 || terminal.Failed != 1 || calls.Load() != 2 ||
		!reflect.DeepEqual(terminal.UpdatedIDs, []string{"one"}) || strings.Contains(terminal.Message, "private") {
		t.Fatalf("invalid partial result: %+v", terminal)
	}
	terminal.UpdatedIDs[0] = "mutated"
	if queue.views()[0].UpdatedIDs[0] != "one" {
		t.Fatal("caller can mutate terminal results")
	}
	for index := 0; index < postResetMaxHistory+2; index++ {
		view, accepted := queue.submit([]string{fmt.Sprint(index)}, func(string) error { return errors.New("private failure") })
		if !accepted {
			t.Fatal("completed batch blocked a new batch")
		}
		result := waitPostResetTask(t, &queue, view.ID)
		if result.Status != syncTaskFailed || result.Failed != 1 || result.Completed != 1 {
			t.Fatalf("all-failure status mismatch: %+v", result)
		}
	}
	if len(queue.views()) != postResetMaxHistory {
		t.Fatal("task history is not bounded")
	}
}

func TestPostResetTaskPanicReleasesWorker(t *testing.T) {
	var queue postResetTasks
	view, _ := queue.submit([]string{"one", "two", "three"}, func(id string) error {
		if id == "two" {
			panic("private-token")
		}
		return nil
	})
	result := waitPostResetTask(t, &queue, view.ID)
	if result.Status != syncTaskCompleted || result.Completed != 3 || result.Failed != 2 || len(result.UpdatedIDs) != 1 || strings.Contains(result.Message, "private") {
		t.Fatalf("panic did not produce a safe terminal result: %+v", result)
	}
	next, accepted := queue.submit([]string{"next"}, func(string) error { return nil })
	if !accepted || waitPostResetTask(t, &queue, next.ID).Failed != 0 {
		t.Fatal("panic leaked the active worker")
	}
}

func TestPostResetTaskRequestGuardsAndCanceledSubmit(t *testing.T) {
	b := &BilibiliStore{content: &Store{}}
	for _, body := range []string{`{`, `{}`, `{"ids":[]}`, `{"ids":[""]}`, `{"ids":[" a"]}`, `{"ids":["a\u0000b"]}`, `{"ids":["a"]} {}`, `{"ids":["` + strings.Repeat("x", 129) + `"]}`} {
		response := httptest.NewRecorder()
		b.resetTasksHandler(response, httptest.NewRequest(http.MethodPost, "/api/posts/reset/tasks", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("invalid body accepted: %s %d", body, response.Code)
		}
	}
	ids := make([]string, postResetMaxIDs+1)
	for index := range ids {
		ids[index] = fmt.Sprint(index)
	}
	if _, err := resetTaskIDs(ids); err == nil {
		t.Fatal("more than 1000 distinct IDs accepted")
	}
	ids = append(ids[:postResetMaxIDs], ids[0])
	if unique, err := resetTaskIDs(ids); err != nil || len(unique) != postResetMaxIDs {
		t.Fatal("limit must apply after deduplication")
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		anonymous := httptest.NewRecorder()
		authMiddleware(newSessionStore(nil), http.HandlerFunc(b.resetTasksHandler)).ServeHTTP(anonymous, httptest.NewRequest(method, "/api/posts/reset/tasks", nil))
		if anonymous.Code != http.StatusUnauthorized {
			t.Fatal("task API must require authentication")
		}
	}
	wrongMethod := httptest.NewRecorder()
	b.resetTasksHandler(wrongMethod, httptest.NewRequest(http.MethodDelete, "/api/posts/reset/tasks", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != "GET, POST" {
		t.Fatal("invalid task API method accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/posts/reset/tasks", strings.NewReader(`{"ids":["missing-one","missing-two","missing-one"]}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	b.resetMu.Lock()
	b.resetTasksHandler(response, request)
	cancel()
	b.resetMu.Unlock()
	var view postResetTaskView
	if response.Code != http.StatusAccepted || response.Header().Get("Content-Type") != "application/json; charset=utf-8" || json.Unmarshal(response.Body.Bytes(), &view) != nil || view.Total != 2 {
		t.Fatalf("submission did not return deduplicated 202: %d %s", response.Code, response.Body.String())
	}
	result := waitPostResetTask(t, &b.resetTasks, view.ID)
	if result.Completed != 2 || result.Failed != 2 {
		t.Fatal("canceling the submission context stopped the batch")
	}
}

func TestPostResetTaskContinuesAfterHTTPClientClosesAndPersists(t *testing.T) {
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	b.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{
		{ID: "weibo-status-123", Source: SourceWeibo, Author: "author", Caption: "old one", Edited: true},
		{ID: "weibo-status-124", Source: SourceWeibo, Author: "author", Caption: "old two", Edited: true},
	}}
	gate, entered := make(chan struct{}), make(chan struct{})
	var release sync.Once
	var calls atomic.Int32
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/statuses/show" {
			t.Error("unexpected reset route", r.URL.Path)
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-gate
		}
		id := r.URL.Query().Get("id")
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{
			"id": id, "text_raw": "fresh " + id, "created_at": "Sun Nov 08 12:00:00 +0800 2026",
			"user": map[string]any{"id": "42", "screen_name": "author"},
		}})
	})
	var taskID string
	t.Cleanup(func() {
		release.Do(func() { close(gate) })
		if taskID != "" {
			waitPostResetTask(t, &b.resetTasks, taskID)
		}
	})
	mux := http.NewServeMux()
	registerAPIV1Routes(mux, nil, nil, b.content, b)
	mux.HandleFunc("/api/posts/reset/tasks", b.resetTasksHandler)
	server := httptest.NewServer(mux)
	defer server.Close()
	transport := &http.Transport{}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/posts/reset/tasks", strings.NewReader(`{"ids":["weibo-status-123","weibo-status-124"]}`))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var initial postResetTaskView
	err = json.NewDecoder(response.Body).Decode(&initial)
	response.Body.Close()
	cancel()
	transport.CloseIdleConnections()
	if err != nil || response.StatusCode != http.StatusAccepted || initial.Status != syncTaskRunning {
		t.Fatalf("short submission failed: %v %+v", err, initial)
	}
	taskID = initial.ID
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("background reset did not start")
	}
	newTransport := &http.Transport{}
	defer newTransport.CloseIdleConnections()
	newClient := &http.Client{Transport: newTransport, Timeout: 3 * time.Second}
	status, err := newClient.Get(server.URL + "/api/posts/reset/tasks")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Tasks []postResetTaskView `json:"tasks"`
	}
	err = json.NewDecoder(status.Body).Decode(&snapshot)
	status.Body.Close()
	if err != nil || status.Header.Get("Cache-Control") != "no-store" || len(snapshot.Tasks) != 1 || snapshot.Tasks[0].ID != taskID || snapshot.Tasks[0].Completed != 0 {
		t.Fatal("new HTTP client cannot discover the still-running task")
	}
	if legacy := resetRequest(b, "weibo-status-124"); legacy.Code != http.StatusConflict {
		t.Fatal("legacy reset overlapped the batch worker")
	}
	release.Do(func() { close(gate) })
	deadline := time.Now().Add(5 * time.Second)
	var terminal postResetTaskView
	for time.Now().Before(deadline) {
		status, err = newClient.Get(server.URL + "/api/v1/posts/reset/tasks")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(status.Body)
		status.Body.Close()
		if json.Unmarshal(body, &snapshot) != nil || len(snapshot.Tasks) != 1 {
			t.Fatal("invalid polling response")
		}
		terminal = snapshot.Tasks[0]
		if terminal.Status != syncTaskRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if terminal.Status != syncTaskCompleted || terminal.Completed != 2 || terminal.Failed != 0 || len(terminal.UpdatedIDs) != 2 || calls.Load() != 2 {
		t.Fatalf("HTTP disconnect stopped or duplicated the batch: %+v calls=%d", terminal, calls.Load())
	}
	saved, err := loadStoreFile(b.content.file)
	if err != nil || len(saved.posts) != 2 || saved.posts[0].Caption != "fresh 123" || saved.posts[1].Caption != "fresh 124" || saved.posts[0].Edited || saved.posts[1].Edited {
		t.Fatalf("background results were not persisted: %v", err)
	}
}

func TestPostResetTaskWaitsForLegacyWorkerAndRereadsPosts(t *testing.T) {
	b := &BilibiliStore{content: &Store{posts: []Post{{ID: "weibo-status-123", Source: SourceWeibo, Author: "author"}}}}
	b.resetMu.Lock()
	response := httptest.NewRecorder()
	b.resetTasksHandler(response, httptest.NewRequest(http.MethodPost, "/api/posts/reset/tasks", strings.NewReader(`{"ids":["weibo-status-123","missing"]}`)))
	var initial postResetTaskView
	if json.Unmarshal(response.Body.Bytes(), &initial) != nil || response.Code != http.StatusAccepted {
		b.resetMu.Unlock()
		t.Fatal("batch submission blocked behind legacy reset")
	}
	if b.resetTasks.views()[0].Completed != 0 {
		b.resetMu.Unlock()
		t.Fatal("batch failed instead of waiting for the legacy worker")
	}
	b.content.Lock()
	b.content.posts = nil
	b.content.Unlock()
	b.resetMu.Unlock()
	result := waitPostResetTask(t, &b.resetTasks, initial.ID)
	if result.Completed != 2 || result.Failed != 2 || len(result.UpdatedIDs) != 0 {
		t.Fatal("batch did not reread posts after waiting")
	}
}
