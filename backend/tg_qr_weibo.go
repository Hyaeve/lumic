package main

import (
	"context"
	"strings"
)

func checkTGQRWeibo(ctx context.Context, credentials WeiboCredentials, proxyURL string) tgQRCredentialStatus {
	if ctx.Err() != nil {
		return tgQRCredentialUnknown
	}
	credentials.Cookie = normalizeWeiboCookie(credentials.Cookie)
	credentials.MobileCookie = normalizeWeiboCookie(credentials.MobileCookie)
	credentials.UserID = strings.TrimSpace(credentials.UserID)
	desktop := hasTGQRWeiboSessionCookie(credentials.Cookie)
	mobile := hasTGQRWeiboSessionCookie(credentials.MobileCookie)
	if !validWeiboUID(credentials.UserID) || !desktop && !mobile {
		return tgQRCredentialInvalid
	}

	var err error
	if !desktop {
		err = validateWeiboMobileIdentityContext(ctx, &credentials, proxyURL)
	} else {
		var routeMissing bool
		routeMissing, err = validateWeiboDesktopIdentityContext(ctx, &credentials, proxyURL, credentials.QRConnected)
		legacyMobile := !credentials.CookieScoped && credentials.MobileCookie == ""
		if routeMissing {
			// A mobile failure cannot prove a possibly valid desktop session expired.
			if (mobile || legacyMobile) && validateWeiboMobileIdentityContext(ctx, &credentials, proxyURL) == nil {
				err = nil
			}
		} else if mobile && weiboErrorKind(err) == "auth" {
			err = validateWeiboMobileIdentityContext(ctx, &credentials, proxyURL)
		}
	}
	if ctx.Err() != nil {
		return tgQRCredentialUnknown
	}
	if err == nil {
		return tgQRCredentialValid
	}
	switch weiboErrorKind(err) {
	case "auth", "identity":
		return tgQRCredentialInvalid
	default:
		return tgQRCredentialUnknown
	}
}

func hasTGQRWeiboSessionCookie(cookie string) bool {
	return weiboCookieValue(cookie, "SUB") != "" || weiboCookieValue(cookie, "SUBP") != ""
}
