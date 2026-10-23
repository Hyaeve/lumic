package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWeiboQRHandshakePreservesRequestHeaders(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax/config/get":
			fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","islogin":true}}`)
		case "/ajax/profile/info":
			fmt.Fprint(w, `{"ok":1,"data":{"user":{"idstr":"42","screen_name":"fixture"}}}`)
		default:
			t.Error("unexpected API request")
		}
	})
	var first http.Header
	handshakes := 0
	result := runWeiboSSOQR(t, store, func(w http.ResponseWriter, r *http.Request) {
		if first == nil {
			first = r.Header.Clone()
		}
		for _, name := range []string{"User-Agent", "Referer", "Accept", "Origin"} {
			if r.Header.Get(name) != first.Get(name) {
				t.Errorf("QR handshake changed %s at %s", name, r.URL.Path)
			}
		}
		switch r.URL.Path {
		case "/sso/qrcode/check":
			fmt.Fprint(w, `{"retcode":20000000,"data":{"alt":"private-fixture"}}`)
		case "/sso/login.php":
			fmt.Fprint(w, `{"retcode":0,"uid":"42","ticket":"private-fixture"}`)
		case "/sso/crossdomain.php":
			handshakes++
			fmt.Fprint(w, `cb({"retcode":0,"arrURL":["https://passport.weibo.com/wbsso/login?ticket=private-fixture"]})`)
		case "/wbsso/login":
			handshakes++
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "desktop-fixture", Domain: ".weibo.com", Path: "/", Secure: true})
			fmt.Fprint(w, `cb({"retcode":0})`)
		case "/sso/wblogin":
			handshakes++
			fmt.Fprint(w, `cb({"result":true})`)
		default:
			t.Error("unexpected handshake request")
		}
	})
	if handshakes != 3 || result["credentialVerified"] != true {
		t.Fatal("consistent QR handshake did not activate the verified session")
	}
}

func TestWeiboSSOCallbackFailureDiagnosticsAreScopedAndRedacted(t *testing.T) {
	for _, scenario := range []struct {
		name, target, body, stage, code, result, kind string
	}{
		{"desktop-reject", "https://passport.weibo.com/wbsso/login", `{"retcode":50112011,"result":false}`, "站点回调", "50112011", "false", ""},
		{"mobile-reject", "https://passport.weibo.cn/sso/crossdomain", `{"retcode":"1"}`, "站点回调", "1", "缺失", ""},
		{"discovery-reject", weiboSSODiscoveryURL, `{"retcode":1}`, "地址发现", "1", "缺失", ""},
		{"app-reject", "https://passport.weibo.com/sso/wblogin", `{"retcode":1,"result":"false"}`, "应用回调", "1", "false", ""},
		{"private-values", "https://passport.weibo.com/wbsso/login", `{"retcode":"private-fixture","result":"private-fixture","reason":"private-fixture"}`, "站点回调", "已隐藏", "已隐藏", ""},
		{"large-number", "https://passport.weibo.com/wbsso/login", `{"retcode":123456789012345678901234567890}`, "站点回调", "已隐藏", "缺失", ""},
		{"missing-marker", "https://passport.weibo.com/wbsso/login", `{}`, "站点回调", "缺失", "缺失", ""},
		{"conflicting-success", "https://passport.weibo.com/wbsso/login", `{"retcode":0,"result":false}`, "站点回调", "0", "false", ""},
		{"captcha", "https://passport.weibo.com/wbsso/login", `{"retcode":1,"reason":"需要验证码 private-fixture"}`, "站点回调", "1", "缺失", "restricted"},
		{"auth", "https://passport.weibo.com/wbsso/login", `{"ok":-100,"reason":"private-fixture"}`, "站点回调", "缺失", "缺失", "auth"},
		{"limited", "https://passport.weibo.com/wbsso/login", `{"error_code":10023,"reason":"private-fixture"}`, "站点回调", "缺失", "缺失", "limited"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprintf(w, "cb(%s)", scenario.body)
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
			endpoint, _ := url.Parse(scenario.target)
			query := endpoint.Query()
			query.Set("ticket", "private-fixture")
			endpoint.RawQuery = query.Encode()
			err := completeWeiboSSO(context.Background(), client, []string{endpoint.String()})
			if err == nil || calls != 1 {
				t.Fatal("rejected callback was accepted or retried")
			}
			if weiboErrorKind(err) != scenario.kind {
				t.Fatal("callback diagnostic changed the upstream error classification")
			}
			for _, expected := range []string{endpoint.Hostname(), scenario.stage, "retcode=" + scenario.code, "result=" + scenario.result} {
				if !strings.Contains(err.Error(), expected) {
					t.Errorf("callback diagnostic missing safe field %q: %v", expected, err)
				}
			}
			for _, secret := range []string{"private-fixture", "123456789012345678901234567890", "ticket=", endpoint.Path} {
				if strings.Contains(err.Error(), secret) {
					t.Error("callback diagnostic leaked response content or URL path")
				}
			}
		})
	}
}
