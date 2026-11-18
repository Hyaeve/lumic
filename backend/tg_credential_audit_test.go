package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tgAuditMonday() time.Time {
	return time.Date(2026, time.November, 23, 8, 0, 0, 0, time.FixedZone("fixture Beijing", 8*60*60))
}

func TestTGCredentialAuditCalendarUsesBeijingMonday(t *testing.T) {
	monday := tgAuditMonday()
	for _, tc := range []struct {
		name string
		at   time.Time
		week string
	}{
		{"Sunday", monday.Add(-9 * time.Hour), ""},
		{"before eight", monday.Add(-time.Nanosecond), ""},
		{"eight", monday, "2026-11-23"},
		{"late Monday catchup", monday.Add(15*time.Hour + 59*time.Minute), "2026-11-23"},
		{"Tuesday", monday.Add(16 * time.Hour), ""},
		{"UTC before eight", monday.Add(-time.Second).UTC(), ""},
		{"UTC midnight", monday.UTC(), "2026-11-23"},
		{"next week", monday.AddDate(0, 0, 7), "2026-11-30"},
		{"year boundary", time.Date(2027, time.January, 4, 0, 0, 0, 0, time.UTC), "2027-01-04"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := telegramCredentialCheckWeek(tc.at); got != tc.week {
				t.Fatalf("week=%q want=%q at=%s", got, tc.week, tc.at)
			}
		})
	}
}

func TestTGCredentialAuditTextOnlyGivesRelevantRecoverySteps(t *testing.T) {
	for _, tc := range []struct {
		name     string
		results  []tgCredentialResult
		contains []string
		absent   []string
	}{
		{
			"all valid",
			[]tgCredentialResult{{SourceBilibili, tgQRCredentialValid}, {SourceWeibo, tgQRCredentialValid}, {SourcePixiv, tgQRCredentialValid}},
			[]string{"2026/11/23 08:00", "北京时间", "哔哩哔哩：有效", "微博：有效", "Pixiv：有效"},
			[]string{"/qr", "更新浏览器", "无法确认", "无效"},
		},
		{
			"bilibili invalid and pixiv uncertain",
			[]tgCredentialResult{{SourceBilibili, tgQRCredentialInvalid}, {SourceWeibo, tgQRCredentialValid}, {SourcePixiv, tgQRCredentialUnknown}},
			[]string{"哔哩哔哩：无效或未配置", "微博：有效", "Pixiv：暂时无法确认", "/qr", "扫码授权", "不代表凭证失效"},
			[]string{"更新浏览器", "/pixiv"},
		},
		{
			"pixiv invalid only",
			[]tgCredentialResult{{SourceBilibili, tgQRCredentialValid}, {SourceWeibo, tgQRCredentialValid}, {SourcePixiv, tgQRCredentialInvalid}},
			[]string{"Pixiv：无效或未配置", "设置", "平台凭证", "Cookie", "UA", "用户 ID", "暂无扫码授权命令"},
			[]string{"/qr", "/pixiv", "不代表凭证失效"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := telegramCredentialAuditText("平台凭证每周检查", tgAuditMonday().UTC(), tc.results)
			for _, value := range tc.contains {
				if !strings.Contains(text, value) {
					t.Errorf("missing %q in %s", value, text)
				}
			}
			for _, value := range tc.absent {
				if strings.Contains(text, value) {
					t.Errorf("unrelated recovery step %q in %s", value, text)
				}
			}
		})
	}
}

