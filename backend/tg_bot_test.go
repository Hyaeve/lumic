package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const tgTestToken = "123456:fixture_token_not_a_real_secret"

type tgTestRoundTripper func(*http.Request) (*http.Response, error)

func (transport tgTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func tgConfigRequest(store *BilibiliStore, method, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	store.telegramBotHandler(recorder, httptest.NewRequest(method, "/api/telegram-bot", strings.NewReader(body)))
	return recorder
}

func stopTGTestManager(t *testing.T, store *BilibiliStore) {
	t.Helper()
	if store.tgManagerCancel == nil {
		return
	}
	store.tgManagerCancel()
	select {
	case <-store.tgManagerDone:
	case <-time.After(3 * time.Second):
		t.Error("Telegram manager did not stop")
	}
}

func tgRuntimeProxy(t *testing.T, handler http.HandlerFunc, extraHosts ...string) string {
	t.Helper()
	api := httptest.NewTLSServer(handler)
	t.Cleanup(api.Close)
	hosts := map[string]bool{"api.telegram.org:443": true}
	for _, host := range extraHosts {
		hosts[host] = true
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || !hosts[r.Host] {
			http.Error(w, "fixture rejects external destinations", http.StatusForbidden)
			return
		}
		target, err := net.Dial("tcp", api.Listener.Addr().String())
		if err != nil {
			http.Error(w, "fixture dial failed", http.StatusBadGateway)
			return
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			target.Close()
			return
		}
		fmt.Fprint(connection, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(target, connection); target.Close() }()
		go func() { _, _ = io.Copy(connection, target); connection.Close() }()
	}))
	t.Cleanup(proxy.Close)
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Loopback TLS fixture only.
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
	return proxy.URL
}

