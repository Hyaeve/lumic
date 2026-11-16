package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type tgCommandCall struct {
	Method  string
	Payload map[string]any
}

func commandTestBot(t *testing.T, store *BilibiliStore) (*tgBot, func() []tgCommandCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []tgCommandCall
	var sequence int64 = 100
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("command fixture expected JSON")
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, tgCommandCall{method, input})
		result := any(true)
		if method == "sendMessage" {
			sequence++
			result = map[string]any{"message_id": sequence}
		}
		writeJSON(w, map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(server.Close)
	bot := &tgBot{store: store, config: TGBotConfig{ChatID: "42", AuthorizedUserID: "42", TimeZone: "Asia/Shanghai"}, client: server.Client(), apiBase: server.URL}
	return bot, func() []tgCommandCall { mu.Lock(); defer mu.Unlock(); return append([]tgCommandCall(nil), calls...) }
}

func commandMessage(text string) tgUpdate {
	return tgUpdate{Message: &tgMessage{MessageID: 7, Chat: tgChat{ID: 42, Type: "private"}, From: tgUser{ID: 42}, Text: text}}
}
func commandCallback(bot *tgBot, data string) tgUpdate {
	query := &tgCallbackQuery{ID: "fixture", Data: data, Message: &tgMessage{MessageID: bot.commands.messageID, Chat: tgChat{ID: 42, Type: "private"}}}
	query.From.ID = 42
	return tgUpdate{CallbackQuery: query}
}
func lastCommandText(calls []tgCommandCall) string {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == "sendMessage" || calls[i].Method == "editMessageText" {
			return fmt.Sprint(calls[i].Payload["text"])
		}
	}
	return ""
}
func commandButton(t *testing.T, calls []tgCommandCall, text string) string {
	t.Helper()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method != "sendMessage" && calls[i].Method != "editMessageText" {
			continue
		}
		markup, _ := calls[i].Payload["reply_markup"].(map[string]any)
		rows, _ := markup["inline_keyboard"].([]any)
		for _, row := range rows {
			for _, raw := range row.([]any) {
				button := raw.(map[string]any)
				if strings.Contains(fmt.Sprint(button["text"]), text) {
					return fmt.Sprint(button["callback_data"])
				}
			}
		}
		break
	}
	t.Fatalf("button %s missing", text)
	return ""
}

func TestTGCommandAliasesAndUnicodeArguments(t *testing.T) {
	for _, test := range []struct {
		input, action, arg string
		platforms          []Source
	}{
		{"/start", "start", "", nil}, {"/sh@LumicBot 六月 夏天", "search", "六月 夏天", tgSearchPlatforms},
		{"搜索 六月夏天", "search", "六月夏天", tgSearchPlatforms}, {"哔哩哔哩搜索 摄影", "search", "摄影", []Source{SourceBilibili}},
		{"bilibili搜索 摄影", "search", "摄影", []Source{SourceBilibili}}, {"pixiv搜索 绘画", "search", "绘画", []Source{SourcePixiv}},
		{"微博搜索 摄影", "search", "摄影", []Source{SourceWeibo}}, {"查看订阅", "subscriptions", "", nil},
		{"更新订阅 六月 夏天", "update", "六月 夏天", nil}, {"/update 7", "update", "7", nil},
		{"unsubscribe 六月夏天", "unsubscribe", "六月夏天", nil}, {"删除订阅 2", "unsubscribe", "2", nil}, {"取消订阅 作者", "unsubscribe", "作者", nil},
	} {
		action, arg, platforms := tgCommand(test.input)
		if action != test.action || arg != test.arg || !reflect.DeepEqual(platforms, test.platforms) {
			t.Fatalf("%s => %s %q %v", test.input, action, arg, platforms)
		}
	}
	if got := tgClip(strings.Repeat("星", 100), 64); telegramTextUnits(got) > 64 {
		t.Fatal("button text exceeds UTF16 limit")
	}
	if got := tgClip(strings.Repeat("\U0001F31F", 2200), 2200); telegramTextUnits(got) > 2200 {
		t.Fatal("astral caption clipping exceeds UTF16 limit")
	}
}

