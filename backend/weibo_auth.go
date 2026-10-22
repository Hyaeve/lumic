package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type weiboRequestError struct {
	kind    string
	message string
}

func (e *weiboRequestError) Error() string { return e.message }

func weiboErrorKind(err error) string {
	var failure *weiboRequestError
	if errors.As(err, &failure) {
		return failure.kind
	}
	return ""
}

var weiboHTTPClient = externalHTTPClient
var weiboPasswordLogin = loginWeiboWithPassword

type weiboRecoveryState struct {
	session WeiboCredentials
	at      time.Time
	err     error
}

func hasWeiboPassword(credentials WeiboCredentials) bool {
	return strings.TrimSpace(credentials.PasswordUsername) != "" && credentials.Password != ""
}

func hasWeiboAccount(credentials WeiboCredentials) bool {
	return credentials.Cookie != "" || credentials.MobileCookie != "" || hasWeiboPassword(credentials)
}

func sameWeiboSession(a, b WeiboCredentials) bool {
	return a.UserID == b.UserID && a.Cookie == b.Cookie && a.MobileCookie == b.MobileCookie && a.CookieScoped == b.CookieScoped && a.PasswordUsername == b.PasswordUsername && a.Password == b.Password
}

func retainWeiboPassword(previous WeiboCredentials, next *WeiboCredentials) {
	if previous.UserID != "" && previous.UserID == next.UserID {
		next.PasswordUsername, next.Password = previous.PasswordUsername, previous.Password
	}
}

func weiboPayloadFailure(payload map[string]any) error {
	code := firstNonEmptyRemoteText(jsonValueString(payload["errno"]), jsonValueString(payload["error_code"]))
	message := strings.ToLower(firstNonEmptyRemoteText(jsonValueString(payload["msg"]), jsonValueString(payload["message"]), jsonValueString(payload["errmsg"]), jsonValueString(payload["error"])))
	for _, text := range []string{"验证码", "安全验证", "访问频繁", "风控", "captcha", "verify", "risk"} {
		if strings.Contains(message, text) {
			return &weiboRequestError{"restricted", "微博要求安全验证或限制了当前请求，请在官方页面完成验证后重试"}
		}
	}
	for _, text := range []string{"请先登录", "请登录", "未登录", "登录失效", "登录过期", "登录已失效", "登录已过期", "重新登录", "login required", "not logged in", "session expired", "invalid session", "token expired", "cookie expired"} {
		if strings.Contains(message, text) {
			return &weiboRequestError{"auth", "微博登录会话已失效，请重新扫码或更新凭证"}
		}
	}
	switch code {
	case "100001", "21301", "21314", "21315", "21317", "21327", "21332":
		return &weiboRequestError{"auth", "微博登录会话已失效，请重新扫码或更新凭证"}
	case "10022", "10023", "10024":
		return &weiboRequestError{"limited", "微博请求已达到频率限制，请稍后重试"}
	}
	if jsonValueString(payload["ok"]) == "-100" {
		return &weiboRequestError{"auth", "微博接口明确要求登录（ok=-100），当前 Cookie 尚未建立有效站点会话"}
	}
	if ok, exists := payload["ok"]; exists && jsonValueString(ok) == "0" || code != "" && code != "0" {
		return &weiboRequestError{"upstream", "微博接口拒绝了当前请求，请检查博主可见性或稍后重试"}
	}
	return nil
}

