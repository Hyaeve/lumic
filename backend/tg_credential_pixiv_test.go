package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTGCredentialPixivViewerIdentity(t *testing.T) {
	next := func(data string) string {
		return `<html><head></head><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":` + data + `}}</script></body></html>`
	}
	for _, tc := range []struct {
		name, body string
		status     int
		want       tgQRCredentialStatus
	}{
		{"current viewer", next(`{"isLoggedIn":true,"gaUserData":{"login":true,"userId":"42"}}`), 200, tgQRCredentialValid},
		{"numeric viewer ID", next(`{"isLoggedIn":true,"gaUserData":{"login":true,"userId":42}}`), 200, tgQRCredentialValid},
		{"logged out", next(`{"isLoggedIn":false,"gaUserData":{"login":false,"userId":""}}`), 200, tgQRCredentialInvalid},
		{"explicit guest", next(`{"isLoggedIn":false}`), 200, tgQRCredentialInvalid},
		{"different viewer", next(`{"isLoggedIn":true,"gaUserData":{"login":true,"userId":"99"}}`), 200, tgQRCredentialInvalid},
		{"conflicting login flags", next(`{"isLoggedIn":true,"gaUserData":{"login":false,"userId":"42"}}`), 200, tgQRCredentialUnknown},
		{"missing viewer flag", next(`{"gaUserData":{"login":true,"userId":"42"}}`), 200, tgQRCredentialUnknown},
		{"missing analytics login", next(`{"isLoggedIn":true,"gaUserData":{"userId":"42"}}`), 200, tgQRCredentialUnknown},
		{"missing viewer identity", next(`{"isLoggedIn":true,"gaUserData":{"login":true}}`), 200, tgQRCredentialUnknown},
		{"invalid viewer identity", next(`{"isLoggedIn":true,"gaUserData":{"login":true,"userId":0}}`), 200, tgQRCredentialUnknown},
		{"wrong typed login", next(`{"isLoggedIn":"true","gaUserData":{"login":true,"userId":"42"}}`), 200, tgQRCredentialUnknown},
		{"public profile is not identity", `{"error":false,"body":{"userId":"42","name":"fixture"}}`, 200, tgQRCredentialUnknown},
		{"HTML profile is not identity", `<meta id="meta-preload-data" content='{"user":{"42":{"userId":"42"}}}'>`, 200, tgQRCredentialUnknown},
		{"legacy viewer", `<meta content='{"userData":{"id":"42"}}' id="meta-global-data">`, 200, tgQRCredentialValid},
		{"legacy guest", `<meta id="meta-global-data" content='{"userData":null}'>`, 200, tgQRCredentialInvalid},
		{"legacy malformed", `<meta id="meta-global-data" content='{"userData":{}}'>`, 200, tgQRCredentialUnknown},
		{"unrecognized page", `<html>maintenance</html>`, 200, tgQRCredentialUnknown},
		{"malformed next data", next(`!`), 200, tgQRCredentialUnknown},
		{"unauthorized", "", 401, tgQRCredentialInvalid},
		{"forbidden", next(`{"isLoggedIn":false}`), 403, tgQRCredentialUnknown},
		{"limited", next(`{"isLoggedIn":false}`), 429, tgQRCredentialUnknown},
		{"bad gateway", next(`{"isLoggedIn":false}`), 502, tgQRCredentialUnknown},
		{"oversized page", strings.Repeat("x", 2<<20) + next(`{"isLoggedIn":false}`), 200, tgQRCredentialUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.Host != "www.pixiv.net" || r.URL.Path != "/" || r.Header.Get("Cookie") != "PHPSESSID=fixture-session" || r.UserAgent() != "fixture-browser" {
					t.Error("unexpected request or credentials")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			credentials := PixivCredentials{UserAgent: "fixture-browser", Cookie: "PHPSESSID=fixture-session", UserID: "42"}
			original := credentials
			if got := checkTGCredentialPixiv(context.Background(), credentials, proxy); got != tc.want || requests.Load() != 1 {
				t.Fatalf("status=%v want=%v requests=%d", got, tc.want, requests.Load())
			}
			if credentials != original {
				t.Fatal("read-only check changed credentials")
			}
		})
	}
}

