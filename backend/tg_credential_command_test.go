package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTGCredentialCommandReportsToAuthorizedChatOnly(t *testing.T) {
	var bili, weibo, pixiv atomic.Int32
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "api.bilibili.com" && r.URL.Path == "/x/web-interface/nav":
			bili.Add(1)
			fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":42}}`)
		case r.Host == "weibo.com" && r.URL.Path == "/ajax/config/get":
			weibo.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		case r.Host == "www.pixiv.net" && r.URL.Path == "/":
			pixiv.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			t.Error("unexpected credential command request", r.Host, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}, "api.bilibili.com:443", "weibo.com:443", "www.pixiv.net:443")
	bot.store.config.Credentials = BilibiliCredentials{DedeUserID: "42", SESSDATA: "bili-command-fixture"}
	bot.store.config.Weibo = WeiboCredentials{UserID: "42", Cookie: "SUB=weibo-command-fixture"}
	bot.store.config.Pixiv = PixivCredentials{UserID: "42", Cookie: "PHPSESSID=pixiv-command-fixture", UserAgent: "fixture-agent"}
	bot.store.config.TelegramCredentialCheckWeek = "2026-11-23"
	bot.store.tgCredentialSentWeek = "2026-11-23"
	bot.store.tgCredentialRetryAt = time.Now().Add(time.Hour)
	before, retryAt := bot.store.config, bot.store.tgCredentialRetryAt

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	for i, input := range []string{"/checkauth", "检查授权", "/checkauth@LumicBot"} {
		bot.handleUpdate(ctx, commandMessage(input))
		waitTGQRWorkers(t, bot)
		if expected := int32(i); bili.Load() != expected || weibo.Load() != expected || pixiv.Load() != expected {
			t.Fatalf("%q probed before the user confirmed", input)
		}
		confirm := commandButton(t, calls(), "检查全部")
		bot.handleUpdate(ctx, commandCallback(bot, confirm))
		waitTGQRWorkers(t, bot)
		result := lastCommandText(calls())
		if !strings.Contains(result, "平台凭证检查") ||
			!strings.Contains(result, "哔哩哔哩：有效") || !strings.Contains(result, "微博：无效或未配置") ||
			!strings.Contains(result, "Pixiv：暂时无法确认") || !strings.Contains(result, "/qr") ||
			strings.Contains(result, "暂无扫码授权命令") {
			t.Fatalf("%q did not return the three statuses and relevant recovery steps: %q", input, result)
		}
		bot.handleUpdate(ctx, commandCallback(bot, confirm))
		waitTGQRWorkers(t, bot)
		if expected := int32(i + 1); bili.Load() != expected || weibo.Load() != expected || pixiv.Load() != expected ||
			lastCommandText(calls()) != result {
			t.Fatal("a used confirmation triggered another check or altered its result")
		}
	}
	if bili.Load() != 3 || weibo.Load() != 3 || pixiv.Load() != 3 {
		t.Fatalf("manual command did not probe each platform once per run: %d %d %d", bili.Load(), weibo.Load(), pixiv.Load())
	}
	sends, edits, acknowledgments := 0, 0, 0
	for _, call := range calls() {
		switch call.Method {
		case "sendMessage":
			sends++
		case "editMessageText":
			edits++
			if fmt.Sprint(call.Payload["message_id"]) != "31" {
				t.Fatal("command edited a different root message")
			}
		case "answerCallbackQuery":
			acknowledgments++
			continue
		default:
			t.Fatalf("unexpected Telegram method %q", call.Method)
		}
		if fmt.Sprint(call.Payload["chat_id"]) != "42" {
			t.Fatal("command reported to the notification recipient instead of its caller")
		}
		for _, marker := range []string{"bili-command-fixture", "weibo-command-fixture", "pixiv-command-fixture", tgTestToken} {
			if strings.Contains(fmt.Sprint(call.Payload), marker) {
				t.Fatal("command exposed a credential")
			}
		}
	}
	if sends != 1 || edits != 8 || acknowledgments != 6 {
		t.Fatalf("confirmation, progress and reports did not stay in one command message: %d sends, %d edits, %d acknowledgments", sends, edits, acknowledgments)
	}
	if !reflect.DeepEqual(bot.store.config, before) || bot.store.tgCredentialSentWeek != "2026-11-23" ||
		bot.store.tgCredentialRetryAt != retryAt {
		t.Fatal("manual check changed saved credentials or weekly notification state")
	}
	if _, err := os.Stat(bilibiliFile); !os.IsNotExist(err) {
		t.Fatal("manual check attempted to write configuration", err)
	}
}

