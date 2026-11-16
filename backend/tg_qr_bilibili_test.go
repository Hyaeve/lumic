package main

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTGQRBilibiliCredentialResponse(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   int
		body     string
		expected tgQRCredentialStatus
	}{
		{"valid", 200, `{"code":0,"data":{"isLogin":true,"mid":42}}`, tgQRCredentialValid},
		{"not logged in code", 200, `{"code":-101,"data":null}`, tgQRCredentialInvalid},
		{"not logged in flag", 200, `{"code":0,"data":{"isLogin":false}}`, tgQRCredentialInvalid},
		{"different account", 200, `{"code":0,"data":{"isLogin":true,"mid":24}}`, tgQRCredentialInvalid},
		{"missing code", 200, `{"data":{"isLogin":true,"mid":42}}`, tgQRCredentialUnknown},
		{"missing login flag", 200, `{"code":0,"data":{"mid":42}}`, tgQRCredentialUnknown},
		{"missing uid", 200, `{"code":0,"data":{"isLogin":true}}`, tgQRCredentialUnknown},
		{"invalid uid", 200, `{"code":0,"data":{"isLogin":true,"mid":0}}`, tgQRCredentialUnknown},
		{"unexpected uid format", 200, `{"code":0,"data":{"isLogin":true,"mid":"42"}}`, tgQRCredentialUnknown},
		{"unexpected login flag format", 200, `{"code":0,"data":{"isLogin":"true","mid":42}}`, tgQRCredentialUnknown},
		{"malformed JSON", 200, `<html>unavailable</html>`, tgQRCredentialUnknown},
		{"provider refusal", 200, `{"code":-412,"data":{"isLogin":false}}`, tgQRCredentialUnknown},
		{"blocked", 403, `{"code":-101}`, tgQRCredentialUnknown},
		{"throttled", 429, `{"code":-101}`, tgQRCredentialUnknown},
		{"upstream error", 503, `{"code":-101}`, tgQRCredentialUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/x/web-interface/nav" {
					t.Errorf("unexpected credential check: %s %s", r.Method, r.URL.Path)
				}
				session, sessionErr := r.Cookie("SESSDATA")
				uid, uidErr := r.Cookie("DedeUserID")
				if sessionErr != nil || uidErr != nil || session.Value != "fixture-bili" || uid.Value != "42" {
					t.Error("credential check did not use the stored Cookie")
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			credentials := BilibiliCredentials{Cookie: "SESSDATA=fixture-bili; DedeUserID=42"}
			if got := checkTGQRBilibili(context.Background(), credentials, proxy); got != tt.expected {
				t.Fatalf("credential status = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTGQRBilibiliCredentialLocalState(t *testing.T) {
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("locally decided credential status must not contact the provider")
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, credentials := range []BilibiliCredentials{
		{},
		{DedeUserID: "42"},
		{SESSDATA: "fixture-bili"},
		{SESSDATA: "fixture-bili", DedeUserID: "-1"},
		{SESSDATA: "fixture-bili", DedeUserID: "broken"},
	} {
		if got := checkTGQRBilibili(context.Background(), credentials, proxy); got != tgQRCredentialInvalid {
			t.Errorf("incomplete credential status = %v, want invalid", got)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := checkTGQRBilibili(ctx, BilibiliCredentials{SESSDATA: "fixture-bili", DedeUserID: "42"}, proxy); got != tgQRCredentialUnknown {
		t.Errorf("canceled credential status = %v, want unknown", got)
	}
}

func TestTGQRBilibiliCredentialCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
			t.Error("upstream request was not canceled")
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	credentials := BilibiliCredentials{SESSDATA: "fixture-bili", DedeUserID: "42"}
	if got := checkTGQRBilibili(ctx, credentials, proxy); got != tgQRCredentialUnknown {
		t.Errorf("canceled request status = %v, want unknown", got)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("fixture did not cancel the active request: %v", ctx.Err())
	}
}
