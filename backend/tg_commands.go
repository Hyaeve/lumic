package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const tgFlowLifetime = 30 * time.Minute

var tgSearchPlatforms = []Source{SourceBilibili, SourceWeibo, SourcePixiv}

type tgAuthor struct {
	Source           Source
	ID, Name, Avatar string
}

type tgSearchState struct {
	Query     string
	Platforms []Source
	Page      int
	Results   map[Source][]tgAuthor
	Errors    map[Source]bool
}

type tgCommandState struct {
	mu           sync.Mutex
	messageID    int64
	mediaIDs     []int64
	nonce        string
	createdAt    time.Time
	search       *tgSearchState
	listSnapshot []SourceConfig
	listAt       time.Time
	choices      []SourceConfig
	action       string
	selected     *tgAuthor
	view         string
	confirmIndex int
	waitCancel   context.CancelFunc
}

func tgPlatformLabel(source Source) string {
	switch source {
	case SourceBilibili:
		return "哔哩哔哩"
	case SourceWeibo:
		return "微博"
	case SourcePixiv:
		return "Pixiv"
	case SourceTwitter:
		return "推特"
	default:
		return "其他"
	}
}

func tgCommand(text string) (string, string, []Source) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", "", nil
	}
	first := strings.SplitN(strings.ToLower(fields[0]), "@", 2)[0]
	argument := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))
	switch first {
	case "/start", "start":
		return "start", argument, nil
	case "/sh", "搜索":
		return "search", argument, tgSearchPlatforms
	case "哔哩哔哩搜索", "bilibili搜索":
		return "search", argument, []Source{SourceBilibili}
	case "微博搜索":
		return "search", argument, []Source{SourceWeibo}
	case "pixiv搜索":
		return "search", argument, []Source{SourcePixiv}
	case "/subscriptions", "查看订阅":
		return "subscriptions", argument, nil
	case "/update", "更新订阅":
		return "update", argument, nil
	case "/unsubscribe", "unsubscribe", "删除订阅", "取消订阅":
		return "unsubscribe", argument, nil
	}
	return "unknown", argument, nil
}

func tgNonce() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func (b *tgBot) button(text, action string, index int) tgInlineButton {
	return tgInlineButton{Text: tgClip(text, 64), Data: fmt.Sprintf("%s:%s:%d", b.commands.nonce, action, index)}
}

func (b *tgBot) clearFlowMedia(ctx context.Context, chatID int64) {
	for _, id := range b.commands.mediaIDs {
		_ = b.deleteMessage(ctx, chatID, id)
	}
	b.commands.mediaIDs = nil
}

func (b *tgBot) showFlow(ctx context.Context, chatID int64, text string, keyboard tgKeyboard) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	text = tgClip(text, 4096)
	if b.commands.messageID != 0 {
		if err := b.editText(ctx, chatID, b.commands.messageID, text, keyboard); err == nil {
			return nil
		}
		// Remove the no-longer-editable flow before replacing it, without effects.
		_ = b.deleteMessage(ctx, chatID, b.commands.messageID)
		b.commands.messageID = 0
	}
	id, err := b.sendText(ctx, chatID, text, keyboard)
	if err == nil {
		b.commands.messageID = id
	}
	return err
}

