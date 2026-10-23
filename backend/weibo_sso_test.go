package main

import (
	"context"
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

func runWeiboSSOQR(t *testing.T, store *BilibiliStore, handler http.HandlerFunc) map[string]any {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	target, _ := url.Parse(upstream.URL)
	jar, _ := cookiejar.New(nil)
	store.weiboQR = map[string]WeiboQRSession{"fixture": {ID: "fixture", QRID: "ticket", ExpiresAt: time.Now().Add(time.Minute)}}
	store.weiboClients = map[string]*http.Client{"fixture": {Jar: jar, Transport: weiboTestTransport{target, upstream.Client().Transport}}}
	response := httptest.NewRecorder()
	store.weiboQRHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/qr?id=fixture", nil))
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK {
		t.Fatalf("QR exchange did not return a connection result: status=%d parse=%v", response.Code, err)
	}
	return result
}

func TestWeiboQRBuildsDomainSessionsFromCrossDomainReplies(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		callbacks bool
		nested    bool
		discover  bool
		appTicket bool
	}{
		{"different-site-cookies", false, false, false, false},
		{"callback-handshake", true, false, false, false},
		{"nested-site-handshake", true, true, false, false},
		{"discover-site-handshakes", true, false, true, false},
		{"application-ticket", true, false, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{})
			mobileChecks, mobileHandshakes, appLogins := 0, 0, 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ajax/config/get" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				mobileChecks++
				if r.Header.Get("Cookie") == "SUB=mobile-fixture" {
					fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","login":true}}`)
				} else {
					fmt.Fprint(w, `{"ok":1,"data":{"login":false}}`)
				}
			})
			previousProfile := fetchWeiboAccountProfile
			fetchWeiboAccountProfile = func(string, WeiboCredentials, string) (WeiboUser, error) {
				return WeiboUser{UserID: "42", Name: "示例账号"}, nil
			}
			t.Cleanup(func() { fetchWeiboAccountProfile = previousProfile })
			result := runWeiboSSOQR(t, store, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sso/qrcode/check":
					fmt.Fprint(w, `{"retcode":20000000,"data":{"alt":"fixture-ticket"}}`)
				case "/sso/login.php":
					http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "sina-fixture", Domain: ".sina.com.cn", Path: "/", Secure: true})
					urls := []string{"https://passport.weibo.com/wbsso/login?ticket=desktop-fixture"}
					if !scenario.nested {
						urls = append(urls, "https://passport.weibo.cn/sso/crossdomain?ticket=mobile-fixture")
					}
					if scenario.discover {
						urls = nil
					}
					payload := map[string]any{"retcode": 0, "uid": "42", "crossDomainUrlList": urls}
					if scenario.appTicket {
						payload["ticket"] = "application-fixture"
					}
					writeJSON(w, payload)
				case "/sso/crossdomain.php":
					if r.URL.Query().Get("action") != "login" || r.URL.Query().Get("domain") != "weibo.com" || r.URL.Query().Get("callback") != "sinaSSOController.crossDomainCallBack" {
						t.Error("discovery did not follow the official SSO callback contract")
					}
					fmt.Fprint(w, `cb({"retcode":0,"arrURL":["https://passport.weibo.com/wbsso/login?ticket=desktop-fixture","https://passport.weibo.cn/sso/crossdomain?ticket=mobile-fixture"]})`)
				case "/sso/wblogin":
					appLogins++
					if r.URL.Query().Get("ticket") != "application-fixture" || r.URL.Query().Get("ssosavestate") == "" {
						t.Error("explicit application ticket was not exchanged")
					}
					fmt.Fprint(w, `cb({"result":true})`)
				case "/wbsso/login", "/sso/crossdomain":
					if scenario.callbacks && (r.URL.Query().Get("callback") == "" || r.URL.Query().Get("scriptId") == "") {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if r.URL.Path == "/sso/crossdomain" {
						mobileHandshakes++
						http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "mobile-fixture", Domain: ".weibo.cn", Path: "/", Secure: true})
						fmt.Fprint(w, `cb({"retcode":0})`)
						return
					}
					http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "desktop-fixture", Domain: ".weibo.com", Path: "/", Secure: true})
					if scenario.nested {
						fmt.Fprint(w, `cb({"retcode":0,"arrURL":["https://passport.weibo.cn/sso/crossdomain?ticket=mobile-fixture"]})`)
					} else {
						fmt.Fprint(w, `cb({"retcode":0})`)
					}
				}
			})
			if result["credentialVerified"] != true || mobileChecks != 1 || mobileHandshakes != 1 {
				t.Fatalf("cross-domain sessions were not established/used: verified=%v checks=%d handshakes=%d", result["credentialVerified"], mobileChecks, mobileHandshakes)
			}
			if scenario.appTicket && appLogins != 1 || !scenario.appTicket && appLogins != 0 {
				t.Fatal("application ticket exchange was skipped or fabricated")
			}
			if store.config.Weibo.Cookie != "SUB=desktop-fixture" || result["connectionWarning"] != "" {
				t.Fatal("validated QR lost the desktop session or retained a warning")
			}
			if store.config.Weibo.MobileCookie != "SUB=mobile-fixture" || !store.config.Weibo.CookieScoped {
				t.Fatal("validated QR did not retain independent site credentials")
			}
			stored, err := os.ReadFile(bilibiliFile)
			if err != nil || strings.Contains(string(stored), "mobile-fixture") || strings.Contains(string(stored), "desktop-fixture") {
				t.Fatal("site credentials were not stored encrypted")
			}
			var reloaded BilibiliConfig
			if err := decryptJSON(store.key, stored, &reloaded); err != nil || reloaded.Weibo != store.config.Weibo {
				t.Fatal("site credentials were not restored from encrypted storage")
			}
			view := httptest.NewRecorder()
			store.weiboAccountHandler(view, httptest.NewRequest(http.MethodGet, "/api/weibo/account", nil))
			if strings.Contains(view.Body.String(), "mobileCookie") || strings.Contains(view.Body.String(), "desktop-fixture") || strings.Contains(view.Body.String(), "mobile-fixture") {
				t.Fatal("account response exposed stored site credentials")
			}
		})
	}
}

