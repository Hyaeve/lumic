package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRememberedLoginCookieAcrossRestart(t *testing.T) {
	auth := &AuthConfig{Username: "test", PasswordHash: hashPassword("password", []byte("lumic-default-salt-v1"))}
	path := filepath.Join(t.TempDir(), "sessions.json")
	store, err := loadSessionStore(path, auth)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	loginHandler(store, auth)(response, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"test","password":"password","keepLoggedIn":true}`)))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != int(rememberedSessionLifetime.Seconds()) {
		t.Fatal("missing persistent cookie")
	}
	restarted, err := loadSessionStore(path, auth)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(cookies[0])
	check := httptest.NewRecorder()
	sessionHandler(restarted)(check, request)
	if !strings.Contains(check.Body.String(), `"authenticated":true`) || check.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("remembered browser cookie rejected after restart", check.Body.String())
	}
}

func TestRememberedSessionLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	auth := &AuthConfig{Username: "test", PasswordHash: "credential-hash"}
	store, err := loadSessionStore(path, auth)
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.createRemembered()
	if err != nil {
		t.Fatal(err)
	}
	ephemeral, _ := store.create()
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), token) {
		t.Fatal("raw bearer secret stored on disk")
	}
	restarted, err := loadSessionStore(path, auth)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.valid(token) || restarted.valid(ephemeral) {
		t.Fatal("incorrect restart persistence")
	}
	expiry, ok := restarted.expiration(token)
	if !ok || time.Until(expiry) < 29*24*time.Hour {
		t.Fatal("remembered expiry too short")
	}
	auth.PasswordHash = "changed"
	if restarted.valid(token) {
		t.Fatal("changed credentials accepted")
	}
	auth.PasswordHash = "credential-hash"
	if err := restarted.revoke(token); err != nil {
		t.Fatal(err)
	}
	afterLogout, err := loadSessionStore(path, auth)
	if err != nil || afterLogout.valid(token) {
		t.Fatal("logout did not survive restart")
	}
}

func TestRememberedSessionWriteFailure(t *testing.T) {
	store := &SessionStore{sessionFile: t.TempDir()}
	token, err := store.createRemembered()
	if err == nil || token != "" || len(store.remembered) != 0 {
		t.Fatal("failed persistence issued a session")
	}
}

func TestRememberedLogoutWriteFailure(t *testing.T) {
	store := &SessionStore{}
	token, err := store.createRemembered()
	if err != nil {
		t.Fatal(err)
	}
	store.sessionFile = t.TempDir()
	request := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	request.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
	response := httptest.NewRecorder()
	logoutHandler(store)(response, request)
	if response.Code != http.StatusInternalServerError || !store.valid(token) || len(response.Result().Cookies()) != 0 {
		t.Fatal("failed revocation must not falsely report success")
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