func (b *tgBot) handleUpdate(ctx context.Context, update tgUpdate) {
	if ctx.Err() != nil || !b.currentGeneration() {
		return
	}
	var message *tgMessage
	if update.CallbackQuery != nil {
		message = update.CallbackQuery.Message
	} else {
		message = update.Message
	}
	if message == nil || message.Chat.Type != "private" || strconv.FormatInt(message.Chat.ID, 10) != b.config.ChatID {
		return
	}
	b.commands.mu.Lock()
	defer b.commands.mu.Unlock()
	if ctx.Err() != nil || !b.currentGeneration() {
		return
	}
	chatID := message.Chat.ID
	if callback := update.CallbackQuery; callback != nil {
		if callback.From.ID != chatID || callback.Message.MessageID != b.commands.messageID || time.Since(b.commands.createdAt) > tgFlowLifetime {
			_ = b.answerCallback(ctx, callback.ID, "操作已过期，请重新发送命令")
			return
		}
		parts := strings.Split(callback.Data, ":")
		if len(parts) != 3 || parts[0] != b.commands.nonce {
			_ = b.answerCallback(ctx, callback.ID, "操作已过期，请重新发送命令")
			return
		}
		if !tgCallbackAllowed(b.commands.view, parts[1]) {
			_ = b.answerCallback(ctx, callback.ID, "操作已过期，请重新发送命令")
			return
		}
		index, err := strconv.Atoi(parts[2])
		if err != nil || !b.callbackIndexAllowed(parts[1], index) {
			_ = b.answerCallback(ctx, callback.ID, "无效操作")
			return
		}
		_ = b.answerCallback(ctx, callback.ID, "")
		if b.commands.waitCancel != nil {
			b.commands.waitCancel()
			b.commands.waitCancel = nil
		}
		// A button belongs to exactly one view, not the entire conversation.
		b.commands.nonce, b.commands.createdAt = tgNonce(), time.Now()
		b.handleCallback(ctx, chatID, parts[1], index)
		return
	}
	action, argument, platforms := tgCommand(message.Text)
	if b.commands.waitCancel != nil {
		b.commands.waitCancel()
		b.commands.waitCancel = nil
	}
	b.clearFlowMedia(ctx, chatID)
	b.commands.nonce, b.commands.createdAt = tgNonce(), time.Now()
	b.commands.search, b.commands.selected, b.commands.choices = nil, nil, nil
	b.commands.action, b.commands.confirmIndex = "", -1
	b.commands.view = "result"
	switch action {
	case "start":
		_ = b.showFlow(ctx, chatID, "Lumic · 拾光\n聚合、订阅与归档喜欢的图文。\n\n/sh 关键词 · 搜索作者\n搜索 关键词 · 搜索三个平台\n哔哩哔哩搜索 / 微博搜索 / pixiv搜索 + 关键词\n/subscriptions · 查看订阅（每页七条）\n/update 完整作者名或序号 · 更新订阅\n/unsubscribe 完整作者名或序号 · 取消订阅\n\n取消订阅只移除计划，已下载图文会保留。", nil)
	case "search":
		if argument == "" || len([]rune(argument)) > 40 {
			_ = b.showFlow(ctx, chatID, "请输入搜索关键词，例如：搜索 六月夏天（最多40字）", nil)
			return
		}
		b.commands.search = &tgSearchState{Query: argument, Platforms: append([]Source(nil), platforms...), Results: make(map[Source][]tgAuthor), Errors: make(map[Source]bool)}
		_ = b.showFlow(ctx, chatID, "正在搜索作者…", nil)
		b.showSearch(ctx, chatID, 0)
	case "subscriptions":
		b.commands.listSnapshot, b.commands.listAt = b.store.allSourceConfigs(), time.Now()
		b.showSubscriptions(ctx, chatID, 0)
	case "update", "unsubscribe":
		b.startSubscriptionAction(ctx, chatID, action, argument)
	default:
		_ = b.showFlow(ctx, chatID, "暂不支持这条命令。发送 /start 查看可用命令。", nil)
	}
}

func (b *tgBot) showSearch(ctx context.Context, chatID int64, page int) {
	s := b.commands.search
	if s == nil || page < 0 || page >= len(s.Platforms) {
		return
	}
	b.clearFlowMedia(ctx, chatID)
	b.commands.view = "search"
	s.Page = page
	source := s.Platforms[page]
	if _, loaded := s.Results[source]; !loaded {
		users, err := b.store.searchTelegramAuthors(ctx, source, s.Query)
		if len(users) > 5 {
			users = users[:5]
		}
		s.Results[source], s.Errors[source] = users, err != nil
	}
	text := fmt.Sprintf("搜索：%s\n%s · %d / %d", s.Query, tgPlatformLabel(source), page+1, len(s.Platforms))
	keyboard := tgKeyboard{}
	for index, user := range s.Results[source] {
		keyboard = append(keyboard, []tgInlineButton{b.button(user.Name, "author", index)})
	}
	if len(keyboard) == 0 {
		if s.Errors[source] {
			text += "\n当前平台搜索失败，请检查平台连接后重新搜索。"
		} else {
			text += "\n没有找到作者。"
		}
	}
	navigation := []tgInlineButton{}
	if page > 0 {
		navigation = append(navigation, b.button("上一平台", "search", page-1))
	}
	if page+1 < len(s.Platforms) {
		navigation = append(navigation, b.button("下一平台", "search", page+1))
	}
	if len(navigation) > 0 {
		keyboard = append(keyboard, navigation)
	}
	_ = b.showFlow(ctx, chatID, text, keyboard)
}

