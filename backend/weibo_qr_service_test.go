package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWeiboQRServiceStatusAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		upstreamCode int
		expired      bool
		failure      int
	}{
		{name: "waiting", status: "waiting", upstreamCode: 50114001},
		{name: "scanned", status: "scanned", upstreamCode: 50114002},
		{name: "upstream expiry", upstreamCode: 50114003, failure: http.StatusGone},
		{name: "local deadline", expired: true, failure: http.StatusGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.expired || r.URL.Path != "/sso/qrcode/check" {
					t.Error("unexpected platform request")
				}
				fmt.Fprintf(w, `{"retcode":%d}`, tc.upstreamCode)
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
			deadline := time.Now().Add(time.Minute)
			if tc.expired {
				deadline = time.Now().Add(-time.Second)
			}
			store.weiboClients = map[string]*http.Client{"fixture": client}
			store.weiboQR = map[string]WeiboQRSession{"fixture": {ID: "fixture", QRID: "qr-key", ExpiresAt: deadline}}
			result, failure := store.pollWeiboQR(context.Background(), "fixture", func(BilibiliConfig) bool { return true })
			if tc.failure != 0 {
				if failure == nil || failure.status != tc.failure || result != nil {
					t.Fatalf("wanted failure %d, got %v, %v", tc.failure, result, failure)
				}
				if len(store.weiboQR) != 0 || len(store.weiboClients) != 0 {
					t.Fatal("expired session and cookie jar were not discarded")
				}
			} else if failure != nil || result["status"] != tc.status {
				t.Fatalf("wanted status %s, got %v, %v", tc.status, result, failure)
			}
		})
	}
}

func TestWeiboQRServiceGuardsCredentialCommit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure int
	}{
		{name: "valid"},
		{name: "unverified", failure: http.StatusUnprocessableEntity},
		{name: "canceled", failure: http.StatusConflict},
		{name: "revoked", failure: http.StatusConflict},
		{name: "discarded", failure: http.StatusConflict},
		{name: "disk failure", failure: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := WeiboCredentials{UserID: "99", Cookie: "SUB=old"}
			store := newWeiboAuthTestStore(t, previous)
			store.config.Telegram.AuthorizedUserID = "42"
			if err := store.save(); err != nil {
				t.Fatal(err)
			}
			originalFile := bilibiliFile
			before, err := os.ReadFile(originalFile)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "disk failure" {
				bilibiliFile = filepath.Join(t.TempDir(), "blocked")
				if err := os.MkdirAll(filepath.Join(bilibiliFile, "child"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			prepareWeiboConnectionQR(t, store, `{"retcode":0,"uid":"42","nick":"example"}`, tc.name != "unverified")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ajax/config/get":
					fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","islogin":true}}`)
				case "/ajax/profile/info":
					switch tc.name {
					case "canceled":
						cancel()
					case "revoked":
						store.Lock()
						store.config.Telegram.AuthorizedUserID = ""
						store.Unlock()
					case "discarded":
						store.discardPlatformQR(SourceWeibo, "fixture")
					}
					fmt.Fprint(w, `{"ok":1,"data":{"user":{"idstr":"42","screen_name":"example"}}}`)
				default:
					t.Error("unexpected identity or profile endpoint")
				}
			})
			guard := func(config BilibiliConfig) bool { return config.Telegram.AuthorizedUserID == "42" }
			result, failure := store.pollWeiboQR(ctx, "fixture", guard)
			after, err := os.ReadFile(originalFile)
			if err != nil {
				t.Fatal(err)
			}
			if tc.failure != 0 {
				if failure == nil || failure.status != tc.failure || result != nil || store.config.Weibo != previous || !bytes.Equal(before, after) {
					t.Fatalf("guarded QR changed credentials on failure: %v, %v", result, failure)
				}
			} else {
				var saved BilibiliConfig
				if failure != nil || result["status"] != "connected" || result["credentialVerified"] != true ||
					result["userId"] != "42" || len(store.weiboQR) != 0 || len(store.weiboClients) != 0 ||
					decryptJSON(store.key, after, &saved) != nil || saved.Weibo != store.config.Weibo || saved.Weibo.Cookie == previous.Cookie {
					t.Fatalf("verified QR did not durably activate: %v, %v", result, failure)
				}
				if _, exists := result["cookie"]; exists || bytes.Contains(after, []byte("pending-qr-fixture")) {
					t.Fatal("new credentials leaked in the result or on disk")
				}
			}
		})
	}
}

func TestWeiboQRServiceCancelsValidationRequests(t *testing.T) {
	for _, blockPath := range []string{"/ajax/config/get", "/ajax/profile/info"} {
		t.Run(blockPath, func(t *testing.T) {
			previous := WeiboCredentials{UserID: "99", Cookie: "SUB=old"}
			store := newWeiboAuthTestStore(t, previous)
			if err := store.save(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(bilibiliFile)
			if err != nil {
				t.Fatal(err)
			}
			prepareWeiboConnectionQR(t, store, `{"retcode":0,"uid":"42","nick":"example"}`, true)
			entered, release := make(chan struct{}), make(chan struct{})
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == blockPath {
					close(entered)
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				if r.URL.Path != "/ajax/config/get" {
					t.Error("unexpected request")
				}
				fmt.Fprint(w, `{"ok":1,"data":{"uid":"42","islogin":true}}`)
			})
			ctx, cancel := context.WithCancel(context.Background())
			finished := make(chan *platformQRError, 1)
			var worker sync.WaitGroup
			worker.Add(1)
			go func() {
				defer worker.Done()
				_, failure := store.pollWeiboQR(ctx, "fixture", func(BilibiliConfig) bool { return true })
				finished <- failure
			}()
			defer func() {
				cancel()
				close(release)
				worker.Wait()
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("validation did not reach the expected endpoint")
			}
			cancel()
			select {
			case failure := <-finished:
				if failure == nil || failure.status != http.StatusConflict {
					t.Fatalf("expected canceled QR validation, got %v", failure)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation did not stop the in-flight validation")
			}
			after, err := os.ReadFile(bilibiliFile)
			if err != nil || !bytes.Equal(before, after) || store.config.Weibo != previous {
				t.Fatal("canceled validation changed existing credentials", err)
			}
		})
	}
}