func TestWeiboSSORejectsUntrustedURLsAndRedirects(t *testing.T) {
	for _, target := range []string{
		"http://unrelated.invalid/login?ticket=private-fixture",
		"https://passport.weibo.com.evil.invalid/login?ticket=private-fixture",
		"https://passport.weibo.com@evil.invalid/login?ticket=private-fixture",
		"https://user@passport.weibo.com/login?ticket=private-fixture",
		"https://passport.weibo.com:8443/login?ticket=private-fixture",
		"https://127.0.0.1/login?ticket=private-fixture",
		"https://passport.weibo.com/login?action=logout&ticket=private-fixture",
	} {
		if normalized, err := weiboCrossDomainURL(target, 0); err == nil || normalized != "" {
			t.Fatal("untrusted cross-domain target accepted")
		}
	}
	upgraded, upgradeErr := weiboCrossDomainURL("http://passport.weibo.com/wbsso/login?ticket=private-fixture", 0)
	if upgradeErr != nil || !strings.HasPrefix(upgraded, "https://passport.weibo.com/") {
		t.Fatal("legacy official URL must use HTTPS before transmitting its ticket")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "https://unrelated.invalid/?ticket=private-fixture", http.StatusFound)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
	err := completeWeiboSSO(context.Background(), client, []string{"https://passport.weibo.com/wbsso/login?ticket=private-fixture"})
	if err == nil || calls != 1 || strings.Contains(err.Error(), "private-fixture") {
		t.Fatal("external redirect followed or login ticket exposed")
	}
}

func TestWeiboSSOAcceptsEquivalentOfficialURLForms(t *testing.T) {
	for index, rawURL := range []string{
		"https://PASSPORT.WEIBO.COM/wbsso/login?ticket=private-fixture",
		"HTTPS://PASSPORT.WEIBO.COM/wbsso/login?ticket=private-fixture",
		"https://passport.weibo.com:443/wbsso/login?ticket=private-fixture",
		"//passport.weibo.com/wbsso/login?ticket=private-fixture",
		"http://passport.weibo.com:80/wbsso/login?ticket=private-fixture",
		"  https://passport.weibo.com/wbsso/login?ticket=private-fixture  ",
		"https://passport.weibo.com./wbsso/login?ticket=private-fixture",
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			normalized, err := weiboCrossDomainURL(rawURL, 0)
			if err != nil {
				t.Fatal("equivalent official URL was rejected")
			}
			target, err := url.Parse(normalized)
			if err != nil || target.Scheme != "https" || target.Host != "passport.weibo.com" || target.Query().Get("ticket") != "private-fixture" {
				t.Fatal("equivalent official URL was rejected or its ticket changed")
			}
		})
	}
}

