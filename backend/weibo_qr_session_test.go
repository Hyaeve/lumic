package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestWeiboQRDesktopCapabilityDoesNotRequireMobileLogin(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		status     int
		verified   bool
	}{
		{"desktop-list", `{"ok":1,"statuses":[]}`, 200, true},
		{"desktop-nested-list", `{"ok":1,"data":{"list":[]}}`, 200, true},
		{"login-required", `{"ok":-100}`, 200, false},
		{"forbidden", `{}`, 403, false},
		{"limited", `{}`, 429, false},
		{"captcha", `{"ok":0,"msg":"需要验证码"}`, 200, false},
		{"public-profile-only", `{"ok":1,"data":{"user":{"idstr":"42"}}}`, 200, false},
		{"missing-result", `{"ok":1}`, 200, false},
		{"mismatched-account", `{"ok":1,"uid":"99","statuses":[]}`, 200, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{})
			capabilityCalls, mobileCalls, profileCalls := 0, 0, 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ajax/config/get":
					w.WriteHeader(http.StatusNotFound)
				case "/ajax/statuses/likelist":
					capabilityCalls++
					if r.URL.Query().Get("uid") != "42" || r.Header.Get("Cookie") != "SUB=pending-qr-fixture" {
						t.Error("capability probe lost the confirmed UID or desktop session")
					}
					w.WriteHeader(scenario.status)
					fmt.Fprint(w, scenario.body)
				case "/api/config":
					mobileCalls++
					fmt.Fprint(w, `{"ok":1,"data":{"login":false}}`)
				default:
					t.Error("unexpected authentication probe")
				}
			})
			previousProfile := fetchWeiboAccountProfile
			fetchWeiboAccountProfile = func(string, WeiboCredentials, string) (WeiboUser, error) {
				profileCalls++
				return WeiboUser{UserID: "42", Name: "示例账号"}, nil
			}
			t.Cleanup(func() { fetchWeiboAccountProfile = previousProfile })
			prepareWeiboConnectionQR(t, store, `{"retcode":0,"uid":"42"}`, true)
			response := httptest.NewRecorder()
			store.weiboQRHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK {
				t.Fatal("SSO connection result was lost")
			}
			if result["credentialVerified"] != scenario.verified || capabilityCalls != 1 || mobileCalls != 0 {
				t.Fatalf("desktop session depended on unrelated mobile login: verified=%v capability=%d mobile=%d", result["credentialVerified"], capabilityCalls, mobileCalls)
			}
			if scenario.verified && (!hasWeiboAccount(store.config.Weibo) || profileCalls != 1 || result["connectionWarning"] != "") {
				t.Fatal("working QR session was not automatically activated")
			}
			if !scenario.verified && (hasWeiboAccount(store.config.Weibo) || profileCalls != 0 || result["connectionWarning"] == "") {
				t.Fatal("failed probe was bypassed using public profile metadata")
			}
			if strings.Contains(response.Body.String(), `"cookie":`) || strings.Contains(response.Body.String(), "pending-qr-fixture") {
				t.Fatal("QR login must not require returning a Cookie for frontend save")
			}
		})
	}
}

