package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const authorInitialLookback = 72 * time.Hour

// This cursor is independent of the visible last-attempt time and the user's
// optional historical start-date filter.
type authorSyncWindow struct {
	SubscribedAt  time.Time `json:"subscribedAt"`
	PulledThrough time.Time `json:"pulledThrough"`
}

func sourceIncrementalBoundary(feed SourceConfig) time.Time {
	if feed.AuthorSync == nil {
		return feed.LastSyncedAt
	}
	if feed.AuthorSync.PulledThrough.IsZero() {
		return feed.AuthorSync.SubscribedAt.Add(-authorInitialLookback)
	}
	// Some providers only report seconds. Include that boundary second so a
	// post published during the previous request cannot fall into the gap.
	return feed.AuthorSync.PulledThrough.Truncate(time.Second).Add(-time.Nanosecond)
}

func sourcePageBeforeBoundary(posts []Post, boundary time.Time) bool {
	if len(posts) == 0 || boundary.IsZero() {
		return false
	}
	for _, post := range posts {
		if post.Published.IsZero() || post.Published.After(boundary) {
			return false
		}
	}
	return true
}

func sameAuthorSyncGeneration(left, right SourceConfig) bool {
	if left.AuthorSync == nil || right.AuthorSync == nil {
		return left.AuthorSync == nil && right.AuthorSync == nil
	}
	return left.AuthorSync.SubscribedAt.Equal(right.AuthorSync.SubscribedAt)
}

func sourceScheduleMatches(feed SourceConfig, at time.Time) bool {
	if feed.ScheduleTimeZone != "" {
		zone, err := time.LoadLocation(feed.ScheduleTimeZone)
		if err != nil {
			return false
		}
		at = at.In(zone)
	}
	return cronMatches(normalizeSchedule(feed.Schedule), at)
}

func weeklyAuthorSchedule(at time.Time, zone string) (string, error) {
	if zone != "" {
		if zone == "Local" || len(zone) > 100 {
			return "", fmt.Errorf("无效的本地时区")
		}
		location, err := time.LoadLocation(zone)
		if err != nil {
			return "", fmt.Errorf("无效的本地时区")
		}
		at = at.In(location)
	}
	return fmt.Sprintf("0 %d * * %d", at.Hour(), at.Weekday()), nil
}

func newAuthorSubscription(target authorSubscriptionTarget, at time.Time, schedule, zone string) SourceConfig {
	prefix, handle := "", "UID "+target.ID
	switch target.Source {
	case SourceBilibili:
		prefix = "bili-"
	case SourceWeibo:
		prefix = "weibo-"
	case SourcePixiv:
		prefix = "pixiv-"
	case SourceTwitter:
		prefix, handle = "twitter-author-", "@"+target.ID
	}
	feed := SourceConfig{
		ID: prefix + target.ID, Source: target.Source, Name: target.Name, Handle: handle,
		Avatar: target.Avatar, Enabled: true, Schedule: schedule, ScheduleTimeZone: zone,
		StoragePath: sourceStoragePath(target.Source, target.Name), OnlyWithImages: true,
		AuthorSync: &authorSyncWindow{SubscribedAt: at.UTC()},
	}
	if target.Source == SourceBilibili {
		feed.ContentTypes = []string{"DRAW"}
	}
	return feed
}

type authorSubscriptionInput struct {
	Source   Source `json:"source"`
	Author   string `json:"author"`
	PostID   string `json:"postId"`
	FeedID   string `json:"feedId"`
	TimeZone string `json:"timeZone"`
}

type authorSubscriptionResponse struct {
	Available  bool          `json:"available"`
	Subscribed bool          `json:"subscribed"`
	Feed       *apiV1Feed    `json:"feed,omitempty"`
	Task       *syncTaskView `json:"task,omitempty"`
	Message    string        `json:"message,omitempty"`
}

func authorSubscriptionIndex(feeds []SourceConfig, target authorSubscriptionTarget) int {
	for i, feed := range feeds {
		if feed.Source == target.Source && sourceAuthorIDFromFeedID(feed.Source, feed.ID) == target.ID {
			return i
		}
	}
	return -1
}

func (b *BilibiliStore) authorSubscriptionList(source Source) *[]SourceConfig {
	switch source {
	case SourceBilibili:
		return &b.config.Subscriptions
	case SourceWeibo:
		return &b.config.WeiboSubscriptions
	case SourcePixiv:
		return &b.config.PixivSubscriptions
	case SourceTwitter:
		return &b.config.TwitterSubscriptions
	}
	return nil
}

func (b *BilibiliStore) authorSubscriptionCredentialError(source Source) string {
	switch source {
	case SourceBilibili:
		if b.config.Credentials.SESSDATA == "" {
			return "请先连接哔哩哔哩账号"
		}
	case SourcePixiv:
		if b.config.Pixiv.Cookie == "" || b.config.Pixiv.UserID == "" {
			return "请先连接 Pixiv 账号"
		}
	case SourceTwitter:
		if b.config.Twitter.APIKey == "" || b.config.Twitter.UserName == "" {
			return "请先连接推特账号"
		}
	}
	return ""
}

func subscribedAuthorResponse(feed SourceConfig) authorSubscriptionResponse {
	view := toAPIV1Feed(feed)
	return authorSubscriptionResponse{Available: true, Subscribed: true, Feed: &view}
}

