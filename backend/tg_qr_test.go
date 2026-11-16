package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

func tgQRTestBot(t *testing.T, backend http.HandlerFunc, hosts ...string) (*tgBot, func() []tgCommandCall) {
	t.Helper()
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	var calls []tgCommandCall
	var mu sync.Mutex
	proxy := tgRuntimeProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "api.telegram.org" {
			backend(w, r)
			return
		}
		method := path.Base(r.URL.Path)
		var input map[string]any
		var result any = true
		if method == "sendPhoto" {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error("expected QR photo upload", err)
				w.WriteHeader(400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			file, _, err := r.FormFile("photo")
			if err != nil {
				t.Error("missing QR photo", err)
				w.WriteHeader(400)
				return
			}
			defer file.Close()
			cfg, format, err := image.DecodeConfig(file)
			if err != nil || format != "png" || cfg.Width != cfg.Height || cfg.Width < 120 {
				t.Error("invalid QR upload", cfg, format, err)
			}
			input = map[string]any{"chat_id": r.FormValue("chat_id"), "caption": r.FormValue("caption"), "protect_content": r.FormValue("protect_content")}
			result = map[string]int{"message_id": 99}
		} else {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error("expected command JSON", err)
			}
			if method == "sendMessage" {
				result = map[string]int{"message_id": 31}
			}
		}
		mu.Lock()
		calls = append(calls, tgCommandCall{method, input})
		mu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "result": result})
	}, hosts...)
	store.config.Telegram = TGBotConfig{Enabled: true, Token: tgTestToken, ChatID: "55", AuthorizedUserID: "42"}
	store.config.ProxyURL = proxy
	client, err := externalHTTPClient(proxy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	bot := &tgBot{store: store, config: store.config.Telegram, proxyURL: proxy, client: client, apiBase: telegramAPI, managed: true}
	return bot, func() []tgCommandCall { mu.Lock(); defer mu.Unlock(); return append([]tgCommandCall(nil), calls...) }
}

func waitTGQRWorkers(t *testing.T, bot *tgBot) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		bot.commands.qrWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("QR worker did not exit")
	}
}

func TestTGQRCommandAuthorizationAndPayloadAllowlist(t *testing.T) {
	bot, calls := commandTestBot(t, newWeiboAuthTestStore(t, WeiboCredentials{}))
	ctx := context.Background()
	for _, input := range []string{"/qr", "扫码授权", "授权扫码", "/qr@lumic_bot"} {
		if action, _, _ := tgCommand(input); action != "qr" {
			t.Fatalf("unexpected QR command action: %s", action)
		}
	}
	unauthorized := commandMessage("/qr")
	unauthorized.Message.From.ID = 55
	bot.handleUpdate(ctx, unauthorized)
	if len(calls()) != 0 {
		t.Fatal("unauthorized user reached the QR menu")
	}
	bot.handleUpdate(ctx, commandMessage("/qr"))
	bili := commandButton(t, calls(), "哔哩哔哩")
	_ = commandButton(t, calls(), "微博")
	if !strings.Contains(lastCommandText(calls()), "扫码授权") {
		t.Fatal("QR menu text missing")
	}
	before := len(calls())
	bad := commandCallback(bot, bili)
	bad.CallbackQuery.From.ID = 55
	bot.handleUpdate(ctx, bad)
	if len(calls()) != before {
		t.Fatal("unauthorized user reached QR callback")
	}
	bot.handleUpdate(ctx, commandCallback(bot, bot.commands.nonce+":qr-platform:3"))
	if len(calls()) != before+1 || calls()[before].Method != "answerCallbackQuery" || bot.commands.view != "qr-platforms" {
		t.Fatal("forged platform index was accepted")
	}
	for _, payload := range []string{"https://passport.bilibili.com/h5-app/passport/login/scan", "https://account.bilibili.com/h5/account-h5/auth/scan-web"} {
		png, err := bot.tgQRImage(ctx, SourceBilibili, payload)
		cfg, format, readErr := image.DecodeConfig(bytes.NewReader(png))
		if err != nil || readErr != nil || format != "png" || cfg.Width != 480 {
			t.Fatal("official Bilibili QR link was not encoded", err, readErr)
		}
	}
	for _, payload := range []string{"http://v2.qr.weibo.cn/image", "https://v2.qr.weibo.cn@127.0.0.1/image", "https://v2.qr.weibo.cn.evil.invalid/image", "https://v2.qr.weibo.cn:444/image"} {
		if _, ok := weiboQRImageURL(payload); ok {
			t.Fatal("untrusted QR image URL accepted")
		}
	}
	if link, ok := weiboQRImageURL("//v2.qr.weibo.cn/inf/gen"); !ok || link.Scheme != "https" {
		t.Fatal("official protocol-relative QR image not supported")
	}
}