// Only accept same-site, root-path cookie updates from successful API replies.
// Authentication failures never replace a working stored session with deletions.
func mergeWeiboResponseCookies(original string, response *http.Response) string {
	parts := strings.Split(normalizeWeiboCookie(original), "; ")
	host := response.Request.URL.Hostname()
	for _, cookie := range response.Cookies() {
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		if domain != "" && host != domain && !strings.HasSuffix(host, "."+domain) || cookie.Path != "" && cookie.Path != "/" {
			continue
		}
		filtered := parts[:0]
		for _, part := range parts {
			if pair := strings.SplitN(part, "=", 2); len(pair) == 2 && !strings.EqualFold(pair[0], cookie.Name) {
				filtered = append(filtered, part)
			}
		}
		parts = filtered
		if cookie.MaxAge >= 0 && (cookie.Expires.IsZero() || cookie.Expires.After(time.Now())) {
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	return normalizeWeiboCookie(strings.Join(parts, "; "))
}

func weiboSessionRequest(endpoint string, credentials *WeiboCredentials, proxyURL string) (map[string]any, int, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	cookie := &credentials.Cookie
	if request.URL.Hostname() == "m.weibo.cn" && (credentials.CookieScoped || credentials.MobileCookie != "") {
		cookie = &credentials.MobileCookie
	}
	weiboRequestHeaders(request, *cookie)
	client, err := weiboHTTPClient(proxyURL)
	if err != nil {
		return nil, 0, err
	}
	copyClient := *client
	copyClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || next.URL.Host != request.URL.Host || next.URL.Scheme != request.URL.Scheme {
			return http.ErrUseLastResponse
		}
		return nil
	}
	response, err := copyClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	status := response.StatusCode
	if status == http.StatusUnauthorized || status >= 300 && status < 400 && (strings.Contains(response.Header.Get("Location"), "login") || strings.Contains(response.Header.Get("Location"), "passport")) {
		return nil, status, &weiboRequestError{"auth", "微博登录会话已失效，请重新扫码或更新凭证"}
	}
	if status == http.StatusForbidden {
		return nil, status, &weiboRequestError{"restricted", "微博拒绝了当前访问（HTTP 403），请检查官方账号状态与代理出口"}
	}
	if status == http.StatusTooManyRequests || status == 418 {
		return nil, status, &weiboRequestError{"limited", "微博请求受到频率限制，请稍后重试"}
	}
	if status != http.StatusOK {
		return nil, status, &weiboRequestError{"upstream", fmt.Sprintf("微博接口返回 HTTP %d，请稍后重试", status)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, status, err
	}
	if len(body) > 4<<20 {
		return nil, status, errors.New("微博接口响应超过大小限制")
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || payload == nil {
		lower := bytes.ToLower(body)
		if bytes.Contains(lower, []byte("passport.weibo")) || bytes.Contains(lower, []byte("login.sina")) {
			return nil, status, &weiboRequestError{"auth", "微博登录会话已失效，请重新扫码或更新凭证"}
		}
		return nil, status, &weiboRequestError{"restricted", "微博未返回可用数据，可能要求登录或网页安全验证，请在官方页面检查"}
	}
	if err := weiboPayloadFailure(payload); err != nil {
		return nil, status, err
	}
	if *cookie != "" {
		*cookie = mergeWeiboResponseCookies(*cookie, response)
	}
	return payload, status, nil
}

// Compare the session used by the request, not just the account ID: a late
// renewal or password login must not overwrite a newer user-supplied session.
func (b *BilibiliStore) persistWeiboSession(previous WeiboCredentials, next *WeiboCredentials) error {
	if sameWeiboSession(previous, *next) {
		return nil
	}
	b.Lock()
	if !sameWeiboSession(b.config.Weibo, previous) {
		*next = b.config.Weibo
		b.Unlock()
		return nil
	}
	stored := b.config.Weibo
	b.config.Weibo.Cookie = next.Cookie
	b.config.Weibo.MobileCookie = next.MobileCookie
	b.config.Weibo.CookieScoped = next.CookieScoped
	b.config.Weibo.UserID = next.UserID
	if next.UserName != "" {
		b.config.Weibo.UserName = next.UserName
	}
	if next.Avatar != "" {
		b.config.Weibo.Avatar = next.Avatar
	}
	applied := b.config.Weibo
	b.Unlock()
	if err := b.save(); err != nil {
		b.Lock()
		if sameWeiboSession(b.config.Weibo, applied) {
			b.config.Weibo = stored
		}
		b.Unlock()
		*next = previous
		return errors.New("微博会话更新未能保存，请检查配置存储后重试")
	}
	return nil
}

func (b *BilibiliStore) requestWeiboWithRecovery(endpoint string, credentials *WeiboCredentials, proxyURL string) (map[string]any, int, error) {
	previous := *credentials
	payload, status, err := weiboSessionRequest(endpoint, credentials, proxyURL)
	if err == nil {
		return payload, status, b.persistWeiboSession(previous, credentials)
	}
	if !isWeiboSessionFailure(err) || !hasWeiboPassword(previous) {
		return nil, status, err
	}
	b.weiboRecoveryMu.Lock()
	defer b.weiboRecoveryMu.Unlock()
	b.RLock()
	latest := b.config.Weibo
	b.RUnlock()
	if !sameWeiboSession(previous, latest) {
		if previous.UserID != latest.UserID || previous.PasswordUsername != latest.PasswordUsername {
			return nil, status, errors.New("微博账号配置已变更，请重试当前操作")
		}
		*credentials = latest
		payload, status, err = weiboSessionRequest(endpoint, credentials, proxyURL)
		if err == nil {
			return payload, status, b.persistWeiboSession(latest, credentials)
		}
		if !isWeiboSessionFailure(err) {
			return nil, status, err
		}
		previous = latest
	}
	if !hasWeiboPassword(previous) {
		return nil, status, err
	}
	if b.weiboRecovery.err != nil && sameWeiboSession(b.weiboRecovery.session, previous) && time.Since(b.weiboRecovery.at) < time.Minute {
		return nil, status, b.weiboRecovery.err
	}
	refreshed, loginErr := weiboPasswordLogin(previous.PasswordUsername, previous.Password, proxyURL)
	if loginErr == nil && previous.UserID != "" && refreshed.UserID != previous.UserID {
		loginErr = errors.New("备用密码对应的微博账号与当前账号不一致，请重新配置")
	}
	if loginErr != nil {
		kind := "auth"
		if failure := weiboPayloadFailure(map[string]any{"msg": loginErr.Error()}); weiboErrorKind(failure) == "restricted" || weiboErrorKind(failure) == "limited" {
			kind = weiboErrorKind(failure)
		}
		recoveryErr := &weiboRequestError{kind, fmt.Sprintf("微博会话恢复失败，请在官方页面检查或重新扫码：%s", loginErr)}
		b.weiboRecovery = weiboRecoveryState{previous, time.Now(), recoveryErr}
		return nil, status, recoveryErr
	}
	refreshed.PasswordUsername, refreshed.Password = previous.PasswordUsername, previous.Password
	if err := b.persistWeiboSession(previous, &refreshed); err != nil {
		return nil, status, err
	}
	if refreshed.UserID != previous.UserID && previous.UserID != "" {
		return nil, status, errors.New("微博账号配置已变更，请重试当前操作")
	}
	*credentials = refreshed
	b.weiboRecovery = weiboRecoveryState{}
	payload, status, err = weiboSessionRequest(endpoint, credentials, proxyURL)
	if err == nil {
		err = b.persistWeiboSession(refreshed, credentials)
	} else {
		b.weiboRecovery = weiboRecoveryState{refreshed, time.Now(), err}
	}
	return payload, status, err
}