func TestTGBotConfigProtectsSecretsPreservesTokenAndRollsBack(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.Telegram = TGBotConfig{Token: tgTestToken, ChatID: "42", AuthorizedUserID: "77", Notifications: true}
	t.Cleanup(func() { stopTGTestManager(t, store) })
	view := tgConfigRequest(store, http.MethodGet, "")
	if view.Code != 200 || view.Header().Get("Cache-Control") != "no-store" || strings.Contains(view.Body.String(), tgTestToken) || strings.Contains(view.Body.String(), `"token":`) || !strings.Contains(view.Body.String(), `"tokenConfigured":true`) || !strings.Contains(view.Body.String(), `"authorizedUserId":"77"`) {
		t.Fatalf("unsafe config view: %d %s", view.Code, view.Body.String())
	}
	response := tgConfigRequest(store, http.MethodPut, `{"enabled":false,"token":"","chatId":"42","authorizedUserId":" 77 ","notifications":false,"timeZone":"Asia/Shanghai"}`)
	if response.Code != 200 || store.config.Telegram.Token != tgTestToken || store.config.Telegram.Notifications || store.config.Telegram.AuthorizedUserID != "77" {
		t.Fatalf("blank token did not preserve saved secret: %d", response.Code)
	}
	data, err := os.ReadFile(bilibiliFile)
	if err != nil || strings.Contains(string(data), tgTestToken) {
		t.Fatal("config was not saved as encrypted data", err)
	}
	var restored BilibiliConfig
	if err := decryptJSON(store.key, data, &restored); err != nil || restored.Telegram != store.config.Telegram {
		t.Fatal("encrypted config did not restore", err)
	}
	previous := store.config.Telegram
	for _, input := range []string{
		`{`, `{} {}`, `{"token":"bad"}`, `{"chatId":"-1"}`, `{"chatId":"0"}`, `{"chatId":"042"}`,
		`{"timeZone":"not/a/timezone"}`, `{"enabled":true,"clearToken":true,"chatId":"42"}`,
		`{"authorizedUserId":"-1"}`, `{"authorizedUserId":"0"}`, `{"authorizedUserId":"077"}`, `{"authorizedUserId":"user"}`, `{"authorizedUserId":"9223372036854775808"}`,
	} {
		response = tgConfigRequest(store, http.MethodPut, input)
		if response.Code != 400 || store.config.Telegram != previous {
			t.Fatalf("invalid input changed config: %q => %d", input, response.Code)
		}
	}
	file := bilibiliFile
	bilibiliFile = t.TempDir()
	if err := os.WriteFile(filepath.Join(bilibiliFile, "keep"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	response = tgConfigRequest(store, http.MethodPut, `{"chatId":"77"}`)
	bilibiliFile = file
	if response.Code != 500 || store.config.Telegram != previous {
		t.Fatal("failed write did not roll back runtime configuration", response.Code)
	}
	response = tgConfigRequest(store, http.MethodPut, `{"clearToken":true}`)
	if response.Code != 200 || store.config.Telegram.Token != "" {
		t.Fatal("explicit token clear failed", response.Code)
	}
}

func TestTGBotConfigDefaultsAndEmptyAuthorization(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.Telegram = TGBotConfig{Token: tgTestToken, ChatID: "42", AuthorizedUserID: "77", Notifications: false, TimeZone: "Asia/Shanghai"}
	t.Cleanup(func() { stopTGTestManager(t, store) })
	response := tgConfigRequest(store, http.MethodPut, `{"chatId":"42"}`)
	if response.Code != 200 || !store.config.Telegram.Notifications || store.config.Telegram.TimeZone != "Asia/Shanghai" || store.config.Telegram.AuthorizedUserID != "" {
		t.Fatalf("omitted settings did not use secure defaults: %d %+v", response.Code, store.config.Telegram)
	}
	bot := &tgBot{config: store.config.Telegram}
	if bot.authorized(tgUpdate{Message: &tgMessage{Chat: tgChat{ID: 42, Type: "private"}, From: tgUser{ID: 42}}}) {
		t.Fatal("empty authorization incorrectly fell back to notification Chat ID")
	}
	response = tgConfigRequest(store, http.MethodPut, `{"chatId":"42","authorizedUserId":"77","notifications":false}`)
	if response.Code != 200 || store.config.Telegram.Notifications || store.config.Telegram.AuthorizedUserID != "77" {
		t.Fatal("legacy explicit notification setting was not retained")
	}
	response = tgConfigRequest(store, http.MethodPut, `{"chatId":"42","authorizedUserId":"  "}`)
	if response.Code != 200 || store.config.Telegram.AuthorizedUserID != "" {
		t.Fatal("blank authorization did not clear permission")
	}
	store.Lock()
	store.config.Telegram.TimeZone = ""
	store.Unlock()
	response = tgConfigRequest(store, http.MethodPut, `{"chatId":"42"}`)
	if response.Code != 200 || store.config.Telegram.TimeZone != "UTC" {
		t.Fatal("missing legacy timezone did not get a stable default")
	}
	var legacy TGBotConfig
	if err := json.Unmarshal([]byte(`{"token":"fixture","chatId":"42"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	bot.config = legacy
	if bot.authorized(tgUpdate{Message: &tgMessage{Chat: tgChat{ID: 42, Type: "private"}, From: tgUser{ID: 42}}}) {
		t.Fatal("legacy config without authorization allowed a command")
	}
}

func TestTGBotAuthorizationChecksIdentityAndGeneration(t *testing.T) {
	bot := &tgBot{config: TGBotConfig{ChatID: "42", AuthorizedUserID: "77"}}
	message := func(chatID, senderID int64, chatType string) tgUpdate {
		return tgUpdate{Message: &tgMessage{Chat: tgChat{ID: chatID, Type: chatType}, From: tgUser{ID: senderID}}}
	}
	callback := func(chatID, senderID int64, chatType string) tgUpdate {
		return tgUpdate{CallbackQuery: &tgCallbackQuery{Message: &tgMessage{Chat: tgChat{ID: chatID, Type: chatType}}, From: tgUser{ID: senderID}}}
	}
	for _, test := range []struct {
		name   string
		update tgUpdate
		allow  bool
	}{
		{"authorized private sender", message(77, 77, "private"), true},
		{"notification recipient is not authorized", message(42, 42, "private"), false},
		{"missing sender", message(77, 0, "private"), false},
		{"different sender", message(77, 42, "private"), false},
		{"different private chat", message(42, 77, "private"), false},
		{"group message", message(77, 77, "group"), false},
		{"authorized callback", callback(77, 77, "private"), true},
		{"callback missing sender", callback(77, 0, "private"), false},
		{"callback different sender", callback(77, 42, "private"), false},
		{"callback different private chat", callback(42, 77, "private"), false},
		{"group callback", callback(77, 77, "supergroup"), false},
		{"inline callback without chat", tgUpdate{CallbackQuery: &tgCallbackQuery{From: tgUser{ID: 77}}}, false},
		{"empty update", tgUpdate{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if bot.authorized(test.update) != test.allow {
				t.Fatal("authorization result differs from expected", test.allow)
			}
		})
	}
	for _, invalid := range []string{"", "-77", "077", "not-an-id"} {
		bot.config.AuthorizedUserID = invalid
		if bot.authorized(message(77, 77, "private")) || bot.authorized(callback(77, 77, "private")) {
			t.Fatal("invalid or missing authorization accepted", invalid)
		}
	}
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.Telegram = TGBotConfig{ChatID: "42", AuthorizedUserID: "77"}
	bot.config, bot.store, bot.managed = store.config.Telegram, store, true
	if !bot.authorized(message(77, 77, "private")) {
		t.Fatal("current configured generation was rejected")
	}
	var requests atomic.Int32
	bot.client = &http.Client{Transport: tgTestRoundTripper(func(_ *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("unexpected network request")
	})}
	bot.apiBase = telegramAPI
	store.config.Telegram.AuthorizedUserID = "88"
	if bot.currentGeneration() || bot.authorized(message(77, 77, "private")) || bot.authorized(callback(77, 77, "private")) {
		t.Fatal("authorization edit did not invalidate the old generation")
	}
	if _, err := bot.sendText(context.Background(), 77, "stale", nil); !errors.Is(err, context.Canceled) || requests.Load() != 0 {
		t.Fatal("old generation sent after authorization changed", err)
	}
}

func TestTGBotDraftTestUsesProjectProxyWithoutSaving(t *testing.T) {
	var calls atomic.Int32
	draftToken := "654321:draft_token_not_a_real_secret"
	proxy := tgRuntimeProxy(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/bot"+draftToken+"/") {
			t.Error("draft test did not use draft credentials and POST")
		}
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			writeJSON(w, map[string]any{"ok": true, "result": map[string]any{"username": "fixture"}})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") || input["chat_id"] != float64(77) {
			t.Error("draft test sent to incorrect recipient")
		}
		markup, _ := input["reply_markup"].(map[string]any)
		if keyboard, ok := markup["inline_keyboard"].([]any); !ok || len(keyboard) != 0 {
			t.Error("empty keyboard was not serialized as []")
		}
		writeJSON(w, map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
	})
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.ProxyURL = proxy
	store.config.Telegram = TGBotConfig{Token: tgTestToken, ChatID: "42", AuthorizedUserID: "55"}
	previous := store.config.Telegram
	response := tgConfigRequest(store, http.MethodPost, fmt.Sprintf(`{"action":"test","token":%q,"chatId":"77"}`, draftToken))
	if response.Code != 200 || calls.Load() != 2 || store.config.Telegram != previous || store.tgManagerCancel != nil || store.tgRuntime != nil {
		t.Fatalf("draft test mutated saved config/runtime: %d %s calls=%d", response.Code, response.Body.String(), calls.Load())
	}
	if _, err := os.Stat(bilibiliFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("draft test wrote saved config", err)
	}
	if strings.Contains(response.Body.String(), draftToken) || strings.Contains(response.Body.String(), tgTestToken) {
		t.Fatal("test response exposed a token")
	}
}

func TestTGBotTransportSanitizesErrorsAndRetriesOnlyExplicitRateLimit(t *testing.T) {
	var calls atomic.Int32
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		switch mode.Load() {
		case 0:
			if count == 1 {
				w.WriteHeader(429)
				fmt.Fprint(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`)
				return
			}
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":9}}`)
		case 1:
			w.WriteHeader(400)
			fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: message is not modified: fixture"}`)
		case 2:
			w.WriteHeader(401)
			fmt.Fprintf(w, `{"ok":false,"error_code":401,"description":%q}`, tgTestToken)
		default:
			w.WriteHeader(429)
			fmt.Fprint(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":900}}`)
		}
	}))
	t.Cleanup(server.Close)
	bot := &tgBot{config: TGBotConfig{Token: tgTestToken}, client: server.Client(), apiBase: server.URL}
	start := time.Now()
	if id, err := bot.sendText(context.Background(), 42, "fixture", nil); err != nil || id != 9 || calls.Load() != 2 || time.Since(start) < 950*time.Millisecond {
		t.Fatal("explicit retry-after was not honored", id, err, calls.Load())
	}
	mode.Store(1)
	if err := bot.editText(context.Background(), 42, 9, "same", nil); err != nil {
		t.Fatal("message-not-modified should be success", err)
	}
	mode.Store(2)
	if _, err := bot.sendText(context.Background(), 42, "fixture", nil); err == nil || strings.Contains(err.Error(), tgTestToken) {
		t.Fatal("upstream description leaked", err)
	}
	mode.Store(3)
	before := calls.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := bot.sendText(ctx, 42, "fixture", nil); !errors.Is(err, context.DeadlineExceeded) || calls.Load() != before+1 {
		t.Fatal("rate limit wait ignored cancellation", err)
	}
	var attempts atomic.Int32
	bot.client = &http.Client{Transport: tgTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, &url.Error{Op: "Post", URL: request.URL.String(), Err: errors.New("fixture failure")}
	})}
	if _, err := bot.sendText(context.Background(), 42, "fixture", nil); err == nil || strings.Contains(err.Error(), tgTestToken) || attempts.Load() != 1 {
		t.Fatal("network send was retried or exposed request URL", err, attempts.Load())
	}
}

