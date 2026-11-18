package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

var telegramCredentialLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
var tgCredentialPlatforms = [...]Source{SourceBilibili, SourceWeibo, SourcePixiv}

type tgCredentialResult struct {
	source Source
	status tgQRCredentialStatus
}

func telegramCredentialCheckWeek(at time.Time) string {
	local := at.In(telegramCredentialLocation)
	if local.Weekday() != time.Monday || local.Hour() < 8 {
		return ""
	}
	return local.Format("2006-01-02")
}

func (bot *tgBot) runWeeklyCredentialChecks(ctx context.Context) {
	bot.checkWeeklyCredentials(ctx, time.Now())
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-timer.C:
			bot.checkWeeklyCredentials(ctx, at)
		}
	}
}

func probeTelegramCredential(ctx context.Context, config BilibiliConfig, source Source) tgQRCredentialStatus {
	probe, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	status := tgQRCredentialUnknown
	switch source {
	case SourceBilibili:
		status = checkTGQRBilibili(probe, config.Credentials, config.ProxyURL)
	case SourceWeibo:
		status = checkTGQRWeibo(probe, config.Weibo, config.ProxyURL)
	case SourcePixiv:
		status = checkTGCredentialPixiv(probe, config.Pixiv, config.ProxyURL)
	}
	if probe.Err() != nil {
		return tgQRCredentialUnknown
	}
	return status
}

func collectTelegramCredentialResults(ctx context.Context, config BilibiliConfig, sources []Source) []tgCredentialResult {
	results := make([]tgCredentialResult, 0, len(sources))
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		results = append(results, tgCredentialResult{source, probeTelegramCredential(ctx, config, source)})
	}
	return results
}

func telegramCredentialAuditText(heading string, at time.Time, results []tgCredentialResult) string {
	text := fmt.Sprintf("%s\n%s（北京时间）", heading, at.In(telegramCredentialLocation).Format("2006/01/02 15:04"))
	var qrPlatforms []string
	var pixiv, unknown bool
	for _, result := range results {
		label := tgPlatformLabel(result.source)
		state := "暂时无法确认（网络或平台限制）"
		switch result.status {
		case tgQRCredentialValid:
			state = "有效"
		case tgQRCredentialInvalid:
			state = "无效或未配置"
			if result.source == SourcePixiv {
				pixiv = true
			} else {
				qrPlatforms = append(qrPlatforms, label)
			}
		default:
			unknown = true
		}
		text += "\n" + label + "：" + state
	}
	if len(qrPlatforms) != 0 {
		text += "\n\n重新授权：发送 /qr 或“扫码授权”，选择" + strings.Join(qrPlatforms, "、") + "，扫描二维码并确认。"
	}
	if pixiv {
		text += "\n\nPixiv：在 Lumic「设置 → 平台凭证 → Pixiv」更新浏览器 Cookie、UA 和用户 ID；暂无扫码授权命令。"
	}
	if unknown {
		text += "\n\n无法确认不代表凭证失效，请检查项目代理、容器网络或平台限制后重试。"
	}
	return text
}

func sameTelegramCredentialAuditConfig(current, checked BilibiliConfig, sources []Source) bool {
	if current.Telegram != checked.Telegram || current.ProxyURL != checked.ProxyURL {
		return false
	}
	for _, source := range sources {
		switch source {
		case SourceBilibili:
			if current.Credentials != checked.Credentials {
				return false
			}
		case SourceWeibo:
			if !sameWeiboSession(current.Weibo, checked.Weibo) || current.Weibo.QRConnected != checked.Weibo.QRConnected {
				return false
			}
		case SourcePixiv:
			if current.Pixiv != checked.Pixiv {
				return false
			}
		}
	}
	return true
}

func (bot *tgBot) startCredentialCheck(ctx context.Context, chatID int64, platform string) {
	var source Source
	switch strings.ToLower(platform) {
	case "":
		bot.commands.view = "auth-confirm"
		_ = bot.showFlow(ctx, chatID, "要检查哔哩哔哩、微博和 Pixiv 的凭证吗？",
			tgKeyboard{{bot.button("检查全部", "auth-confirm", 0), bot.button("取消", "auth-cancel", 0)}})
		return
	case "哔哩哔哩", "bilibili":
		source = SourceBilibili
	case "微博", "weibo":
		source = SourceWeibo
	case "pixiv":
		source = SourcePixiv
	default:
		_ = bot.showFlow(ctx, chatID, "不支持该平台。请发送 /checkauth 哔哩哔哩、/checkauth 微博 或 /checkauth pixiv。", nil)
		return
	}
	bot.beginCredentialCheck(ctx, chatID, []Source{source})
}