func TestTGSubscriptionsStableNumbersAndConfirmation(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{{ID: "keep", Source: SourceWeibo, Caption: "archive stays"}}}
	for i := 1; i <= 9; i++ {
		store.config.WeiboSubscriptions = append(store.config.WeiboSubscriptions, SourceConfig{ID: fmt.Sprintf("weibo-%d", i), Source: SourceWeibo, Name: fmt.Sprintf("作者%d", i)})
	}
	bot, calls := commandTestBot(t, store)
	ctx := context.Background()
	unauthorized := commandMessage("查看订阅")
	unauthorized.Message.Chat.ID = 99
	bot.handleUpdate(ctx, unauthorized)
	if len(calls()) != 0 {
		t.Fatal("unconfigured chat received command access")
	}
	bot.handleUpdate(ctx, commandMessage("/start"))
	rootID := bot.commands.messageID
	bot.handleUpdate(ctx, commandMessage("查看订阅"))
	if text := lastCommandText(calls()); !strings.Contains(text, "7. [微博] 作者7") || strings.Contains(text, "8. [微博]") {
		t.Fatal("subscription page is not seven items", text)
	}
	pageButton := commandButton(t, calls(), "下一页")
	bot.handleUpdate(ctx, commandCallback(bot, pageButton))
	if text := lastCommandText(calls()); !strings.Contains(text, "8. [微博] 作者8") || !strings.Contains(text, "9. [微博] 作者9") {
		t.Fatal("second page numbering changed", text)
	}
	if bot.commands.messageID != rootID {
		t.Fatal("pagination replaced the root message")
	}
	bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "上一页")))
	if bot.commands.messageID != rootID || !strings.Contains(lastCommandText(calls()), "1. [微博] 作者1") {
		t.Fatal("backward pagination did not edit the same message")
	}
	for _, call := range calls() {
		if call.Method == "deleteMessage" {
			t.Fatal("ordinary pagination deleted a message")
		}
	}
	store.Lock()
	store.config.WeiboSubscriptions = append([]SourceConfig{{ID: "weibo-99", Source: SourceWeibo, Name: "后来添加"}}, store.config.WeiboSubscriptions...)
	store.Unlock()
	bot.handleUpdate(ctx, commandMessage("取消订阅 2"))
	if !strings.Contains(lastCommandText(calls()), "作者2") {
		t.Fatal("number addressed a shifted list")
	}
	keep := commandButton(t, calls(), "保留订阅")
	bot.handleUpdate(ctx, commandCallback(bot, keep))
	if _, ok := store.findSyncSource("weibo-2", SourceWeibo); !ok {
		t.Fatal("cancel confirmation removed subscription")
	}
	bot.handleUpdate(ctx, commandMessage("删除订阅 2"))
	confirm := commandButton(t, calls(), "确认取消")
	bot.handleUpdate(ctx, commandCallback(bot, confirm))
	if _, ok := store.findSyncSource("weibo-2", SourceWeibo); ok {
		t.Fatal("confirmed source was not removed")
	}
	if _, ok := store.findSyncSource("weibo-99", SourceWeibo); !ok || len(store.content.posts) != 1 || store.content.posts[0].Caption != "archive stays" {
		t.Fatal("unsubscribe damaged unrelated source or archive")
	}
	before := len(calls())
	bot.handleUpdate(ctx, commandCallback(bot, confirm))
	if len(calls()) != before+1 || calls()[before].Method != "answerCallbackQuery" {
		t.Fatal("stale confirm was not rejected")
	}
}

