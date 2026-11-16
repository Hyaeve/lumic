package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func bilibiliQRFixture(t *testing.T, store *BilibiliStore, response func(*http.Request) *http.Response) BilibiliQRSession {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	session := BilibiliQRSession{ID: randomSessionID(), Key: "fixture-qr-key", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if store.bilibiliQR == nil {
		store.bilibiliQR = make(map[string]BilibiliQRSession)
		store.bilibiliClients = make(map[string]*http.Client)
	}
	store.bilibiliQR[session.ID] = session
	store.bilibiliClients[session.ID] = &http.Client{Jar: jar, Transport: tgTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "passport.bilibili.com" || request.URL.Path != "/x/passport-login/web/qrcode/poll" || request.URL.Query().Get("qrcode_key") != session.Key {
			t.Errorf("unexpected poll endpoint: %v", request.URL)
		}
		return response(request), nil
	})}
	return session
}

func bilibiliQRFixtureResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestBilibiliQRServiceWebPollKeepsPublicStatuses(t *testing.T) {
	store := &BilibiliStore{}
	for _, tc := range []struct {
		name string
		code int
		http int
		want string
	}{
		{"waiting", 86101, http.StatusOK, "waiting"},
		{"scanned", 86090, http.StatusOK, "scanned"},
		{"upstream expiration", 86038, http.StatusGone, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := bilibiliQRFixture(t, store, func(_ *http.Request) *http.Response {
				return bilibiliQRFixtureResponse(http.StatusOK, fmt.Sprintf(`{"code":0,"data":{"code":%d}}`, tc.code))
			})
			recorder := httptest.NewRecorder()
			store.bilibiliQRHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/bilibili/qr?id="+session.ID, nil))
			var result map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != tc.http || recorder.Header().Get("Cache-Control") != "no-store" || (tc.want != "" && result["status"] != tc.want) {
				t.Fatalf("wrong web QR response: HTTP %d, %v", recorder.Code, result)
			}
			if tc.http == http.StatusGone && (store.bilibiliQR[session.ID].ID != "" || store.bilibiliClients[session.ID] != nil) {
				t.Fatal("expired session still present")
			}
		})
	}
}

func TestBilibiliQRServiceCommitsOnlyCurrentAuthorizedSession(t *testing.T) {
	for _, outcome := range []string{"success", "revoked", "canceled", "discarded", "write failure"} {
		t.Run(outcome, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{})
			previous := BilibiliCredentials{SESSDATA: "old-session", DedeUserID: "17"}
			store.config.Credentials = previous
			store.config.Telegram.AuthorizedUserID = "42"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session := bilibiliQRFixture(t, store, func(request *http.Request) *http.Response {
				if request.Context() != ctx {
					t.Error("poll request did not carry cancelable context")
				}
				response := bilibiliQRFixtureResponse(http.StatusOK, `{"code":0,"data":{"code":0,"refresh_token":"fixture-refresh"}}`)
				for _, cookie := range []string{"SESSDATA=fixture-session", "bili_jct=fixture-csrf", "DedeUserID=42", "buvid3=fixture-device"} {
					response.Header.Add("Set-Cookie", cookie+"; Path=/; Secure")
				}
				return response
			})
			store.config.ProxyURL = authorWindowProxy(t, func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/x/web-interface/nav" {
					t.Errorf("unexpected profile endpoint: %v", request.URL)
				}
				switch outcome {
				case "revoked":
					store.Lock()
					store.config.Telegram.AuthorizedUserID = ""
					store.Unlock()
				case "canceled":
					cancel()
				case "discarded":
					store.discardPlatformQR(SourceBilibili, session.ID)
				}
				writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"mid": 42, "uname": "fixture"}})
			})
			if outcome == "write failure" {
				// Writing over a non-empty directory must fail without changing live credentials.
				bilibiliFile = t.TempDir()
				if err := os.WriteFile(bilibiliFile+"/keep", []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, failure := store.pollBilibiliQR(ctx, session.ID, func(config BilibiliConfig) bool {
				return config.Telegram.AuthorizedUserID == "42"
			})
			if outcome != "success" {
				if failure == nil || result != nil || store.config.Credentials != previous {
					t.Fatalf("failed QR attempt changed credentials: %v, %v", result, failure)
				}
				expected := http.StatusConflict
				if outcome == "write failure" {
					expected = http.StatusInternalServerError
				}
				if failure.status != expected {
					t.Fatalf("unexpected failure: %+v", failure)
				}
				if outcome != "write failure" {
					if _, err := os.Stat(bilibiliFile); !os.IsNotExist(err) {
						t.Fatal("revoked QR attempt persisted credentials")
					}
				}
				return
			}
			if failure != nil || result["status"] != "connected" || result["userId"] != "42" || result["userName"] != "fixture" {
				t.Fatalf("unexpected status: %v, %v", result, failure)
			}
			if store.bilibiliQR[session.ID].ID != "" || store.bilibiliClients[session.ID] != nil {
				t.Fatal("confirmed session could be reused")
			}
			data, err := os.ReadFile(bilibiliFile)
			if err != nil {
				t.Fatal(err)
			}
			var persisted BilibiliConfig
			if err := decryptJSON(store.key, data, &persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.Credentials != store.config.Credentials || persisted.Credentials.AccessTimeValue != "fixture-refresh" || persisted.Credentials.SESSDATA != "fixture-session" {
				t.Fatal("successful authorization did not persist credentials")
			}
			if strings.Contains(string(data), "fixture-session") {
				t.Fatal("platform credentials were not encrypted")
			}
		})
	}
}
