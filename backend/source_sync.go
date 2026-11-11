package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

func (b *BilibiliStore) findSyncSource(id string, source Source) (SourceConfig, bool) {
	b.RLock()
	defer b.RUnlock()
	for _, feeds := range [][]SourceConfig{b.config.Subscriptions, b.config.WeiboSubscriptions, b.config.PixivSubscriptions, b.config.TwitterSubscriptions} {
		for _, feed := range feeds {
			if feed.ID == id && (source == "" || feed.Source == source) {
				return feed, true
			}
		}
	}
	return SourceConfig{}, false
}

func (b *BilibiliStore) enqueueSourceSync(feed SourceConfig, full bool) (*syncTask, bool) {
	return b.syncTasks.submit(feed, full, false, func() syncTaskResult {
		current, found := b.findSyncSource(feed.ID, feed.Source)
		if !found {
			return syncTaskResult{source: feed, err: errors.New("订阅已删除，未执行同步")}
		}
		updated, err := b.syncSource(current, full)
		return syncTaskResult{source: updated, err: err}
	})
}

func (b *BilibiliStore) respondSourceSync(w http.ResponseWriter, r *http.Request, feed SourceConfig) {
	task, compatible := b.enqueueSourceSync(feed, r.URL.Query().Get("action") == "resync")
	if !compatible {
		writeAPIError(w, http.StatusConflict, "该订阅已有其他同步任务，请等待完成")
		return
	}
	result, finished := task.wait(r.Context())
	if !finished {
		return
	}
	if result.err != nil {
		writeJSON(w, map[string]any{"status": "failed", "message": result.err.Error(), "source": result.source})
		return
	}
	writeJSON(w, map[string]any{"status": "completed", "message": result.source.LastSyncMessage, "source": result.source})
}

func (b *BilibiliStore) syncTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		SourceID string `json:"sourceId"`
		Full     bool   `json:"full"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || strings.TrimSpace(input.SourceID) == "" {
		writeAPIError(w, http.StatusBadRequest, "订阅编号无效")
		return
	}
	feed, found := b.findSyncSource(strings.TrimSpace(input.SourceID), "")
	if !found {
		writeAPIError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	task, compatible := b.enqueueSourceSync(feed, input.Full)
	if !compatible {
		writeAPIError(w, http.StatusConflict, "该订阅已有其他同步任务，请等待完成")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, b.syncTasks.view(task))
}

func (b *BilibiliStore) syncStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ids := r.URL.Query().Get("ids")
	if len(ids) > 16<<10 {
		writeAPIError(w, http.StatusBadRequest, "查询的同步任务过多")
		return
	}
	writeJSON(w, map[string]any{"tasks": b.syncTasks.views(strings.Split(ids, ","))})
}

func (b *BilibiliStore) waitForSyncBatch(feeds []SourceConfig) {
	tasks := make([]*syncTask, 0, len(feeds))
	for _, feed := range feeds {
		if feed.Enabled {
			task, _ := b.enqueueSourceSync(feed, false)
			tasks = append(tasks, task)
		}
	}
	for _, task := range tasks {
		task.wait(context.Background())
	}
}

func (b *BilibiliStore) syncLegacyWeiboLikes(feed SourceConfig) syncTaskResult {
	b.RLock()
	credentials := b.config.Weibo
	b.RUnlock()
	if feed.ID != "weibo-likes-"+credentials.UserID {
		return syncTaskResult{err: errors.New("微博账号已更换，请重新同步点赞"), httpStatus: http.StatusConflict}
	}
	if current, found := b.findSyncSource(feed.ID, SourceWeibo); found {
		feed = current
	}
	posts, err := b.fetchWeiboLikedPosts(feed, false)
	if err != nil {
		return syncTaskResult{source: feed, err: err, httpStatus: http.StatusBadGateway}
	}
	unlock := b.lockSourceArchive(SourceWeibo)
	defer unlock()
	feed, posts, err = b.archiveSourceContent(feed, posts)
	if err != nil {
		return syncTaskResult{source: feed, err: fmt.Errorf("无法归档微博点赞动态：%w", err), httpStatus: http.StatusBadGateway}
	}
	if b.content == nil {
		return syncTaskResult{source: feed, err: errors.New("动态存储未初始化"), httpStatus: http.StatusInternalServerError}
	}
	added, err := b.content.mergePosts(posts)
	if err != nil {
		return syncTaskResult{source: feed, err: errors.New("无法保存微博点赞动态"), httpStatus: http.StatusInternalServerError}
	}
	feed.LastSyncMessage = fmt.Sprintf("已同步 %d 条微博点赞动态", len(posts))
	return syncTaskResult{source: feed, added: added, count: len(posts)}
}

func (b *BilibiliStore) lockSourceArchive(source Source) func() {
	var mutex *sync.Mutex
	switch source {
	case SourceBilibili:
		mutex = &b.bilibiliArchiveMu
	case SourceWeibo:
		mutex = &b.weiboArchiveMu
	case SourcePixiv:
		mutex = &b.pixivArchiveMu
	default:
		mutex = &b.otherArchiveMu
	}
	mutex.Lock()
	return mutex.Unlock
}