func TestTGExactNameChoicesAndExpiredNumbers(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.Subscriptions = []SourceConfig{{ID: "bili-1", Source: SourceBilibili, Name: "六月夏天"}}
	store.config.WeiboSubscriptions = []SourceConfig{{ID: "weibo-2", Source: SourceWeibo, Name: "六月夏天"}, {ID: "weibo-likes-9", Source: SourceWeibo, Name: "我的点赞"}}
	bot, calls := commandTestBot(t, store)
	ctx := context.Background()
	bot.handleUpdate(ctx, commandMessage("/update 1"))
	if !strings.Contains(lastCommandText(calls()), "/subscriptions") {
		t.Fatal("number worked without listing")
	}
	bot.handleUpdate(ctx, commandMessage("/update 六月"))
	if !strings.Contains(lastCommandText(calls()), "未找到") {
		t.Fatal("partial author name matched")
	}
	bot.handleUpdate(ctx, commandMessage("unsubscribe 六月夏天"))
	if !strings.Contains(lastCommandText(calls()), "多个同名") {
		t.Fatal("duplicate author did not ask for platform")
	}
	choice := commandButton(t, calls(), "weibo-2")
	bot.handleUpdate(ctx, commandCallback(bot, choice))
	if !strings.Contains(lastCommandText(calls()), "取消订阅“六月夏天”") {
		t.Fatal("choice did not open confirmation")
	}
	bot.commands.createdAt = time.Now().Add(-tgFlowLifetime - time.Second)
	confirmation := commandButton(t, calls(), "确认取消")
	bot.handleUpdate(ctx, commandCallback(bot, confirmation))
	if _, ok := store.findSyncSource("weibo-2", SourceWeibo); !ok {
		t.Fatal("expired confirmation mutated subscription")
	}
	bot.handleUpdate(ctx, commandMessage("查看订阅"))
	bot.commands.listAt = time.Now().Add(-tgFlowLifetime - time.Second)
	bot.handleUpdate(ctx, commandMessage("更新订阅 2"))
	if !strings.Contains(lastCommandText(calls()), "过期") {
		t.Fatal("expired numeric snapshot accepted")
	}
}

func TestTGSearchPlatformPagingAndFirstFive(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.content = &Store{posts: []Post{{ID: "weibo-status-100", Source: SourceWeibo, Author: "作者1", AuthorID: "1", Caption: "最新图文", Published: time.Now()}}}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		var users []map[string]any
		for i := 1; i <= 6; i++ {
			users = append(users, map[string]any{"idstr": fmt.Sprint(i), "screen_name": fmt.Sprintf("作者%d", i)})
		}
		writeJSON(w, map[string]any{"ok": 1, "data": map[string]any{"cards": users}})
	})
	bot, calls := commandTestBot(t, store)
	ctx := context.Background()
	bot.handleUpdate(ctx, commandMessage("/sh 作者"))
	if !strings.Contains(lastCommandText(calls()), "哔哩哔哩 · 1 / 3") {
		t.Fatal("sidebar platform ordering changed")
	}
	next := commandButton(t, calls(), "下一平台")
	bot.handleUpdate(ctx, commandCallback(bot, next))
	if !strings.Contains(lastCommandText(calls()), "微博 · 2 / 3") || len(bot.commands.search.Results[SourceWeibo]) != 5 {
		t.Fatal("search did not limit first five users")
	}
	oldAuthor := commandButton(t, calls(), "作者1")
	next = commandButton(t, calls(), "下一平台")
	bot.handleUpdate(ctx, commandCallback(bot, next))
	if !strings.Contains(lastCommandText(calls()), "Pixiv · 3 / 3") {
		t.Fatal("third platform incorrect")
	}
	before := len(calls())
	bot.handleUpdate(ctx, commandCallback(bot, oldAuthor))
	if len(calls()) != before+1 || calls()[before].Method != "answerCallbackQuery" {
		t.Fatal("old platform button selected a current-platform author")
	}
	bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "上一平台")))
	// commandButton skips the stale callback answer and still finds the last UI.
	author := commandButton(t, calls(), "作者1")
	bot.handleUpdate(ctx, commandCallback(bot, author))
	if !strings.Contains(lastCommandText(calls()), "最新图文") {
		t.Fatal("author preview did not read latest archived post")
	}
	bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "返回搜索结果")))
	if !strings.Contains(lastCommandText(calls()), "微博 · 2 / 3") {
		t.Fatal("back did not preserve search platform")
	}
}