func TestWeiboQRDesktopOnlySessionPersistsAndFetchesLikes(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "42", Cookie: "SUB=expired-fixture"})
	likesCalls, mobileCalls := 0, 0
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ajax/config/get" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Hostname() == "m.weibo.cn" || r.URL.Path == "/api/config" {
			mobileCalls++
			fmt.Fprint(w, `{"ok":1,"data":{"login":false}}`)
			return
		}
		if r.Header.Get("Cookie") != "SUB=desktop-qr-fixture" {
			t.Error("API request did not use the newly established desktop session")
			fmt.Fprint(w, `{"ok":-100}`)
			return
		}
		switch r.URL.Path {
		case "/ajax/statuses/likelist":
			likesCalls++
			fmt.Fprint(w, `{"ok":1,"statuses":[{"id":"12345","user":{"idstr":"99","screen_name":"示例作者"},"text_raw":"示例点赞内容"}]}`)
		case "/ajax/profile/info":
			fmt.Fprint(w, `{"ok":1,"data":{"user":{"idstr":"42","screen_name":"示例账号"}}}`)
		default:
			t.Error("unexpected API route")
		}
	})
	result := runWeiboSSOQR(t, store, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sso/qrcode/check":
			fmt.Fprint(w, `{"retcode":20000000,"data":{"alt":"fixture-ticket"}}`)
		case "/sso/login.php":
			fmt.Fprint(w, `{"retcode":0,"uid":"42","crossDomainUrlList":["https://passport.weibo.com/wbsso/login?ticket=desktop-fixture"]}`)
		case "/wbsso/login":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "desktop-qr-fixture", Domain: ".weibo.com", Path: "/", Secure: true})
			fmt.Fprint(w, `cb({"retcode":0})`)
		default:
			t.Error("unexpected QR route")
		}
	})
	if result["credentialVerified"] != true || result["sessionReady"] != true || result["connectionWarning"] != "" || mobileCalls != 0 || likesCalls != 1 {
		t.Fatal("desktop-only QR session still required mobile login or another save operation")
	}
	if _, present := result["cookie"]; present || store.config.Weibo.MobileCookie != "" || store.config.Weibo.Cookie != "SUB=desktop-qr-fixture" || hasWeiboPassword(store.config.Weibo) {
		t.Fatal("QR leaked credentials, fabricated a mobile/password credential or retained the stale session")
	}
	stored, err := os.ReadFile(bilibiliFile)
	var restored BilibiliConfig
	if err != nil || strings.Contains(string(stored), "desktop-qr-fixture") || decryptJSON(store.key, stored, &restored) != nil || restored.Weibo != store.config.Weibo {
		t.Fatal("desktop-only QR session did not survive encrypted persistence")
	}
	posts, err := store.fetchWeiboLikedPosts(SourceConfig{ID: "weibo-likes-42", Source: SourceWeibo}, false)
	if err != nil || len(posts) != 1 || likesCalls != 2 || mobileCalls != 0 {
		t.Fatal("activated QR session could not fetch account likes without frontend save")
	}
}

func TestWeiboQRDesktopIdentityRejectionsNeverUseCapabilityFallback(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		status     int
	}{
		{"unauthorized", `{}`, 401},
		{"forbidden", `{}`, 403},
		{"limited", `{}`, 429},
		{"logged-out", `{"ok":1,"data":{"uid":"42","islogin":false}}`, 200},
		{"other-account", `{"ok":1,"data":{"uid":"99","islogin":true}}`, 200},
		{"captcha", `{"ok":0,"msg":"需要验证码"}`, 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/ajax/config/get" {
					t.Error("explicit identity rejection must not use a capability or mobile fallback")
				}
				w.WriteHeader(scenario.status)
				fmt.Fprint(w, scenario.body)
			})
			credentials := WeiboCredentials{UserID: "42", Cookie: "SUB=qr-fixture", CookieScoped: true}
			if err := validateWeiboQRSessionIdentity(&credentials, ""); err == nil || calls != 1 {
				t.Fatal("rejected or mismatched QR identity was accepted or retried")
			}
		})
	}
}

func TestWeiboManualCookieDoesNotUseQRConfirmedCapabilityProbe(t *testing.T) {
	probeCalls := 0
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax/config/get":
			w.WriteHeader(http.StatusNotFound)
		case "/ajax/statuses/likelist":
			probeCalls++
			fmt.Fprint(w, `{"ok":1,"statuses":[]}`)
		case "/api/config":
			fmt.Fprint(w, `{"ok":1,"data":{"login":false}}`)
		}
	})
	if _, err := validateWeiboCredentials("SUB=manual-fixture", "42", ""); weiboErrorKind(err) != "auth" || probeCalls != 0 {
		t.Fatal("unconfirmed manual Cookie inherited QR identity proof")
	}
}
