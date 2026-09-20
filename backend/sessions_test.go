package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRememberedLoginCookieAcrossRestart(t *testing.T) {
	auth := &AuthConfig{Username: "test", PasswordHash: hashPassword("password", []byte("lumic-default-salt-v1"))}
	store := newSessionStore(auth)
	response := httptest.NewRecorder()
	loginHandler(store, auth)(response, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"test","password":"password","keepLoggedIn":true}`)))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != 7*24*60*60 || !store.valid(cookies[0].Value) {
		t.Fatal("missing persistent cookie")
	}
	restarted := newSessionStore(auth)
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(cookies[0])
	check := httptest.NewRecorder()
	sessionHandler(restarted)(check, request)
	if !strings.Contains(check.Body.String(), `"authenticated":false`) || check.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("remembered browser cookie must be rejected after restart", check.Body.String())
	}
	if len(check.Result().Cookies()) != 1 || check.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("restart must clear stale browser cookie")
	}
}

func TestRememberedSessionLifecycle(t *testing.T) {
	auth := &AuthConfig{Username: "test", PasswordHash: "credential-hash"}
	store := newSessionStore(auth)
	token, err := store.createRemembered()
	if err != nil {
		t.Fatal(err)
	}
	ephemeral, _ := store.create()
	restarted := newSessionStore(auth)
	if restarted.valid(token) || restarted.valid(ephemeral) {
		t.Fatal("all sessions must be invalid after restart")
	}
	expiry, ok := store.expiration(token)
	if !ok || time.Until(expiry) < 7*24*time.Hour-time.Minute || time.Until(expiry) > 7*24*time.Hour {
		t.Fatal("remembered expiry must be seven days")
	}
	auth.PasswordHash = "changed"
	if store.valid(token) {
		t.Fatal("changed credentials accepted")
	}
	auth.PasswordHash = "credential-hash"
	if err := store.revoke(token); err != nil {
		t.Fatal(err)
	}
	if store.valid(token) {
		t.Fatal("revoked session accepted")
	}
}

func TestRememberedLogout(t *testing.T) {
	store := &SessionStore{}
	token, err := store.createRemembered()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	request.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
	response := httptest.NewRecorder()
	logoutHandler(store)(response, request)
	if response.Code != http.StatusOK || store.valid(token) || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout must revoke session and clear browser cookie")
	}
}

func TestRememberedSessionExpiry(t *testing.T) {
	store := &SessionStore{remembered: map[string]rememberedSession{
		sessionDigest("expired"): {Expires: time.Now().Add(-time.Second)},
	}}
	if store.valid("expired") {
		t.Fatal("expired session accepted")
	}
}