func TestTGBotPollSkipsPendingRestrictsPrivateChatAndCancels(t *testing.T) {
	var sends atomic.Int32
	blocked := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		switch method {
		case "getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"username":"fixture"}}`)
		case "getUpdates":
			switch input["offset"] {
			case float64(-1):
				fmt.Fprint(w, `{"ok":true,"result":[{"update_id":100,"message":{"message_id":1,"chat":{"id":42,"type":"private"},"from":{"id":42},"text":"/start"}}]}`)
			case float64(101):
				fmt.Fprint(w, `{"ok":true,"result":[{"update_id":101,"message":{"message_id":2,"chat":{"id":99,"type":"private"},"from":{"id":99},"text":"/start"}},{"update_id":102,"message":{"message_id":3,"chat":{"id":42,"type":"group"},"from":{"id":42},"text":"/start"}},{"update_id":103,"message":{"message_id":4,"chat":{"id":42,"type":"private"},"from":{"id":42},"text":"/start"}}]}`)
			case float64(104):
				select {
				case blocked <- struct{}{}:
				default:
				}
				<-r.Context().Done()
			default:
				t.Error("unexpected poll offset", input["offset"])
				fmt.Fprint(w, `{"ok":true,"result":[]}`)
			}
		case "sendMessage":
			sends.Add(1)
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":9}}`)
		default:
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	bot := &tgBot{store: newWeiboAuthTestStore(t, WeiboCredentials{}), config: TGBotConfig{Token: tgTestToken, ChatID: "42", AuthorizedUserID: "42"}, client: server.Client(), apiBase: server.URL, cancel: cancel, done: make(chan struct{}), notifications: make(chan tgSyncNotification, 1)}
	go bot.run(ctx)
	t.Cleanup(func() {
		cancel()
		select {
		case <-bot.done:
		case <-time.After(3 * time.Second):
			t.Error("poll worker did not stop")
		}
	})
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("poll did not reach expected offset")
	}
	if sends.Load() != 1 {
		t.Fatal("backlog or unauthorized chat executed", sends.Load())
	}
	query := &tgCallbackQuery{Message: &tgMessage{Chat: tgChat{ID: 42, Type: "private"}}}
	query.From.ID = 99
	if bot.authorized(tgUpdate{CallbackQuery: query}) {
		t.Fatal("callback from foreign sender was authorized")
	}
	cancel()
	select {
	case <-bot.done:
	case <-time.After(3 * time.Second):
		t.Fatal("poll cancellation hung")
	}
}

