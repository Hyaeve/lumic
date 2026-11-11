package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	syncTaskQueued    = "queued"
	syncTaskRunning   = "running"
	syncTaskCompleted = "completed"
	syncTaskFailed    = "failed"
	syncMaxActive     = 5
	syncMaxHistory    = 256
)

var syncTaskSequence atomic.Uint64

var syncTaskPrefix = func() string {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		return hex.EncodeToString(nonce[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}()

type syncTaskResult struct {
	source                   SourceConfig
	err                      error
	added, count, httpStatus int
}

type syncTaskView struct {
	ID       string        `json:"id"`
	SourceID string        `json:"sourceId"`
	Status   string        `json:"status"`
	Full     bool          `json:"full"`
	Message  string        `json:"message,omitempty"`
	Source   *SourceConfig `json:"source,omitempty"`
}

type syncSourceKey struct {
	source Source
	id     string
}

type syncTask struct {
	id           string
	sequence     uint64
	key          syncSourceKey
	full, legacy bool
	done         chan struct{}
	run          func() syncTaskResult
	status       string
	result       syncTaskResult
}

type syncQueue struct {
	mu                sync.Mutex
	active            map[syncSourceKey]*syncTask
	byID              map[string]*syncTask
	pending           []*syncTask
	history           []string
	running           int
	runningByPlatform map[Source]int
}

func syncPlatform(source Source) (Source, int) {
	switch source {
	case SourceWeibo, SourcePixiv, SourceBilibili:
		return source, 2
	case SourceTwitter:
		return source, 1
	default:
		return "", 1
	}
}

// submit reuses an active task only when the requested mode matches.
func (q *syncQueue) submit(feed SourceConfig, full, legacy bool, run func() syncTaskResult) (*syncTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := syncSourceKey{source: feed.Source, id: feed.ID}
	if task := q.active[key]; task != nil {
		return task, task.full == full && task.legacy == legacy
	}
	if q.active == nil {
		q.active = make(map[syncSourceKey]*syncTask)
		q.byID = make(map[string]*syncTask)
		q.runningByPlatform = make(map[Source]int)
	}
	sequence := syncTaskSequence.Add(1)
	task := &syncTask{
		id: syncTaskPrefix + "-" + strconv.FormatUint(sequence, 36), sequence: sequence,
		key: key, full: full, legacy: legacy, done: make(chan struct{}), run: run, status: syncTaskQueued,
	}
	q.active[key] = task
	q.byID[task.id] = task
	q.pending = append(q.pending, task)
	q.dispatchLocked()
	return task, true
}

func (q *syncQueue) dispatchLocked() {
	if q.running >= syncMaxActive {
		return
	}
	// A saturated platform must not prevent an older eligible task on another
	// platform from using the remaining global slots.
	remaining := q.pending[:0]
	for _, task := range q.pending {
		platform, limit := syncPlatform(task.key.source)
		if q.running >= syncMaxActive || q.runningByPlatform[platform] >= limit {
			remaining = append(remaining, task)
			continue
		}
		q.running++
		q.runningByPlatform[platform]++
		task.status = syncTaskRunning
		go q.execute(task)
	}
	clear(q.pending[len(remaining):])
	if len(remaining) == 0 {
		remaining = nil
	}
	q.pending = remaining
}

func (q *syncQueue) execute(task *syncTask) {
	interrupted := syncTaskResult{err: errors.New("同步任务异常终止"), httpStatus: http.StatusInternalServerError}
	result := interrupted
	defer func() {
		if recover() != nil {
			log.Printf("event=sync_task_panic id=%s", task.id)
			result = interrupted
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		task.result = result
		task.run = nil
		task.status = syncTaskCompleted
		if result.err != nil {
			task.status = syncTaskFailed
		}
		delete(q.active, task.key)
		platform, _ := syncPlatform(task.key.source)
		q.running--
		q.runningByPlatform[platform]--
		if len(q.history) >= syncMaxHistory {
			delete(q.byID, q.history[0])
			q.history = q.history[1:]
		}
		q.history = append(q.history, task.id)
		close(task.done)
		q.dispatchLocked()
	}()
	result = task.run()
}

func (t *syncTask) wait(ctx context.Context) (syncTaskResult, bool) {
	select {
	case <-t.done:
		return t.result, true
	default:
	}
	select {
	case <-t.done:
		return t.result, true
	case <-ctx.Done():
		return syncTaskResult{}, false
	}
}

func syncTaskViewLocked(task *syncTask) syncTaskView {
	view := syncTaskView{ID: task.id, SourceID: task.key.id, Status: task.status, Full: task.full}
	if view.Status == syncTaskCompleted || view.Status == syncTaskFailed {
		source := task.result.source
		view.Message = source.LastSyncMessage
		if source.ID != "" {
			view.Source = &source
		}
		if task.result.err != nil {
			view.Message = task.result.err.Error()
		}
	}
	return view
}

func (q *syncQueue) view(task *syncTask) syncTaskView {
	q.mu.Lock()
	defer q.mu.Unlock()
	return syncTaskViewLocked(task)
}

func (q *syncQueue) views(ids []string) []syncTaskView {
	q.mu.Lock()
	defer q.mu.Unlock()
	selected := make(map[string]*syncTask, len(q.active)+len(ids))
	for _, task := range q.active {
		selected[task.id] = task
	}
	for _, id := range ids {
		if task := q.byID[id]; task != nil {
			selected[id] = task
		}
	}
	tasks := make([]*syncTask, 0, len(selected))
	for _, task := range selected {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].sequence < tasks[j].sequence })
	views := make([]syncTaskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, syncTaskViewLocked(task))
	}
	return views
}
