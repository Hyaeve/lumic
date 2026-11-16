package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestTGQRWeiboCredentialChecksSessionScopes(t *testing.T) {
	const (
		desktopPath = "/ajax/config/get"
		mobilePath  = "/api/config"
		likesPath   = "/ajax/statuses/likelist"
		loggedIn    = `{"ok":1,"data":{"uid":"42","login":true}}`
		loggedOut   = `{"ok":1,"data":{"login":false}}`
		differentID = `{"ok":1,"data":{"uid":"99","islogin":true}}`
		desktopCK   = "SUB=desktop-fixture"
		mobileCK    = "SUBP=mobile-fixture"
	)
	type step struct {
		path, cookie string
		status       int
		body         string
	}
	legacy := WeiboCredentials{UserID: "42", Cookie: desktopCK}
	scopedDesktop := WeiboCredentials{UserID: "42", Cookie: desktopCK, CookieScoped: true}
	scopedBoth := WeiboCredentials{UserID: "42", Cookie: desktopCK, MobileCookie: mobileCK, CookieScoped: true}
	mobileOnly := WeiboCredentials{UserID: "42", MobileCookie: mobileCK, CookieScoped: true}
	qr := scopedDesktop
	qr.QRConnected = true
	desktopSuccess := step{desktopPath, desktopCK, http.StatusOK, loggedIn}
	desktopMissing := step{desktopPath, desktopCK, http.StatusNotFound, ""}
	desktopExpired := step{desktopPath, desktopCK, http.StatusUnauthorized, ""}
	mobileSuccess := step{mobilePath, mobileCK, http.StatusOK, loggedIn}

	for _, tc := range []struct {
		name        string
		credentials WeiboCredentials
		want        tgQRCredentialStatus
		requests    []step
	}{
		{"no session", WeiboCredentials{UserID: "42", PasswordUsername: "fixture", Password: "fixture"}, tgQRCredentialInvalid, nil},
		{"visitor cookie", WeiboCredentials{UserID: "42", Cookie: "SUB=; visitor=fixture"}, tgQRCredentialInvalid, nil},
		{"malformed saved UID", WeiboCredentials{UserID: "none", Cookie: desktopCK}, tgQRCredentialInvalid, nil},
		{"desktop UID verified", scopedBoth, tgQRCredentialValid, []step{desktopSuccess}},
		{"mobile only", mobileOnly, tgQRCredentialValid, []step{mobileSuccess}},
		{"mobile only wrong UID", mobileOnly, tgQRCredentialInvalid, []step{{mobilePath, mobileCK, http.StatusOK, differentID}}},
		{"scoped desktop route missing", scopedDesktop, tgQRCredentialUnknown, []step{desktopMissing}},
		{"legacy desktop route missing", legacy, tgQRCredentialValid, []step{desktopMissing, {mobilePath, desktopCK, http.StatusOK, loggedIn}}},
		{"scoped desktop route missing with mobile", scopedBoth, tgQRCredentialValid, []step{desktopMissing, mobileSuccess}},
		{"legacy desktop unknown mobile logged out", legacy, tgQRCredentialUnknown, []step{desktopMissing, {mobilePath, desktopCK, http.StatusOK, loggedOut}}},
		{"scoped desktop unknown mobile logged out", scopedBoth, tgQRCredentialUnknown, []step{desktopMissing, {mobilePath, mobileCK, http.StatusOK, loggedOut}}},
		{"scoped desktop unknown mobile wrong UID", scopedBoth, tgQRCredentialUnknown, []step{desktopMissing, {mobilePath, mobileCK, http.StatusOK, differentID}}},
		{"legacy desktop expired", legacy, tgQRCredentialInvalid, []step{desktopExpired}},
		{"desktop expired but mobile valid", scopedBoth, tgQRCredentialValid, []step{desktopExpired, mobileSuccess}},
		{"both logged out", scopedBoth, tgQRCredentialInvalid, []step{desktopExpired, {mobilePath, mobileCK, http.StatusOK, loggedOut}}},
		{"desktop expired mobile uncertain", scopedBoth, tgQRCredentialUnknown, []step{desktopExpired, {mobilePath, mobileCK, http.StatusTooManyRequests, ""}}},
		{"desktop wrong UID", scopedBoth, tgQRCredentialInvalid, []step{{desktopPath, desktopCK, http.StatusOK, differentID}}},
		{"desktop guest payload", legacy, tgQRCredentialInvalid, []step{{desktopPath, desktopCK, http.StatusOK, loggedOut}}},
		{"desktop login required payload", legacy, tgQRCredentialInvalid, []step{{desktopPath, desktopCK, http.StatusOK, `{"ok":-100}`}}},
		{"captcha", scopedBoth, tgQRCredentialUnknown, []step{{desktopPath, desktopCK, http.StatusOK, `{"ok":0,"msg":"captcha"}`}}},
		{"forbidden", scopedBoth, tgQRCredentialUnknown, []step{{desktopPath, desktopCK, http.StatusForbidden, ""}}},
		{"rate limited", scopedBoth, tgQRCredentialUnknown, []step{{desktopPath, desktopCK, http.StatusTooManyRequests, ""}}},
		{"upstream error", scopedBoth, tgQRCredentialUnknown, []step{{desktopPath, desktopCK, http.StatusBadGateway, ""}}},
		{"unrecognized response", scopedBoth, tgQRCredentialUnknown, []step{{desktopPath, desktopCK, http.StatusOK, `{"ok":1,"data":{"uid":"42"}}`}}},
		{"saved QR proof", qr, tgQRCredentialValid, []step{desktopMissing, {likesPath, desktopCK, http.StatusOK, `{"ok":1,"data":{"statuses":[]}}`}}},
		{"saved QR proof wrong UID", qr, tgQRCredentialInvalid, []step{desktopMissing, {likesPath, desktopCK, http.StatusOK, `{"ok":1,"data":{"uid":"99","statuses":[]}}`}}},
		{"saved QR proof lacks likes list", qr, tgQRCredentialUnknown, []step{desktopMissing, {likesPath, desktopCK, http.StatusOK, `{"ok":1,"data":{"uid":"42"}}`}}},
		{"both QR routes missing", qr, tgQRCredentialUnknown, []step{desktopMissing, {likesPath, desktopCK, http.StatusGone, ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				index := int(requests.Add(1)) - 1
				if index >= len(tc.requests) {
					t.Errorf("unexpected request to %s", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				want := tc.requests[index]
				if r.URL.Path != want.path || r.Header.Get("Cookie") != want.cookie || r.Method != http.MethodGet {
					t.Errorf("request %d: wrong route, HTTP method, or cookie scope", index+1)
				}
				if r.URL.Path == likesPath && r.URL.Query().Get("uid") != "42" {
					t.Error("QR proof did not check the saved account")
				}
				w.WriteHeader(want.status)
				fmt.Fprint(w, want.body)
			})
			if got := checkTGQRWeibo(context.Background(), tc.credentials, ""); got != tc.want || int(requests.Load()) != len(tc.requests) {
				t.Fatalf("status=%v want=%v; requests=%d want=%d", got, tc.want, requests.Load(), len(tc.requests))
			}
		})
	}
}

func TestTGQRWeiboCredentialCannotConfirmOnNetworkErrorOrCancellation(t *testing.T) {
	previous := weiboHTTPClient
	t.Cleanup(func() { weiboHTTPClient = previous })
	credentials := WeiboCredentials{UserID: "42", Cookie: "SUB=desktop-fixture", MobileCookie: "SUB=mobile-fixture", CookieScoped: true}
	requests := 0
	weiboHTTPClient = func(proxyURL string) (*http.Client, error) {
		requests++
		if proxyURL != "fixture-proxy" {
			t.Error("credential check did not use the configured proxy")
		}
		return nil, errors.New("fixture network error")
	}
	if got := checkTGQRWeibo(context.Background(), credentials, "fixture-proxy"); got != tgQRCredentialUnknown || requests != 1 {
		t.Fatalf("network error: status=%v requests=%d", got, requests)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := checkTGQRWeibo(ctx, credentials, "fixture-proxy"); got != tgQRCredentialUnknown || requests != 1 {
		t.Fatalf("cancellation: status=%v requests=%d", got, requests)
	}
}