func (b *tgBot) showSubscriptions(ctx context.Context, chatID int64, page int) {
	feeds := b.commands.listSnapshot
	pages := (len(feeds) + 6) / 7
	if pages == 0 {
		pages = 1
	}
	if page < 0 || page >= pages {
		return
	}
	b.commands.view = "list"
	var text strings.Builder
	fmt.Fprintf(&text, "订阅列表 · %d / %d", page+1, pages)
	for index := page * 7; index < len(feeds) && index < (page+1)*7; index++ {
		fmt.Fprintf(&text, "\n%d. [%s] %s", index+1, tgPlatformLabel(feeds[index].Source), feeds[index].Name)
	}
	if len(feeds) == 0 {
		text.WriteString("\n暂无订阅。")
	}
	row := []tgInlineButton{}
	if page > 0 {
		row = append(row, b.button("上一页", "subscriptions", page-1))
	}
	if page+1 < pages {
		row = append(row, b.button("下一页", "subscriptions", page+1))
	}
	keyboard := tgKeyboard{}
	if len(row) > 0 {
		keyboard = append(keyboard, row)
	}
	_ = b.showFlow(ctx, chatID, text.String(), keyboard)
}

func (b *tgBot) startSubscriptionAction(ctx context.Context, chatID int64, action, argument string) {
	if argument == "" {
		_ = b.showFlow(ctx, chatID, "请在命令后空格填写完整作者名，或填写最近一次订阅列表中的序号。", nil)
		return
	}
	var choices []SourceConfig
	if number, err := strconv.Atoi(argument); err == nil {
		if time.Since(b.commands.listAt) > tgFlowLifetime || number < 1 || number > len(b.commands.listSnapshot) {
			_ = b.showFlow(ctx, chatID, "序号无效或列表已过期，请先发送 /subscriptions。", nil)
			return
		}
		selected := b.commands.listSnapshot[number-1]
		if current, ok := b.store.findSyncSource(selected.ID, selected.Source); ok && sameAuthorSyncGeneration(current, selected) {
			choices = append(choices, current)
		}
	} else {
		for _, feed := range b.store.allSourceConfigs() {
			if feed.Name == argument {
				choices = append(choices, feed)
			}
		}
	}
	if len(choices) == 0 {
		_ = b.showFlow(ctx, chatID, "未找到该订阅，请使用完整作者名或重新查看订阅列表。", nil)
		return
	}
	b.commands.choices, b.commands.action = choices, action
	if len(choices) == 1 {
		b.chooseSubscription(ctx, chatID, 0)
		return
	}
	keyboard := tgKeyboard{}
	for index, feed := range choices {
		keyboard = append(keyboard, []tgInlineButton{b.button("["+tgPlatformLabel(feed.Source)+"] "+feed.Name+" · "+feed.ID, "choose", index)})
	}
	b.commands.view = "choice"
	_ = b.showFlow(ctx, chatID, "存在多个同名订阅，请选择平台：", keyboard)
}

func (b *tgBot) chooseSubscription(ctx context.Context, chatID int64, index int) {
	if b.commands.action != "update" && b.commands.action != "unsubscribe" {
		return
	}
	if index < 0 || index >= len(b.commands.choices) {
		return
	}
	feed := b.commands.choices[index]
	current, exists := b.store.findSyncSource(feed.ID, feed.Source)
	if !exists || !sameAuthorSyncGeneration(current, feed) {
		_ = b.showFlow(ctx, chatID, "该订阅已更改，请重新发送命令。", nil)
		return
	}
	if b.commands.action == "unsubscribe" {
		b.commands.view = "confirm"
		b.commands.confirmIndex = index
		keyboard := tgKeyboard{{b.button("确认取消订阅", "confirm", index), b.button("保留订阅", "cancel", index)}}
		_ = b.showFlow(ctx, chatID, "取消订阅“"+feed.Name+"”？\n只移除订阅计划，已下载图文会保留。", keyboard)
		return
	}
	if ctx.Err() != nil || !b.currentGeneration() {
		return
	}
	b.commands.action = ""
	b.commands.view = "pending"
	task, compatible := b.store.enqueueSourceSync(current, false)
	if !compatible {
		_ = b.showFlow(ctx, chatID, "该订阅正在执行其他同步，请稍后重试。", nil)
		return
	}
	view := b.store.syncTasks.view(task)
	_ = b.showFlow(ctx, chatID, "["+tgPlatformLabel(feed.Source)+"] "+feed.Name+"\n已进入更新队列（"+view.Status+"）。", nil)
	b.watchTask(ctx, chatID, task, b.commands.nonce)
}