func TestTGCredentialCommandProbesOnlyRequestedPlatform(t *testing.T) {
	var bili, weibo, pixiv atomic.Int32
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "api.bilibili.com" && r.URL.Path == "/x/web-interface/nav":
			bili.Add(1)
			fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":42}}`)
		case r.Host == "weibo.com" && r.URL.Path == "/ajax/config/get":
			weibo.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		case r.Host == "www.pixiv.net" && r.URL.Path == "/":
			pixiv.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			t.Error("unexpected single-platform request", r.Host, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}, "api.bilibili.com:443", "weibo.com:443", "www.pixiv.net:443")
	bot.store.config.Credentials = BilibiliCredentials{DedeUserID: "42", SESSDATA: "bili-command-fixture"}
	bot.store.config.Weibo = WeiboCredentials{UserID: "42", Cookie: "SUB=weibo-command-fixture"}
	bot.store.config.Pixiv = PixivCredentials{UserID: "42", Cookie: "PHPSESSID=pixiv-command-fixture", UserAgent: "fixture-agent"}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	for _, tt := range []struct {
		input, want string
		noResults   []string
		advice      string
		probeCounts [3]int32
	}{
		{"/checkauth 哔哩哔哩", "哔哩哔哩：有效", []string{"微博：", "Pixiv："}, "", [3]int32{1, 0, 0}},
		{"检查授权　BILIBILI", "哔哩哔哩：有效", []string{"微博：", "Pixiv："}, "", [3]int32{2, 0, 0}},
		{"检查授权 微博", "微博：无效或未配置", []string{"哔哩哔哩：", "Pixiv："}, "/qr", [3]int32{2, 1, 0}},
		{"/checkauth@LumicBot PiXiV", "Pixiv：无效或未配置", []string{"哔哩哔哩：", "微博："}, "Cookie", [3]int32{2, 1, 1}},
	} {
		bot.handleUpdate(ctx, commandMessage(tt.input))
		waitTGQRWorkers(t, bot)
		if got := [3]int32{bili.Load(), weibo.Load(), pixiv.Load()}; got != tt.probeCounts {
			t.Fatalf("%q probes = %v, want %v", tt.input, got, tt.probeCounts)
		}
		result := lastCommandText(calls())
		if !strings.Contains(result, tt.want) || (tt.advice != "" && !strings.Contains(result, tt.advice)) ||
			(tt.advice != "/qr" && strings.Contains(result, "/qr")) ||
			(tt.advice != "Cookie" && strings.Contains(result, "Cookie")) {
			t.Fatalf("%q did not give the expected status and recovery advice: %q", tt.input, result)
		}
		for _, invalid := range tt.noResults {
			if strings.Contains(result, invalid) {
				t.Fatalf("%q reported an unchecked platform: %q", tt.input, result)
			}
		}
	}
}

func TestTGCredentialCommandAllConfirmationIsBoundToCurrentPrivateFlow(t *testing.T) {
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no unconfirmed or invalid action may contact a platform", r.Host, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}, "api.bilibili.com:443")
	bot.store.config.Credentials = BilibiliCredentials{DedeUserID: "42", SESSDATA: "bili-command-fixture"}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	for _, input := range []string{"/checkauth unknown", "/checkauth 微博 pixiv"} {
		bot.handleUpdate(ctx, commandMessage(input))
		waitTGQRWorkers(t, bot)
		if got := lastCommandText(calls()); !strings.Contains(got, "/checkauth") || strings.Contains(got, "平台凭证检查") {
			t.Fatalf("%q did not return usage without probing: %q", input, got)
		}
	}

	bot.handleUpdate(ctx, commandMessage("/checkauth"))
	waitTGQRWorkers(t, bot)
	confirm := commandButton(t, calls(), "检查全部")
	n := len(calls())
	foreign := commandCallback(bot, confirm)
	foreign.CallbackQuery.From.ID = 55
	bot.handleUpdate(ctx, foreign)
	if len(calls()) != n {
		t.Fatal("a foreign sender accessed the confirmation flow")
	}

	wrongMessage := commandCallback(bot, confirm)
	wrongMessage.CallbackQuery.Message.MessageID++
	bot.handleUpdate(ctx, wrongMessage)
	bot.handleUpdate(ctx, commandCallback(bot, strings.TrimSuffix(confirm, ":0")+":1"))
	bot.commands.createdAt = time.Now().Add(-tgFlowLifetime - time.Minute)
	bot.handleUpdate(ctx, commandCallback(bot, confirm))
	waitTGQRWorkers(t, bot)
	if len(calls()) != n+3 || commandButton(t, calls(), "检查全部") != confirm {
		t.Fatal("invalid callback changed the confirmation prompt")
	}

	bot.handleUpdate(ctx, commandMessage("检查授权"))
	abort := commandButton(t, calls(), "取消")
	currentConfirm := commandButton(t, calls(), "检查全部")
	bot.handleUpdate(ctx, commandCallback(bot, confirm))
	bot.handleUpdate(ctx, commandCallback(bot, abort))
	bot.handleUpdate(ctx, commandCallback(bot, currentConfirm))
	waitTGQRWorkers(t, bot)
	if got := lastCommandText(calls()); !strings.Contains(got, "已取消") {
		t.Fatal("cancel or stale callback changed the cancellation result", got)
	}
	afterCancel := lastCommandText(calls())
	bot.handleUpdate(ctx, commandCallback(bot, bot.commands.nonce+":auth-confirm:0"))
	waitTGQRWorkers(t, bot)
	if got := lastCommandText(calls()); got != afterCancel {
		t.Fatal("confirmation forged with a current nonce escaped its permitted view", got)
	}
}

