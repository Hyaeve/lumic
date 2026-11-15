package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const telegramAPI = "https://api.telegram.org"

type TGBotConfig struct {
	Enabled       bool   `json:"enabled"`
	Token         string `json:"token,omitempty"`
	ChatID        string `json:"chatId"`
	Notifications bool   `json:"notifications"`
	TimeZone      string `json:"timeZone"`
}

type tgBotView struct {
	Enabled         bool   `json:"enabled"`
	TokenConfigured bool   `json:"tokenConfigured"`
	ChatID          string `json:"chatId"`
	Notifications   bool   `json:"notifications"`
	TimeZone        string `json:"timeZone"`
	Username        string `json:"username,omitempty"`
	Running         bool   `json:"running"`
	Error           string `json:"error,omitempty"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type tgMessage struct {
	MessageID int64  `json:"message_id"`
	Chat      tgChat `json:"chat"`
	Text      string `json:"text"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	Data    string     `json:"data"`
	Message *tgMessage `json:"message"`
	From    struct {
		ID int64 `json:"id"`
	} `json:"from"`
}

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type tgInlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

type tgKeyboard [][]tgInlineButton

type tgBot struct {
	store         *BilibiliStore
	config        TGBotConfig
	proxyURL      string
	client        *http.Client
	apiBase       string
	commands      tgCommandState
	notifications chan tgSyncNotification
	cancel        context.CancelFunc
	done          chan struct{}
	statusMu      sync.Mutex
	username      string
	running       bool
	lastError     string
	managed       bool
}

type tgAPIError struct {
	code        int
	retryAfter  time.Duration
	notModified bool
}

func (e tgAPIError) Error() string {
	switch e.code {
	case 401:
		return "Telegram Bot Token 无效"
	case 403:
		return "Telegram 无权向该会话发送消息，请检查 Chat ID 或机器人是否被屏蔽"
	case 409:
		return "Telegram 更新通道冲突，请停用该机器人的其他轮询或 Webhook"
	case 429:
		return "Telegram 请求过于频繁，请稍后重试"
	default:
		return "Telegram 请求失败，请检查机器人配置与会话权限"
	}
}

func (bot *tgBot) currentGeneration() bool {
	if !bot.managed || bot.store == nil {
		return true
	}
	bot.store.RLock()
	defer bot.store.RUnlock()
	return bot.store.config.Telegram == bot.config && bot.store.config.ProxyURL == bot.proxyURL
}

func (bot *tgBot) request(ctx context.Context, method, contentType string, body []byte, target any) error {
	for attempt := 0; attempt < 2; attempt++ {
		if !bot.currentGeneration() {
			return context.Canceled
		}
		err := bot.requestOnce(ctx, method, contentType, body, target)
		var response tgAPIError
		if attempt == 0 && errors.As(err, &response) && response.code == 429 {
			if !tgWait(ctx, response.retryAfter) {
				return ctx.Err()
			}
			continue
		}
		return err
	}
	return errors.New("Telegram 请求失败")
}

func (bot *tgBot) requestOnce(ctx context.Context, method, contentType string, body []byte, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, bot.apiBase+"/bot"+bot.config.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New("Telegram 请求配置无效")
	}
	request.Header.Set("Content-Type", contentType)
	response, err := bot.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// net/http errors include the request URL, which contains the bot token.
		return errors.New("Telegram 连接失败，请检查项目代理和容器网络")
	}
	defer response.Body.Close()
	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope) != nil {
		return errors.New("Telegram 响应格式无效")
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		code := envelope.ErrorCode
		if code == 0 {
			code = response.StatusCode
		}
		retry := time.Duration(envelope.Parameters.RetryAfter) * time.Second
		if retry < time.Second {
			retry = time.Second
		}
		if retry > 5*time.Minute {
			retry = 5 * time.Minute
		}
		return tgAPIError{code: code, retryAfter: retry, notModified: code == 400 && strings.HasPrefix(envelope.Description, "Bad Request: message is not modified")}
	}
	if target != nil && json.Unmarshal(envelope.Result, target) != nil {
		return errors.New("Telegram 响应内容无效")
	}
	return nil
}

