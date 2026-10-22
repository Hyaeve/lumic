package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func writeWeiboTimelineFixture(w http.ResponseWriter, id, caption string, total int) {
	fmt.Fprintf(w, `{"ok":1,"data":{"total":%d,"list":[{"id":%q,"text":%q,"created_at":"Wed Oct 21 12:00:00 +0800 2026","user":{"idstr":"42","screen_name":"公开作者"}}]}}`, total, id, caption)
}

func TestWeiboHistoryRestartsPublicPaginationAfterLoginExpires(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "99", Cookie: "SUB=expired"})
	var requests []string
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		mode := "public"
		if r.Header.Get("Cookie") != "" {
			mode = "login"
		}
		page := r.URL.Query().Get("page")
		requests = append(requests, mode+":"+page)
		if mode == "login" && page == "1" {
			writeWeiboTimelineFixture(w, "private-first", "登录可见", 20)
		} else if mode == "public" && page == "1" {
			writeWeiboTimelineFixture(w, "public-first", "公开可见", 999)
		} else {
			fmt.Fprint(w, `{"ok":0,"msg":"请先登录"}`)
		}
	})
	posts, report, err := store.fetchWeiboPostsWithReport(SourceConfig{ID: "weibo-42", Source: SourceWeibo}, true)
	if err != nil || len(posts) != 1 || posts[0].ID != "weibo-status-public-first" {
		t.Fatalf("mixed visibility scopes or lost public history: posts=%v err=%v", posts, err)
	}
	if strings.Join(requests, ",") != "login:1,login:2,public:1,public:2" || !report.Anonymous || !report.LoginExpired || report.Fetched != 1 || report.ReportedTotal != 20 {
		t.Fatalf("incorrect fallback report or pagination: %v %+v", requests, report)
	}
	warning := report.warning(true)
	if !strings.Contains(warning, "仅获取 1 条") || !strings.Contains(warning, "约 19 条未返回") || !strings.Contains(warning, "需登录后重新拉取历史动态") {
		t.Fatal("history scope was not reported", warning)
	}
}

func TestWeiboAnonymousHistoryDoesNotInventGlobalTotals(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			writeWeiboTimelineFixture(w, "public", "公开动态", 999)
		} else {
			fmt.Fprint(w, `{"ok":1,"data":{"cards":[],"cardlistInfo":{"total":999}}}`)
		}
	})
	posts, report, err := store.fetchWeiboPostsWithReport(SourceConfig{ID: "weibo-42", Source: SourceWeibo}, true)
	if err != nil || len(posts) != 1 || report.ReportedTotal != 0 || strings.Contains(report.warning(true), "尚有") || !strings.Contains(report.warning(true), "需登录") {
		t.Fatalf("public list count was treated as global history: %+v %v", report, err)
	}
}

func TestWeiboRecoveredLoginDoesNotDowngradeToAnonymous(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "99", Cookie: "SUB=old", PasswordUsername: "fixture", Password: "secret"})
	previous := weiboPasswordLogin
	weiboPasswordLogin = func(_, _, _ string) (WeiboCredentials, error) {
		return WeiboCredentials{UserID: "99", Cookie: "SUB=renewed"}, nil
	}
	t.Cleanup(func() { weiboPasswordLogin = previous })
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			t.Error("working login should never use anonymous access")
		}
		if r.Header.Get("Cookie") != "SUB=renewed" {
			w.WriteHeader(http.StatusUnauthorized)
		} else if r.URL.Query().Get("page") == "1" {
			writeWeiboTimelineFixture(w, "post", "动态", 1)
		} else {
			fmt.Fprint(w, `{"ok":1,"data":{"list":[]}}`)
		}
	})
	posts, report, err := store.fetchWeiboPostsWithReport(SourceConfig{ID: "weibo-42", Source: SourceWeibo}, true)
	if err != nil || len(posts) != 1 || report.Anonymous || report.warning(true) != "" {
		t.Fatalf("recovered login was not preferred: %+v %v", report, err)
	}
}

func TestWeiboCaptchaRecoveryNeverDowngradesToAnonymous(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "99", Cookie: "SUB=old", PasswordUsername: "fixture", Password: "secret"})
	previous := weiboPasswordLogin
	weiboPasswordLogin = func(_, _, _ string) (WeiboCredentials, error) {
		return WeiboCredentials{}, errors.New("请完成安全验证和验证码")
	}
	t.Cleanup(func() { weiboPasswordLogin = previous })
	requests := 0
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(http.StatusUnauthorized) })
	_, report, err := store.fetchWeiboPostsWithReport(SourceConfig{ID: "weibo-42", Source: SourceWeibo}, true)
	if err == nil || requests != 1 || report.Anonymous || weiboErrorKind(err) != "restricted" {
		t.Fatal("security verification must be surfaced without changing access mode", err)
	}
}

func TestWeiboHistoryWarningPersistsAndClearsAfterAuthenticatedSync(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	feed := SourceConfig{ID: "weibo-42", Source: SourceWeibo, Name: "公开作者", Enabled: true, IncludeKeywords: []string{"keep"}}
	store.config.WeiboSubscriptions = []SourceConfig{feed}
	store.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{}, feeds: []SourceConfig{}}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ajax/profile/info" {
			fmt.Fprint(w, `{"ok":1,"data":{"user":{"idstr":"42","screen_name":"公开作者"}}}`)
		} else if r.URL.Query().Get("page") == "1" {
			fmt.Fprint(w, `{"ok":1,"data":{"list":[{"id":"1","text":"keep","user":{"idstr":"42","screen_name":"公开作者"}},{"id":"2","text":"excluded","user":{"idstr":"42","screen_name":"公开作者"}}]}}`)
		} else {
			fmt.Fprint(w, `{"ok":1,"data":{"list":[]}}`)
		}
	})
	updated, err := store.syncSource(feed, true)
	if err != nil || updated.LastSyncStatus != "success" || updated.LastSyncCount != 1 || !strings.Contains(updated.LastSyncWarning, "仅获取 2 条") || !strings.Contains(updated.LastSyncMessage, "符合设置 1 条") || strings.Contains(updated.LastSyncMessage, "历史动态获取完成") {
		t.Fatalf("raw count, archive count, or incomplete scope was lost: %+v %v", updated, err)
	}
	if toAPIV1Feed(updated).LastSyncWarning != updated.LastSyncWarning {
		t.Fatal("v1 clients lost the history warning")
	}
	store.config.Weibo = WeiboCredentials{UserID: "99", Cookie: "SUB=valid"}
	updated, err = store.syncSource(updated, true)
	if err != nil || updated.LastSyncWarning != "" || strings.Contains(updated.LastSyncMessage, "免登录") {
		t.Fatal("successful authenticated sync retained an old anonymous warning", err)
	}
}

func TestWeiboHistoryPageLimitDoesNotClaimCompleteness(t *testing.T) {
	store := newWeiboAuthTestStore(t, WeiboCredentials{UserID: "99", Cookie: "SUB=valid"})
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeWeiboTimelineFixture(w, r.URL.Query().Get("page"), "动态", 1000)
	})
	_, report, err := store.fetchWeiboPostsWithReport(SourceConfig{ID: "weibo-42", Source: SourceWeibo}, true)
	if err != nil || report.Fetched != 100 || !report.PageLimited || !strings.Contains(report.warning(true), "不代表完整历史") {
		t.Fatalf("page cap was presented as complete history: %+v %v", report, err)
	}
}