func TestTGCredentialAuditReportsOncePersistsAndKeepsCommandFlow(t *testing.T) {
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("empty credentials unexpectedly reached a platform", r.Host, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	})
	bot.commands.nonce, bot.commands.view, bot.commands.messageID = "existing-flow", "search-results", 77
	bot.commands.mediaIDs = []int64{78}
	before := bot.store.config
	before.Subscriptions = []SourceConfig{{ID: "keep", Source: SourceBilibili, Name: "fixture", Enabled: true}}
	bot.store.config.Subscriptions = before.Subscriptions
	bot.checkWeeklyCredentials(context.Background(), tgAuditMonday())
	if len(calls()) != 1 || calls()[0].Method != "sendMessage" || fmt.Sprint(calls()[0].Payload["chat_id"]) != "55" {
		t.Fatal("audit did not send exactly one notification to the configured recipient", calls())
	}
	text := fmt.Sprint(calls()[0].Payload["text"])
	for _, want := range []string{"微博", "哔哩哔哩", "Pixiv", "/qr", "设置"} {
		if !strings.Contains(text, want) {
			t.Errorf("audit message missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, tgTestToken) || bot.commands.nonce != "existing-flow" || bot.commands.view != "search-results" || bot.commands.messageID != 77 || !reflect.DeepEqual(bot.commands.mediaIDs, []int64{78}) {
		t.Fatal("weekly notification exposed a secret or disturbed the command flow")
	}
	data, err := os.ReadFile(bilibiliFile)
	if err != nil || strings.Contains(string(data), tgTestToken) {
		t.Fatal("audit checkpoint not durably encrypted", err)
	}
	var restored BilibiliConfig
	if err := decryptJSON(bot.store.key, data, &restored); err != nil {
		t.Fatal(err)
	}
	before.TelegramCredentialCheckWeek = "2026-11-23"
	if !reflect.DeepEqual(restored, before) || !reflect.DeepEqual(bot.store.config, before) {
		t.Fatal("checkpoint changed credentials, subscriptions, or unrelated configuration")
	}
	bot.checkWeeklyCredentials(context.Background(), tgAuditMonday().Add(time.Minute))
	if len(calls()) != 1 {
		t.Fatal("same week sent a duplicate report")
	}
	restoredStore := &BilibiliStore{config: restored, key: bot.store.key}
	restoredBot := &tgBot{store: restoredStore, config: restored.Telegram, proxyURL: restored.ProxyURL, client: bot.client, apiBase: telegramAPI, managed: true}
	restoredBot.checkWeeklyCredentials(context.Background(), tgAuditMonday().Add(2*time.Hour))
	if len(calls()) != 1 {
		t.Fatal("restart lost the weekly checkpoint")
	}
	restoredBot.checkWeeklyCredentials(context.Background(), tgAuditMonday().AddDate(0, 0, 7))
	if len(calls()) != 2 || restoredStore.config.TelegramCredentialCheckWeek != "2026-11-30" {
		t.Fatal("next Monday did not send a fresh report")
	}
}

func TestTGCredentialAuditSkipsOutsideWindowAndStaleBots(t *testing.T) {
	for _, mode := range []string{"before eight", "Tuesday", "disabled", "invalid chat", "canceled", "stale bot", "completed"} {
		t.Run(mode, func(t *testing.T) {
			bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("skipped audit unexpectedly checked a platform")
				w.WriteHeader(400)
			})
			at := tgAuditMonday()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "before eight":
				at = at.Add(-time.Second)
			case "Tuesday":
				at = at.AddDate(0, 0, 1)
			case "disabled":
				bot.config.Enabled = false
				bot.store.config.Telegram = bot.config
			case "invalid chat":
				bot.config.ChatID = "-42"
				bot.store.config.Telegram = bot.config
			case "canceled":
				cancel()
			case "stale bot":
				bot.store.config.Telegram.ChatID = "99"
			case "completed":
				bot.store.config.TelegramCredentialCheckWeek = "2026-11-23"
			}
			bot.checkWeeklyCredentials(ctx, at)
			if len(calls()) != 0 {
				t.Fatal("non-due or inactive audit sent a notification", calls())
			}
		})
	}
}

func TestTGCredentialAuditDeliveryFailureHasBoundedRetry(t *testing.T) {
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) })
	base := bot.client.Transport
	var attempts atomic.Int32
	bot.client.Transport = tgTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.telegram.org" && path.Base(r.URL.Path) == "sendMessage" && attempts.Add(1) == 1 {
			return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":403}`)), Request: r}, nil
		}
		return base.RoundTrip(r)
	})
	at := tgAuditMonday()
	bot.checkWeeklyCredentials(context.Background(), at)
	if attempts.Load() != 1 || bot.store.config.TelegramCredentialCheckWeek != "" {
		t.Fatal("failed notification was marked delivered")
	}
	bot.checkWeeklyCredentials(context.Background(), at.Add(4*time.Minute))
	if attempts.Load() != 1 {
		t.Fatal("failed report retried before the five-minute cooldown")
	}
	bot.checkWeeklyCredentials(context.Background(), at.Add(5*time.Minute))
	if attempts.Load() != 2 || len(calls()) != 1 || bot.store.config.TelegramCredentialCheckWeek != "2026-11-23" {
		t.Fatal("failed report did not recover after the cooldown")
	}
}