func TestTGCallbackRejectsForgedViewTargetAndIndex(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.Subscriptions = []SourceConfig{{ID: "bili-1", Source: SourceBilibili, Name: "同名作者"}}
	store.config.WeiboSubscriptions = []SourceConfig{{ID: "weibo-2", Source: SourceWeibo, Name: "同名作者"}}
	bot, calls := commandTestBot(t, store)
	ctx := context.Background()
	bot.handleUpdate(ctx, commandMessage("查看订阅"))
	nonce := bot.commands.nonce
	for _, action := range []string{"confirm:0", "subscribe:0", "subscriptions:-1", "subscriptions:999"} {
		before := len(calls())
		bot.handleUpdate(ctx, commandCallback(bot, nonce+":"+action))
		if len(calls()) != before+1 || calls()[before].Method != "answerCallbackQuery" || bot.commands.nonce != nonce {
			t.Fatalf("forged %s changed the view or invalidated valid buttons", action)
		}
	}
	bot.handleUpdate(ctx, commandMessage("取消订阅 同名作者"))
	bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "weibo-2")))
	confirmation := commandButton(t, calls(), "确认取消")
	nonce = bot.commands.nonce
	forged := commandCallback(bot, nonce+":confirm:0")
	bot.handleUpdate(ctx, forged)
	for _, mutation := range []func(*tgUpdate){
		func(u *tgUpdate) { u.CallbackQuery.From.ID = 99 },
		func(u *tgUpdate) { u.CallbackQuery.Message.MessageID++ },
		func(u *tgUpdate) { u.CallbackQuery.Message.Chat.Type = "group" },
	} {
		update := commandCallback(bot, confirmation)
		mutation(&update)
		bot.handleUpdate(ctx, update)
	}
	if len(store.allSourceConfigs()) != 2 || bot.commands.nonce != nonce {
		t.Fatal("forged confirmation changed a different target or invalidated valid confirmation")
	}
	bot.handleUpdate(ctx, commandCallback(bot, confirmation))
	if _, ok := store.findSyncSource("weibo-2", SourceWeibo); ok {
		t.Fatal("valid confirmation was lost after rejected callbacks")
	}
	if _, ok := store.findSyncSource("bili-1", SourceBilibili); !ok {
		t.Fatal("forged confirmation removed the other platform")
	}
}

func TestTGNewCommandClearsMediaAndCancelsWaitNotTask(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	bot, calls := commandTestBot(t, store)
	ctx := context.Background()
	bot.handleUpdate(ctx, commandMessage("/start"))
	rootID := bot.commands.messageID
	bot.commands.mediaIDs = []int64{501, 502}
	var queue syncQueue
	entered, release := make(chan struct{}), make(chan struct{})
	task, _ := queue.submit(SourceConfig{ID: "held", Source: SourceWeibo, Name: "旧任务"}, false, false, func() syncTaskResult {
		close(entered)
		<-release
		return syncTaskResult{source: SourceConfig{Name: "旧任务"}, added: 9}
	})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("fixture task never started")
	}
	bot.watchTask(ctx, 42, task, bot.commands.nonce)
	cancel := bot.commands.waitCancel
	waitCanceled := false
	bot.commands.waitCancel = func() { waitCanceled = true; cancel() }
	bot.handleUpdate(ctx, commandMessage("/start"))
	if !waitCanceled || bot.commands.waitCancel != nil || len(bot.commands.mediaIDs) != 0 || bot.commands.messageID != rootID {
		t.Fatal("new command did not clear prior flow resources in place")
	}
	deleted := map[int64]bool{}
	for _, call := range calls() {
		if call.Method == "deleteMessage" {
			deleted[int64(call.Payload["message_id"].(float64))] = true
		}
	}
	if !deleted[501] || !deleted[502] || deleted[rootID] {
		t.Fatal("new command did not remove only prior preview messages")
	}
	select {
	case <-task.done:
		t.Fatal("new command stopped the background synchronization")
	default:
	}
	close(release)
	deadline, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if result, done := task.wait(deadline); !done || result.added != 9 {
		t.Fatal("background task did not finish after flow cancellation")
	}
	time.Sleep(20 * time.Millisecond)
	if text := lastCommandText(calls()); !strings.HasPrefix(text, "Lumic · 拾光") || strings.Contains(text, "旧任务") {
		t.Fatal("late old task result replaced the new command", text)
	}
}

