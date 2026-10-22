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

func prepareWeiboConnectionQR(t *testing.T, store *BilibiliStore, loginBody string, cookie bool) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sso/qrcode/check":
			fmt.Fprint(w, `{"retcode":20000000,"data":{"alt":"fixture-ticket"}}`)
		case "/sso/login.php":
			fmt.Fprint(w, loginBody)
		case "/sso/crossdomain.php":
			fmt.Fprint(w, `cb({"retcode":0,"arrURL":[]})`)
		}
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	jar, _ := cookiejar.New(nil)
	if cookie {
		cookieURL, _ := url.Parse("https://weibo.com/")
		jar.SetCookies(cookieURL, []*http.Cookie{{Name: "SUB", Value: "pending-qr-fixture"}})
	}
	client := &http.Client{Jar: jar, Transport: weiboTestTransport{target, server.Client().Transport}}
	store.weiboQR = map[string]WeiboQRSession{"fixture": {ID: "fixture", QRID: "ticket", ExpiresAt: time.Now().Add(time.Minute)}}
	store.weiboClients = map[string]*http.Client{"fixture": client}
}

func TestWeiboConfirmedQRConnectsWithoutUsableCredentials(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		previous WeiboCredentials
		cookie   bool
		ready    bool
	}{
		{"no-cookie-or-password", WeiboCredentials{}, false, false},
		{"unverified-cookie", WeiboCredentials{}, true, false},
		{"same-account-keeps-active-session", WeiboCredentials{UserID: "42", Cookie: "SUB=old", PasswordUsername: "fixture", Password: "old-password"}, false, true},
		{"switch-account-with-no-cookie", WeiboCredentials{UserID: "99", Cookie: "SUB=other", PasswordUsername: "other", Password: "other-password"}, false, false},
		{"switch-account-with-unverified-cookie", WeiboCredentials{UserID: "99", Cookie: "SUB=other", MobileCookie: "SUB=other-mobile", CookieScoped: true, PasswordUsername: "other", Password: "other-password"}, true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, scenario.previous)
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
			prepareWeiboConnectionQR(t, store, `{"retcode":0,"uid":"42","nick":"示例账号"}`, scenario.cookie)
			response := httptest.NewRecorder()
			store.weiboQRHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || result["status"] != "connected" || result["configured"] != true || result["qrConnected"] != true || result["sessionReady"] != scenario.ready || result["credentialVerified"] != false || result["connectionWarning"] == "" {
				t.Fatal("confirmed QR did not persist a connected state separate from credential readiness")
			}
			if (result["cookie"] != "") != scenario.cookie {
				t.Fatal("missing Cookie was fabricated or newly captured Cookie was lost")
			}
			if scenario.previous.UserID == "42" {
				if store.config.Weibo.Cookie != scenario.previous.Cookie || store.config.Weibo.Password != scenario.previous.Password {
					t.Fatal("same-account pending QR discarded active credentials")
				}
			} else if store.config.Weibo.Cookie != "" || store.config.Weibo.MobileCookie != "" || hasWeiboPassword(store.config.Weibo) {
				t.Fatal("pending account switch inherited another account's credentials")
			}
			data, err := os.ReadFile(bilibiliFile)
			var reloaded BilibiliConfig
			if err != nil {
				t.Fatal(err)
			}
			if err = decryptJSON(store.key, data, &reloaded); err != nil || !reloaded.Weibo.QRConnected || reloaded.Weibo.UserID != "42" {
				t.Fatal("connection was lost after persistence")
			}
			if strings.Contains(string(data), "pending-qr-fixture") {
				t.Fatal("saved data was not encrypted")
			}
			if !scenario.ready {
				view := httptest.NewRecorder()
				store.weiboAccountHandler(view, httptest.NewRequest(http.MethodGet, "/api/weibo/account", nil))
				var account map[string]any
				if err = json.Unmarshal(view.Body.Bytes(), &account); err != nil || account["configured"] != true || account["sessionReady"] != false || account["passwordConfigured"] != false {
					t.Fatal("account query lost pending connection")
				}
				if strings.Contains(view.Body.String(), `"cookie":`) {
					t.Fatal("account query returned a secret")
				}
				likes := httptest.NewRecorder()
				store.weiboSubscriptionsHandler(likes, httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions?type=likes", nil))
				if likes.Code != http.StatusPreconditionFailed {
					t.Fatal("QR metadata without credentials unlocked account likes")
				}
			}
		})
	}
}

func TestWeiboQRConnectionSaveFailureRollsBack(t *testing.T) {
	previous := WeiboCredentials{UserID: "99", Cookie: "SUB=old"}
	store := newWeiboAuthTestStore(t, previous)
	if err := store.save(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(bilibiliFile)
	if err != nil {
		t.Fatal(err)
	}
	prepareWeiboConnectionQR(t, store, `{"retcode":0,"uid":"42"}`, false)
	store.key = []byte("invalid-key")
	response := httptest.NewRecorder()
	store.weiboQRHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
	if response.Code != http.StatusInternalServerError || store.config.Weibo != previous || strings.Contains(response.Body.String(), `"status":"connected"`) {
		t.Fatal("failed persistence reported a connected state or changed the saved account")
	}
	if _, exists := store.weiboQR["fixture"]; !exists {
		t.Fatal("failed save consumed the QR session")
	}
	after, err := os.ReadFile(bilibiliFile)
	if err != nil || string(before) != string(after) {
		t.Fatal("failed save modified the previous persisted configuration")
	}
}

func TestWeiboQRDoesNotConnectWhenSSORejectsLogin(t *testing.T) {
	for _, body := range []string{`{"retcode":1,"uid":"42"}`, `{"retcode":0}`, `{"retcode":0,"uid":"not-a-uid"}`} {
		t.Run(body, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{})
			prepareWeiboConnectionQR(t, store, body, false)
			response := httptest.NewRecorder()
			store.weiboQRHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
			if response.Code != http.StatusBadGateway || store.config.Weibo.QRConnected {
				t.Fatal("unconfirmed SSO login became connected")
			}
		})
	}
}

func TestWeiboManualSaveClearsPendingConnectionWarning(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{QRConnected: true, UserID: "42", ConnectionWarning: "扫码已连接，凭证待验证"})
	previousValidator := validateWeiboLoginSession
	validateWeiboLoginSession = func(string, string, string) (WeiboCredentials, error) {
		return WeiboCredentials{Cookie: "SUB=validated", UserID: "42", UserName: "示例账号"}, nil
	}
	t.Cleanup(func() { validateWeiboLoginSession = previousValidator })
	response := httptest.NewRecorder()
	store.weiboAccountHandler(response, httptest.NewRequest(http.MethodPut, "/api/weibo/account", strings.NewReader(`{"cookie":"SUB=validated","userId":"42"}`)))
	if response.Code != http.StatusOK || store.config.Weibo.ConnectionWarning != "" || !hasWeiboAccount(store.config.Weibo) {
		t.Fatal("validated credentials did not clear the pending connection warning")
	}
	data, err := os.ReadFile(bilibiliFile)
	var reloaded BilibiliConfig
	if err != nil {
		t.Fatal(err)
	}
	if err := decryptJSON(store.key, data, &reloaded); err != nil || reloaded.Weibo.ConnectionWarning != "" {
		t.Fatal("cleared warning did not persist")
	}
}