func TestTGCredentialPixivMissingCancellationAndRedirects(t *testing.T) {
	var requests atomic.Int32
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		t.Error("locally invalid or canceled check contacted provider")
		w.WriteHeader(500)
	})
	for _, credentials := range []PixivCredentials{
		{},
		{Cookie: "fixture", UserID: "42"},
		{UserAgent: "fixture", UserID: "42"},
		{UserAgent: "fixture", Cookie: "fixture", UserID: "-1"},
		{UserAgent: "fixture", Cookie: "fixture", UserID: "invalid"},
	} {
		if got := checkTGCredentialPixiv(context.Background(), credentials, proxy); got != tgQRCredentialInvalid {
			t.Errorf("missing credentials returned %v", got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	credentials := PixivCredentials{UserAgent: "fixture-browser", Cookie: "PHPSESSID=fixture-session", UserID: "42"}
	if got := checkTGCredentialPixiv(ctx, credentials, proxy); got != tgQRCredentialUnknown || requests.Load() != 0 {
		t.Fatal("canceled check did not stop locally")
	}
	if got := checkTGCredentialPixiv(context.Background(), credentials, "://invalid-proxy"); got != tgQRCredentialUnknown {
		t.Fatal("proxy error was treated as an expired credential")
	}

	for _, tc := range []struct {
		location string
		want     tgQRCredentialStatus
	}{
		{"https://accounts.pixiv.net/login?return_to=fixture", tgQRCredentialInvalid},
		{"https://www.pixiv.net/login.php", tgQRCredentialInvalid},
		{"https://untrusted.invalid/login", tgQRCredentialUnknown},
		{"http://www.pixiv.net/login.php", tgQRCredentialUnknown},
	} {
		t.Run(tc.location, func(t *testing.T) {
			var redirects atomic.Int32
			redirectProxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
				redirects.Add(1)
				w.Header().Set("Location", tc.location)
				w.WriteHeader(http.StatusFound)
			})
			if got := checkTGCredentialPixiv(context.Background(), credentials, redirectProxy); got != tc.want || redirects.Load() != 1 {
				t.Fatalf("redirect status=%v want=%v requests=%d", got, tc.want, redirects.Load())
			}
		})
	}
}

func TestTGCredentialPixivCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
			t.Error("Pixiv check ignored cancellation")
		}
	})
	credentials := PixivCredentials{UserAgent: "fixture-browser", Cookie: "PHPSESSID=fixture-session", UserID: "42"}
	if got := checkTGCredentialPixiv(ctx, credentials, proxy); got != tgQRCredentialUnknown {
		t.Fatal("canceled request was not unknown")
	}
}

func TestTGCredentialPixivMetadataConsistencyAndSameOriginRedirect(t *testing.T) {
	current := `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"isLoggedIn":true,"gaUserData":{"login":true,"userId":"42"}}}}</script>`
	legacy := `<meta id="meta-global-data" content='{&quot;userData&quot;:{&quot;id&quot;:&quot;42&quot;}}'>`
	for _, tc := range []struct {
		name, body string
		want       tgQRCredentialStatus
	}{
		{"escaped legacy", legacy, tgQRCredentialValid},
		{"agreeing formats", current + legacy, tgQRCredentialValid},
		{"conflicting formats", current + `<meta id="meta-global-data" content='{"userData":null}'>`, tgQRCredentialUnknown},
		{"duplicate current", current + current, tgQRCredentialUnknown},
		{"duplicate legacy", legacy + legacy, tgQRCredentialUnknown},
		{"only public schema", `<script type="application/ld+json">{"userData":{"id":"42"}}</script>`, tgQRCredentialUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tgPixivPageViewerStatus([]byte(tc.body), "42"); got != tc.want {
				t.Errorf("status=%v want=%v", got, tc.want)
			}
		})
	}
	var requests atomic.Int32
	proxy := authorWindowProxy(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Cookie") != "PHPSESSID=fixture-session" || r.Host != "www.pixiv.net" {
			t.Error("same-origin redirect changed request credentials or host")
		}
		if r.URL.Path == "/" {
			w.Header().Set("Location", "/en/")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path != "/en/" {
			t.Error("unexpected redirected route")
		}
		fmt.Fprint(w, current)
	})
	credentials := PixivCredentials{UserAgent: "fixture-browser", Cookie: "PHPSESSID=fixture-session", UserID: "42"}
	if got := checkTGCredentialPixiv(context.Background(), credentials, proxy); got != tgQRCredentialValid || requests.Load() != 2 {
		t.Fatalf("same-origin redirect status=%v requests=%d", got, requests.Load())
	}
}