func (b *tgBot) watchTask(ctx context.Context, chatID int64, task *syncTask, nonce string) {
	if b.commands.waitCancel != nil {
		b.commands.waitCancel()
	}
	waitContext, cancel := context.WithCancel(ctx)
	b.commands.waitCancel = cancel
	go func() {
		defer cancel()
		result, done := task.wait(waitContext)
		if !done {
			return
		}
		b.commands.mu.Lock()
		defer b.commands.mu.Unlock()
		if waitContext.Err() != nil || !b.currentGeneration() || b.commands.nonce != nonce {
			return
		}
		b.commands.view = "result"
		text := "同步完成。"
		if result.err != nil {
			text = "更新失败，请检查平台凭证、访问权限或网络。"
		} else {
			text = fmt.Sprintf("“%s”更新完成，新增 %d 条动态。", result.source.Name, result.added)
		}
		keyboard := tgKeyboard{}
		if b.commands.search != nil {
			keyboard = append(keyboard, []tgInlineButton{b.button("返回搜索结果", "back", 0)})
		}
		_ = b.showFlow(ctx, chatID, text, keyboard)
	}()
}

func (b *tgBot) handleCallback(ctx context.Context, chatID int64, action string, index int) {
	switch action {
	case "search":
		b.showSearch(ctx, chatID, index)
	case "subscriptions":
		b.showSubscriptions(ctx, chatID, index)
	case "author":
		s := b.commands.search
		if s == nil {
			return
		}
		users := s.Results[s.Platforms[s.Page]]
		if index < 0 || index >= len(users) {
			return
		}
		b.commands.selected = &users[index]
		b.showAuthor(ctx, chatID, users[index])
	case "back":
		if b.commands.search != nil {
			b.showSearch(ctx, chatID, b.commands.search.Page)
		}
	case "subscribe":
		if b.commands.selected == nil {
			return
		}
		author := *b.commands.selected
		at := time.Now()
		schedule, err := weeklyAuthorSchedule(at, b.config.TimeZone)
		if err != nil {
			_ = b.showFlow(ctx, chatID, "订阅计划时区无效，请在设置中重新保存。", nil)
			return
		}
		if ctx.Err() != nil || !b.currentGeneration() {
			return
		}
		response, status := b.store.changeAuthorSubscription(authorSubscriptionTarget{Source: author.Source, ID: author.ID, Name: author.Name, Avatar: author.Avatar}, false, at, schedule, b.config.TimeZone)
		b.commands.view = "result"
		if status >= 400 {
			_ = b.showFlow(ctx, chatID, response.Message, tgKeyboard{{b.button("返回搜索结果", "back", 0)}})
			return
		}
		_ = b.showFlow(ctx, chatID, "已订阅“"+author.Name+"”。\n每周计划："+response.Feed.Schedule+"\n最近三天动态将在后台拉取。", tgKeyboard{{b.button("返回搜索结果", "back", 0)}})
		if response.Task != nil {
			b.store.syncTasks.mu.Lock()
			task := b.store.syncTasks.byID[response.Task.ID]
			b.store.syncTasks.mu.Unlock()
			if task != nil {
				b.watchTask(ctx, chatID, task, b.commands.nonce)
			}
		}
	case "choose":
		b.chooseSubscription(ctx, chatID, index)
	case "cancel":
		b.commands.action = ""
		b.commands.view = "result"
		_ = b.showFlow(ctx, chatID, "已保留订阅。", nil)
	case "confirm":
		if b.commands.action != "unsubscribe" || index < 0 || index >= len(b.commands.choices) {
			return
		}
		feed := b.commands.choices[index]
		if ctx.Err() != nil || !b.currentGeneration() {
			return
		}
		if err := b.store.removeTelegramSubscription(feed); err != nil {
			b.commands.view = "result"
			_ = b.showFlow(ctx, chatID, "取消订阅失败，请重新查看订阅后重试。", nil)
			return
		}
		b.commands.action = ""
		b.commands.view = "result"
		_ = b.showFlow(ctx, chatID, "已取消订阅“"+feed.Name+"”，已下载图文保留。", nil)
	}
}