func TestWeiboSSOUnsupportedAddressReportsOnlySafeHost(t *testing.T) {
	err := completeWeiboSSO(context.Background(), &http.Client{}, []string{"https://unsupported.invalid/private-path?ticket=private-fixture#private-fragment"})
	if err == nil || !strings.Contains(err.Error(), "unsupported.invalid") || strings.Contains(err.Error(), "private-") {
		t.Fatal("unsupported target must report its host without exposing ticket, path or fragment")
	}
}

func TestWeiboSSOEquivalentURLsAreNormalizedBeforeRequestAndDedup(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("ticket") != "private-fixture" || r.URL.Query().Get("callback") == "" {
			t.Error("normalization changed the login ticket or omitted its callback")
		}
		fmt.Fprint(w, `cb({"retcode":0})`)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
	err := completeWeiboSSO(context.Background(), client, []string{
		"//PASSPORT.WEIBO.COM:443/wbsso/login?ticket=private-fixture",
		"https://passport.weibo.com/wbsso/login?ticket=private-fixture",
		"http://passport.weibo.com:80/wbsso/login?ticket=private-fixture",
	})
	if err != nil || calls != 1 {
		t.Fatalf("equivalent callbacks were rejected or sent multiple times: calls=%d failure=%v", calls, err != nil)
	}
}

func TestWeiboSSOAddressRejectionsAreSpecificAndRedacted(t *testing.T) {
	for _, scenario := range []struct {
		name, target, reason string
	}{
		{"userinfo", "https://private-fixture@passport.weibo.com/login", "账号信息"},
		{"port", "https://passport.weibo.com:8443/login?ticket=private-fixture", "端口"},
		{"scheme", "ftp://passport.weibo.com/login?ticket=private-fixture", "协议"},
		{"query", "https://passport.weibo.com/login?ticket=private-fixture%ZZ", "参数无法解析"},
		{"action", "https://passport.weibo.com/login?action=private-fixture", "不是登录操作"},
		{"conflicting-actions", "https://passport.weibo.com/login?action=login&action=logout&ticket=private-fixture", "不是登录操作"},
		{"malformed", "https://passport.weibo.com:private-fixture/login", "无法解析"},
		{"suffix-host", "https://passport.weibo.com.evil.invalid/login?ticket=private-fixture", "域名不受支持"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			err := completeWeiboSSO(context.Background(), &http.Client{}, []string{scenario.target})
			if err == nil || !strings.Contains(err.Error(), scenario.reason) || strings.Contains(err.Error(), "private-fixture") {
				t.Fatal("rejection reason was absent, incorrect, or exposed a login ticket")
			}
		})
	}
	for index, scenario := range []struct {
		target  string
		allowed bool
	}{
		{"https://PASSPORT.WEIBO.COM:443/next", true},
		{"https://passport.weibo.com./next", true},
		{"http://passport.weibo.com/next", false},
		{"https://passport.weibo.com:8443/next", false},
		{"https://passport.weibo.com.evil.invalid/next", false},
	} {
		target, _ := url.Parse(scenario.target)
		if allowedWeiboSSOTarget(target) != scenario.allowed {
			t.Fatalf("incorrect redirect allowlist result for scenario %d", index)
		}
	}
}

func TestWeiboSSOBoundsAndRedactedNetworkFailures(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		maxCalls int
		fails    bool
	}{
		{"duplicate-followup", 1, false},
		{"too-many-followups", 12, true},
		{"oversized-response", 1, true},
		{"cancelled-request", 0, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if scenario.name == "oversized-response" {
					fmt.Fprint(w, strings.Repeat("x", (1<<20)+1))
					return
				}
				ticket := "private-fixture"
				if scenario.name == "too-many-followups" {
					ticket += fmt.Sprint(calls)
				}
				writeJSON(w, map[string]any{"retcode": 0, "arrURL": []string{"https://passport.weibo.com/wbsso/login?ticket=" + ticket}})
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario.name == "cancelled-request" {
				cancel()
			}
			err := completeWeiboSSO(ctx, client, []string{"https://passport.weibo.com/wbsso/login?ticket=private-fixture"})
			if (err != nil) != scenario.fails || calls != scenario.maxCalls {
				t.Fatalf("unbounded callback chain or unexpected outcome: calls=%d failure=%v", calls, err != nil)
			}
			if err != nil && strings.Contains(err.Error(), "private-fixture") {
				t.Fatal("network/size diagnostic exposed a login ticket")
			}
		})
	}
}