func TestTGBotManagerRechecksGenerationAfterShutdown(t *testing.T) {
	var requests atomic.Int32
	polling := make(chan struct{}, 1)
	proxy := tgRuntimeProxy(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			fmt.Fprint(w, `{"ok":true,"result":{"username":"fixture"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["offset"] != float64(-1) {
				select {
				case polling <- struct{}{}:
				default:
				}
				<-r.Context().Done()
				return
			}
			fmt.Fprint(w, `{"ok":true,"result":[]}`)
			return
		}
		fmt.Fprint(w, `{"ok":true,"result":true}`)
	})
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	store.config.Telegram = TGBotConfig{Token: tgTestToken, ChatID: "42"}
	store.config.ProxyURL = proxy
	oldCanceled, release := make(chan struct{}), make(chan struct{})
	var stopped atomic.Bool
	old := &tgBot{config: store.config.Telegram, done: make(chan struct{}), cancel: func() {
		if stopped.CompareAndSwap(false, true) {
			close(oldCanceled)
		}
	}}
	old.config.Enabled = true
	store.tgRuntime = old
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
		stopTGTestManager(t, store)
	})
	go func() { <-release; close(old.done) }()
	store.startTelegramBot()
	select {
	case <-oldCanceled:
	case <-time.After(3 * time.Second):
		t.Fatal("old runtime not canceled")
	}
	store.Lock()
	store.config.Telegram.ChatID = "77"
	store.config.Telegram.AuthorizedUserID = "77"
	store.Unlock()
	close(release)
	released = true
	time.Sleep(50 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatal("disabled config revived stale recipient")
	}
	store.Lock()
	store.config.Telegram.Enabled = true
	store.Unlock()
	store.startTelegramBot()
	select {
	case <-polling:
	case <-time.After(3 * time.Second):
		t.Fatal("current runtime did not start")
	}
	store.tgMu.Lock()
	current := store.tgRuntime
	store.tgMu.Unlock()
	if current == nil || current.config.ChatID != "77" || current.config.AuthorizedUserID != "77" || !current.managed {
		t.Fatal("manager did not re-read latest configuration")
	}
	before := requests.Load()
	store.Lock()
	store.config.Telegram.Enabled = false
	store.Unlock()
	if current.authorized(tgUpdate{Message: &tgMessage{Chat: tgChat{ID: 77, Type: "private"}}}) {
		t.Fatal("old recipient remained authorized after config update")
	}
	if _, err := current.sendText(context.Background(), 77, "stale", nil); !errors.Is(err, context.Canceled) || requests.Load() != before {
		t.Fatal("stale runtime attempted a new network send", err)
	}
	store.startTelegramBot()
	select {
	case <-current.done:
	case <-time.After(3 * time.Second):
		t.Fatal("old polling worker did not stop")
	}
}