func TestTGCredentialAuditSaveFailureDoesNotRepeatDeliveredMessage(t *testing.T) {
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) })
	file := bilibiliFile
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "keep"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	bilibiliFile = broken
	defer func() { bilibiliFile = file }()
	bot.checkWeeklyCredentials(context.Background(), tgAuditMonday())
	bot.checkWeeklyCredentials(context.Background(), tgAuditMonday().Add(time.Minute))
	if len(calls()) != 1 {
		t.Fatal("successful delivery repeated after checkpoint write failure", calls())
	}
	bilibiliFile = file
	bot.checkWeeklyCredentials(context.Background(), tgAuditMonday().AddDate(0, 0, 7))
	if len(calls()) != 2 || bot.store.config.TelegramCredentialCheckWeek != "2026-11-30" {
		t.Fatal("following week did not recover from checkpoint write failure")
	}
}

func TestTGCredentialAuditDiscardsCanceledOrChangedProbe(t *testing.T) {
	for _, mode := range []string{"cancel", "credentials changed", "bot replaced"} {
		t.Run(mode, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Host != "api.bilibili.com" || r.URL.Path != "/x/web-interface/nav" {
					t.Error("unexpected audit request", r.Host, r.URL.Path)
					w.WriteHeader(400)
					return
				}
				close(started)
				select {
				case <-release:
					fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":42}}`)
				case <-r.Context().Done():
				}
			}, "api.bilibili.com:443")
			bot.store.config.Credentials = BilibiliCredentials{SESSDATA: "audit-fixture", DedeUserID: "42"}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				bot.checkWeeklyCredentials(ctx, tgAuditMonday())
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("audit probe did not start")
			}
			switch mode {
			case "cancel":
				cancel()
			case "credentials changed":
				bot.store.Lock()
				bot.store.config.Credentials.SESSDATA = "new-fixture"
				bot.store.Unlock()
			case "bot replaced":
				bot.store.Lock()
				bot.store.config.Telegram.ChatID = "99"
				bot.store.Unlock()
			}
			close(release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("canceled or stale audit did not stop")
			}
			if len(calls()) != 0 || bot.store.config.TelegramCredentialCheckWeek != "" {
				t.Fatal("canceled or stale audit sent a report or checkpoint")
			}
		})
	}
}

func TestTGCredentialAuditConcurrentTickDoesNotStartAnotherProbe(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var probes atomic.Int32
	bot, calls := tgQRTestBot(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/nav" || probes.Add(1) != 1 {
			t.Error("concurrent audit started another probe", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		close(started)
		select {
		case <-release:
			fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":42}}`)
		case <-r.Context().Done():
		}
	}, "api.bilibili.com:443")
	bot.store.config.Credentials = BilibiliCredentials{SESSDATA: "audit-fixture", DedeUserID: "42"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := make(chan struct{})
	go func() {
		defer close(first)
		bot.checkWeeklyCredentials(ctx, tgAuditMonday())
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first audit did not reach the credential probe")
	}
	second := make(chan struct{})
	go func() {
		defer close(second)
		bot.checkWeeklyCredentials(ctx, tgAuditMonday())
	}()
	select {
	case <-second:
	case <-time.After(time.Second):
		cancel()
		t.Error("concurrent tick blocked on an existing audit")
	}
	close(release)
	select {
	case <-first:
	case <-time.After(3 * time.Second):
		t.Fatal("credential probe did not finish")
	}
	if probes.Load() != 1 || len(calls()) != 1 {
		t.Fatal("concurrent ticks generated duplicate probes or notifications", probes.Load(), len(calls()))
	}
}
