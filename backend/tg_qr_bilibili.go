package main

import (
	"context"
	"encoding/json"
	"strconv"
)

func checkTGQRBilibili(ctx context.Context, credentials BilibiliCredentials, proxyURL string) tgQRCredentialStatus {
	if ctx.Err() != nil {
		return tgQRCredentialUnknown
	}
	credentials = normalizeBilibiliCredentials(credentials)
	expectedMID, err := strconv.ParseInt(credentials.DedeUserID, 10, 64)
	if credentials.SESSDATA == "" || err != nil || expectedMID <= 0 {
		return tgQRCredentialInvalid
	}

	var response struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := bilibiliRequestContext(ctx, "https://api.bilibili.com/x/web-interface/nav", credentials, proxyURL, &response); err != nil || response.Code == nil {
		return tgQRCredentialUnknown
	}
	switch *response.Code {
	case -101:
		return tgQRCredentialInvalid
	case 0:
	default:
		return tgQRCredentialUnknown
	}

	var identity struct {
		IsLogin *bool           `json:"isLogin"`
		MID     json.RawMessage `json:"mid"`
	}
	if err := json.Unmarshal(response.Data, &identity); err != nil || identity.IsLogin == nil {
		return tgQRCredentialUnknown
	}
	if !*identity.IsLogin {
		return tgQRCredentialInvalid
	}
	var actualMID int64
	if err := json.Unmarshal(identity.MID, &actualMID); err != nil || actualMID <= 0 {
		return tgQRCredentialUnknown
	}
	if actualMID != expectedMID {
		return tgQRCredentialInvalid
	}
	return tgQRCredentialValid
}
