package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWeiboQRReturnsOnlyNewValidatedCookieOnce(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "42", Cookie: "SUB=old", PasswordUsername: "fixture", Password: "backup-secret"})
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if valid {
					fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","islogin":true}}`)
				} else {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"ok":0,"msg":"验证失败"}`)
				}
			})
			previousProfile := fetchWeiboAccountProfile
			fetchWeiboAccountProfile = func(string, WeiboCredentials, string) (WeiboUser, error) {
				return WeiboUser{UserID: "42", Name: "示例账号"}, nil
			}
			t.Cleanup(func() { fetchWeiboAccountProfile = previousProfile })
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sso/qrcode/check":
					fmt.Fprint(w, `{"retcode":20000000,"data":{"alt":"fixture-ticket"}}`)
				case "/sso/login.php":
					fmt.Fprint(w, `{"retcode":0,"uid":"42","nick":"示例账号"}`)
				}
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			jar, _ := cookiejar.New(nil)
			cookieURL, _ := url.Parse("https://weibo.com/")
			jar.SetCookies(cookieURL, []*http.Cookie{{Name: "SUB", Value: "new-qr-fixture"}, {Name: "SUBP", Value: "new-qr-profile"}})
			client := &http.Client{Jar: jar, Transport: weiboTestTransport{target, upstream.Client().Transport}}
			store.weiboQR = map[string]WeiboQRSession{"fixture": {ID: "fixture", QRID: "ticket", ExpiresAt: time.Now().Add(time.Minute)}}
			store.weiboClients = map[string]*http.Client{"fixture": client}
			response := httptest.NewRecorder()
			store.weiboQRHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("QR response must never be cached")
			}
			if !valid {
				if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "new-qr-fixture") {
					t.Fatal("failed validation exposed a Cookie or reported success")
				}
				return
			}
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || result["status"] != "connected" || result["userId"] != "42" || result["cookie"] != store.config.Weibo.Cookie || !strings.Contains(result["cookie"].(string), "SUB=new-qr-fixture") || result["passwordConfigured"] != true {
				t.Fatal("QR exchange did not return the validated saved credentials")
			}
			if strings.Contains(response.Body.String(), "backup-secret") {
				t.Fatal("QR response exposed backup password")
			}
			stored, err := os.ReadFile(bilibiliFile)
			if err != nil || strings.Contains(string(stored), "new-qr-fixture") {
				t.Fatal("QR credentials were not persisted encrypted")
			}
			for _, path := range []string{"/api/weibo/qr", "/api/weibo/qr?id=fixture", "/api/weibo/account"} {
				repeat := httptest.NewRecorder()
				if path == "/api/weibo/account" {
					store.weiboAccountHandler(repeat, httptest.NewRequest(http.MethodGet, path, nil))
				} else {
					store.weiboQRHandler(repeat, httptest.NewRequest(http.MethodGet, path, nil))
				}
				if strings.Contains(repeat.Body.String(), "new-qr-fixture") || strings.Contains(repeat.Body.String(), `"cookie":`) || strings.Contains(repeat.Body.String(), "backup-secret") {
					t.Fatal("account query or consumed exchange exposed saved secrets")
				}
			}
		})
	}
}

func TestWeiboQRRequiresLumicSession(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{Cookie: "SUB=fixture"})
	response := httptest.NewRecorder()
	authMiddleware(newSessionStore(nil), http.HandlerFunc(store.weiboQRHandler)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "SUB=") {
		t.Fatal("unauthenticated QR request was not blocked")
	}
}
