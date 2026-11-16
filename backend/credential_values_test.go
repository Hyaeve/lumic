package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCredentialValuesOnlyRevealRequestedField(t *testing.T) {
	store := &BilibiliStore{config: BilibiliConfig{
		Telegram: TGBotConfig{Token: "fixture-telegram-token"},
		Credentials: BilibiliCredentials{
			Cookie: "fixture-bilibili-cookie", SESSDATA: "fixture-sessdata", BiliJCT: "fixture-bili-jct",
			Buvid3: "fixture-buvid3", DedeUserID: "123", AccessTimeValue: "not-a-web-form-field",
		},
		Weibo: WeiboCredentials{
			PasswordUsername: "fixture-weibo-user", Password: " fixture-weibo-password ",
			UserID: "234", Cookie: "fixture-weibo-cookie", MobileCookie: "not-a-web-form-field",
		},
		Pixiv: PixivCredentials{
			UserAgent: "fixture-ua", UserID: "345", Baggage: "fixture-baggage",
			Cookie: "fixture-pixiv-cookie", SentryTrace: "fixture-sentry", CSRFToken: "fixture-csrf",
			RefreshToken: "not-a-web-form-field",
		},
		Twitter: TwitterCredentials{APIKey: "fixture-twitter-key", UserName: "fixture-twitter-handle"},
	}}
	const path = "/api/credential-values"
	token := "fixture-credential-value-session"
	sessions := newSessionStore(nil)
	sessions.records[sessionDigest(token)] = sessionRecord{Expires: time.Now().Add(time.Hour)}
	// All /api/ routes use the same middleware and authenticated browser cookie.
	mux := http.NewServeMux()
	mux.HandleFunc(path, store.credentialValuesHandler)
	handler := authMiddleware(sessions, mux)
	serve := func(method, target, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
		r.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("missing sensitive-response headers: %v", w.Header())
		}
		return w
	}
	for _, scope := range []struct {
		name       string
		configured []string
		field      string
		value      string
	}{
		{"telegram", []string{"token"}, "token", "fixture-telegram-token"},
		{"bilibili", []string{"cookie", "SESSDATA", "bili_jct", "buvid3", "DedeUserID"}, "SESSDATA", "fixture-sessdata"},
		{"weibo", []string{"username", "password", "userId", "cookie"}, "password", " fixture-weibo-password "},
		{"pixiv", []string{"userAgent", "userId", "baggage", "cookie", "sentryTrace", "csrfToken"}, "cookie", "fixture-pixiv-cookie"},
		{"twitter", []string{"apiKey", "username"}, "username", "fixture-twitter-handle"},
	} {
		t.Run(scope.name, func(t *testing.T) {
			w := serve(http.MethodGet, path+"?scope="+scope.name+"&field="+scope.field, "")
			var metadata map[string][]string
			err := json.Unmarshal(w.Body.Bytes(), &metadata)
			if w.Code != http.StatusOK || err != nil || !reflect.DeepEqual(metadata, map[string][]string{"configured": scope.configured}) {
				t.Fatalf("unexpected credential metadata: %d %s", w.Code, w.Body)
			}
			payload, err := json.Marshal(map[string]string{"scope": scope.name, "field": scope.field})
			if err != nil {
				t.Fatal(err)
			}
			w = serve(http.MethodPost, path, string(payload))
			var reveal map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &reveal)
			if w.Code != http.StatusOK || err != nil || !reflect.DeepEqual(reveal, map[string]string{"value": scope.value}) {
				t.Fatalf("unexpected single-field reveal: %d %s", w.Code, w.Body)
			}
		})
	}
	for _, body := range []string{
		`{"scope":"bilibili","field":"ac_time_value"}`,
		`{"scope":"weibo","field":"mobileCookie"}`,
		`{"scope":"pixiv","field":"refreshToken"}`,
		`{"scope":"project","field":"proxy"}`,
		`{"scope":"telegram","field":"Token"}`,
		`{"scope":"telegram"}`,
		`{"scope":"telegram","field":"token","extra":true}`,
		`{"scope":"telegram","field":"token"}{}`,
	} {
		w := serve(http.MethodPost, path, body)
		if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "fixture-") || strings.Contains(w.Body.String(), "not-a-web-form-field") {
			t.Fatalf("invalid credential request accepted: %s (%d %s)", body, w.Code, w.Body)
		}
	}
	store.config.Weibo = WeiboCredentials{MobileCookie: "not-a-web-form-field"}
	if w := serve(http.MethodGet, path+"?scope=weibo", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"configured":[]}` {
		t.Fatalf("missing fields must have empty metadata: %d %s", w.Code, w.Body)
	}
	if w := serve(http.MethodPost, path, `{"scope":"weibo","field":"cookie"}`); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "not-a-web-form-field") {
		t.Fatalf("missing desktop cookie was exposed: %d %s", w.Code, w.Body)
	}
}

func TestCredentialValuesRequiresSessionAndJSON(t *testing.T) {
	store := &BilibiliStore{config: BilibiliConfig{Telegram: TGBotConfig{Token: "fixture-private-generator-token"}}}
	sessions := newSessionStore(nil)
	sessions.records[sessionDigest("current")] = sessionRecord{Expires: time.Now().Add(time.Hour)}
	sessions.records[sessionDigest("expired")] = sessionRecord{Expires: time.Now().Add(-time.Hour)}
	handler := authMiddleware(sessions, http.HandlerFunc(store.credentialValuesHandler))
	for _, test := range []struct {
		name, session, site, contentType string
		status                           int
	}{
		{"unauthenticated", "", "same-origin", "application/json", http.StatusUnauthorized},
		{"expired", "expired", "same-origin", "application/json", http.StatusUnauthorized},
		{"cross-site", "current", "cross-site", "application/json", http.StatusForbidden},
		{"same-site sibling origin", "current", "same-site", "application/json", http.StatusForbidden},
		{"simple cross-origin body without fetch metadata", "current", "", "text/plain", http.StatusUnsupportedMediaType},
		{"missing content type", "current", "same-origin", "", http.StatusUnsupportedMediaType},
		{"older same-origin browser sending JSON", "current", "", "application/json", http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/credential-values", strings.NewReader(`{"scope":"telegram","field":"token"}`))
			r.Header.Set("Sec-Fetch-Site", test.site)
			r.Header.Set("Content-Type", test.contentType)
			if test.session != "" {
				r.AddCookie(&http.Cookie{Name: "lumic_session", Value: test.session})
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			revealed := strings.Contains(w.Body.String(), "fixture-private-generator-token")
			if w.Code != test.status || revealed != (test.status == http.StatusOK) {
				t.Fatalf("unexpected status or reveal: %d (expected %d), revealed=%t", w.Code, test.status, revealed)
			}
		})
	}
}