func (b *tgBot) showAuthor(ctx context.Context, chatID int64, author tgAuthor) {
	b.clearFlowMedia(ctx, chatID)
	b.commands.view = "preview"
	text := "[" + tgPlatformLabel(author.Source) + "] " + author.Name
	photos := []string{}
	if author.Avatar != "" {
		photos = append(photos, author.Avatar)
	}
	post, err := b.store.telegramAuthorPreview(ctx, author)
	if ctx.Err() != nil {
		return
	}
	if err == nil {
		photos = append(photos, post.Media...)
		if len(photos) > 10 {
			photos = photos[:10]
		}
		if post.Caption != "" {
			text += "\n\n" + tgClip(post.Caption, 2200)
		}
	} else {
		text += "\n\n最新动态预览暂不可用，仍可订阅后后台拉取。"
	}
	if len(photos) > 0 {
		var photoErr error
		b.commands.mediaIDs, photoErr = b.sendPhotos(ctx, chatID, photos, "")
		if photoErr != nil {
			text += "\n\n部分图片预览暂不可用。"
		}
	}
	keyboard := tgKeyboard{{b.button("返回搜索结果", "back", 0), b.button("订阅", "subscribe", 0)}}
	_ = b.showFlow(ctx, chatID, text, keyboard)
}

func tgClip(text string, max int) string {
	units := 0
	for index, r := range text {
		units++
		if r > 0xffff {
			units++
		}
		if units > max-1 {
			return text[:index] + "…"
		}
	}
	return text
}

func tgCallbackAllowed(view, action string) bool {
	switch view {
	case "search":
		return action == "search" || action == "author"
	case "list":
		return action == "subscriptions"
	case "preview":
		return action == "back" || action == "subscribe"
	case "choice":
		return action == "choose"
	case "confirm":
		return action == "confirm" || action == "cancel"
	case "result":
		return action == "back"
	}
	return false
}

func (b *tgBot) callbackIndexAllowed(action string, index int) bool {
	s := b.commands.search
	switch action {
	case "search":
		return s != nil && index >= 0 && index < len(s.Platforms)
	case "author":
		return s != nil && s.Page >= 0 && s.Page < len(s.Platforms) && index >= 0 && index < len(s.Results[s.Platforms[s.Page]])
	case "subscriptions":
		pages := (len(b.commands.listSnapshot) + 6) / 7
		return index == 0 || index > 0 && index < pages
	case "choose":
		return (b.commands.action == "update" || b.commands.action == "unsubscribe") && index >= 0 && index < len(b.commands.choices)
	case "confirm", "cancel":
		return b.commands.action == "unsubscribe" && index == b.commands.confirmIndex && index >= 0 && index < len(b.commands.choices)
	case "subscribe":
		return index == 0 && b.commands.selected != nil
	case "back":
		return index == 0 && s != nil
	}
	return false
}

func (b *BilibiliStore) removeTelegramSubscription(feed SourceConfig) error {
	b.saveMu.Lock()
	defer b.saveMu.Unlock()
	b.Lock()
	defer b.Unlock()
	list := b.authorSubscriptionList(feed.Source)
	if list == nil {
		return errors.New("unsupported source")
	}
	for index, current := range *list {
		if current.ID != feed.ID || !sameAuthorSyncGeneration(current, feed) {
			continue
		}
		previous := *list
		*list = append(append([]SourceConfig(nil), previous[:index]...), previous[index+1:]...)
		data, err := encryptJSON(b.key, b.config)
		if err == nil {
			err = atomicWriteFile(bilibiliFile, data, 0600)
		}
		if err != nil {
			*list = previous
		}
		return err
	}
	return errors.New("subscription changed")
}