func (bot *tgBot) beginCredentialCheck(ctx context.Context, chatID int64, sources []Source) {
	bot.commands.view = "result"
	if err := bot.showFlow(ctx, chatID, "正在检查平台凭证…", nil); err != nil {
		return
	}
	retryCommand := "/checkauth"
	if len(sources) == 1 {
		retryCommand += " " + tgPlatformLabel(sources[0])
	}
	check, cancel := context.WithCancel(ctx)
	bot.commands.waitCancel = cancel
	nonce := bot.commands.nonce
	bot.commands.workers.Add(1)
	go func() {
		defer bot.commands.workers.Done()
		defer cancel()
		if check.Err() != nil || !bot.currentGeneration() {
			return
		}
		b := bot.store
		var config BilibiliConfig
		var results []tgCredentialResult
		started := b.tgCredentialAuditMu.TryLock()
		if started {
			b.RLock()
			config = b.config
			b.RUnlock()
			results = collectTelegramCredentialResults(check, config, sources)
			b.tgCredentialAuditMu.Unlock()
		}

		bot.commands.mu.Lock()
		defer bot.commands.mu.Unlock()
		if check.Err() != nil || !bot.currentGeneration() || bot.commands.nonce != nonce {
			return
		}
		bot.commands.waitCancel = nil
		text := "已有平台凭证检查在进行，请稍后再发送 " + retryCommand + "。"
		if started {
			b.RLock()
			unchanged := sameTelegramCredentialAuditConfig(b.config, config, sources)
			b.RUnlock()
			if unchanged {
				text = telegramCredentialAuditText("平台凭证检查", time.Now(), results)
			} else {
				text = "检查期间凭证配置已更新，请重新发送 " + retryCommand + "。"
			}
		}
		reply, stop := context.WithTimeout(check, 12*time.Second)
		defer stop()
		_ = bot.showFlow(reply, chatID, text, nil)
	}()
}

func (bot *tgBot) checkWeeklyCredentials(ctx context.Context, at time.Time) {
	week := telegramCredentialCheckWeek(at)
	if week == "" || bot.store == nil || !bot.config.Enabled || ctx.Err() != nil || !bot.currentGeneration() {
		return
	}
	chatID, err := validTGChatID(bot.config.ChatID)
	if err != nil || !validTGToken(bot.config.Token) {
		return
	}
	b := bot.store
	// This dedicated lock never blocks settings, commands or subscription syncs.
	if !b.tgCredentialAuditMu.TryLock() {
		return
	}
	defer b.tgCredentialAuditMu.Unlock()
	b.RLock()
	config := b.config
	b.RUnlock()
	if config.TelegramCredentialCheckWeek == week || b.tgCredentialSentWeek == week || at.Before(b.tgCredentialRetryAt) {
		return
	}
	// Failed deliveries are retried at most once every five minutes on Monday.
	b.tgCredentialRetryAt = at.Add(5 * time.Minute)
	sources := tgCredentialPlatforms[:]
	results := collectTelegramCredentialResults(ctx, config, sources)
	if ctx.Err() != nil || !bot.currentGeneration() {
		return
	}
	b.RLock()
	unchanged := sameTelegramCredentialAuditConfig(b.config, config, sources)
	b.RUnlock()
	if !unchanged {
		b.tgCredentialRetryAt = time.Time{}
		return
	}
	delivery, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := bot.sendText(delivery, chatID, telegramCredentialAuditText("平台凭证每周检查", at, results), nil); err != nil {
		if ctx.Err() == nil {
			log.Print("event=telegram_delivery_failed stage=credential_check")
		}
		return
	}
	b.tgCredentialSentWeek = week
	if err := bot.saveCredentialCheckWeek(week); err != nil {
		log.Print("event=telegram_checkpoint_failed stage=credential_check")
	}
}

func (bot *tgBot) saveCredentialCheckWeek(week string) error {
	b := bot.store
	b.saveMu.Lock()
	defer b.saveMu.Unlock()
	b.Lock()
	defer b.Unlock()
	if b.config.Telegram != bot.config || b.config.ProxyURL != bot.proxyURL {
		return context.Canceled
	}
	next := b.config
	next.TelegramCredentialCheckWeek = week
	data, err := encryptJSON(b.key, next)
	if err == nil {
		err = atomicWriteFile(bilibiliFile, data, 0600)
	}
	if err == nil {
		b.config.TelegramCredentialCheckWeek = next.TelegramCredentialCheckWeek
	}
	return err
}