func TestTGCredentialCommandRequiresAuthorizedPrivateSender(t *testing.T) {
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("unauthorized sender started a platform check")
		w.WriteHeader(http.StatusBadRequest)
	})
	for _, sender := range []tgUpdate{
		{Message: &tgMessage{Chat: tgChat{ID: 42, Type: "group"}, From: tgUser{ID: 42}, Text: "/checkauth"}},
		{Message: &tgMessage{Chat: tgChat{ID: 42, Type: "private"}, From: tgUser{ID: 55}, Text: "/checkauth"}},
		{Message: &tgMessage{Chat: tgChat{ID: 55, Type: "private"}, From: tgUser{ID: 55}, Text: "检查授权"}},
	} {
		bot.handleUpdate(context.Background(), sender)
	}
	bot.store.config.Telegram.AuthorizedUserID = ""
	bot.config = bot.store.config.Telegram
	bot.handleUpdate(context.Background(), commandMessage("/checkauth"))
	waitTGQRWorkers(t, bot)
	if len(calls()) != 0 || bot.commands.messageID != 0 || bot.commands.nonce != "" {
		t.Fatal("unauthorized messages reached the command flow")
	}
}

func TestTGCredentialCommandDiscardsStaleProbe(t *testing.T) {
	for _, scenario := range []string{"new command", "changed credential", "unrelated credential", "revoked authorization"} {
		t.Run(scenario, func(t *testing.T) {
			started, canceled := make(chan struct{}, 1), make(chan struct{}, 1)
			release := make(chan struct{})
			bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Host != "api.bilibili.com" || r.URL.Path != "/x/web-interface/nav" {
					t.Error("unexpected request for cancellation test", r.Host, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				started <- struct{}{}
				select {
				case <-release:
					fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":42}}`)
				case <-r.Context().Done():
					canceled <- struct{}{}
				}
			}, "api.bilibili.com:443")
			bot.store.config.Credentials = BilibiliCredentials{DedeUserID: "42", SESSDATA: "bili-command-fixture"}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer waitTGQRWorkers(t, bot)
			defer cancel()
			bot.handleUpdate(ctx, commandMessage("/checkauth bilibili"))
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("manual credential check did not start")
			}
			if text := lastCommandText(calls()); !strings.Contains(text, "正在检查平台凭证") {
				t.Fatal("manual credential check did not display progress", text)
			}
			switch scenario {
			case "new command":
				bot.handleUpdate(ctx, commandMessage("/start"))
				select {
				case <-canceled:
				case <-ctx.Done():
					t.Fatal("new command did not cancel the active platform request")
				}
			case "changed credential":
				bot.store.Lock()
				bot.store.config.Credentials.SESSDATA = "new-command-fixture"
				bot.store.Unlock()
			case "unrelated credential":
				bot.store.Lock()
				bot.store.config.Weibo.Cookie = "SUB=new-command-fixture"
				bot.store.Unlock()
			case "revoked authorization":
				bot.store.Lock()
				bot.store.config.Telegram.AuthorizedUserID = ""
				bot.store.Unlock()
			}
			n := len(calls())
			close(release)
			waitTGQRWorkers(t, bot)
			text := lastCommandText(calls())
			switch scenario {
			case "new command":
				if !strings.Contains(text, "/checkauth") || !strings.Contains(text, "/subscriptions") || len(calls()) != n {
					t.Fatal("replaced audit edited or duplicated the next command response", text)
				}
			case "changed credential":
				if !strings.Contains(text, "凭证配置已更新") || !strings.Contains(text, "/checkauth 哔哩哔哩") ||
					strings.Contains(text, "哔哩哔哩：有效") {
					t.Fatal("manual audit reported stale data after credential change", text)
				}
			case "unrelated credential":
				if !strings.Contains(text, "哔哩哔哩：有效") || strings.Contains(text, "微博：") {
					t.Fatal("an unchecked platform change discarded a current result", text)
				}
			case "revoked authorization":
				if len(calls()) != n {
					t.Fatal("manual audit sent a result after authorization was revoked", text)
				}
			}
		})
	}
}

func TestTGCredentialCommandDoesNotOverlapWeeklyProbe(t *testing.T) {
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a busy audit started another platform request")
		w.WriteHeader(http.StatusBadRequest)
	})
	bot.store.config.Credentials = BilibiliCredentials{DedeUserID: "42", SESSDATA: "bili-command-fixture"}
	bot.store.tgCredentialAuditMu.Lock()
	defer bot.store.tgCredentialAuditMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bot.handleUpdate(ctx, commandMessage("检查授权 哔哩哔哩"))
	waitTGQRWorkers(t, bot)
	if text := lastCommandText(calls()); !strings.Contains(text, "稍后") || !strings.Contains(text, "/checkauth 哔哩哔哩") {
		t.Fatal("a busy audit did not report that the user could retry", text)
	}
}