func (b *BilibiliStore) searchTelegramAuthors(ctx context.Context, source Source, keyword string) ([]tgAuthor, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	b.RLock()
	config := b.config
	b.RUnlock()
	users := []tgAuthor{}
	switch source {
	case SourceBilibili:
		if config.Credentials.SESSDATA == "" {
			return nil, errors.New("B站未连接")
		}
		var payload struct {
			Code int `json:"code"`
			Data struct {
				Result []struct {
					ID     json.Number `json:"mid"`
					Name   string      `json:"uname"`
					Avatar string      `json:"upic"`
				} `json:"result"`
			} `json:"data"`
		}
		endpoint := "https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&page=1&page_size=20&keyword=" + url.QueryEscape(keyword)
		if err := bilibiliRequest(endpoint, config.Credentials, config.ProxyURL, &payload); err != nil || payload.Code != 0 {
			return nil, errors.New("B站搜索失败")
		}
		for _, user := range payload.Data.Result {
			if id := normalizeSubscriptionAuthorID(source, user.ID.String()); id != "" {
				users = append(users, tgAuthor{source, id, plainBilibiliText(user.Name), normalizeRemoteImage(user.Avatar)})
			}
		}
	case SourceWeibo:
		if id, err := weiboAuthorID(keyword); err == nil && id != "" {
			user, err := b.fetchPublicWeiboUser(id, &config.Weibo, config.ProxyURL)
			if err != nil {
				return nil, errors.New("微博搜索失败")
			}
			return []tgAuthor{{source, user.UserID, user.Name, user.Avatar}}, nil
		}
		endpoint := "https://m.weibo.cn/api/container/getIndex?containerid=" + url.QueryEscape("100103type=3&q="+keyword) + "&page_type=searchall"
		payload, _, err := b.weiboPublicRequest(endpoint, &config.Weibo, config.ProxyURL)
		if err != nil {
			return nil, errors.New("微博搜索失败")
		}
		var found []WeiboUser
		collectWeiboUsers(payload, &found, make(map[string]bool))
		for _, user := range found {
			if id := normalizeSubscriptionAuthorID(source, user.UserID); id != "" {
				users = append(users, tgAuthor{source, id, user.Name, user.Avatar})
			}
		}
	case SourcePixiv:
		if config.Pixiv.Cookie == "" {
			return nil, errors.New("Pixiv未连接")
		}
		client, err := externalHTTPClient(config.ProxyURL)
		if err != nil {
			return nil, err
		}
		if pixivNumericID(keyword) {
			user, err := fetchPixivUserProfile(client, config.Pixiv, keyword)
			if err != nil {
				return nil, errors.New("Pixiv搜索失败")
			}
			return []tgAuthor{{source, keyword, user.Name, user.Avatar}}, nil
		}
		var payload map[string]any
		endpoint := "https://www.pixiv.net/ajax/search/users?" + url.Values{"nick": {keyword}, "s_mode": {"s_usr"}, "i": {"1"}, "p": {"1"}, "lang": {"zh"}}.Encode()
		if err := pixivRequestJSON(client, endpoint, config.Pixiv, &payload); err != nil || pixivPayloadFailed(payload) {
			return nil, errors.New("Pixiv搜索失败")
		}
		var ok bool
		users, ok = tgPixivSearchUsers(payload)
		if !ok {
			return nil, errors.New("Pixiv搜索响应格式不可用")
		}
		if len(users) > 5 {
			users = users[:5]
		}
		for index := range users {
			if users[index].Name != "" {
				continue
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			profile, err := fetchPixivUserProfile(client, config.Pixiv, users[index].ID)
			if err != nil {
				return nil, errors.New("Pixiv作者资料读取失败")
			}
			users[index].Name, users[index].Avatar = profile.Name, profile.Avatar
		}
	default:
		return nil, errors.New("unsupported source")
	}
	if len(users) > 5 {
		users = users[:5]
	}
	return users, nil
}

func tgPixivSearchUsers(payload map[string]any) ([]tgAuthor, bool) {
	body, ok := payload["body"].(map[string]any)
	if !ok {
		return nil, false
	}
	page, _ := body["page"].(map[string]any)
	ids, ordered := page["userIds"].([]any)
	var rows []any
	switch users := body["users"].(type) {
	case []any:
		rows = users
	case map[string]any:
		if !ordered {
			return nil, false
		}
		for _, id := range ids {
			if user, exists := users[jsonValueString(id)]; exists {
				rows = append(rows, user)
			}
		}
	default:
		previews, hasPreviews := body["userPreviews"].([]any)
		if !ordered && !hasPreviews {
			return nil, false
		}
		for _, preview := range previews {
			if item, ok := preview.(map[string]any); ok {
				rows = append(rows, item["user"])
			}
		}
	}
	seen := make(map[string]bool)
	result := []tgAuthor{}
	for _, row := range rows {
		user, ok := row.(map[string]any)
		if !ok {
			continue
		}
		id := normalizeSubscriptionAuthorID(SourcePixiv, firstNonEmptyRemoteText(jsonValueString(user["userId"]), jsonValueString(user["id"])))
		name := firstNonEmptyRemoteText(jsonValueString(user["userName"]), jsonValueString(user["name"]))
		if id == "" || name == "" || seen[id] {
			continue
		}
		avatar := firstNonEmptyRemoteText(jsonValueString(user["profileImageUrl"]), jsonValueString(user["imageBig"]), jsonValueString(user["image"]))
		result = append(result, tgAuthor{SourcePixiv, id, name, normalizeRemoteImage(avatar)})
		seen[id] = true
	}
	if ordered {
		byID := make(map[string]tgAuthor, len(result))
		for _, author := range result {
			byID[author.ID] = author
		}
		result, seen = []tgAuthor{}, map[string]bool{}
		for _, raw := range ids {
			id := normalizeSubscriptionAuthorID(SourcePixiv, jsonValueString(raw))
			if id == "" || seen[id] {
				continue
			}
			author, found := byID[id]
			if !found {
				// Some responses contain only ordered IDs and artwork thumbnails.
				// Resolve those IDs from the author profile, never artwork titles.
				author = tgAuthor{Source: SourcePixiv, ID: id}
			}
			result = append(result, author)
			seen[id] = true
		}
	}
	return result, true
}

func (b *BilibiliStore) telegramAuthorPreview(ctx context.Context, author tgAuthor) (Post, error) {
	var latest Post
	if b.content != nil {
		b.content.RLock()
		for _, post := range b.content.posts {
			if post.Source == author.Source && archivedPostAuthorID(post) == author.ID && (latest.ID == "" || post.Published.After(latest.Published)) {
				latest = post
			}
		}
		b.content.RUnlock()
	}
	cached := latest
	latest = Post{}
	if ctx.Err() != nil {
		return Post{}, ctx.Err()
	}
	feed := newAuthorSubscription(authorSubscriptionTarget{author.Source, author.ID, author.Name, author.Avatar}, time.Now(), "", "")
	feed.AuthorSync = nil
	var posts []Post
	var err error
	switch author.Source {
	case SourceBilibili:
		posts, err = b.fetchBilibiliPosts(feed, false)
	case SourceWeibo:
		posts, _, err = b.fetchWeiboPostsWithReport(feed, false)
	case SourcePixiv:
		b.RLock()
		credentials, proxyURL := b.config.Pixiv, b.config.ProxyURL
		b.RUnlock()
		client, clientErr := externalHTTPClient(proxyURL)
		if clientErr != nil {
			err = clientErr
			break
		}
		var payload map[string]any
		err = pixivRequestJSON(client, "https://www.pixiv.net/ajax/user/"+url.PathEscape(author.ID)+"/profile/all?lang=zh", credentials, &payload)
		if err != nil || pixivPayloadFailed(payload) {
			err = errors.New("Pixiv预览失败")
			break
		}
		ids := []string{}
		collectPixivIDs(payload["body"], &ids, map[string]bool{})
		sort.Slice(ids, func(i, j int) bool {
			left, _ := strconv.ParseUint(ids[i], 10, 64)
			right, _ := strconv.ParseUint(ids[j], 10, 64)
			return left > right
		})
		if len(ids) == 0 {
			err = errors.New("没有动态")
			break
		}
		var detail map[string]any
		if err = pixivRequestJSON(client, "https://www.pixiv.net/ajax/illust/"+ids[0]+"?lang=zh", credentials, &detail); err == nil {
			latest, err = pixivDetailPost(client, credentials, ids[0], detail, feed)
			posts = append(posts, latest)
		}
	}
	if ctx.Err() != nil {
		return Post{}, ctx.Err()
	}
	if err != nil {
		if cached.ID != "" {
			return withAvailablePostImages(cached), nil
		}
		return Post{}, err
	}
	for _, post := range posts {
		if latest.ID == "" || post.Published.After(latest.Published) {
			latest = post
		}
	}
	if latest.ID == "" {
		if cached.ID != "" {
			return withAvailablePostImages(cached), nil
		}
		return Post{}, errors.New("没有动态")
	}
	return latest, nil
}