func (b *BilibiliStore) authorSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	at := time.Now()
	var input authorSubscriptionInput
	switch r.Method {
	case http.MethodGet, http.MethodDelete:
		query := r.URL.Query()
		input = authorSubscriptionInput{Source: Source(query.Get("source")), Author: query.Get("author"), PostID: query.Get("postId"), FeedID: query.Get("feedId")}
	case http.MethodPost:
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "作者订阅信息无效")
			return
		}
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.authorSubscriptionList(input.Source) == nil || strings.TrimSpace(input.Author) == "" || len(input.Author) > 512 || len(input.PostID) > 128 || len(input.FeedID) > 128 {
		writeAPIError(w, http.StatusBadRequest, "作者订阅信息无效")
		return
	}
	var schedule string
	if r.Method == http.MethodPost {
		var err error
		schedule, err = weeklyAuthorSchedule(at, input.TimeZone)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	target, found := b.resolveAuthorSubscription(input.Source, input.Author, input.PostID, input.FeedID)
	if !found {
		legacy := input.FeedID == "" && b.canResolveLegacyAuthorSubscription(input.Source, input.Author, input.PostID)
		verifyExisting := r.Method == http.MethodGet && (input.Source == SourceBilibili || input.Source == SourcePixiv) && b.hasPossibleLegacyAuthorSubscription(input)
		if legacy && (r.Method == http.MethodPost || verifyExisting) {
			target, found = b.resolveLegacyAuthorSubscription(input.Source, input.Author, input.PostID)
		}
		if !found {
			if r.Method == http.MethodGet {
				writeJSON(w, authorSubscriptionResponse{Available: legacy})
				return
			}
			writeAPIError(w, http.StatusUnprocessableEntity, "无法确认作者的账号，请前往订阅平台按 UID 或用户名添加")
			return
		}
	}
	if r.Method == http.MethodGet {
		b.RLock()
		feeds := *b.authorSubscriptionList(target.Source)
		index := authorSubscriptionIndex(feeds, target)
		var response = authorSubscriptionResponse{Available: true}
		if index >= 0 {
			response = subscribedAuthorResponse(feeds[index])
		}
		b.RUnlock()
		writeJSON(w, response)
		return
	}
	response, status := b.changeAuthorSubscription(target, r.Method == http.MethodDelete, at, schedule, input.TimeZone)
	if status >= 400 {
		writeAPIError(w, status, response.Message)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	writeJSON(w, response)
}

func (b *BilibiliStore) hasPossibleLegacyAuthorSubscription(input authorSubscriptionInput) bool {
	if input.PostID == "" || input.FeedID != "" {
		return false
	}
	b.RLock()
	defer b.RUnlock()
	for _, feed := range *b.authorSubscriptionList(input.Source) {
		if feed.Source == input.Source && strings.TrimSpace(feed.Name) == strings.TrimSpace(input.Author) && sourceAuthorIDFromFeedID(feed.Source, feed.ID) != "" {
			return true
		}
	}
	return false
}

func (b *BilibiliStore) changeAuthorSubscription(target authorSubscriptionTarget, remove bool, at time.Time, schedule, zone string) (authorSubscriptionResponse, int) {
	// Save and mutate in one transaction. A disk error must not leave a
	// scheduled subscription behind or restore an unrelated concurrent edit.
	b.saveMu.Lock()
	defer b.saveMu.Unlock()
	b.Lock()
	defer b.Unlock()
	list := b.authorSubscriptionList(target.Source)
	previous := *list
	index := authorSubscriptionIndex(previous, target)
	response := authorSubscriptionResponse{Available: true}
	var feed SourceConfig
	if remove {
		response.Message = "已取消订阅，已归档动态保留"
		if index < 0 {
			return response, http.StatusOK
		}
		*list = append(append([]SourceConfig(nil), previous[:index]...), previous[index+1:]...)
	} else {
		if index >= 0 {
			return subscribedAuthorResponse(previous[index]), http.StatusOK
		}
		if message := b.authorSubscriptionCredentialError(target.Source); message != "" {
			return authorSubscriptionResponse{Message: message}, http.StatusPreconditionFailed
		}
		feed = newAuthorSubscription(target, at, schedule, zone)
		if b.syncTasks.hasActiveAuthor(target) {
			return authorSubscriptionResponse{Message: "该作者旧的同步任务仍在执行，请稍后重新订阅"}, http.StatusConflict
		}
		*list = append(append([]SourceConfig(nil), previous...), feed)
	}
	data, err := encryptJSON(b.key, b.config)
	if err == nil {
		err = atomicWriteFile(bilibiliFile, data, 0600)
	}
	if err != nil {
		*list = previous
		return authorSubscriptionResponse{Message: "无法保存作者订阅设置"}, http.StatusInternalServerError
	}
	if remove {
		return response, http.StatusOK
	}
	task, _ := b.enqueueSourceSync(feed, false)
	view := b.syncTasks.view(task)
	view.Source = nil
	response = subscribedAuthorResponse(feed)
	response.Task = &view
	response.Message = "已订阅，正在后台拉取最近三天的动态"
	return response, http.StatusAccepted
}
