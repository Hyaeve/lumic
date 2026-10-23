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

func allowedWeiboSSOHost(host string) bool {
	switch host {
	case "login.sina.com.cn", "passport.weibo.com", "weibo.com", "www.weibo.com", "passport.weibo.cn", "m.weibo.cn", "passport.sina.cn":
		return true
	}
	return false
}

func allowedWeiboSSOTarget(target *url.URL) bool {
	return strings.EqualFold(target.Scheme, "https") && target.User == nil &&
		(target.Port() == "" || target.Port() == "443") &&
		allowedWeiboSSOHost(strings.ToLower(strings.TrimSuffix(target.Hostname(), ".")))
}

func isWeiboSSOAncillaryURL(rawURL string) bool {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.User != nil || strings.ToLower(strings.TrimSuffix(target.Hostname(), ".")) != "passport.krcom.cn" {
		return false
	}
	scheme, port := strings.ToLower(target.Scheme), target.Port()
	if scheme != "" && scheme != "https" && scheme != "http" ||
		(scheme == "http" && port != "" && port != "80") ||
		(scheme != "http" && port != "" && port != "443") {
		return false
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return false
	}
	for _, action := range query["action"] {
		if action != "" && action != "login" {
			return false
		}
	}
	return true
}

func weiboCrossDomainURL(rawURL string, index int) (string, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", errors.New("微博跨域会话地址无法解析，已停止交换（地址参数未显示）")
	}
	if target.User != nil {
		return "", errors.New("微博跨域会话地址含有不允许的账号信息，已停止交换")
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if !allowedWeiboSSOHost(host) {
		// Do not echo the full URL: its path, query or fragment may contain tickets.
		label := "无法识别"
		if host != "" && len(host) <= 253 && strings.Trim(host, "abcdefghijklmnopqrstuvwxyz0123456789.-") == "" {
			label = host
		}
		return "", fmt.Errorf("微博跨域会话地址的域名不受支持（域名：%s），已停止交换", label)
	}
	target.Scheme = strings.ToLower(target.Scheme)
	if target.Scheme == "" {
		target.Scheme = "https"
	}
	port := target.Port()
	if target.Scheme == "http" && (port == "" || port == "80") {
		target.Scheme = "https"
		port = ""
	}
	if target.Scheme != "https" {
		return "", errors.New("微博跨域会话地址的协议不受支持，已停止交换")
	}
	if port != "" && port != "443" {
		return "", errors.New("微博跨域会话地址使用非标准端口，已停止交换")
	}
	target.Host = host
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return "", errors.New("微博跨域会话地址参数无法解析，已停止交换（参数内容未显示）")
	}
	for _, action := range query["action"] {
		if action != "" && action != "login" {
			return "", errors.New("微博跨域会话地址请求的不是登录操作，已停止交换（操作参数未显示）")
		}
	}
	query.Set("action", "login")
	query.Set("callback", "sinaSSOController.doCrossDomainCallBack")
	query.Set("scriptId", fmt.Sprintf("ssoscript%d", index))
	if target.Host == "login.sina.com.cn" && target.Path == "/sso/crossdomain.php" {
		query.Set("callback", "sinaSSOController.crossDomainCallBack")
		query.Set("scriptId", "ssoCrossDomainScriptId")
	}
	target.RawQuery, target.Fragment = query.Encode(), ""
	return target.String(), nil
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
		// The reported companion is not a source of weibo.com/.cn cookies. Do not
		// send it a ticket or allow redirects to it; verify the Weibo session below.
		if isWeiboSSOAncillaryURL(queue[index]) {
			continue
		}
		rawURL, err := weiboCrossDomainURL(queue[index], index)
		if err != nil {
			return err
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