func (bot *tgBot) call(ctx context.Context, method string, input any, target any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return errors.New("Telegram 消息数据无效")
	}
	return bot.request(ctx, method, "application/json", body, target)
}

func (bot *tgBot) sendText(ctx context.Context, chatID int64, text string, keyboard tgKeyboard) (int64, error) {
	if keyboard == nil {
		keyboard = tgKeyboard{}
	}
	var message tgMessage
	err := bot.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text, "reply_markup": map[string]any{"inline_keyboard": keyboard}, "link_preview_options": map[string]bool{"is_disabled": true}}, &message)
	return message.MessageID, err
}

func (bot *tgBot) editText(ctx context.Context, chatID, messageID int64, text string, keyboard tgKeyboard) error {
	if keyboard == nil {
		keyboard = tgKeyboard{}
	}
	err := bot.call(ctx, "editMessageText", map[string]any{"chat_id": chatID, "message_id": messageID, "text": text, "reply_markup": map[string]any{"inline_keyboard": keyboard}, "link_preview_options": map[string]bool{"is_disabled": true}}, nil)
	var response tgAPIError
	if errors.As(err, &response) && response.notModified {
		return nil
	}
	return err
}

func (bot *tgBot) deleteMessage(ctx context.Context, chatID, messageID int64) error {
	return bot.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}

func (bot *tgBot) answerCallback(ctx context.Context, id, text string) error {
	return bot.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

func validTGChatID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0, errors.New("Chat ID 必须为正整数私人会话编号")
	}
	return id, nil
}

func validTGToken(value string) bool {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[1]) < 20 || len(value) > 160 {
		return false
	}
	if _, err := strconv.ParseUint(parts[0], 10, 64); err != nil {
		return false
	}
	for _, character := range parts[1] {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func (bot *tgBot) authorized(update tgUpdate) bool {
	if !bot.currentGeneration() {
		return false
	}
	id, err := validTGChatID(bot.config.ChatID)
	if err != nil {
		return false
	}
	if callback := update.CallbackQuery; callback != nil {
		return callback.Message != nil && callback.Message.Chat.ID == id && callback.Message.Chat.Type == "private" && callback.From.ID == id
	}
	return update.Message != nil && update.Message.Chat.ID == id && update.Message.Chat.Type == "private"
}

func (bot *tgBot) setStatus(running bool, message string) {
	bot.statusMu.Lock()
	bot.running, bot.lastError = running, message
	bot.statusMu.Unlock()
}

func (bot *tgBot) poll(ctx context.Context) {
	defer func() {
		if ctx.Err() != nil {
			bot.setStatus(false, "")
		}
	}()
	var identity struct {
		Username string `json:"username"`
	}
	if err := bot.call(ctx, "getMe", map[string]any{}, &identity); err != nil {
		bot.setStatus(false, err.Error())
		tgWait(ctx, 5*time.Second)
		return
	}
	bot.statusMu.Lock()
	bot.username = identity.Username
	bot.statusMu.Unlock()
	_ = bot.call(ctx, "setMyCommands", map[string]any{"commands": []map[string]string{
		{"command": "start", "description": "Lumic 简介与命令"}, {"command": "sh", "description": "搜索作者"},
		{"command": "subscriptions", "description": "查看订阅"}, {"command": "update", "description": "更新指定订阅"},
		{"command": "unsubscribe", "description": "取消指定订阅"},
	}}, nil)
	var offset int64
	for ctx.Err() == nil {
		var updates []tgUpdate
		err := bot.call(ctx, "getUpdates", map[string]any{"offset": -1, "timeout": 0, "allowed_updates": []string{"message", "callback_query"}}, &updates)
		if err == nil {
			for _, update := range updates {
				if update.UpdateID >= offset {
					offset = update.UpdateID + 1
				}
			}
			break
		}
		bot.setStatus(false, err.Error())
		if !tgWait(ctx, tgRetryDelay(err)) {
			return
		}
	}
	for ctx.Err() == nil {
		var updates []tgUpdate
		err := bot.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 25, "limit": 20, "allowed_updates": []string{"message", "callback_query"}}, &updates)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			bot.setStatus(false, err.Error())
			if !tgWait(ctx, tgRetryDelay(err)) {
				return
			}
			continue
		}
		bot.setStatus(true, "")
		for _, update := range updates {
			if update.UpdateID < offset {
				continue
			}
			offset = update.UpdateID + 1
			if !bot.authorized(update) || ctx.Err() != nil {
				continue
			}
			bot.handleUpdate(ctx, update)
		}
		if len(updates) == 0 && !tgWait(ctx, 200*time.Millisecond) {
			return
		}
	}
}

