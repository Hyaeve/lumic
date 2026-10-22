package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestWeiboIdentityUsesMobileConfigOnlyForMissingDesktopRoute(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			requests := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("Cookie") != "SUB=fixture" {
					t.Error("identity fallback must validate the same session")
				}
				switch r.URL.Path {
				case "/ajax/config/get":
					w.WriteHeader(status)
				case "/api/config":
					fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","login":true}}`)
				default:
					t.Error("identity validation used a public profile or unrelated route")
				}
			})
			credentials := WeiboCredentials{Cookie: "SUB=fixture", UserID: "42"}
			if err := validateWeiboSessionIdentity(&credentials, ""); err != nil || requests != 2 {
				t.Fatalf("missing identity route blocked a valid session: requests=%d err=%v", requests, err)
			}
		})
	}
}

func TestWeiboIdentityDoesNotRetryAuthenticationOrRestrictedResponses(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
	}{
		{"expired", 401, `{}`},
		{"forbidden", 403, `{}`},
		{"limited", 429, `{}`},
		{"upstream-error", 500, `{}`},
		{"captcha", 200, `{"ok":0,"msg":"需要验证码"}`},
		{"logged-out", 200, `{"ok":1,"data":{"uid":"42","islogin":false}}`},
		{"different-user", 200, `{"ok":1,"data":{"uid":"99","islogin":true}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			requests := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/ajax/config/get" {
					t.Error("authentication or restriction failure must not use a fallback")
				}
				w.WriteHeader(scenario.status)
				fmt.Fprint(w, scenario.body)
			})
			credentials := WeiboCredentials{Cookie: "SUB=fixture", UserID: "42"}
			if err := validateWeiboSessionIdentity(&credentials, ""); err == nil || requests != 1 {
				t.Fatalf("invalid session accepted or retried: requests=%d err=%v", requests, err)
			}
		})
	}
}

func TestWeiboMobileIdentityStillRequiresMatchingLoggedInUID(t *testing.T) {
	for _, body := range []string{
		`{"ok":1,"data":{"uid":"42","login":false}}`,
		`{"ok":1,"data":{"uid":"99","login":true}}`,
		`{"ok":1,"data":{"login":true}}`,
		`{"ok":1,"data":{"uid":"42"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ajax/config/get" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fmt.Fprint(w, body)
			})
			credentials := WeiboCredentials{Cookie: "SUB=fixture", UserID: "42"}
			if err := validateWeiboSessionIdentity(&credentials, ""); err == nil {
				t.Fatal("mobile config accepted an anonymous or mismatched session")
			}
		})
	}
}

func TestWeiboCredentialValidationReportsFailingStage(t *testing.T) {
	for _, identityValid := range []bool{true, false} {
		t.Run(fmt.Sprint(identityValid), func(t *testing.T) {
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if !identityValid {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","islogin":true}}`)
			})
			previousProfile := fetchWeiboAccountProfile
			profileCalls := 0
			fetchWeiboAccountProfile = func(string, WeiboCredentials, string) (WeiboUser, error) {
				profileCalls++
				return WeiboUser{}, &weiboRequestError{"upstream", "微博接口返回 HTTP 404，请稍后重试"}
			}
			t.Cleanup(func() { fetchWeiboAccountProfile = previousProfile })
			_, err := validateWeiboCredentials("SUB=fixture", "42", "")
			if err == nil {
				t.Fatal("failed verification was accepted")
			}
			expected, calls := "微博登录身份验证失败", 0
			if identityValid {
				expected, calls = "微博账号资料读取失败", 1
			}
			if !strings.Contains(err.Error(), expected) || profileCalls != calls {
				t.Fatalf("wrong verification stage reported: calls=%d err=%v", profileCalls, err)
			}
		})
	}
}

func TestWeiboIdentityAcceptsOnlyExplicitMatchingSessionShapes(t *testing.T) {
	for _, scenario := range []struct {
		name string
		body string
		kind string
	}{
		{"wrapped", `{"ok":1,"data":{"uid":"42","islogin":true}}`, ""},
		{"top-level", `{"uid":"42","islogin":true}`, ""},
		{"top-level-numeric", `{"ok":1,"uid":42,"login":1}`, ""},
		{"empty-wrapper", `{"data":{},"uid":"42","login":true}`, ""},
		{"explicit-logged-out", `{"ok":1,"data":{"login":false}}`, "auth"},
		{"missing-identity", `{"ok":1,"data":{}}`, "upstream"},
		{"missing-login-flag", `{"ok":1,"data":{"uid":"42"}}`, "upstream"},
		{"invalid-login-flag", `{"ok":1,"data":{"uid":"42","islogin":"unknown"}}`, "upstream"},
		{"public-profile-is-not-a-session", `{"ok":1,"data":{"user":{"idstr":"42","screen_name":"公开作者"}}}`, "upstream"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, scenario.body) })
			credentials := WeiboCredentials{Cookie: "SUB=fixture", UserID: "42"}
			err := validateWeiboSessionIdentity(&credentials, "")
			if scenario.kind == "" && err != nil || scenario.kind != "" && (err == nil || weiboErrorKind(err) != scenario.kind) {
				t.Fatalf("wrong identity result: expected=%q kind=%q err=%v", scenario.kind, weiboErrorKind(err), err)
			}
		})
	}
}

func TestWeiboDesktopNegativeLoginCodeIsAuthenticationFailure(t *testing.T) {
	if err := weiboPayloadFailure(map[string]any{"ok": -100, "url": "https://weibo.com/login.php"}); weiboErrorKind(err) != "auth" {
		t.Fatalf("desktop login-required response was not classified: kind=%q", weiboErrorKind(err))
	}
}
