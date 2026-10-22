package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type weiboTestTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport weiboTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = transport.target.Scheme, transport.target.Host
	return transport.base.RoundTrip(clone)
}

func mockWeiboAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	target, _ := url.Parse(server.URL)
	previous := weiboHTTPClient
	weiboHTTPClient = func(string) (*http.Client, error) {
		return &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}, nil
	}
	t.Cleanup(func() { weiboHTTPClient = previous; server.Close() })
}

func TestWeiboPublicAuthorIDParsing(t *testing.T) {
	for _, raw := range []string{"123456", "https://weibo.com/u/123456?tabtype=feed", "https://m.weibo.cn/profile/123456", "weibo.com/123456"} {
		id, err := weiboAuthorID(raw)
		if err != nil || id != "123456" {
			t.Fatalf("ID not resolved: %q %q %v", raw, id, err)
		}
	}
	for _, raw := range []string{"https://example.com/u/123456", "https://weibo.com@localhost/u/123456", "https://weibo.com:9999/u/123456", "https://weibo.com/u/../123456"} {
		if _, err := weiboAuthorID(raw); err == nil {
			t.Fatal("untrusted or unsupported URL accepted", raw)
		}
	}
	if id, err := weiboAuthorID("普通昵称"); err != nil || id != "" {
		t.Fatal("name search must remain available")
	}
}

func TestWeiboTimelinePrefersStoredAccountAndSupportsAnonymous(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{Cookie: "SUB=account", UserID: "99"})
	requests := 0
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Cookie") == "" && r.URL.Query().Get("containerid") != "10760342" || r.Header.Get("Cookie") != "" && r.URL.Path != "/ajax/statuses/mymblog" {
			t.Error("author request used the wrong access mode or source")
		}
		fmt.Fprint(w, `{"ok":1,"data":{"cards":[{"mblog":{"id":"900","text":"公开动态","user":{"idstr":"42","screen_name":"公开作者"}}}]}}`)
	})
	posts, err := store.fetchWeiboPosts(SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "公开作者"}, false)
	if err != nil || len(posts) != 1 || requests != 1 {
		t.Fatalf("public timeline failed: count=%d requests=%d err=%v", len(posts), requests, err)
	}
	store.config.Weibo = WeiboCredentials{}
	if posts, err = store.fetchWeiboPosts(SourceConfig{ID: "weibo-42", Source: SourceWeibo}, false); err != nil || len(posts) != 1 {
		t.Fatal("anonymous timeline was blocked", err)
	}
}

func TestWeiboAuthorAccessOnlyFallsBackWhenSessionExpires(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			store := newWeiboAuthTestStore(t, WeiboCredentials{Cookie: "SUB=account", UserID: "99"})
			requests := 0
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("Cookie") == "" {
					fmt.Fprint(w, `{"ok":1}`)
					return
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `{"ok":0,"msg":"请先登录"}`)
			})
			credentials := store.config.Weibo
			_, _, err := store.weiboPublicRequest("https://m.weibo.cn/api/container/getIndex", &credentials, "")
			if status == 401 && (err != nil || requests != 2) || status != 401 && (err == nil || requests != 1) {
				t.Fatalf("unexpected fallback: requests=%d err=%v", requests, err)
			}
		})
	}
}

func TestWeiboPublicSearchAndSubscriptionWithoutLogin(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("anonymous search sent a Cookie")
		}
		fmt.Fprint(w, `{"ok":1,"data":{"userInfo":{"idstr":"42","screen_name":"公开作者"}}}`)
	})
	for _, keyword := range []string{"42", "https://m.weibo.cn/profile/42?test=long-homepage-query", "公开作者"} {
		response := httptest.NewRecorder()
		store.weiboSearchHandler(response, httptest.NewRequest(http.MethodGet, "/api/weibo/search?keyword="+url.QueryEscape(keyword), nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"userId":"42"`) {
			t.Fatalf("anonymous search failed: %d %s", response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(response, httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions", strings.NewReader(`{"userId":"42","name":"公开作者","avatar":"/flow/fixture/avatar.jpg"}`)))
	if response.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 1 {
		t.Fatalf("anonymous author subscription blocked: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	store.weiboSubscriptionsHandler(response, httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions?type=likes", nil))
	if response.Code != http.StatusPreconditionFailed {
		t.Fatal("account likes must still require authentication", response.Code)
	}
}

func TestWeiboCredentialsRequireSessionIdentityNotPublicProfile(t *testing.T) {
	for _, test := range []struct {
		body  string
		valid bool
	}{
		{`{"ok":1,"data":{"uid":"42","islogin":true}}`, true},
		{`{"ok":1,"data":{"uid":"42","islogin":false}}`, false},
		{`{"ok":1,"data":{"uid":"99","islogin":true}}`, false},
		{`{"ok":1,"data":{}}`, false},
	} {
		t.Run(test.body, func(t *testing.T) {
			mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/ajax/config/get" {
					t.Error("validation must probe the actual session identity")
				}
				fmt.Fprint(w, test.body)
			})
			original := fetchWeiboAccountProfile
			profileCalls := 0
			fetchWeiboAccountProfile = func(string, WeiboCredentials, string) (WeiboUser, error) {
				profileCalls++
				return WeiboUser{UserID: "42", Name: "账号"}, nil
			}
			t.Cleanup(func() { fetchWeiboAccountProfile = original })
			_, err := validateWeiboCredentials("SUB=fixture", "42", "")
			if (err == nil) != test.valid || test.valid && profileCalls != 1 || !test.valid && profileCalls != 0 {
				t.Fatalf("invalid session accepted using public profile: calls=%d err=%v", profileCalls, err)
			}
		})
	}
}

func TestWeiboAccountSwitchClearsPreviousBackupPassword(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "42", Cookie: "SUB=old", PasswordUsername: "old-account", Password: "old-secret"})
	original := validateWeiboLoginSession
	validateWeiboLoginSession = func(string, string, string) (WeiboCredentials, error) {
		return WeiboCredentials{UserID: "99", Cookie: "SUB=new"}, nil
	}
	t.Cleanup(func() { validateWeiboLoginSession = original })
	response := httptest.NewRecorder()
	store.weiboAccountHandler(response, httptest.NewRequest(http.MethodPut, "/api/weibo/account", strings.NewReader(`{"cookie":"SUB=new","userId":"99"}`)))
	if response.Code != http.StatusOK || hasWeiboPassword(store.config.Weibo) {
		t.Fatal("switching accounts retained the old account's backup password")
	}
}

func TestWeiboLikesNeverFallBackToAnonymousOrAnotherAccount(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid account likes request reached the upstream")
	})
	if _, err := store.fetchWeiboLikedPosts(SourceConfig{}, false); err == nil {
		t.Fatal("anonymous account likes accepted")
	}
	store.config.Weibo = WeiboCredentials{UserID: "99", Cookie: "SUB=session"}
	if _, err := store.fetchWeiboLikedPosts(SourceConfig{ID: "weibo-likes-42"}, false); err == nil {
		t.Fatal("another account's likes source was accepted")
	}
}
