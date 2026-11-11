package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitQueueTestTask(t *testing.T, task *syncTask) syncTaskResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, ok := task.wait(ctx)
	if !ok {
		t.Fatalf("task %s did not finish", task.id)
	}
	return result
}

func queueTestGate(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return gate, release
}

func requireQueueTestStatus(t *testing.T, q *syncQueue, task *syncTask, want string) {
	t.Helper()
	if got := q.view(task).Status; got != want {
		t.Fatalf("task %s status = %q, want %q", task.key.id, got, want)
	}
}

func TestSyncQueuePlatformAndGlobalLimits(t *testing.T) {
	var q syncQueue
	submit := func(platform Source, id string) (*syncTask, func()) {
		gate, release := queueTestGate(t)
		task, accepted := q.submit(SourceConfig{ID: id, Source: platform}, false, false, func() syncTaskResult {
			<-gate
			return syncTaskResult{}
		})
		if !accepted {
			t.Fatalf("new task %s was not accepted", id)
		}
		return task, release
	}
	w1, releaseW1 := submit(SourceWeibo, "weibo-1")
	w2, releaseW2 := submit(SourceWeibo, "weibo-2")
	w3, releaseW3 := submit(SourceWeibo, "weibo-3")
	requireQueueTestStatus(t, &q, w1, syncTaskRunning)
	requireQueueTestStatus(t, &q, w2, syncTaskRunning)
	requireQueueTestStatus(t, &q, w3, syncTaskQueued)
	p1, releaseP1 := submit(SourcePixiv, "pixiv-1")
	p2, releaseP2 := submit(SourcePixiv, "pixiv-2")
	p3, releaseP3 := submit(SourcePixiv, "pixiv-3")
	b1, releaseB1 := submit(SourceBilibili, "bili-1")
	requireQueueTestStatus(t, &q, p1, syncTaskRunning)
	requireQueueTestStatus(t, &q, p2, syncTaskRunning)
	requireQueueTestStatus(t, &q, p3, syncTaskQueued)
	requireQueueTestStatus(t, &q, b1, syncTaskRunning)
	b2, releaseB2 := submit(SourceBilibili, "bili-2")
	b3, releaseB3 := submit(SourceBilibili, "bili-3")
	x1, releaseX1 := submit(SourceTwitter, "x-1")
	x2, releaseX2 := submit(SourceTwitter, "x-2")
	for _, task := range []*syncTask{b2, b3, x1, x2} {
		requireQueueTestStatus(t, &q, task, syncTaskQueued)
	}
	releaseP1()
	waitQueueTestTask(t, p1)
	requireQueueTestStatus(t, &q, w3, syncTaskQueued)
	requireQueueTestStatus(t, &q, p3, syncTaskRunning)
	requireQueueTestStatus(t, &q, b2, syncTaskQueued)
	releaseP2()
	waitQueueTestTask(t, p2)
	requireQueueTestStatus(t, &q, b2, syncTaskRunning)
	requireQueueTestStatus(t, &q, b3, syncTaskQueued)
	releaseP3()
	waitQueueTestTask(t, p3)
	requireQueueTestStatus(t, &q, b3, syncTaskQueued)
	requireQueueTestStatus(t, &q, x1, syncTaskRunning)
	releaseW1()
	waitQueueTestTask(t, w1)
	requireQueueTestStatus(t, &q, w3, syncTaskRunning)
	releaseW2()
	waitQueueTestTask(t, w2)
	requireQueueTestStatus(t, &q, x2, syncTaskQueued)
	u1, releaseU1 := submit(Source("unknown-one"), "unknown-1")
	u2, releaseU2 := submit(Source("unknown-two"), "unknown-2")
	requireQueueTestStatus(t, &q, u1, syncTaskRunning)
	requireQueueTestStatus(t, &q, u2, syncTaskQueued)
	releaseB1()
	waitQueueTestTask(t, b1)
	requireQueueTestStatus(t, &q, b3, syncTaskRunning)
	releaseX1()
	waitQueueTestTask(t, x1)
	requireQueueTestStatus(t, &q, x2, syncTaskRunning)
	releaseU1()
	waitQueueTestTask(t, u1)
	requireQueueTestStatus(t, &q, u2, syncTaskRunning)
	for _, release := range []func(){releaseW3, releaseB2, releaseB3, releaseX2, releaseU2} {
		release()
	}
	for _, task := range []*syncTask{w3, b2, b3, x2, u2} {
		waitQueueTestTask(t, task)
	}
	if active := q.views(nil); active == nil || len(active) != 0 {
		t.Fatalf("want an empty non-nil snapshot, got %#v", active)
	}
}

