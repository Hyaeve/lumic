package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTestSessionStore(t *testing.T, auth *AuthConfig, path string) *SessionStore {
	t.Helper()
	store, err := loadSessionStore(auth, path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestBrowserLoginPersistsAcrossRestart(t *testing.T) {
	auth := &AuthConfig{Username: "test", PasswordHash: hashPassword("password", []byte("lumic-default-salt-v1"))}
	path := filepath.Join(t.TempDir(), "sessions-v1.json")
	store := openTestSessionStore(t, auth, path)
	t.Setenv("LUMIC_COOKIE_SECURE", "true")
	signIn := func(body string, persistent bool) *http.Cookie {
		t.Helper()
		start := time.Now().Truncate(time.Second)
		response := httptest.NewRecorder()
		loginHandler(store)(response, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body)))
		cookies := response.Result().Cookies()
		if response.Code != http.StatusOK || len(cookies) != 1 {
			t.Fatalf("login failed: status=%d", response.Code)
		}
		cookie := cookies[0]
		if cookie.Name != "lumic_session" || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode || !store.valid(cookie.Value) {
			t.Fatal("invalid session cookie")
		}
		if persistent && (cookie.MaxAge != 2592000 || cookie.Expires.Before(start.Add(30*24*time.Hour)) || cookie.Expires.After(time.Now().Add(30*24*time.Hour))) {
			t.Fatal("persistent cookie must expire in 30 days")
		}
		if !persistent && (cookie.MaxAge != 0 || !cookie.Expires.IsZero()) {
			t.Fatal("unchecked login must use a browser-session cookie")
		}
		return cookie
	}
	defaultCookie := signIn(`{"username":"test","password":"password"}`, true)
	otherCookie := signIn(`{"username":"test","password":"password","keepLoggedIn":true}`, true)
	tempCookie := signIn(`{"username":"test","password":"password","keepLoggedIn":false}`, false)
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{defaultCookie.Value, otherCookie.Value, tempCookie.Value, auth.PasswordHash, "password"} {
		if bytes.Contains(saved, []byte(secret)) {
			t.Fatal("session snapshot contains a raw credential")
		}
	}
	var records map[string]sessionRecord
	if err := json.Unmarshal(saved, &records); err != nil || len(records) != 2 {
		t.Fatal("only the two persistent digests should be saved", err)
	}

	restarted := openTestSessionStore(t, auth, path)
	for _, cookie := range []*http.Cookie{defaultCookie, otherCookie, tempCookie} {
		request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		sessionHandler(restarted)(response, request)
		wantAuth := cookie != tempCookie
		var result struct{ Authenticated bool }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || result.Authenticated != wantAuth || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected session status after restart: %d %s", response.Code, response.Body.String())
		}
		cleared := response.Result().Cookies()
		if wantAuth && len(cleared) != 0 || !wantAuth && (len(cleared) != 1 || cleared[0].MaxAge != -1) {
			t.Fatal("session check must only clear invalid cookies")
		}
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(saved, after) {
		t.Fatal("session checks must not renew or rewrite the snapshot")
	}

	request := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	request.AddCookie(defaultCookie)
	response := httptest.NewRecorder()
	logoutHandler(restarted)(response, request)
	cookies := response.Result().Cookies()
	if response.Code != http.StatusOK || restarted.valid(defaultCookie.Value) || len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatal("logout did not revoke the session and clear the cookie")
	}
	restarted = openTestSessionStore(t, auth, path)
	if restarted.valid(defaultCookie.Value) || !restarted.valid(otherCookie.Value) {
		t.Fatal("logout must stay revoked after restart without signing out another client")
	}
}

func TestSessionRejectsExpiredAndChangedCredentials(t *testing.T) {
	auth := &AuthConfig{Username: "test", PasswordHash: "fixture-hash"}
	path := filepath.Join(t.TempDir(), "sessions-v1.json")
	store := openTestSessionStore(t, auth, path)
	token, expires, err := store.create(store.credentialsDigest(), true)
	if err != nil {
		t.Fatal(err)
	}
	expiredKey := sessionDigest("expired-fixture-token")
	store.records[expiredKey] = sessionRecord{Expires: time.Now().Add(-time.Second), Credentials: store.credentialsDigest(), persistent: true}
	// Seed an on-disk record that expired while the service was stopped.
	data, err := json.Marshal(store.records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	restarted := openTestSessionStore(t, auth, path)
	restoredExpiry, ok := restarted.expiration(token)
	if !ok || !restoredExpiry.Equal(expires) || store.valid("expired-fixture-token") || restarted.valid("expired-fixture-token") || len(restarted.records) != 1 {
		t.Fatal("restart must keep the original expiry and discard expired sessions")
	}
	auth.PasswordHash = "changed"
	if restarted.valid(token) || openTestSessionStore(t, auth, path).valid(token) {
		t.Fatal("credential change must invalidate saved and in-memory sessions")
	}
}

func TestSessionStorageFailureDoesNotReportSuccess(t *testing.T) {
	auth := &AuthConfig{Username: "test", PasswordHash: hashPassword("password", []byte("lumic-default-salt-v1"))}
	path := filepath.Join(t.TempDir(), "sessions-v1.json")
	store := openTestSessionStore(t, auth, path)
	token, _, err := store.create(store.credentialsDigest(), true)
	if err != nil {
		t.Fatal(err)
	}
	backup := path + ".backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	loginHandler(store)(login, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"test","password":"password"}`)))
	if login.Code != http.StatusInternalServerError || len(login.Result().Cookies()) != 0 || len(store.records) != 1 {
		t.Fatal("failed save must not issue a session")
	}

	logout := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	request.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
	logoutHandler(store)(logout, request)
	if logout.Code != http.StatusInternalServerError || len(logout.Result().Cookies()) != 0 || !store.valid(token) {
		t.Fatal("failed revocation must not claim logout or clear the cookie")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatal("failed rename must not delete the target")
	}
	if !openTestSessionStore(t, auth, backup).valid(token) {
		t.Fatal("original snapshot must still be usable")
	}
}