func TestTGQRBilibiliLifecycle(t *testing.T) {
	for _, mode := range []string{"verified", "expired", "canceled", "replaced", "revoked", "changed", "failure"} {
		t.Run(mode, func(t *testing.T) {
			pollStarted := make(chan struct{}, 1)
			release := make(chan struct{})
			var expiredProbes atomic.Int32
			bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
				case "/x/passport-login/web/qrcode/generate":
					if mode == "verified" && expiredProbes.Load() != 1 {
						t.Error("QR generated before the saved credential was checked")
					}
					fmt.Fprint(w, `{"code":0,"data":{"url":"https://account.bilibili.com/h5/account-h5/auth/scan-web?qrcode_key=fixture","qrcode_key":"fixture-ticket"}}`)
				case "/x/passport-login/web/qrcode/poll":
					if r.URL.Query().Get("qrcode_key") != "fixture-ticket" {
						t.Error("wrong QR session")
					}
					pollStarted <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					switch mode {
					case "expired":
						fmt.Fprint(w, `{"code":0,"data":{"code":86038}}`)
					case "failure":
						fmt.Fprint(w, `{"code":0,"data":{"code":0}}`)
					default:
						for name, value := range map[string]string{"SESSDATA": "fixture-cookie", "bili_jct": "fixture-csrf", "DedeUserID": "42", "buvid3": "fixture-buvid"} {
							http.SetCookie(w, &http.Cookie{Name: name, Value: value})
						}
						fmt.Fprint(w, `{"code":0,"data":{"code":0,"refresh_token":"fixture-refresh"}}`)
					}
				case "/x/web-interface/nav":
					if cookie, err := r.Cookie("SESSDATA"); err == nil && cookie.Value == "expired-fixture" {
						expiredProbes.Add(1)
						fmt.Fprint(w, `{"code":-101}`)
						return
					}
					fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":42,"uname":"fixture"}}`)
				default:
					t.Error("unexpected Bilibili fixture request", r.URL.Path)
					w.WriteHeader(404)
				}
			}, "www.bilibili.com:443", "passport.bilibili.com:443", "api.bilibili.com:443")
			if mode == "verified" {
				bot.store.config.Credentials = BilibiliCredentials{SESSDATA: "expired-fixture", DedeUserID: "42"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer func() { cancel(); waitTGQRWorkers(t, bot) }()
			bot.handleUpdate(ctx, commandMessage("/qr"))
			bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "哔哩哔哩")))
			select {
			case <-pollStarted:
			case <-ctx.Done():
				t.Fatal("enabled QR flow did not poll")
			}
			switch mode {
			case "canceled":
				bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "取消扫码")))
			case "replaced":
				bot.handleUpdate(ctx, commandMessage("/start"))
			case "revoked":
				bot.store.Lock()
				bot.store.config.Telegram.AuthorizedUserID = ""
				bot.store.Unlock()
			case "changed":
				bot.store.Lock()
				bot.store.config.Credentials = BilibiliCredentials{SESSDATA: "new-fixture", DedeUserID: "42"}
				bot.store.Unlock()
			}
			close(release)
			waitTGQRWorkers(t, bot)
			logs := calls()
			want := map[string]string{"verified": "授权成功", "expired": "授权失败：二维码已过期", "canceled": "已取消扫码", "replaced": "聚合、订阅与归档", "revoked": "等待扫码确认", "changed": "凭证配置已更新", "failure": "授权失败"}[mode]
			if !strings.Contains(lastCommandText(logs), want) {
				t.Fatal("missing terminal QR feedback", mode, lastCommandText(logs))
			}
			var methods []string
			for _, call := range logs {
				if call.Method == "sendPhoto" {
					if call.Payload["chat_id"] != "42" || call.Payload["protect_content"] != "true" {
						t.Fatal("QR photo was not private to the authorized user")
					}
					methods = append(methods, call.Method)
				}
				if call.Method == "deleteMessage" {
					methods = append(methods, call.Method)
				}
				for _, value := range call.Payload {
					if strings.Contains(fmt.Sprint(value), "fixture-ticket") || strings.Contains(fmt.Sprint(value), "fixture-cookie") {
						t.Fatal("QR ticket or credential exposed in Telegram text")
					}
				}
			}
			wantMethods := []string{"sendPhoto", "deleteMessage"}
			if mode == "revoked" {
				wantMethods = wantMethods[:1]
			}
			if !reflect.DeepEqual(methods, wantMethods) || len(bot.store.bilibiliQR) != 0 || len(bot.store.bilibiliClients) != 0 {
				t.Fatal("QR media or session cleanup differs", methods)
			}
			if mode == "verified" {
				data, err := os.ReadFile(bilibiliFile)
				var disk BilibiliConfig
				if err != nil || decryptJSON(bot.store.key, data, &disk) != nil || disk.Credentials.DedeUserID != "42" {
					t.Fatal("verified QR credentials not durable", err)
				}
			} else if mode == "changed" {
				if bot.store.config.Credentials.SESSDATA != "new-fixture" {
					t.Fatal("QR flow overwrote the updated credential")
				}
			} else if bot.store.config.Credentials.SESSDATA != "" {
				t.Fatal("unsuccessful QR flow changed credentials")
			}
		})
	}
}