func TestSyncQueueDeduplicatesActiveSourceAndRejectsModeConflicts(t *testing.T) {
	var q syncQueue
	gate, release := queueTestGate(t)
	feed := SourceConfig{ID: "same-id", Source: SourceWeibo}
	var calls atomic.Int32
	run := func() syncTaskResult {
		calls.Add(1)
		<-gate
		return syncTaskResult{source: feed}
	}
	first, ok := q.submit(feed, false, false, run)
	if !ok {
		t.Fatal("initial task was not accepted")
	}
	for _, mode := range []struct {
		full, legacy, allowed bool
	}{{false, false, true}, {true, false, false}, {false, true, false}} {
		task, allowed := q.submit(feed, mode.full, mode.legacy, run)
		if task != first || allowed != mode.allowed {
			t.Fatalf("mode %+v got task %p, allowed %v", mode, task, allowed)
		}
	}
	other, allowed := q.submit(SourceConfig{ID: feed.ID, Source: SourcePixiv}, false, false, func() syncTaskResult {
		return syncTaskResult{}
	})
	if !allowed || other == first {
		t.Fatal("identical source IDs on distinct platforms were incorrectly deduplicated")
	}
	waitQueueTestTask(t, other)
	release()
	waitQueueTestTask(t, first)
	if got := calls.Load(); got != 1 {
		t.Fatalf("duplicate callback ran: %d calls", got)
	}
	second, accepted := q.submit(feed, true, false, run)
	if !accepted || first.id == second.id {
		t.Fatal("completed job prevented submitting a fresh task")
	}
	waitQueueTestTask(t, second)
}

func TestSyncQueueFailureAndPanicReleaseCapacity(t *testing.T) {
	var q syncQueue
	failureGate, releaseFailure := queueTestGate(t)
	panicGate, releasePanic := queueTestGate(t)
	failure := errors.New("upstream unavailable")
	first, _ := q.submit(SourceConfig{ID: "failure", Source: SourceTwitter}, false, true, func() syncTaskResult {
		<-failureGate
		return syncTaskResult{err: failure, httpStatus: http.StatusBadGateway}
	})
	second, _ := q.submit(SourceConfig{ID: "panic", Source: SourceTwitter}, true, false, func() syncTaskResult {
		<-panicGate
		panic("credentials should not appear in task status")
	})
	third, _ := q.submit(SourceConfig{ID: "next", Source: SourceTwitter}, false, false, func() syncTaskResult {
		return syncTaskResult{source: SourceConfig{ID: "next", LastSyncMessage: "done"}, added: 2, count: 3}
	})
	requireQueueTestStatus(t, &q, second, syncTaskQueued)
	requireQueueTestStatus(t, &q, third, syncTaskQueued)
	releaseFailure()
	result := waitQueueTestTask(t, first)
	if result.err != failure || result.httpStatus != http.StatusBadGateway || q.view(first).Message != failure.Error() {
		t.Fatalf("failure result was lost: %+v", result)
	}
	requireQueueTestStatus(t, &q, first, syncTaskFailed)
	requireQueueTestStatus(t, &q, second, syncTaskRunning)
	releasePanic()
	result = waitQueueTestTask(t, second)
	view := q.view(second)
	if view.Status != syncTaskFailed || !view.Full || result.httpStatus != http.StatusInternalServerError || result.err == nil || strings.Contains(view.Message, "credentials") {
		t.Fatalf("panic did not produce a sanitized failure: %+v, %+v", view, result)
	}
	result = waitQueueTestTask(t, third)
	view = q.view(third)
	if view.Status != syncTaskCompleted || view.Message != "done" || view.Source == nil || view.Source.ID != "next" || result.added != 2 || result.count != 3 {
		t.Fatalf("next task did not finish normally: %+v, %+v", view, result)
	}
}

func TestSyncQueueCanceledWaitDoesNotCancelTask(t *testing.T) {
	var q syncQueue
	gate, release := queueTestGate(t)
	task, _ := q.submit(SourceConfig{ID: "test", Source: SourceWeibo}, false, false, func() syncTaskResult {
		<-gate
		return syncTaskResult{source: SourceConfig{ID: "test"}}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := task.wait(ctx); ok {
		t.Fatal("waiting on an unfinished task did not honor cancellation")
	}
	requireQueueTestStatus(t, &q, task, syncTaskRunning)
	release()
	if result := waitQueueTestTask(t, task); result.source.ID != "test" {
		t.Fatalf("task did not complete after wait cancellation: %+v", result)
	}
}

func TestSyncQueueViewsKeepBoundedHistoryInSubmissionOrder(t *testing.T) {
	var q syncQueue
	if views := q.views(nil); views == nil || len(views) != 0 {
		t.Fatalf("empty queue should return an empty array: %#v", views)
	}
	ids := make([]string, 0, syncMaxHistory+2)
	feed := SourceConfig{ID: "same-source", Source: SourceBilibili}
	for i := 0; i < syncMaxHistory+2; i++ {
		task, _ := q.submit(feed, false, false, func() syncTaskResult {
			return syncTaskResult{source: feed}
		})
		waitQueueTestTask(t, task)
		ids = append(ids, task.id)
	}
	gate, release := queueTestGate(t)
	active, _ := q.submit(feed, false, false, func() syncTaskResult {
		<-gate
		return syncTaskResult{}
	})
	views := q.views([]string{active.id, ids[0], ids[len(ids)-1], ids[2], ids[2], "missing"})
	want := []string{ids[2], ids[len(ids)-1], active.id}
	if len(views) != len(want) {
		t.Fatalf("wanted retained results and one active task, got %#v", views)
	}
	for i, view := range views {
		if view.ID != want[i] {
			t.Fatalf("snapshot[%d] = %s, want %s", i, view.ID, want[i])
		}
	}
	if retained := q.views(ids); len(retained) != syncMaxHistory+1 {
		t.Fatalf("got %d terminal/active entries, want %d", len(retained), syncMaxHistory+1)
	}
	release()
	waitQueueTestTask(t, active)
}
