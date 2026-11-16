package main

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

type credentialValue struct {
	field string
	value string
}

// Restrict this projection to the fields shown by the web credential forms.
func savedCredentialValues(config *BilibiliConfig, scope string) []credentialValue {
	switch scope {
	case "telegram":
		return []credentialValue{{"token", config.Telegram.Token}}
	case "bilibili":
		account := config.Credentials
		return []credentialValue{
			{"cookie", account.Cookie}, {"SESSDATA", account.SESSDATA},
			{"bili_jct", account.BiliJCT}, {"buvid3", account.Buvid3}, {"DedeUserID", account.DedeUserID},
		}
	case "weibo":
		account := config.Weibo
		return []credentialValue{
			{"username", account.PasswordUsername}, {"password", account.Password},
			{"userId", account.UserID}, {"cookie", account.Cookie},
		}
	case "pixiv":
		account := config.Pixiv
		return []credentialValue{
			{"userAgent", account.UserAgent}, {"userId", account.UserID}, {"baggage", account.Baggage},
			{"cookie", account.Cookie}, {"sentryTrace", account.SentryTrace}, {"csrfToken", account.CSRFToken},
		}
	case "twitter":
		account := config.Twitter
		return []credentialValue{{"apiKey", account.APIKey}, {"username", account.UserName}}
	default:
		return nil
	}
}

func (b *BilibiliStore) credentialValuesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		writeAPIError(w, http.StatusForbidden, "只能在当前站点查看凭证")
		return
	}
	var scope, field string
	switch r.Method {
	case http.MethodGet:
		scope = r.URL.Query().Get("scope")
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeAPIError(w, http.StatusUnsupportedMediaType, "请求必须使用 JSON")
			return
		}
		var input struct {
			Scope string `json:"scope"`
			Field string `json:"field"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Field == "" {
			writeAPIError(w, http.StatusBadRequest, "凭证读取请求无效")
			return
		}
		scope, field = input.Scope, input.Field
	default:
		w.Header().Set("Allow", "GET, POST")
		writeAPIError(w, http.StatusMethodNotAllowed, "不支持此请求方法")
		return
	}

	b.RLock()
	values := savedCredentialValues(&b.config, scope)
	b.RUnlock()
	if values == nil {
		writeAPIError(w, http.StatusBadRequest, "不支持此凭证类型")
		return
	}
	if r.Method == http.MethodGet {
		configured := make([]string, 0, len(values))
		for _, item := range values {
			if item.value != "" {
				configured = append(configured, item.field)
			}
		}
		writeJSON(w, map[string][]string{"configured": configured})
		return
	}
	for _, item := range values {
		if item.field != field {
			continue
		}
		if item.value == "" {
			writeAPIError(w, http.StatusNotFound, "此字段尚未保存")
			return
		}
		writeJSON(w, map[string]string{"value": item.value})
		return
	}
	writeAPIError(w, http.StatusBadRequest, "不支持查看此字段")
}