func TestTGQRValidOrUnknownCredentialsDoNotGenerateQR(t *testing.T) {
	for _, tc := range []struct {
		name, platform, host, route, body, want string
		status                                  int
	}{
		{"bilibili valid", "哔哩哔哩", "api.bilibili.com", "/x/web-interface/nav", `{"code":0,"data":{"isLogin":true,"mid":42}}`, "凭证有效，无需重新扫码授权", 200},
		{"bilibili limited", "哔哩哔哩", "api.bilibili.com", "/x/web-interface/nav", "", "暂时无法确认凭证状态", 429},
		{"weibo valid", "微博", "weibo.com", "/ajax/config/get", `{"ok":1,"data":{"uid":"42","islogin":true}}`, "凭证有效，无需重新扫码授权", 200},
		{"weibo restricted", "微博", "weibo.com", "/ajax/config/get", "", "暂时无法确认凭证状态", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var probes atomic.Int32
			bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Host != tc.host || r.URL.Path != tc.route || r.Header.Get("Cookie") == "" {
					t.Error("unexpected probe request", r.Host, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				probes.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}, tc.host+":443", "passport.bilibili.com:443", "www.bilibili.com:443", "login.sina.com.cn:443")
			bili := BilibiliCredentials{SESSDATA: "saved-fixture", DedeUserID: "42"}
			weibo := WeiboCredentials{Cookie: "SUB=saved-fixture", CookieScoped: true, UserID: "42"}
			bot.store.config.Credentials = bili
			bot.store.config.Weibo = weibo
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer func() { cancel(); waitTGQRWorkers(t, bot) }()
			bot.handleUpdate(ctx, commandMessage("/qr"))
			bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), tc.platform)))
			waitTGQRWorkers(t, bot)

			if probes.Load() != 1 || !strings.Contains(lastCommandText(calls()), tc.want) ||
				bot.commands.view != "qr-platforms" || bot.commands.waitCancel != nil {
				t.Fatal("unexpected preflight result", probes.Load(), lastCommandText(calls()))
			}
			for _, call := range calls() {
				if call.Method == "sendPhoto" {
					t.Fatal("unexpected QR upload")
				}
			}
			if len(bot.store.bilibiliQR) != 0 || len(bot.store.weiboQR) != 0 ||
				bot.store.config.Credentials != bili || bot.store.config.Weibo != weibo {
				t.Fatal("read-only check changed stored credentials or created a QR session")
			}
		})
	}
}

func TestTGQRProbeCancellationAndCredentialChange(t *testing.T) {
	for _, mode := range []string{"cancel", "changed"} {
		t.Run(mode, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/x/web-interface/nav" {
					t.Error("unexpected QR request", r.URL.Path)
					w.WriteHeader(404)
					return
				}
				close(started)
				select {
				case <-release:
					fmt.Fprint(w, `{"code":-101}`)
				case <-r.Context().Done():
				}
			}, "api.bilibili.com:443", "passport.bilibili.com:443", "www.bilibili.com:443")
			bot.store.config.Credentials = BilibiliCredentials{SESSDATA: "expired-fixture", DedeUserID: "42"}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer func() { cancel(); waitTGQRWorkers(t, bot) }()
			bot.handleUpdate(ctx, commandMessage("/qr"))
			bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "哔哩哔哩")))
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("credential check did not start")
			}
			want := "已取消扫码"
			if mode == "cancel" {
				bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "取消扫码")))
			} else {
				bot.store.Lock()
				bot.store.config.Credentials.SESSDATA = "new-fixture"
				bot.store.Unlock()
				want = "凭证配置已更新"
			}
			close(release)
			waitTGQRWorkers(t, bot)
			if !strings.Contains(lastCommandText(calls()), want) {
				t.Fatal("stale probe result was used", lastCommandText(calls()))
			}
			for _, call := range calls() {
				if call.Method == "sendPhoto" {
					t.Fatal("canceled or stale probe triggered a QR upload")
				}
			}
		})
	}
}

