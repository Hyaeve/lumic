package main

import "strings"

// Connection confirms QR consent; sessionReady describes stored fetch credentials.
// Never include saved secrets in ordinary account responses.
func weiboAccountView(account WeiboCredentials) map[string]any {
	return map[string]any{
		"configured":         account.QRConnected || hasWeiboAccount(account) || strings.TrimSpace(account.PasswordUsername) != "",
		"qrConnected":        account.QRConnected,
		"sessionReady":       hasWeiboAccount(account),
		"cookieConfigured":   account.Cookie != "" || account.MobileCookie != "",
		"passwordConfigured": hasWeiboPassword(account),
		"userId":             account.UserID,
		"userName":           account.UserName,
		"avatar":             account.Avatar,
		"connectionWarning":  account.ConnectionWarning,
	}
}
