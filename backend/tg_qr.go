package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

var tgQRPlatforms = [...]Source{SourceBilibili, SourceWeibo}

type tgQRCredentialStatus int

const (
	tgQRCredentialUnknown tgQRCredentialStatus = iota
	tgQRCredentialInvalid
	tgQRCredentialValid
)

type tgPlatformQR struct {
	id, payload string
	expiresAt   time.Time
}

func (b *tgBot) showQRPlatforms(ctx context.Context, chatID int64, result string) {
	b.commands.view = "qr-platforms"
	text := "请选择要扫码授权的平台。"
	if result != "" {
		text = result + "\n\n" + text
	}
	keyboard := tgKeyboard{{b.button("哔哩哔哩", "qr-platform", 0), b.button("微博", "qr-platform", 1)}}
	_ = b.showFlow(ctx, chatID, text, keyboard)
}

func (b *tgBot) beginTGQR(ctx context.Context, chatID int64, source Source) {
	b.commands.view = "qr-active"
	if err := b.showFlow(ctx, chatID, tgPlatformLabel(source)+"：正在检查已保存的凭证…", tgKeyboard{{b.button("取消扫码", "qr-cancel", 0)}}); err != nil {
		return
	}
	qrContext, cancel := context.WithCancel(ctx)
	b.commands.waitCancel = cancel
	nonce := b.commands.nonce
	b.commands.qrWorkers.Add(1)
	go func() {
		defer b.commands.qrWorkers.Done()
		b.runTGQR(ctx, qrContext, cancel, chatID, source, nonce)
	}()
}

func (b *tgBot) createTGQR(ctx context.Context, source Source) (tgPlatformQR, *platformQRError) {
	switch source {
	case SourceBilibili:
		session, err := b.store.startBilibiliQR(ctx)
		return tgPlatformQR{session.ID, session.URL, session.ExpiresAt}, err
	case SourceWeibo:
		session, err := b.store.startWeiboQR(ctx)
		return tgPlatformQR{session.ID, session.Image, session.ExpiresAt}, err
	default:
		return tgPlatformQR{}, &platformQRError{status: http.StatusBadRequest}
	}
}

func (b *tgBot) runTGQR(parent, ctx context.Context, cancel context.CancelFunc, chatID int64, source Source, nonce string) {
	defer cancel()
	label := tgPlatformLabel(source)
	failure := label + "授权失败，请检查平台或容器网络后重试。"
	expired := label + "授权失败：二维码已过期，请重新获取。"
	finish := func(result string) {
		if ctx.Err() != nil {
			return
		}
		b.commands.mu.Lock()
		defer b.commands.mu.Unlock()
		if ctx.Err() != nil || !b.currentGeneration() || b.commands.nonce != nonce {
			return
		}
		reply, stop := context.WithTimeout(parent, 12*time.Second)
		defer stop()
		b.commands.waitCancel = nil
		b.clearFlowMedia(reply, chatID)
		b.commands.nonce, b.commands.createdAt = tgNonce(), time.Now()
		b.showQRPlatforms(reply, chatID, result)
	}
	if ctx.Err() != nil || !b.currentGeneration() {
		return
	}
	b.store.RLock()
	config := b.store.config
	b.store.RUnlock()
	// Keep a QR attempt tied to the credentials it checked, including at commit.
	guard := func(current BilibiliConfig) bool {
		if current.Telegram != b.config || current.ProxyURL != b.proxyURL {
			return false
		}
		switch source {
		case SourceBilibili:
			return current.Credentials == config.Credentials
		case SourceWeibo:
			return sameWeiboSession(current.Weibo, config.Weibo) && current.Weibo.QRConnected == config.Weibo.QRConnected
		}
		return false
	}
	credentialsUnchanged := func() bool {
		b.store.RLock()
		defer b.store.RUnlock()
		return guard(b.store.config)
	}
	recheck := label + "凭证配置已更新，请重新选择平台检查。"
	probe, stopProbe := context.WithTimeout(ctx, 12*time.Second)
	status := tgQRCredentialUnknown
	switch source {
	case SourceBilibili:
		status = checkTGQRBilibili(probe, config.Credentials, config.ProxyURL)
	case SourceWeibo:
		status = checkTGQRWeibo(probe, config.Weibo, config.ProxyURL)
	}
	if probe.Err() != nil {
		status = tgQRCredentialUnknown
	}
	stopProbe()
	if ctx.Err() != nil || !b.currentGeneration() {
		return
	}
	if !credentialsUnchanged() {
		finish(recheck)
		return
	}
	switch status {
	case tgQRCredentialValid:
		finish(label + "凭证有效，无需重新扫码授权。")
		return
	case tgQRCredentialInvalid:
		// Missing or explicitly invalid credentials require a new authorization.
	default:
		finish(label + "：暂时无法确认凭证状态，请检查网络或平台状态后重试。")
		return
	}
	qr, problem := b.createTGQR(ctx, source)
	if problem != nil {
		finish(failure)
		return
	}
	defer b.store.discardPlatformQR(source, qr.id)

	active, stop := context.WithDeadline(ctx, qr.expiresAt)
	defer stop()
	imageData, err := b.tgQRImage(active, source, qr.payload)
	var messageID int64
	if err == nil {
		if !credentialsUnchanged() {
			finish(recheck)
			return
		}
		messageID, err = b.sendTGQRImage(active, chatID, imageData, "请使用"+label+"客户端扫描二维码，并在客户端确认授权。")
	}
	if err != nil {
		if errors.Is(active.Err(), context.DeadlineExceeded) {
			finish(expired)
		} else {
			finish(failure)
		}
		return
	}
	b.commands.mu.Lock()
	keep := ctx.Err() == nil && b.currentGeneration() && b.commands.nonce == nonce
	if keep {
		b.commands.mediaIDs = append(b.commands.mediaIDs, messageID)
		_ = b.showFlow(active, chatID, label+"：等待扫码确认。", tgKeyboard{{b.button("取消扫码", "qr-cancel", 0)}})
	}
	b.commands.mu.Unlock()
	if !keep {
		cleanup, stop := context.WithTimeout(parent, 5*time.Second)
		defer stop()
		_ = b.deleteMessage(cleanup, chatID, messageID)
		return
	}

	scanned := false
	for active.Err() == nil && b.currentGeneration() {
		var result map[string]any
		if source == SourceBilibili {
			result, problem = b.store.pollBilibiliQR(active, qr.id, guard)
		} else {
			result, problem = b.store.pollWeiboQR(active, qr.id, guard)
		}
		if problem == nil && result["status"] == "connected" {
			if source == SourceWeibo && result["credentialVerified"] != true {
				finish(label + "授权失败：扫码已确认，但新会话未通过验证。")
			} else {
				finish(label + "授权成功，凭证已保存。")
			}
			return
		}
		if errors.Is(active.Err(), context.DeadlineExceeded) || (problem != nil && problem.status == http.StatusGone) {
			finish(expired)
			return
		}
		if problem != nil {
			if !credentialsUnchanged() {
				finish(recheck)
			} else if source == SourceWeibo && problem.status == http.StatusUnprocessableEntity {
				finish(label + "授权失败：扫码已确认，但新会话未通过验证。")
			} else {
				finish(failure)
			}
			return
		}
		if result["status"] == "scanned" && !scanned {
			b.commands.mu.Lock()
			if active.Err() == nil && b.currentGeneration() && b.commands.nonce == nonce {
				_ = b.showFlow(active, chatID, label+"：已扫码，请在客户端确认。", tgKeyboard{{b.button("取消扫码", "qr-cancel", 0)}})
			}
			b.commands.mu.Unlock()
			scanned = true
		}
		if !tgWait(active, 2*time.Second) {
			break
		}
	}
	if errors.Is(active.Err(), context.DeadlineExceeded) {
		finish(expired)
	}
}

