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
	for _, scenario := range []struct {
		name          string
		desktopStatus int
		mobileStatus  int
		mobileBody    string
		valid         bool
	}{
		{"desktop-success", 200, 0, "", true},
		{"desktop-forbidden", 403, 0, "", false},
		{"desktop-404-mobile-success", 404, 200, `{"ok":1,"data":{"uid":"42","login":true}}`, true},
		{"desktop-410-mobile-success", 410, 200, `{"ok":1,"data":{"uid":"42","login":true}}`, true},
		{"mobile-forbidden", 404, 403, `{}`, false},
		{"mobile-missing", 404, 404, `{}`, false},
		{"mobile-limited", 404, 429, `{}`, false},
		{"mobile-anonymous", 404, 200, `{"ok":1,"data":{"login":false}}`, false},
		{"mobile-other-account", 404, 200, `{"ok":1,"data":{"uid":"99","login":true}}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "42", Cookie: "SUB=old", PasswordUsername: "fixture", Password: "backup-secret"})
			mobileCalls := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/config" {
					mobileCalls++
					if scenario.mobileStatus == 0 {
						t.Error("unexpected mobile retry")
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(scenario.mobileStatus)
					fmt.Fprint(w, scenario.mobileBody)
					return
				}
				if r.URL.Path != "/ajax/config/get" {
					t.Error("unexpected identity endpoint")
				}
				w.WriteHeader(scenario.desktopStatus)
				if scenario.desktopStatus == http.StatusOK {
					fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","islogin":true}}`)
				} else {
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
			if (scenario.mobileStatus != 0 && mobileCalls != 1) || (scenario.mobileStatus == 0 && mobileCalls != 0) {
				t.Fatal("QR validation used the wrong retry policy")
			}
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || result["status"] != "connected" || result["configured"] != true || result["qrConnected"] != true || result["userId"] != "42" || !strings.Contains(result["cookie"].(string), "SUB=new-qr-fixture") || result["passwordConfigured"] != true || result["credentialVerified"] != scenario.valid {
				t.Fatal("confirmed QR exchange did not update the connection status")
			}
			if scenario.valid && (result["cookie"] != store.config.Weibo.Cookie || result["connectionWarning"] != "") {
				t.Fatal("validated QR credentials were not activated")
			}
			if !scenario.valid && (store.config.Weibo.Cookie != "SUB=old" || result["connectionWarning"] == "") {
				t.Fatal("unverified QR Cookie replaced active credentials or lost its warning")
			}
			if strings.Contains(response.Body.String(), "backup-secret") {
				t.Fatal("QR response exposed backup password")
			}
			stored, err := os.ReadFile(bilibiliFile)
			if err != nil || strings.Contains(string(stored), "new-qr-fixture") {
				t.Fatal("QR credentials were not persisted encrypted")
			}
			var reloaded BilibiliConfig
			if err := decryptJSON(store.key, stored, &reloaded); err != nil || !reloaded.Weibo.QRConnected {
				t.Fatal("QR connection status did not survive encrypted persistence")
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
