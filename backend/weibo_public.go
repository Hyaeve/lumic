package main

import (
	"errors"
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
	payload, _, err := weiboSessionRequest("https://weibo.com/ajax/config/get", credentials, proxyURL)
	if err != nil {
		return err
	}
	data, _ := payload["data"].(map[string]any)
	userID := jsonValueString(data["uid"])
	loggedIn := firstNonEmptyRemoteText(jsonValueString(data["islogin"]), jsonValueString(data["login"]))
	if userID == "" || userID == "0" || loggedIn != "true" && loggedIn != "1" {
		return &weiboRequestError{"auth", "微博 Cookie 未通过登录状态验证，请重新扫码或导入有效凭证"}
	}
	if userID != credentials.UserID {
		return errors.New("微博 Cookie 对应账号与填写的 UID 不一致")
	}
	return nil
}