func TestTGQRCancelsBeforeAcknowledgingButton(t *testing.T) {
	bot, _ := commandTestBot(t, newWeiboAuthTestStore(t, WeiboCredentials{}))
	waiting, cancel := context.WithCancel(context.Background())
	defer cancel()
	bot.commands.waitCancel = cancel
	bot.commands.messageID = 31
	bot.commands.view, bot.commands.nonce, bot.commands.createdAt = "qr-active", "current", time.Now()
	base := bot.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	acknowledged := false
	bot.client.Transport = tgTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		if path.Base(request.URL.Path) == "answerCallbackQuery" {
			acknowledged = true
			if waiting.Err() == nil {
				t.Error("platform poll can still commit while Telegram acknowledges cancellation")
			}
		}
		return base.RoundTrip(request)
	})
	bot.handleUpdate(context.Background(), commandCallback(bot, "current:qr-cancel:0"))
	if !acknowledged || bot.commands.view != "qr-platforms" {
		t.Fatal("valid cancellation did not return to QR platform menu")
	}
}

func TestTGQRWeiboImageAndExpiry(t *testing.T) {
	png, err := qrcode.Encode("https://passport.weibo.cn/fixture", qrcode.Medium, 180)
	if err != nil {
		t.Fatal(err)
	}
	var probes, checks atomic.Int32
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax/config/get":
			probes.Add(1)
			fmt.Fprint(w, `{"ok":-100}`)
		case "/sso/qrcode/image":
			if probes.Load() != 1 {
				t.Error("Weibo QR generated before the saved credential was checked")
			}
			fmt.Fprint(w, `{"retcode":20000000,"data":{"qrid":"fixture","image":"//v2.qr.weibo.cn/inf/gen?fixture"}}`)
		case "/inf/gen":
			if r.Host != "v2.qr.weibo.cn" || r.Header.Get("Cookie") != "" {
				t.Error("Weibo QR image request leaked credentials or reached wrong host")
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.Copy(w, bytes.NewReader(png))
		case "/sso/qrcode/check":
			if checks.Add(1) == 1 {
				fmt.Fprint(w, `{"retcode":50114002,"msg":"scanned fixture"}`)
			} else {
				fmt.Fprint(w, `{"retcode":50114003,"msg":"expired fixture"}`)
			}
		default:
			t.Error("unexpected Weibo fixture request", r.URL.Path)
			w.WriteHeader(404)
		}
	}, "weibo.com:443", "login.sina.com.cn:443", "v2.qr.weibo.cn:443")
	bot.store.config.Weibo = WeiboCredentials{Cookie: "SUB=expired-fixture", CookieScoped: true, UserID: "42"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() { cancel(); waitTGQRWorkers(t, bot) }()
	bot.handleUpdate(ctx, commandMessage("扫码授权"))
	bot.handleUpdate(ctx, commandCallback(bot, commandButton(t, calls(), "微博")))
	waitTGQRWorkers(t, bot)
	var media []string
	scanned := false
	for _, call := range calls() {
		if call.Method == "sendPhoto" || call.Method == "deleteMessage" {
			media = append(media, call.Method)
		}
		if strings.Contains(fmt.Sprint(call.Payload["text"]), "已扫码，请在客户端确认") {
			scanned = true
		}
	}
	if !scanned || !reflect.DeepEqual(media, []string{"sendPhoto", "deleteMessage"}) || !strings.Contains(lastCommandText(calls()), "微博授权失败：二维码已过期") || len(bot.store.weiboQR) != 0 || len(bot.store.weiboClients) != 0 {
		t.Fatal("Weibo QR image, expiration feedback, or cleanup failed", media, lastCommandText(calls()))
	}
}
