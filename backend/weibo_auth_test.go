package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newWeiboAuthTestStore(t *testing.T, credentials WeiboCredentials) *BilibiliStore {
	t.Helper()
	oldFile := bilibiliFile
	bilibiliFile = filepath.Join(t.TempDir(), "platform.enc")
	t.Cleanup(func() { bilibiliFile = oldFile })
	return &BilibiliStore{config: BilibiliConfig{Weibo: credentials}, key: make([]byte, 32)}
}

func TestWeiboResponseFailureKinds(t *testing.T) {
	for _, test := range []struct {
		name, body, kind string
		status           int
	}{
		{"unauthorized", `{}`, "auth", 401},
		{"forbidden", `{}`, "restricted", 403},
		{"rate", `{}`, "limited", 429},
		{"expired-code", `{"ok":0,"errno":"100001"}`, "auth", 200},
		{"expired-text", `{"ok":0,"msg":"请先登录"}`, "auth", 200},
		{"captcha", `{"ok":0,"msg":"登录需要验证码"}`, "restricted", 200},
		{"rate-code", `{"ok":0,"error_code":10023}`, "limited", 200},
		{"unknown", `{"ok":0,"msg":"unavailable"}`, "upstream", 200},
		{"html", `<html>security check</html>`, "restricted", 200},
		{"login-html", `<html>passport.weibo.cn</html>`, "auth", 200},
		{"success", `{"ok":1,"data":{"items":[]}}`, "", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			_, _, err := weiboSessionRequest(server.URL, &WeiboCredentials{}, "")
			if weiboErrorKind(err) != test.kind || isWeiboSessionFailure(err) != (test.kind == "auth") {
				t.Fatalf("kind=%q err=%v", weiboErrorKind(err), err)
			}
		})
	}
}

func TestWeiboSessionRenewalPersistsEncrypted(t *testing.T) {
	credentials := WeiboCredentials{UserID: "42", Cookie: "SUB=old; XSRF-TOKEN=old; removed=old", PasswordUsername: "fixture", Password: "fixture-secret"}
	store := newWeiboAuthTestStore(t, credentials)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != credentials.Cookie {
			t.Error("request did not retain the existing session")
		}
		http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "renewed", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "renewed-token", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "removed", MaxAge: -1, Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "foreign", Value: "bad", Domain: "unrelated.invalid", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "path-only", Value: "bad", Path: "/other"})
		fmt.Fprint(w, `{"ok":1}`)
	}))
	defer server.Close()
	if _, _, err := store.weiboRawRequest(server.URL, &credentials, ""); err != nil {
		t.Fatal(err)
	}
	if credentials.Cookie != "SUB=renewed; XSRF-TOKEN=renewed-token" || store.config.Weibo.Cookie != credentials.Cookie {
		t.Fatalf("renewal was not merged: %q", credentials.Cookie)
	}
	data, err := os.ReadFile(bilibiliFile)
	if err != nil || bytes.Contains(data, []byte("fixture-secret")) || bytes.Contains(data, []byte("SUB=renewed")) {
		t.Fatal("renewal must be saved as encrypted configuration", err)
	}
	var restored BilibiliConfig
	if err := decryptJSON(store.key, data, &restored); err != nil || restored.Weibo != store.config.Weibo {
		t.Fatal("encrypted renewal cannot be restored", err)
	}
}

func TestWeiboRecoveryCoalescesConcurrentExpiredRequests(t *testing.T) {
	old := WeiboCredentials{UserID: "42", Cookie: "SUB=expired", PasswordUsername: "fixture", Password: "secret"}
	store := newWeiboAuthTestStore(t, old)
	originalLogin := weiboPasswordLogin
	var logins atomic.Int32
	weiboPasswordLogin = func(username, password, proxy string) (WeiboCredentials, error) {
		logins.Add(1)
		time.Sleep(20 * time.Millisecond)
		return WeiboCredentials{UserID: "42", Cookie: "SUB=fresh"}, nil
	}
	t.Cleanup(func() { weiboPasswordLogin = originalLogin })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "SUB=fresh" {
			fmt.Fprint(w, `{"ok":0,"errno":"100001"}`)
			return
		}
		fmt.Fprint(w, `{"ok":1}`)
	}))
	defer server.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			credentials := old
			if _, _, err := store.weiboRawRequest(server.URL, &credentials, ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if logins.Load() != 1 || store.config.Weibo.Cookie != "SUB=fresh" || !hasWeiboPassword(store.config.Weibo) {
		t.Fatal("expired requests did not share a single account recovery")
	}
}