func TestTGChangedGenerationCannotEnterOrMutateCommands(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.WeiboSubscriptions = []SourceConfig{{ID: "weibo-2", Source: SourceWeibo, Name: "旧会话作者"}}
	bot, calls := commandTestBot(t, store)
	bot.managed = true
	store.config.Telegram = bot.config
	ctx := context.Background()
	bot.handleUpdate(ctx, commandMessage("/start"))
	before := len(calls())
	store.config.Telegram.ChatID = "99"
	bot.handleUpdate(ctx, commandMessage("查看订阅"))
	if len(calls()) != before {
		t.Fatal("changed recipient still had command access")
	}
	feed := store.config.WeiboSubscriptions[0]
	bot.commands.choices, bot.commands.action = []SourceConfig{feed}, "update"
	bot.chooseSubscription(ctx, 42, 0)
	if len(store.syncTasks.byID) != 0 {
		t.Fatal("stale generation enqueued an update")
	}
	bot.commands.action = "unsubscribe"
	bot.handleCallback(ctx, 42, "confirm", 0)
	if _, ok := store.findSyncSource(feed.ID, feed.Source); !ok {
		t.Fatal("stale generation removed a subscription")
	}
	bot.commands.selected = &tgAuthor{Source: SourceWeibo, ID: "123", Name: "新作者"}
	bot.handleCallback(ctx, 42, "subscribe", 0)
	if len(store.allSourceConfigs()) != 1 {
		t.Fatal("stale generation added a subscription")
	}
	store.config.Telegram = bot.config
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	bot.handleUpdate(canceled, commandMessage("/start"))
	if len(calls()) != before {
		t.Fatal("canceled runtime processed commands")
	}
}

func TestTGCommandsRequireExplicitAuthorizedUser(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.WeiboSubscriptions = []SourceConfig{{ID: "weibo-2", Source: SourceWeibo, Name: "作者"}}
	bot, calls := commandTestBot(t, store)
	bot.config.ChatID = "99"
	ctx := context.Background()
	bot.handleUpdate(ctx, commandMessage("取消订阅 作者"))
	confirmation := commandButton(t, calls(), "确认取消")
	nonce, rootID := bot.commands.nonce, bot.commands.messageID
	for _, call := range calls() {
		if call.Method == "sendMessage" || call.Method == "editMessageText" {
			if call.Payload["chat_id"] != float64(42) {
				t.Fatal("command reply was sent to the notification chat")
			}
		}
	}
	bot.commands.mediaIDs = []int64{501}
	waitCanceled := false
	bot.commands.waitCancel = func() { waitCanceled = true }
	before := len(calls())
	for _, authorization := range []string{"", "0", "-42", "invalid"} {
		bot.config.AuthorizedUserID = authorization
		bot.handleUpdate(ctx, commandMessage("/start"))
		bot.handleUpdate(ctx, commandCallback(bot, confirmation))
	}
	bot.config.AuthorizedUserID = "42"
	for _, mutate := range []func(*tgUpdate){
		func(u *tgUpdate) { u.Message.From.ID = 0 },
		func(u *tgUpdate) { u.Message.From.ID = 99 },
		func(u *tgUpdate) { u.Message.Chat.ID, u.Message.From.ID = 99, 99 },
		func(u *tgUpdate) { u.Message.Chat.Type = "group" },
	} {
		update := commandMessage("/start")
		mutate(&update)
		bot.handleUpdate(ctx, update)
	}
	for _, mutate := range []func(*tgUpdate){
		func(u *tgUpdate) { u.CallbackQuery.From.ID = 0 },
		func(u *tgUpdate) { u.CallbackQuery.From.ID = 99 },
		func(u *tgUpdate) { u.CallbackQuery.Message.Chat.ID, u.CallbackQuery.From.ID = 99, 99 },
		func(u *tgUpdate) { u.CallbackQuery.Message.Chat.Type = "supergroup" },
	} {
		update := commandCallback(bot, confirmation)
		mutate(&update)
		bot.handleUpdate(ctx, update)
	}
	if len(calls()) != before || waitCanceled || bot.commands.nonce != nonce || bot.commands.messageID != rootID || !reflect.DeepEqual(bot.commands.mediaIDs, []int64{501}) {
		t.Fatal("unauthorized update altered the active flow or emitted a response")
	}
	if _, ok := store.findSyncSource("weibo-2", SourceWeibo); !ok {
		t.Fatal("unauthorized callback removed a subscription")
	}
	bot.handleUpdate(ctx, commandCallback(bot, confirmation))
	if _, ok := store.findSyncSource("weibo-2", SourceWeibo); ok || !waitCanceled {
		t.Fatal("authorized user could not confirm with a different notification chat")
	}
}