func weiboQRImageURL(value string) (*url.URL, bool) {
	if strings.HasPrefix(value, "//") {
		value = "https:" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || len(value) > 8192 {
		return nil, false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "v2.qr.weibo.cn", "qr.weibo.cn", "login.sina.com.cn", "passport.weibo.cn", "passport.weibo.com":
		return parsed, true
	}
	return nil, false
}

func (b *tgBot) tgQRImage(ctx context.Context, source Source, payload string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source == SourceBilibili {
		link, err := url.Parse(payload)
		if err != nil || len(payload) > 2048 || link.Scheme != "https" || (link.Host != "passport.bilibili.com" && link.Host != "account.bilibili.com") || link.User != nil {
			return nil, errors.New("invalid Bilibili QR link")
		}
		return qrcode.Encode(payload, qrcode.Medium, 480)
	}
	if source != SourceWeibo {
		return nil, errors.New("unsupported QR platform")
	}
	link, ok := weiboQRImageURL(payload)
	if !ok {
		return nil, errors.New("invalid Weibo QR image address")
	}
	client, err := externalHTTPClient(b.proxyURL)
	if err != nil {
		return nil, errors.New("invalid project proxy")
	}
	client.CheckRedirect = func(next *http.Request, previous []*http.Request) error {
		if _, allowed := weiboQRImageURL(next.URL.String()); !allowed || len(previous) > 3 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link.String(), nil)
	if err != nil {
		return nil, errors.New("invalid Weibo QR image request")
	}
	weiboQRRequestHeaders(request)
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("Weibo QR image request failed")
	}
	defer response.Body.Close()
	const maxQRImage = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxQRImage+1))
	if err != nil || response.StatusCode != http.StatusOK || len(data) > maxQRImage {
		return nil, errors.New("Weibo QR image unavailable")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || config.Width < 48 || config.Height < 48 || config.Width > 2048 || config.Height > 2048 || config.Width > config.Height*2 || config.Height > config.Width*2 {
		return nil, errors.New("invalid Weibo QR image")
	}
	return data, nil
}

func (b *tgBot) sendTGQRImage(ctx context.Context, chatID int64, data []byte, caption string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = form.WriteField("caption", caption)
	_ = form.WriteField("protect_content", "true")
	filename := "qr.png"
	if http.DetectContentType(data) == "image/jpeg" {
		filename = "qr.jpg"
	}
	part, err := form.CreateFormFile("photo", filename)
	if err != nil {
		return 0, err
	}
	if _, err := part.Write(data); err != nil {
		return 0, err
	}
	if err := form.Close(); err != nil {
		return 0, err
	}
	var message tgMessage
	err = b.request(ctx, "sendPhoto", form.FormDataContentType(), body.Bytes(), &message)
	if err == nil && message.MessageID == 0 {
		err = errors.New("invalid Telegram QR message")
	}
	return message.MessageID, err
}