func tgRetryDelay(err error) time.Duration {
	var apiError tgAPIError
	if errors.As(err, &apiError) && apiError.code == 429 {
		return apiError.retryAfter
	}
	return 5 * time.Second
}

func tgWait(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (bot *tgBot) run(ctx context.Context) {
	defer close(bot.done)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case notification := <-bot.notifications:
				if ctx.Err() == nil {
					if err := bot.sendSyncNotification(ctx, notification); err != nil && ctx.Err() == nil {
						log.Print("event=telegram_delivery_failed stage=notification")
					}
				}
			}
		}
	}()
	bot.poll(ctx)
	bot.cancel()
	workers.Wait()
}

func (b *BilibiliStore) tgView() tgBotView {
	b.RLock()
	config := b.config.Telegram
	b.RUnlock()
	view := tgBotView{Enabled: config.Enabled, TokenConfigured: config.Token != "", ChatID: config.ChatID, Notifications: config.Notifications, TimeZone: config.TimeZone}
	b.tgMu.Lock()
	bot := b.tgRuntime
	b.tgMu.Unlock()
	if bot != nil && bot.config == config {
		bot.statusMu.Lock()
		view.Username, view.Running, view.Error = bot.username, bot.running, bot.lastError
		bot.statusMu.Unlock()
	}
	return view
}

func (b *BilibiliStore) startTelegramBot() {
	b.tgManagerOnce.Do(func() {
		b.tgRefresh = make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		b.tgManagerCancel, b.tgManagerDone = cancel, make(chan struct{})
		go b.telegramManager(ctx)
	})
	select {
	case b.tgRefresh <- struct{}{}:
	default:
	}
}

func (b *BilibiliStore) telegramManager(ctx context.Context) {
	defer close(b.tgManagerDone)
	defer func() {
		b.tgMu.Lock()
		bot := b.tgRuntime
		b.tgRuntime = nil
		b.tgMu.Unlock()
		if bot != nil {
			bot.cancel()
			<-bot.done
		}
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-b.tgRefresh:
		}
		b.RLock()
		config, proxy := b.config.Telegram, b.config.ProxyURL
		b.RUnlock()
		b.tgMu.Lock()
		previous := b.tgRuntime
		if previous != nil && previous.config == config && previous.proxyURL == proxy {
			select {
			case <-previous.done:
			default:
				b.tgMu.Unlock()
				continue
			}
		}
		b.tgRuntime = nil
		b.tgMu.Unlock()
		if previous != nil {
			previous.cancel()
			<-previous.done
		}
		if ctx.Err() != nil {
			return
		}
		// A second save can arrive while an old slow command is shutting down.
		b.RLock()
		config, proxy = b.config.Telegram, b.config.ProxyURL
		b.RUnlock()
		if !config.Enabled || !validTGToken(config.Token) {
			continue
		}
		if _, err := validTGChatID(config.ChatID); err != nil {
			continue
		}
		client, err := externalHTTPClient(proxy)
		if err != nil {
			continue
		}
		copyClient := *client
		copyClient.Timeout = 40 * time.Second
		copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
		botContext, cancel := context.WithCancel(ctx)
		bot := &tgBot{store: b, config: config, proxyURL: proxy, client: &copyClient, apiBase: telegramAPI, notifications: make(chan tgSyncNotification, 32), done: make(chan struct{}), cancel: cancel, managed: true}
		b.tgMu.Lock()
		b.tgRuntime = bot
		b.tgMu.Unlock()
		go bot.run(botContext)
	}
}

