package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func validWeiboUID(value string) bool {
	if value == "" || len(value) > 20 || strings.Trim(value, "0") == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Parse an ID, never a user-supplied request URL. All network endpoints remain
// fixed to the platform's normal public profile/timeline interfaces.
func weiboAuthorID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if validWeiboUID(value) {
		return value, nil
	}
	if strings.HasPrefix(value, "weibo.com/") || strings.HasPrefix(value, "www.weibo.com/") || strings.HasPrefix(value, "m.weibo.cn/") {
		value = "https://" + value
	}
	if !strings.Contains(value, "://") {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Port() != "" || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("请输入微博 UID 或微博博主主页链接")
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "weibo.com", "www.weibo.com", "m.weibo.cn":
	default:
		return "", errors.New("仅支持微博官方博主主页链接")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 2 && (parts[0] == "u" || parts[0] == "profile") {
		parts = parts[1:]
	}
	if len(parts) != 1 || !validWeiboUID(parts[0]) {
		return "", errors.New("主页链接应包含数字 UID，也可以直接填写 UID")
	}
	return parts[0], nil
}

func (b *BilibiliStore) weiboPublicRequest(endpoint string, credentials *WeiboCredentials, proxyURL string) (map[string]any, int, error) {
	if hasWeiboAccount(*credentials) {
		payload, status, err := b.weiboRawRequest(endpoint, credentials, proxyURL)
		if !isWeiboSessionFailure(err) {
			return payload, status, err
		}
	}
	public := WeiboCredentials{}
	payload, status, err := weiboSessionRequest(endpoint, &public, proxyURL)
	if !isWeiboSessionFailure(err) {
		return payload, status, err
	}
	return nil, status, &weiboRequestError{"auth", "微博当前要求登录才能访问该内容，请在设置中连接微博账号后重试"}
}

func (b *BilibiliStore) fetchPublicWeiboUser(userID string, credentials *WeiboCredentials, proxyURL string) (WeiboUser, error) {
	endpoint := "https://m.weibo.cn/api/container/getIndex?type=uid&value=" + url.QueryEscape(userID) + "&containerid=" + url.QueryEscape("100505"+userID)
	payload, _, err := b.weiboPublicRequest(endpoint, credentials, proxyURL)
	if err != nil {
		return WeiboUser{}, err
	}
	users := make([]WeiboUser, 0)
	collectWeiboUsers(payload, &users, make(map[string]bool))
	for _, user := range users {
		if user.UserID == userID {
			return user, nil
		}
	}
	return WeiboUser{}, errors.New("微博未返回该 UID 的公开博主资料，请核对 UID 或连接账号后重试")
}

func validateWeiboSessionIdentity(credentials *WeiboCredentials, proxyURL string) error {
	return validateWeiboIdentityWithQRProof(credentials, proxyURL, false)
}

func validateWeiboQRSessionIdentity(credentials *WeiboCredentials, proxyURL string) error {
	return validateWeiboIdentityWithQRProof(credentials, proxyURL, true)
}

func validateWeiboIdentityWithQRProof(credentials *WeiboCredentials, proxyURL string, confirmedQR bool) error {
	source := "微博桌面会话接口"
	payload, status, err := weiboSessionRequest("https://weibo.com/ajax/config/get", credentials, proxyURL)
	// A missing route is a compatibility issue, not proof of an expired login.
	// Do not retry authentication, captcha, throttling or other upstream failures.
	if err != nil && (status == http.StatusNotFound || status == http.StatusGone) {
		// QR exchange already bound this cookie jar to a confirmed UID. A missing
		// desktop identity route must not make an independent mobile login mandatory.
		if confirmedQR && credentials.CookieScoped && validWeiboUID(credentials.UserID) && weiboCookieValue(credentials.Cookie, "SUB") != "" {
			endpoint := "https://weibo.com/ajax/statuses/likelist?" + url.Values{"uid": {credentials.UserID}, "page": {"1"}, "relate": {"fans"}}.Encode()
			var probe map[string]any
			probe, status, err = weiboSessionRequest(endpoint, credentials, proxyURL)
			if err == nil {
				if jsonValueString(probe["ok"]) != "1" {
					return &weiboRequestError{"upstream", "微博桌面账号接口未明确确认成功，暂不能激活扫码会话"}
				}
				containers := []map[string]any{probe}
				if data, ok := probe["data"].(map[string]any); ok {
					containers = append(containers, data)
				}
				validList := false
				for _, container := range containers {
					if uid := jsonValueString(container["uid"]); uid != "" && uid != credentials.UserID {
						return errors.New("微博桌面账号接口返回的 UID 与扫码确认账号不一致")
					}
					for _, field := range []string{"statuses", "list"} {
						if _, ok := container[field].([]any); ok {
							validList = true
						}
					}
				}
				if !validList {
					return &weiboRequestError{"upstream", "微博桌面账号接口未返回可识别的点赞列表，暂不能激活扫码会话"}
				}
				return nil
			}
			if status != http.StatusNotFound && status != http.StatusGone {
				return fmt.Errorf("微博桌面扫码会话可用性验证失败：%w", err)
			}
		}
		source = "微博移动端会话接口"
		payload, _, err = weiboSessionRequest("https://m.weibo.cn/api/config", credentials, proxyURL)
		if err != nil {
			return fmt.Errorf("微博移动端会话校验接口异常：%w", err)
		}
	}
	if err != nil {
		return fmt.Errorf("微博桌面会话校验接口异常：%w", err)
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil || data["uid"] == nil && data["islogin"] == nil && data["login"] == nil {
		data = payload
	}
	userID := jsonValueString(data["uid"])
	loggedIn := firstNonEmptyRemoteText(jsonValueString(data["islogin"]), jsonValueString(data["login"]))
	if loggedIn == "false" || loggedIn == "0" {
		return &weiboRequestError{"auth", source + "明确返回未登录，当前 Cookie 未建立可验证的站点登录会话"}
	}
	if loggedIn != "true" && loggedIn != "1" {
		return &weiboRequestError{"upstream", source + "缺少或无法识别登录标记，暂不能判定 Cookie 是否有效"}
	}
	if !validWeiboUID(userID) {
		return &weiboRequestError{"upstream", source + "未返回有效 UID，暂不能确认登录身份"}
	}
	if userID != credentials.UserID {
		return errors.New("微博 Cookie 对应账号与填写的 UID 不一致")
	}
	return nil
}