func TestWeiboRecoveryHonorsRestrictionsAndCooldown(t *testing.T) {
	for _, status := range []int{200, 403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			credentials := WeiboCredentials{UserID: "42", Cookie: "SUB=expired", PasswordUsername: "fixture", Password: "secret"}
			store := newWeiboAuthTestStore(t, credentials)
			originalLogin := weiboPasswordLogin
			logins := 0
			weiboPasswordLogin = func(_, _, _ string) (WeiboCredentials, error) {
				logins++
				return WeiboCredentials{}, errors.New("请完成验证码")
			}
			t.Cleanup(func() { weiboPasswordLogin = originalLogin })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"ok":0,"msg":"请先登录"}`)
			}))
			defer server.Close()
			for i := 0; i < 3; i++ {
				if _, _, err := store.weiboRawRequest(server.URL, &credentials, ""); err == nil {
					t.Fatal("failure was silently accepted")
				}
			}
			if status == 200 && logins != 1 || status != 200 && logins != 0 {
				t.Fatalf("unexpected password retry count %d", logins)
			}
		})
	}
}

func TestWeiboLateRenewalDoesNotOverwriteManualAccountChange(t *testing.T) {
	previous := WeiboCredentials{UserID: "42", Cookie: "SUB=old"}
	store := newWeiboAuthTestStore(t, previous)
	manual := WeiboCredentials{UserID: "99", Cookie: "SUB=manual"}
	store.config.Weibo = manual
	next := previous
	next.Cookie = "SUB=late"
	if err := store.persistWeiboSession(previous, &next); err != nil || next != manual || store.config.Weibo != manual {
		t.Fatal("late request replaced a newer account configuration", err)
	}
}

func TestWeiboPasswordIsAccountBound(t *testing.T) {
	previous := WeiboCredentials{UserID: "42", PasswordUsername: "fixture", Password: "secret"}
	for _, id := range []string{"42", "99", ""} {
		next := WeiboCredentials{UserID: id}
		retainWeiboPassword(previous, &next)
		if hasWeiboPassword(next) != (id == "42") {
			t.Fatal("password was not bound to the original account")
		}
	}
}

func TestWeiboSessionFailureDoesNotLeakResponseSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":0,"msg":"请先登录 SUB=private-cookie"}`)
	}))
	defer server.Close()
	_, _, err := weiboRawRequest(server.URL, WeiboCredentials{}, "")
	if err == nil || strings.Contains(err.Error(), "private-cookie") {
		t.Fatal("raw upstream credentials were exposed in the error")
	}
}

func TestWeiboRejectedRecoverySessionDoesNotRepeatPasswordLogin(t *testing.T) {
	credentials := WeiboCredentials{UserID: "42", Cookie: "SUB=expired", PasswordUsername: "fixture", Password: "secret"}
	store := newWeiboAuthTestStore(t, credentials)
	previousLogin := weiboPasswordLogin
	logins := 0
	weiboPasswordLogin = func(_, _, _ string) (WeiboCredentials, error) {
		logins++
		return WeiboCredentials{UserID: "42", Cookie: "SUB=rejected"}, nil
	}
	t.Cleanup(func() { weiboPasswordLogin = previousLogin })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	for i := 0; i < 3; i++ {
		if _, _, err := store.weiboRawRequest(server.URL, &credentials, ""); err == nil {
			t.Fatal("rejected replacement session was accepted")
		}
	}
	if logins != 1 {
		t.Fatal("rejected replacement session caused repeated password logins", logins)
	}
}

func TestWeiboFailedRenewalSaveRollsBack(t *testing.T) {
	previous := WeiboCredentials{UserID: "42", Cookie: "SUB=old"}
	store := newWeiboAuthTestStore(t, previous)
	store.key = []byte{1}
	next := previous
	next.Cookie = "SUB=renewed"
	if err := store.persistWeiboSession(previous, &next); err == nil || store.config.Weibo != previous || next != previous {
		t.Fatal("failed encrypted persistence did not restore the original session")
	}
}

func TestWeiboRedirectDoesNotForwardCredentialsToAnotherHost(t *testing.T) {
	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/login", http.StatusFound)
	}))
	defer server.Close()
	_, _, err := weiboRawRequest(server.URL, WeiboCredentials{Cookie: "SUB=private"}, "")
	if !isWeiboSessionFailure(err) || forwarded != 0 {
		t.Fatal("cross-host redirect was followed with credentials")
	}
}