func (b *BilibiliStore) telegramBotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		writeJSON(w, b.tgView())
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, PUT, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Enabled       bool   `json:"enabled"`
		Token         string `json:"token"`
		ClearToken    bool   `json:"clearToken"`
		ChatID        string `json:"chatId"`
		Notifications bool   `json:"notifications"`
		TimeZone      string `json:"timeZone"`
		Action        string `json:"action"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		writeAPIError(w, 400, "机器人配置格式无效")
		return
	}
	b.RLock()
	current, proxy := b.config.Telegram, b.config.ProxyURL
	b.RUnlock()
	config := TGBotConfig{Enabled: input.Enabled, Token: current.Token, ChatID: strings.TrimSpace(input.ChatID), Notifications: input.Notifications, TimeZone: strings.TrimSpace(input.TimeZone)}
	if input.ClearToken {
		config.Token = ""
	} else if strings.TrimSpace(input.Token) != "" {
		config.Token = strings.TrimSpace(input.Token)
	}
	if (config.Token != "" && !validTGToken(config.Token)) || len(config.TimeZone) > 100 {
		writeAPIError(w, 400, "Bot Token 或时区格式无效")
		return
	}
	if config.TimeZone != "" {
		if _, err := weeklyAuthorSchedule(time.Now(), config.TimeZone); err != nil {
			writeAPIError(w, 400, "机器人时区无效")
			return
		}
	}
	if config.ChatID != "" || config.Enabled || r.Method == http.MethodPost {
		if _, err := validTGChatID(config.ChatID); err != nil {
			writeAPIError(w, 400, err.Error())
			return
		}
	}
	if (config.Enabled || r.Method == http.MethodPost) && config.Token == "" {
		writeAPIError(w, 400, "请填写 Bot Token")
		return
	}
	if r.Method == http.MethodPost {
		if input.Action != "test" {
			writeAPIError(w, 400, "无效的机器人操作")
			return
		}
		client, err := externalHTTPClient(proxy)
		if err != nil {
			writeAPIError(w, 400, "项目代理配置无效")
			return
		}
		copyClient := *client
		copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
		bot := &tgBot{store: b, config: config, client: &copyClient, apiBase: telegramAPI}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var identity struct {
			Username string `json:"username"`
		}
		err = bot.call(ctx, "getMe", map[string]any{}, &identity)
		if err == nil {
			chatID, _ := validTGChatID(config.ChatID)
			_, err = bot.sendText(ctx, chatID, "Lumic 机器人连接测试成功。", nil)
		}
		if err != nil {
			writeAPIError(w, 502, err.Error())
			return
		}
		writeJSON(w, map[string]string{"message": "机器人测试成功", "username": identity.Username})
		return
	}
	b.saveMu.Lock()
	b.Lock()
	previous := b.config.Telegram
	if previous != current {
		b.Unlock()
		b.saveMu.Unlock()
		writeAPIError(w, 409, "机器人配置已更新，请刷新后重试")
		return
	}
	b.config.Telegram = config
	data, err := encryptJSON(b.key, b.config)
	if err == nil {
		err = atomicWriteFile(bilibiliFile, data, 0600)
	}
	if err != nil {
		b.config.Telegram = previous
	}
	b.Unlock()
	b.saveMu.Unlock()
	if err != nil {
		writeAPIError(w, 500, "无法保存机器人配置")
		return
	}
	b.startTelegramBot()
	writeJSON(w, b.tgView())
}
