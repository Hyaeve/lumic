package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const postResetMaxIDs = 1000
const postResetMaxHistory = 32

type postResetTaskView struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Total      int      `json:"total"`
	Completed  int      `json:"completed"`
	Failed     int      `json:"failed"`
	Message    string   `json:"message"`
	PostIDs    []string `json:"postIds"`
	UpdatedIDs []string `json:"updatedIds"`
}

type postResetTask struct {
	view postResetTaskView
	key  string
}

type postResetTasks struct {
	mu      sync.Mutex
	active  *postResetTask
	history []postResetTaskView
}

func copyPostResetTask(view postResetTaskView) postResetTaskView {
	view.PostIDs = append([]string{}, view.PostIDs...)
	view.UpdatedIDs = append([]string{}, view.UpdatedIDs...)
	return view
}

func resetTaskIDs(ids []string) ([]string, error) {
	seen := make(map[string]bool)
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 128 || strings.TrimSpace(id) != id || strings.ContainsRune(id, '\x00') {
			return nil, errors.New("动态编号无效")
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
			if len(unique) > postResetMaxIDs {
				return nil, errors.New("一次最多重置 1000 条动态")
			}
		}
	}
	if len(unique) == 0 {
		return nil, errors.New("请选择要重置的动态")
	}
	return unique, nil
}

func (q *postResetTasks) submit(ids []string, run func(string) error) (postResetTaskView, bool) {
	ordered := append([]string(nil), ids...)
	sort.Strings(ordered)
	key := strings.Join(ordered, "\x00")
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active != nil {
		return copyPostResetTask(q.active.view), q.active.key == key
	}
	task := &postResetTask{key: key, view: postResetTaskView{
		ID:     syncTaskPrefix + "-reset-" + strconv.FormatUint(syncTaskSequence.Add(1), 36),
		Status: syncTaskRunning, Total: len(ids), Message: "正在后台重新拉取动态",
		PostIDs: append([]string(nil), ids...), UpdatedIDs: []string{},
	}}
	q.active = task
	go q.execute(task, run)
	return copyPostResetTask(task.view), true
}

func (q *postResetTasks) execute(task *postResetTask, run func(string) error) {
	defer func() {
		panicked := recover() != nil
		q.mu.Lock()
		defer q.mu.Unlock()
		if panicked {
			log.Printf("event=post_reset_task_panic id=%s", task.view.ID)
			task.view.Failed += task.view.Total - task.view.Completed
			task.view.Completed = task.view.Total
		}
		task.view.Status = syncTaskCompleted
		if task.view.Failed == task.view.Total {
			task.view.Status = syncTaskFailed
		}
		task.view.Message = fmt.Sprintf("重新拉取完成：成功 %d 条，失败 %d 条", len(task.view.UpdatedIDs), task.view.Failed)
		if task.view.Failed > 0 {
			task.view.Message += "；失败动态原归档已保留"
		}
		q.history = append(q.history, copyPostResetTask(task.view))
		if len(q.history) > postResetMaxHistory {
			q.history = append([]postResetTaskView(nil), q.history[len(q.history)-postResetMaxHistory:]...)
		}
		q.active = nil
	}()
	for _, id := range task.view.PostIDs {
		err := run(id)
		q.mu.Lock()
		task.view.Completed++
		if err != nil {
			task.view.Failed++
		} else {
			task.view.UpdatedIDs = append(task.view.UpdatedIDs, id)
		}
		task.view.Message = fmt.Sprintf("正在后台重新拉取动态：%d/%d", task.view.Completed, task.view.Total)
		q.mu.Unlock()
	}
}

func (q *postResetTasks) views() []postResetTaskView {
	q.mu.Lock()
	defer q.mu.Unlock()
	views := make([]postResetTaskView, 0, len(q.history)+1)
	if q.active != nil {
		views = append(views, copyPostResetTask(q.active.view))
	}
	for index := len(q.history) - 1; index >= 0; index-- {
		views = append(views, copyPostResetTask(q.history[index]))
	}
	return views
}

func (b *BilibiliStore) resetTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"tasks": b.resetTasks.views()})
	case http.MethodPost:
		var input struct {
			IDs []string `json:"ids"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			writeAPIError(w, 400, "动态编号无效")
			return
		}
		ids, err := resetTaskIDs(input.IDs)
		if err != nil {
			writeAPIError(w, 400, err.Error())
			return
		}
		// Background items wait for legacy requests instead of failing with 409.
		// Every item still shares the original single-reset worker and throttle.
		view, accepted := b.resetTasks.submit(ids, func(id string) error {
			b.resetMu.Lock()
			defer b.resetMu.Unlock()
			_, _, err := b.resetPost(context.Background(), id)
			return err
		})
		if !accepted {
			writeAPIError(w, 409, "已有重置任务正在后台执行，请完成后重试")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, view)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