func TestWeiboSSOStopsOnFailureWithoutReplacingActiveCredentials(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
	}{
		{"forbidden", 403, `{}`},
		{"limited", 429, `{}`},
		{"captcha", 200, `cb({"retcode":1,"reason":"需要验证码"})`},
		{"login-rejected", 200, `cb({"retcode":1})`},
		{"missing-marker", 200, `cb({})`},
		{"html-response", 200, `<html>security check</html>`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			previous := WeiboCredentials{UserID: "42", Cookie: "SUB=old-desktop", MobileCookie: "SUB=old-mobile", CookieScoped: true}
			store := newWeiboAuthTestStore(t, previous)
			calls := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("failed handshake must not continue with identity or API requests")
			})
			result := runWeiboSSOQR(t, store, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sso/qrcode/check":
					fmt.Fprint(w, `{"retcode":20000000,"data":{"alt":"private-fixture"}}`)
				case "/sso/login.php":
					fmt.Fprint(w, `{"retcode":0,"uid":"42","crossDomainUrlList":["https://passport.weibo.com/wbsso/login?ticket=private-fixture","https://passport.weibo.cn/sso/crossdomain?ticket=private-fixture"]}`)
				case "/wbsso/login", "/sso/crossdomain":
					calls++
					w.WriteHeader(scenario.status)
					fmt.Fprint(w, scenario.body)
				}
			})
			if calls != 1 || result["credentialVerified"] != false || result["qrConnected"] != true || result["connectionWarning"] == "" || store.config.Weibo.Cookie != previous.Cookie || store.config.Weibo.MobileCookie != previous.MobileCookie {
				t.Fatal("failed handshake did not stop, lost connected state, or overwrote active credentials")
			}
			if strings.Contains(fmt.Sprint(result["connectionWarning"]), "private-fixture") {
				t.Fatal("diagnostic exposed a one-time login ticket")
			}
		})
	}
}

func TestWeiboSiteCookieRenewalIsIsolatedAndPersists(t *testing.T) {
	credentials := WeiboCredentials{UserID: "42", Cookie: "SUB=desktop-fixture", MobileCookie: "SUB=mobile-fixture", CookieScoped: true}
	store := newWeiboAuthTestStore(t, credentials)
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "SUB=mobile-fixture" {
			t.Error("mobile endpoint received another site's session")
		}
		http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "mobile-renewed", Path: "/"})
		fmt.Fprint(w, `{"ok":1}`)
	})
	if _, _, err := store.weiboRawRequest("https://m.weibo.cn/api/config", &credentials, ""); err != nil {
		t.Fatal(err)
	}
	if credentials.Cookie != "SUB=desktop-fixture" || credentials.MobileCookie != "SUB=mobile-renewed" || store.config.Weibo != credentials {
		t.Fatal("mobile renewal was not saved or overwrote desktop credentials")
	}
	previous := credentials
	store.config.Weibo.MobileCookie = "SUB=newer-mobile"
	credentials.MobileCookie = "SUB=late-mobile"
	if err := store.persistWeiboSession(previous, &credentials); err != nil || credentials.MobileCookie != "SUB=newer-mobile" || store.config.Weibo.MobileCookie != "SUB=newer-mobile" {
		t.Fatal("late renewal overwrote a newer mobile session")
	}
}

func TestWeiboScopedCookiesDoNotLeakToMissingSite(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		scoped   bool
		expected string
	}{
		{"scoped-no-mobile", true, ""},
		{"legacy-import-compatible", false, "SUB=desktop-fixture"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != scenario.expected {
					t.Error("incorrect legacy/scoped cookie handling")
				}
				fmt.Fprint(w, `{"ok":1}`)
			})
			credentials := WeiboCredentials{Cookie: "SUB=desktop-fixture", CookieScoped: scenario.scoped}
			if _, _, err := weiboSessionRequest("https://m.weibo.cn/api/config", &credentials, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}
