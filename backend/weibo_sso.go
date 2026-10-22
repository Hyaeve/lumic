package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const weiboSSODiscoveryURL = "https://login.sina.com.cn/sso/crossdomain.php?action=login&domain=weibo.com"

func allowedWeiboSSOTarget(target *url.URL) bool {
	if target.Scheme != "https" || target.User != nil || target.Port() != "" {
		return false
	}
	switch target.Host {
	case "login.sina.com.cn", "passport.weibo.com", "weibo.com", "www.weibo.com", "passport.weibo.cn", "m.weibo.cn", "passport.sina.cn":
		return true
	}
	return false
}

func weiboCrossDomainURL(rawURL string, index int) string {
	target, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if target.Scheme == "http" && target.Port() == "" {
		target.Scheme = "https"
	}
	if !allowedWeiboSSOTarget(target) {
		return ""
	}
	query := target.Query()
	if action := query.Get("action"); action != "" && action != "login" {
		return ""
	}
	query.Set("action", "login")
	query.Set("callback", "sinaSSOController.doCrossDomainCallBack")
	query.Set("scriptId", fmt.Sprintf("ssoscript%d", index))
	if target.Host == "login.sina.com.cn" && target.Path == "/sso/crossdomain.php" {
		query.Set("callback", "sinaSSOController.crossDomainCallBack")
		query.Set("scriptId", "ssoCrossDomainScriptId")
	}
	target.RawQuery, target.Fragment = query.Encode(), ""
	return target.String()
}

// Complete only the URLs supplied by the confirmed SSO exchange. Never execute
// response JavaScript or guess alternate login routes after a rejection.
func completeWeiboSSO(ctx context.Context, client *http.Client, urls []string) error {
	const limit = 12
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ssoClient := *client
	if ssoClient.Timeout == 0 || ssoClient.Timeout > 10*time.Second {
		ssoClient.Timeout = 10 * time.Second
	}
	ssoClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowedWeiboSSOTarget(next.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	queue := append([]string(nil), urls...)
	if len(queue) == 0 {
		queue = []string{weiboSSODiscoveryURL}
	}
	seen := map[string]bool{}
	for index := 0; index < len(queue); index++ {
		if len(queue) > limit {
			return errors.New("微博跨域会话步骤超过限制，请重新扫码")
		}
		rawURL := weiboCrossDomainURL(queue[index], index)
		if rawURL == "" {
			return errors.New("微博跨域会话返回了不支持的地址，已停止交换")
		}
		target, _ := url.Parse(rawURL)
		key := *target
		query := key.Query()
		query.Del("scriptId")
		key.RawQuery = query.Encode()
		if seen[key.String()] {
			continue
		}
		seen[key.String()] = true
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return errors.New("无法创建微博跨域会话请求")
		}
		weiboRequestHeaders(request, "")
		response, err := ssoClient.Do(request)
		if err != nil {
			// net/http errors contain the URL, including its one-time ticket.
			return errors.New("微博跨域会话请求未完成，请检查网络后重新扫码")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		source := target.Hostname()
		switch response.StatusCode {
		case http.StatusOK:
		case http.StatusUnauthorized:
			return &weiboRequestError{"auth", "微博跨域会话未获授权，请重新扫码"}
		case http.StatusForbidden:
			return &weiboRequestError{"restricted", "微博跨域会话被拒绝，请在官方页面检查账号与安全验证"}
		case http.StatusTooManyRequests, 418:
			return &weiboRequestError{"limited", "微博跨域会话受到频率限制，请稍后重新扫码"}
		default:
			return fmt.Errorf("微博跨域会话未完成（%s，HTTP %d）", source, response.StatusCode)
		}
		if readErr != nil || len(body) > 1<<20 {
			return errors.New("无法完整读取微博跨域会话响应，请重新扫码")
		}
		var payload map[string]any
		decoder := json.NewDecoder(bytes.NewReader(unwrapJSONP(body)))
		decoder.UseNumber()
		if decoder.Decode(&payload) != nil || payload == nil {
			return &weiboRequestError{"restricted", "微博跨域会话未返回可识别的回调，请在官方页面检查安全验证"}
		}
		if payload["msg"] == nil {
			payload["msg"] = payload["reason"]
		}
		if err := weiboPayloadFailure(payload); err != nil {
			return fmt.Errorf("微博跨域会话失败：%w", err)
		}
		code, result := jsonValueString(payload["retcode"]), jsonValueString(payload["result"])
		if result == "false" || code != "" && code != "0" && code != "20000000" {
			return errors.New("微博跨域会话回调未确认成功，请在官方页面检查后重新扫码")
		}
		if code == "" && result != "true" && jsonValueString(payload["ok"]) != "1" {
			return errors.New("微博跨域会话回调缺少成功标记，请重新扫码")
		}
		containers := []map[string]any{payload}
		if data, ok := payload["data"].(map[string]any); ok {
			containers = append(containers, data)
		}
		for _, container := range containers {
			for _, name := range []string{"arrURL", "crossDomainUrlList"} {
				if entries, ok := container[name].([]any); ok {
					if len(queue)+len(entries) > limit {
						return errors.New("微博跨域会话步骤超过限制，请重新扫码")
					}
					for _, entry := range entries {
						queue = append(queue, jsonValueString(entry))
					}
				}
			}
		}
	}
	return nil
}

func weiboSiteCookie(client *http.Client, rawURL string) string {
	if client.Jar == nil {
		return ""
	}
	target, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := []string{}
	for _, cookie := range client.Jar.Cookies(target) {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return normalizeWeiboCookie(strings.Join(parts, "; "))
}