func TestTGPixivSearchRouteSchemasAndFirstFive(t *testing.T) {
	var mode string
	var profileCalls []string
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ajax/user/") {
			id := strings.TrimPrefix(r.URL.Path, "/ajax/user/")
			profileCalls = append(profileCalls, id)
			writeJSON(w, map[string]any{"error": false, "body": map[string]any{"name": "作者" + id, "imageBig": "https://i.pximg.net/" + id + ".jpg"}})
			return
		}
		if r.URL.Path != "/ajax/search/users" || r.URL.Query().Get("nick") != "作者 名" || r.URL.Query().Get("s_mode") != "s_usr" || r.URL.Query().Get("p") != "1" || r.URL.Query().Get("i") != "1" || r.URL.Query().Get("word") != "" {
			t.Error("wrong Pixiv user-search route", r.URL)
		}
		ids := []any{"6", "5", "4", "3", "2", "1"}
		rows := []any{}
		users := map[string]any{}
		for _, id := range ids {
			user := map[string]any{"userId": id, "userName": "作者" + id.(string)}
			rows = append(rows, user)
			users[id.(string)] = user
		}
		body := map[string]any{"page": map[string]any{"userIds": ids}}
		switch mode {
		case "array":
			body["users"] = rows
		case "keyed":
			body["users"] = users
		case "previews":
			previews := []any{}
			for _, user := range rows {
				previews = append(previews, map[string]any{"user": user})
			}
			body = map[string]any{"userPreviews": previews}
		case "ids":
			body["thumbnails"] = map[string]any{"illust": []any{map[string]any{"id": "999", "userId": "6", "title": "作品而非作者", "userName": "作品作者"}}}
		case "empty":
			body = map[string]any{"users": []any{}}
		case "malformed":
			body = map[string]any{"illust": rows}
		}
		writeJSON(w, map[string]any{"error": false, "body": body})
	})
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.ProxyURL, store.config.Pixiv.Cookie = proxy, "fixture-cookie"
	for _, fixture := range []string{"array", "keyed", "previews", "ids", "empty", "malformed"} {
		t.Run(fixture, func(t *testing.T) {
			mode, profileCalls = fixture, nil
			users, err := store.searchTelegramAuthors(context.Background(), SourcePixiv, "作者 名")
			if fixture == "malformed" {
				if err == nil {
					t.Fatal("artwork collection was accepted as user search")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fixture == "empty" {
				if len(users) != 0 {
					t.Fatal("empty search manufactured users")
				}
				return
			}
			if len(users) != 5 || users[0].ID != "6" || users[4].ID != "2" || users[0].Name != "作者6" {
				t.Fatalf("wrong first-five provider ordering or author fields: %+v", users)
			}
			if fixture == "ids" && !reflect.DeepEqual(profileCalls, []string{"6", "5", "4", "3", "2"}) {
				t.Fatal("ID-only search must resolve only the first five author profiles", profileCalls)
			}
			if fixture != "ids" && len(profileCalls) != 0 {
				t.Fatal("complete search result unnecessarily fetched profiles")
			}
		})
	}
}

func TestTGBilibiliSearchUsesUserRouteAndFirstFive(t *testing.T) {
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "api.bilibili.com" || r.URL.Path != "/x/web-interface/search/type" || r.URL.Query().Get("search_type") != "bili_user" || r.URL.Query().Get("keyword") != "摄影 作者" || r.URL.Query().Get("page") != "1" {
			t.Error("wrong Bilibili user search route", r.URL)
		}
		users := []any{}
		for id := 1; id <= 6; id++ {
			users = append(users, map[string]any{"mid": id, "uname": fmt.Sprintf("<em>作者</em>%d", id), "upic": "//i0.hdslb.com/face.jpg"})
		}
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"result": users}})
	})
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.ProxyURL, store.config.Credentials.SESSDATA = proxy, "fixture-cookie"
	users, err := store.searchTelegramAuthors(context.Background(), SourceBilibili, "摄影 作者")
	if err != nil || len(users) != 5 || users[0].ID != "1" || users[4].ID != "5" || users[0].Name != "作者1" || users[0].Avatar != "https://i0.hdslb.com/face.jpg" {
		t.Fatalf("invalid Bilibili user search results: %+v, %v", users, err)
	}
}
