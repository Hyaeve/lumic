package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadRemoteImageAndFlowPath(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.bilibili.com/" || !strings.Contains(r.Header.Get("Cookie"), "SESSDATA=test") {
			http.Error(w, "missing anti-hotlink headers", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-image"))
	}))
	defer server.Close()
	targetBase := filepath.Join(sourceStoragePath(SourceBilibili, "测试/UP"), "avatar")
	localPath, err := downloadRemoteImage(server.Client(), server.URL+"/avatar", targetBase, "https://www.bilibili.com/", "SESSDATA=test")
	if err != nil {
		t.Fatalf("download image: %v", err)
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("downloaded image missing: %v", err)
	}
	if got := flowPublicPath(SourceBilibili, "测试/UP", filepath.Base(localPath)); got != "/flow/bilibili/%E6%B5%8B%E8%AF%95_UP/avatar.png" {
		t.Fatalf("unexpected public path: %s", got)
	}
}

func TestPostMediaBaseNameUsesAuthorDateAndImageSequence(t *testing.T) {
	published := time.Date(2026, time.August, 13, 20, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	single := Post{Author: "测试作者", Published: published, Media: []string{"one"}}
	if got := postMediaBaseName(single, 0); got != "测试作者20260813" {
		t.Fatalf("unexpected single image name: %s", got)
	}
	multiple := Post{Author: "测试作者", Published: published, Media: []string{"one", "two", "three"}}
	for index, want := range []string{"测试作者20260813-1", "测试作者20260813-2", "测试作者20260813-3"} {
		if got := postMediaBaseName(multiple, index); got != want {
			t.Fatalf("unexpected multiple image name at %d: got=%s want=%s", index, got, want)
		}
	}
}

func TestAvailableMediaTargetBaseAvoidsOverwrite(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "作者20260813.jpg"), []byte("existing"), 0600); err != nil {
		t.Fatalf("seed existing image: %v", err)
	}
	if got := filepath.Base(availableMediaTargetBase(directory, "作者20260813")); got != "作者20260813-2" {
		t.Fatalf("unexpected collision name: %s", got)
	}
}

func TestBilibiliCaptionPreservesText(t *testing.T) {
	caption := firstNonEmptyRemoteText("\n第一行\n第二行\n")
	if caption != "第一行\n第二行" {
		t.Fatalf("caption lost line breaks: %q", caption)
	}
	if got := firstNonEmptyRemoteText("", "标题"); got != "标题" {
		t.Fatalf("caption fallback: %q", got)
	}
	if got := cleanRemoteText("第一段<br>第二段</p><p>第三段"); got != "第一段\n第二段\n第三段" {
		t.Fatalf("HTML line breaks were lost: %q", got)
	}
	if got := combineRemoteText("动态描述", "标题", "正文第一行<br>正文第二行", "动态描述"); got != "动态描述\n\n标题\n\n正文第一行\n正文第二行" {
		t.Fatalf("combined caption lost content: %q", got)
	}
}

func TestAllowedBilibiliDynamicTypeIncludesTextButNotVideo(t *testing.T) {
	for _, dynamicType := range []string{"DYNAMIC_TYPE_WORD", "DYNAMIC_TYPE_DRAW", "DYNAMIC_TYPE_ARTICLE", "DYNAMIC_TYPE_OPUS"} {
		if !allowedBilibiliDynamicType(dynamicType) {
			t.Fatalf("expected %s to be collected", dynamicType)
		}
	}
	for _, dynamicType := range []string{"DYNAMIC_TYPE_AV", "DYNAMIC_TYPE_FORWARD", "DYNAMIC_TYPE_LIVE_RCMD"} {
		if allowedBilibiliDynamicType(dynamicType) {
			t.Fatalf("expected %s to remain filtered", dynamicType)
		}
	}
}

func TestBilibiliRichTextExtractsOpusCaption(t *testing.T) {
	raw := json.RawMessage(`{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":""},{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"非常好灵梦画了"}]}`)
	if got := bilibiliRichText(raw); got != "非常好灵梦画了" {
		t.Fatalf("opus caption was not extracted: %q", got)
	}
	plain := json.RawMessage(`"普通动态文字"`)
	if got := bilibiliRichText(plain); got != "普通动态文字" {
		t.Fatalf("plain caption was not extracted: %q", got)
	}
}

func TestBilibiliRichTextExtractsNestedDrawCaption(t *testing.T) {
	raw := json.RawMessage(`{"major":{"draw":{"desc":{"rich_text_nodes":[{"text":"图文动态正文"}]}}}}`)
	if got := bilibiliRichText(raw); got != "图文动态正文" {
		t.Fatalf("nested draw caption was not extracted: %q", got)
	}
}

func TestBilibiliCaptionFromRawItemPreservesUnknownNestedText(t *testing.T) {
	raw := json.RawMessage(`{"id_str":"123","modules":{"module_dynamic":{"major":{"opus":{"summary":{"paragraphs":[{"nodes":[{"text":"列表原始 JSON 中的完整文案"}]}]}}}}}}`)
	if got := bilibiliCaptionFromRaw(raw); got != "列表原始 JSON 中的完整文案" {
		t.Fatalf("raw item caption was not extracted: %q", got)
	}
}

func TestBilibiliCaptionDecodesJSONStringContent(t *testing.T) {
	raw := json.RawMessage(`{"content":"{\"paragraphs\":[{\"nodes\":[{\"text\":\"full opus caption\"}]}]}"}`)
	if got := bilibiliRichText(raw); got != "full opus caption" {
		t.Fatalf("JSON string caption was not extracted: %q", got)
	}
}

func TestBilibiliDetailCaptionPrefersFullText(t *testing.T) {
	if got := mergeDetailedRemoteText("caption preview", "caption preview with the complete body"); got != "caption preview with the complete body" {
		t.Fatalf("detail caption did not replace preview: %q", got)
	}
	if !shouldFetchBilibiliDynamicDetail("DYNAMIC_TYPE_OPUS", "preview") || shouldFetchBilibiliDynamicDetail("DYNAMIC_TYPE_DRAW", "complete draw caption") {
		t.Fatal("unexpected Bilibili detail request decision")
	}
}

func TestPixivBrowserCredentialsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax/user/12345" || r.URL.Query().Get("full") != "1" {
			http.Error(w, "unexpected Pixiv endpoint", http.StatusNotFound)
			return
		}
		for header, want := range map[string]string{
			"User-Agent":   "Mozilla/5.0 Test",
			"Baggage":      "sentry-environment=production",
			"Cookie":       "PHPSESSID=test_session",
			"Sentry-Trace": "trace-value",
			"X-CSRF-Token": "csrf-value",
		} {
			if got := r.Header.Get(header); got != want {
				t.Fatalf("unexpected %s header: %q", header, got)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":false,"message":"","body":{"userId":"12345","name":"Pixiv User"}}`))
	}))
	defer server.Close()

	credentials, err := pixivBrowserCredentialsRequest(PixivCredentials{
		UserAgent:   "Mozilla/5.0 Test",
		Baggage:     "sentry-environment=production",
		Cookie:      "PHPSESSID=test_session",
		UserID:      "12345",
		SentryTrace: "trace-value",
		CSRFToken:   "csrf-value",
	}, "", server.URL)
	if err != nil {
		t.Fatalf("validate Pixiv browser credentials: %v", err)
	}
	if credentials.UserName != "Pixiv User" || credentials.UserID != "12345" {
		t.Fatalf("unexpected Pixiv account: %#v", credentials)
	}
}

func TestCollectedWeiboPostIncludesOriginalURL(t *testing.T) {
	payload := map[string]any{"mblog": map[string]any{"id": "AbCd", "text_raw": "正文", "created_at": "Mon Aug 12 10:00:00 +0800 2024", "user": map[string]any{"idstr": "12345", "screen_name": "博主"}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || posts[0].OriginalURL != "https://weibo.com/12345/AbCd" {
		t.Fatalf("unexpected original URL: %#v", posts)
	}
}

func TestFilterSourcePosts(t *testing.T) {
	posts := []Post{
		{ID: "keep", Caption: "今天分享一张插画", Media: []string{"image.jpg"}},
		{ID: "blocked", Caption: "插画抽奖活动", Media: []string{"image.jpg"}},
		{ID: "text", Caption: "插画文字记录"},
		{ID: "miss", Caption: "普通摄影", Media: []string{"image.jpg"}},
	}
	feed := SourceConfig{OnlyWithImages: true, IncludeKeywords: []string{"插画", "绘画"}, ExcludeKeywords: []string{"抽奖"}}
	filtered := filterSourcePosts(posts, feed)
	if len(filtered) != 1 || filtered[0].ID != "keep" {
		t.Fatalf("unexpected filtered posts: %#v", filtered)
	}
}

func TestCronScheduleMatchingAndMigration(t *testing.T) {
	at := time.Date(2026, 8, 13, 6, 0, 0, 0, time.Local)
	for _, expression := range []string{"0 6 * * *", "0 */6 * * *", "0 6 * * 4"} {
		if !validCron(expression) || !cronMatches(expression, at) {
			t.Fatalf("cron should match: %s", expression)
		}
	}
	if cronMatches("30 6 * * *", at) {
		t.Fatal("cron matched the wrong minute")
	}
	if normalizeSchedule("每 6 小时") != "0 */6 * * *" || normalizeSchedule("") != "0 6 * * *" {
		t.Fatal("legacy schedule migration failed")
	}
}

func TestMergePostsUpdatesArchivedMedia(t *testing.T) {
	store := &Store{posts: []Post{{ID: "post-1", Source: SourceBilibili, Author: "UP", Media: []string{"https://remote/image.jpg"}, Liked: true}}}
	added, err := store.mergePosts([]Post{{ID: "post-1", Source: SourceBilibili, Author: "UP", Media: []string{"/flow/bilibili/UP/post-1.jpg"}}})
	if err != nil || added != 0 {
		t.Fatalf("merge existing post: added=%d err=%v", added, err)
	}
	if store.posts[0].Media[0] != "/flow/bilibili/UP/post-1.jpg" || !store.posts[0].Liked {
		t.Fatalf("existing post was not updated correctly: %#v", store.posts[0])
	}
}

func TestPostsAfterUsesStrictIncrementalBoundary(t *testing.T) {
	boundary := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	posts := []Post{
		{ID: "new", Published: boundary.Add(time.Second)},
		{ID: "same", Published: boundary},
		{ID: "old", Published: boundary.Add(-time.Second)},
	}
	filtered := postsAfter(posts, boundary)
	if len(filtered) != 1 || filtered[0].ID != "new" {
		t.Fatalf("incremental sync kept the wrong posts: %#v", filtered)
	}
	all := postsAfter(posts, time.Time{})
	if len(all) != len(posts) {
		t.Fatalf("full sync boundary unexpectedly filtered posts: %#v", all)
	}
}

func TestServeFrontendFallsBackForRoutesButNotMissingAssets(t *testing.T) {
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "public", "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "index.html"), []byte("spa-index"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "assets", "app.js"), []byte("app-code"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })

	for _, route := range []string{"/liked", "/source/bilibili", "/settings/platforms", "/author/bilibili/%E6%B5%8B%E8%AF%95"} {
		response := httptest.NewRecorder()
		serveFrontend(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "spa-index") {
			t.Fatalf("route %s did not receive SPA index: status=%d body=%q", route, response.Code, response.Body.String())
		}
	}
	assetResponse := httptest.NewRecorder()
	serveFrontend(assetResponse, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if assetResponse.Code != http.StatusNotFound {
		t.Fatalf("missing asset status=%d", assetResponse.Code)
	}
}

func TestLoginWeiboWithPasswordExchangesSession(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	oldPreloginEndpoint := weiboPreloginEndpoint
	oldEndpoint := weiboLoginEndpoint
	oldValidator := validateWeiboLoginSession
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prelogin" {
			if r.Method != http.MethodGet || r.URL.Query().Get("su") != encodeWeiboUsername("account") {
				t.Fatalf("unexpected prelogin request: %s %s", r.Method, r.URL.String())
			}
			fmt.Fprintf(w, `sinaSSOController.preloginCallBack({"servertime":123456,"nonce":"nonce","pubkey":"%x","rsakv":"rsa-version","showpin":0})`, privateKey.N)
			return
		}
		if r.URL.Path != "/login" || r.Method != http.MethodPost {
			t.Fatalf("unexpected login request: %s %s", r.Method, r.URL.String())
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		ciphertext, err := hex.DecodeString(r.Form.Get("sp"))
		if err != nil {
			t.Fatalf("password was not hex encoded: %v", err)
		}
		plain, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, ciphertext)
		if err != nil || string(plain) != "123456\tnonce\nsecret" || r.Form.Get("pwencode") != "rsa2" || r.Form.Get("su") != encodeWeiboUsername("account") {
			t.Fatalf("credentials were not SSO encoded correctly: plain=%q form=%#v err=%v", plain, r.Form, err)
		}
		http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "session", Path: "/"})
		writeJSON(w, map[string]any{"retcode": 0, "uid": "42"})
	}))
	defer server.Close()
	weiboPreloginEndpoint = server.URL + "/prelogin"
	weiboLoginEndpoint = server.URL + "/login"
	validateWeiboLoginSession = func(cookie, userID, proxyURL string) (WeiboCredentials, error) {
		if !strings.Contains(cookie, "SUB=session") || userID != "42" {
			t.Fatalf("unexpected session: cookie=%q userID=%q", cookie, userID)
		}
		return WeiboCredentials{Cookie: cookie, UserID: userID, UserName: "测试账号"}, nil
	}
	t.Cleanup(func() {
		weiboPreloginEndpoint = oldPreloginEndpoint
		weiboLoginEndpoint = oldEndpoint
		validateWeiboLoginSession = oldValidator
	})

	credentials, err := loginWeiboWithPassword("account", "secret", "")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if credentials.UserID != "42" || credentials.UserName != "测试账号" {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
}

func TestWeiboAccountHandlerRequiresCompletePasswordLogin(t *testing.T) {
	store := &BilibiliStore{config: BilibiliConfig{}, key: make([]byte, 32)}
	request := httptest.NewRequest(http.MethodPut, "/api/weibo/account", strings.NewReader(`{"username":"account"}`))
	response := httptest.NewRecorder()
	store.weiboAccountHandler(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "账号和密码均不能为空") {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLoginWeiboWithPasswordExplainsPlatformSystemError(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	oldPreloginEndpoint, oldEndpoint := weiboPreloginEndpoint, weiboLoginEndpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prelogin" {
			fmt.Fprintf(w, `sinaSSOController.preloginCallBack({"servertime":123456,"nonce":"nonce","pubkey":"%x","rsakv":"rsa-version","showpin":0})`, privateKey.N)
			return
		}
		writeJSON(w, map[string]any{"retcode": 50000000, "reason": "系统错误，请稍后再试"})
	}))
	defer server.Close()
	weiboPreloginEndpoint, weiboLoginEndpoint = server.URL+"/prelogin", server.URL+"/login"
	t.Cleanup(func() { weiboPreloginEndpoint, weiboLoginEndpoint = oldPreloginEndpoint, oldEndpoint })

	_, err = loginWeiboWithPassword("account", "secret", "")
	if err == nil || !strings.Contains(err.Error(), "扫码或 Cookie 登录") {
		t.Fatalf("unexpected error guidance: %v", err)
	}
}

func TestPostsHandlerPersistsLikedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{{ID: "post-1", Author: "作者"}}, feeds: []SourceConfig{}, file: path}
	request := httptest.NewRequest(http.MethodPatch, "/api/posts?id=post-1", strings.NewReader(`{"liked":true}`))
	recorder := httptest.NewRecorder()
	store.postsHandler(recorder, request)
	if recorder.Code != http.StatusOK || !store.posts[0].Liked {
		t.Fatalf("like request failed: status=%d post=%#v body=%s", recorder.Code, store.posts[0], recorder.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted content: %v", err)
	}
	var content ContentData
	if err := json.Unmarshal(data, &content); err != nil || len(content.Posts) != 1 || !content.Posts[0].Liked {
		t.Fatalf("liked state was not persisted: err=%v content=%#v", err, content)
	}

	missingRequest := httptest.NewRequest(http.MethodPatch, "/api/posts?id=missing", strings.NewReader(`{"liked":true}`))
	missingRecorder := httptest.NewRecorder()
	store.postsHandler(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing post returned status %d", missingRecorder.Code)
	}
}

func TestPostsHandlerDeletesMultipleAndAuthorPosts(t *testing.T) {
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(t.TempDir(), "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	authorDirectory := sourceStoragePath(SourceWeibo, "甲")
	if err := os.MkdirAll(authorDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorDirectory, "media.jpg"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{
		{ID: "one", Source: SourceWeibo, Author: "甲"},
		{ID: "two", Source: SourceWeibo, Author: "乙"},
		{ID: "three", Source: SourceWeibo, Author: "甲"},
		{ID: "four", Source: SourceBilibili, Author: "甲"},
	}, feeds: []SourceConfig{}, file: path}
	request := httptest.NewRequest(http.MethodDelete, "/api/posts", strings.NewReader(`{"ids":["two"]}`))
	response := httptest.NewRecorder()
	store.postsHandler(response, request)
	if response.Code != http.StatusOK || len(store.posts) != 3 {
		t.Fatalf("batch delete failed: status=%d posts=%#v body=%s", response.Code, store.posts, response.Body.String())
	}
	authorRequest := httptest.NewRequest(http.MethodDelete, "/api/posts", strings.NewReader(`{"source":"weibo","author":"甲"}`))
	authorResponse := httptest.NewRecorder()
	store.postsHandler(authorResponse, authorRequest)
	if authorResponse.Code != http.StatusOK || len(store.posts) != 1 || store.posts[0].ID != "four" {
		t.Fatalf("author delete failed: status=%d posts=%#v body=%s", authorResponse.Code, store.posts, authorResponse.Body.String())
	}
	if _, err := os.Stat(authorDirectory); !os.IsNotExist(err) {
		t.Fatalf("author content directory was not deleted: %v", err)
	}
}

func decodeTestResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return payload
}

func TestNormalizeWeiboCookieAndRequestHeaders(t *testing.T) {
	cookie := normalizeWeiboCookie("Cookie: SUB=first; XSRF-TOKEN=token; sub=duplicate; empty\nignored")
	if cookie != "SUB=first; XSRF-TOKEN=token" {
		t.Fatalf("unexpected normalized cookie: %q", cookie)
	}
	if got := weiboCookieValue(cookie, "xsrf-token"); got != "token" {
		t.Fatalf("unexpected cookie value: %q", got)
	}
	mobileRequest := httptest.NewRequest(http.MethodGet, "https://m.weibo.cn/api/container/getIndex", nil)
	weiboRequestHeaders(mobileRequest, cookie)
	if mobileRequest.Header.Get("Origin") != "https://m.weibo.cn" || mobileRequest.Header.Get("Referer") != "https://m.weibo.cn/" {
		t.Fatalf("unexpected mobile headers: origin=%q referer=%q", mobileRequest.Header.Get("Origin"), mobileRequest.Header.Get("Referer"))
	}
	if mobileRequest.Header.Get("X-XSRF-TOKEN") != "token" {
		t.Fatalf("missing XSRF header: %q", mobileRequest.Header.Get("X-XSRF-TOKEN"))
	}
	webRequest := httptest.NewRequest(http.MethodGet, "https://weibo.com/ajax/profile/info", nil)
	weiboRequestHeaders(webRequest, cookie)
	if webRequest.Header.Get("Origin") != "https://weibo.com" || webRequest.Header.Get("Referer") != "https://weibo.com/" {
		t.Fatalf("unexpected web headers: origin=%q referer=%q", webRequest.Header.Get("Origin"), webRequest.Header.Get("Referer"))
	}
}

func TestWeiboJSONPAndCrossDomainURL(t *testing.T) {
	body := []byte(`STK_123({"retcode":20000000,"data":{"alt":"ticket"}})`)
	var payload struct {
		RetCode int `json:"retcode"`
		Data    struct {
			Alt string `json:"alt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(unwrapJSONP(body), &payload); err != nil {
		t.Fatalf("decode JSONP: %v", err)
	}
	if payload.RetCode != 20000000 || payload.Data.Alt != "ticket" {
		t.Fatalf("unexpected JSONP payload: %#v", payload)
	}
	if got := weiboCrossDomainURL("https://example.com/cross?a=1", 0); got != "https://example.com/cross?a=1&action=login" {
		t.Fatalf("unexpected first cross-domain URL: %s", got)
	}
	if got := weiboCrossDomainURL("https://example.com/cross", 0); got != "https://example.com/cross?action=login" {
		t.Fatalf("unexpected URL without query: %s", got)
	}
	if got := weiboCrossDomainURL("https://example.com/other?a=1", 1); got != "https://example.com/other?a=1" {
		t.Fatalf("non-first URL changed: %s", got)
	}
}

func TestRemoteValueParsing(t *testing.T) {
	if got := jsonValueString(float64(3546637624412244)); got != "3546637624412244" {
		t.Fatalf("unexpected float UID: %s", got)
	}
	if got := jsonValueString(json.Number("3546637624412244")); got != "3546637624412244" {
		t.Fatalf("unexpected JSON number UID: %s", got)
	}
	if got := parseRemoteTimestamp(json.RawMessage(`"1723456800"`)); got != 1723456800 {
		t.Fatalf("unexpected string timestamp: %d", got)
	}
}

func TestCollectWeiboUsers(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"cards": []any{
				map[string]any{"user": map[string]any{"idstr": "3546637624412244", "screen_name": "测试博主", "avatar": "//example.com/avatar.jpg", "followers_count": float64(1234)}},
				map[string]any{"user": map[string]any{"id": json.Number("3546637624412244"), "name": "重复用户"}},
			},
		},
	}
	users := make([]WeiboUser, 0)
	collectWeiboUsers(payload, &users, make(map[string]bool))
	if len(users) != 1 {
		t.Fatalf("expected one unique user, got %#v", users)
	}
	if users[0].UserID != "3546637624412244" || users[0].Name != "测试博主" || users[0].Fans != 1234 || users[0].Avatar != "https://example.com/avatar.jpg" {
		t.Fatalf("unexpected parsed user: %#v", users[0])
	}
}

func TestPostDelete(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	localMedia := filepath.Join(root, "weibo", "post-1.jpg")
	outsideMedia := filepath.Join(filepath.Dir(root), "must-remain.jpg")
	if err := os.MkdirAll(filepath.Dir(localMedia), 0755); err != nil {
		t.Fatalf("create media directory: %v", err)
	}
	if err := os.WriteFile(localMedia, []byte("local"), 0600); err != nil {
		t.Fatalf("create local media: %v", err)
	}
	if err := os.WriteFile(outsideMedia, []byte("outside"), 0600); err != nil {
		t.Fatalf("create outside media: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outsideMedia) })
	path := filepath.Join(root, "content.json")
	store := &Store{posts: []Post{{ID: "post-1", Source: SourceWeibo, Author: "测试作者", Media: []string{"/flow/weibo/post-1.jpg", "https://example.com/image.jpg", outsideMedia}}, {ID: "post-2", Source: SourcePixiv, Author: "保留作者"}}, feeds: []SourceConfig{}, file: path}
	store.Lock()
	if err := store.saveLocked(); err != nil {
		t.Fatalf("seed content store: %v", err)
	}
	store.Unlock()

	request := httptest.NewRequest(http.MethodDelete, "/api/posts?id=post-1", nil)
	response := httptest.NewRecorder()
	store.postsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.posts) != 1 || store.posts[0].ID != "post-2" {
		t.Fatalf("unexpected posts after delete: %#v", store.posts)
	}
	if _, err := os.Stat(localMedia); !os.IsNotExist(err) {
		t.Fatalf("local media was not deleted, stat error=%v", err)
	}
	if _, err := os.Stat(outsideMedia); err != nil {
		t.Fatalf("outside media was unexpectedly removed: %v", err)
	}

	reloaded, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("reload content store: %v", err)
	}
	if len(reloaded.posts) != 1 || reloaded.posts[0].ID != "post-2" {
		t.Fatalf("deleted post returned after reload: %#v", reloaded.posts)
	}

	missingRequest := httptest.NewRequest(http.MethodDelete, "/api/posts?id=post-1", nil)
	missingResponse := httptest.NewRecorder()
	store.postsHandler(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
}

func TestPostDeleteHandlesEncodedFlowURLWithQuery(t *testing.T) {
	root := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = root
	t.Cleanup(func() { flowRoot = oldFlowRoot })
	localMedia := filepath.Join(root, "bilibili", "测试作者", "dynamic-1.jpg")
	if err := os.MkdirAll(filepath.Dir(localMedia), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localMedia, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{posts: []Post{{ID: "post-encoded", Source: SourceBilibili, Author: "测试作者", Media: []string{"/flow/bilibili/%E6%B5%8B%E8%AF%95%E4%BD%9C%E8%80%85/dynamic-1.jpg?v=1"}}}, feeds: []SourceConfig{}, file: filepath.Join(root, "content.json")}
	response := httptest.NewRecorder()
	store.postsHandler(response, httptest.NewRequest(http.MethodDelete, "/api/posts?id=post-encoded", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(localMedia); !os.IsNotExist(err) {
		t.Fatalf("encoded local media was not deleted: %v", err)
	}
}

func TestRegularFeedSyncAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store := &Store{posts: []Post{}, feeds: []SourceConfig{{ID: "feed-demo", Source: SourceBilibili, Name: "演示来源", Enabled: true}}, file: path}
	store.Lock()
	if err := store.saveLocked(); err != nil {
		t.Fatalf("seed content store: %v", err)
	}
	store.Unlock()

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/feeds?action=sync&id=feed-demo", nil)
	syncResponse := httptest.NewRecorder()
	store.feedsHandler(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncResponse.Code, syncResponse.Body.String())
	}
	if status := decodeTestResponse(t, syncResponse)["status"]; status != "completed" {
		t.Fatalf("unexpected sync status: %v", status)
	}
	if store.feeds[0].LastSyncedAt.IsZero() {
		t.Fatal("sync did not update LastSyncedAt")
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/feeds?id=feed-demo", nil)
	deleteResponse := httptest.NewRecorder()
	store.feedsHandler(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if len(store.feeds) != 0 {
		t.Fatalf("feed was not deleted: %#v", store.feeds)
	}
	reloaded, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("reload content store: %v", err)
	}
	if len(reloaded.feeds) != 0 {
		t.Fatalf("deleted feed returned after reload: %#v", reloaded.feeds)
	}
}

func TestBilibiliSubscriptionKeepsLargeUserIDAsString(t *testing.T) {
	oldFile, oldFlowRoot := bilibiliFile, flowRoot
	tempDir := t.TempDir()
	bilibiliFile = filepath.Join(tempDir, "platform.enc")
	flowRoot = filepath.Join(tempDir, "flow")
	t.Cleanup(func() { bilibiliFile, flowRoot = oldFile, oldFlowRoot })
	store := &BilibiliStore{key: make([]byte, 32), config: BilibiliConfig{}, bilibiliQR: make(map[string]BilibiliQRSession), bilibiliClients: make(map[string]*http.Client), weiboQR: make(map[string]WeiboQRSession), weiboClients: make(map[string]*http.Client)}
	request := httptest.NewRequest(http.MethodPost, "/api/bilibili/subscriptions", strings.NewReader(`{"userId":"3546637624412244","name":"测试 UP 主","avatar":"https://example.com/up.jpg"}`))
	response := httptest.NewRecorder()
	store.subscriptionsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.config.Subscriptions) != 1 || store.config.Subscriptions[0].ID != "bili-3546637624412244" || store.config.Subscriptions[0].Avatar != "https://example.com/up.jpg" {
		t.Fatalf("large UID lost precision: %#v", store.config.Subscriptions)
	}
}

func TestCollectWeiboPosts(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"list": []any{map[string]any{"id": "987", "text_raw": "测试动态", "text": "<b>测试动态</b>", "created_at": "Mon Aug 12 10:00:00 +0800 2024", "user": map[string]any{"screen_name": "原博主", "avatar_hd": "https://example.com/original.jpg"}}}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{Name: "备用名称", Avatar: "fallback"}, &posts, make(map[string]bool))
	if len(posts) != 1 || posts[0].Author != "原博主" || posts[0].Avatar != "https://example.com/original.jpg" || posts[0].Caption != "测试动态" {
		t.Fatalf("unexpected parsed posts: %#v", posts)
	}
}

func TestCollectWeiboPostsPrefersOriginalImages(t *testing.T) {
	payload := map[string]any{"list": []any{map[string]any{
		"id": "original-image-test", "created_at": "Mon Aug 12 10:00:00 +0800 2024",
		"user": map[string]any{"screen_name": "author", "idstr": "123"},
		"pic_infos": map[string]any{"one": map[string]any{
			"large":    map[string]any{"url": "https://wx1.sinaimg.cn/large/example.jpg"},
			"original": map[string]any{"url": "https://wx1.sinaimg.cn/original/example.jpg"},
		}},
	}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Media) != 1 || posts[0].Media[0] != "https://wx1.sinaimg.cn/original/example.jpg" {
		t.Fatalf("original image was not preferred: %#v", posts)
	}
}

func TestCollectWeiboPostsUpgradesLargeImageURL(t *testing.T) {
	payload := map[string]any{"list": []any{map[string]any{
		"id": "large-image-test", "created_at": "Mon Aug 12 10:00:00 +0800 2024",
		"user": map[string]any{"screen_name": "author", "idstr": "123"},
		"pics": []any{map[string]any{"large": map[string]any{"url": "https://wx2.sinaimg.cn/mw690/example.png"}}},
	}}}
	posts := make([]Post, 0)
	collectWeiboPosts(payload, SourceConfig{}, &posts, make(map[string]bool))
	if len(posts) != 1 || len(posts[0].Media) != 1 || posts[0].Media[0] != "https://wx2.sinaimg.cn/original/example.png" {
		t.Fatalf("large image URL was not upgraded: %#v", posts)
	}
}

func TestBilibiliSubscriptionSyncAndDelete(t *testing.T) {
	oldFile := bilibiliFile
	bilibiliFile = filepath.Join(t.TempDir(), "platform.enc")
	t.Cleanup(func() { bilibiliFile = oldFile })

	store := &BilibiliStore{
		key:             make([]byte, 32),
		config:          BilibiliConfig{Subscriptions: []SourceConfig{{ID: "bili-123", Source: SourceBilibili, Name: "测试 UP 主", Enabled: true}}},
		bilibiliQR:      make(map[string]BilibiliQRSession),
		bilibiliClients: make(map[string]*http.Client),
		weiboQR:         make(map[string]WeiboQRSession),
		weiboClients:    make(map[string]*http.Client),
	}

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/bilibili/subscriptions?action=sync&id=bili-123", nil)
	syncResponse := httptest.NewRecorder()
	store.subscriptionsHandler(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncResponse.Code, syncResponse.Body.String())
	}
	if status := decodeTestResponse(t, syncResponse)["status"]; status != "failed" {
		t.Fatalf("unexpected sync status: %v", status)
	}
	if store.config.Subscriptions[0].LastSyncedAt.IsZero() || store.config.Subscriptions[0].LastSyncStatus != "failed" {
		t.Fatalf("sync failure status was not persisted: %#v", store.config.Subscriptions[0])
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/bilibili/subscriptions?id=bili-123", nil)
	deleteResponse := httptest.NewRecorder()
	store.subscriptionsHandler(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if len(store.config.Subscriptions) != 0 {
		t.Fatalf("subscription was not deleted: %#v", store.config.Subscriptions)
	}
	persisted, err := os.ReadFile(bilibiliFile)
	if err != nil {
		t.Fatalf("read persisted platform store: %v", err)
	}
	var reloaded BilibiliConfig
	if err := decryptJSON(store.key, persisted, &reloaded); err != nil {
		t.Fatalf("decrypt persisted platform store: %v", err)
	}
	if len(reloaded.Subscriptions) != 0 {
		t.Fatalf("deleted subscription returned after reload: %#v", reloaded.Subscriptions)
	}
}

func TestWeiboSubscriptionLifecycle(t *testing.T) {
	oldFile, oldFlowRoot := bilibiliFile, flowRoot
	tempDir := t.TempDir()
	bilibiliFile = filepath.Join(tempDir, "platform.enc")
	flowRoot = filepath.Join(tempDir, "flow")
	t.Cleanup(func() { bilibiliFile, flowRoot = oldFile, oldFlowRoot })

	store := &BilibiliStore{
		key:             make([]byte, 32),
		config:          BilibiliConfig{Weibo: WeiboCredentials{Cookie: "SUB=test", UserID: "42"}, Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}},
		bilibiliQR:      make(map[string]BilibiliQRSession),
		bilibiliClients: make(map[string]*http.Client),
		weiboQR:         make(map[string]WeiboQRSession),
		weiboClients:    make(map[string]*http.Client),
	}

	createRequest := httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions", strings.NewReader(`{"userId":"12345","name":"测试博主","avatar":"https://example.com/avatar.jpg","includePast":true,"schedule":"每 6 小时"}`))
	createResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(createResponse, createRequest)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	if len(store.config.WeiboSubscriptions) != 1 || store.config.WeiboSubscriptions[0].ID != "weibo-12345" || store.config.WeiboSubscriptions[0].Avatar != "https://example.com/avatar.jpg" || !store.config.WeiboSubscriptions[0].OnlyWithImages {
		t.Fatalf("unexpected subscriptions: %#v", store.config.WeiboSubscriptions)
	}
	if _, err := os.Stat(store.config.WeiboSubscriptions[0].StoragePath); err != nil {
		t.Fatalf("source storage was not created: %v", err)
	}

	feed := store.config.WeiboSubscriptions[0]
	feed.Enabled = false
	feed.Schedule = "每 12 小时"
	body, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/weibo/subscriptions", bytes.NewReader(body))
	updateResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK || store.config.WeiboSubscriptions[0].Enabled || store.config.WeiboSubscriptions[0].Schedule != "0 */12 * * *" {
		t.Fatalf("update failed status=%d subscriptions=%#v", updateResponse.Code, store.config.WeiboSubscriptions)
	}

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions?action=sync&id=weibo-12345", nil)
	syncResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK || store.config.WeiboSubscriptions[0].LastSyncedAt.IsZero() {
		t.Fatalf("sync failed status=%d subscriptions=%#v", syncResponse.Code, store.config.WeiboSubscriptions)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/weibo/subscriptions?id=weibo-12345", nil)
	deleteResponse := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 0 {
		t.Fatalf("delete failed status=%d subscriptions=%#v", deleteResponse.Code, store.config.WeiboSubscriptions)
	}
	persisted, err := os.ReadFile(bilibiliFile)
	if err != nil {
		t.Fatalf("read persisted platform store: %v", err)
	}
	var reloaded BilibiliConfig
	if err := decryptJSON(store.key, persisted, &reloaded); err != nil {
		t.Fatalf("decrypt persisted platform store: %v", err)
	}
	if len(reloaded.WeiboSubscriptions) != 0 {
		t.Fatalf("deleted weibo subscription returned after reload: %#v", reloaded.WeiboSubscriptions)
	}
}

func TestWeiboLikesSubscriptionCanBeAddedAndDeleted(t *testing.T) {
	tempDir := t.TempDir()
	oldFile, oldFlowRoot := bilibiliFile, flowRoot
	bilibiliFile, flowRoot = filepath.Join(tempDir, "platform.enc"), filepath.Join(tempDir, "flow")
	t.Cleanup(func() { bilibiliFile, flowRoot = oldFile, oldFlowRoot })
	store := &BilibiliStore{config: BilibiliConfig{Weibo: WeiboCredentials{Cookie: "SUB=test", UserID: "42", UserName: "账号", Avatar: "https://example.com/me.jpg"}, WeiboSubscriptions: []SourceConfig{}}, key: make([]byte, 32)}
	create := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(create, httptest.NewRequest(http.MethodPost, "/api/weibo/subscriptions?type=likes", nil))
	if create.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 1 || store.config.WeiboSubscriptions[0].ID != "weibo-likes-42" || !store.config.WeiboSubscriptions[0].Enabled || !store.config.WeiboSubscriptions[0].OnlyWithImages {
		t.Fatalf("create likes source failed: status=%d feeds=%#v body=%s", create.Code, store.config.WeiboSubscriptions, create.Body.String())
	}
	remove := httptest.NewRecorder()
	store.weiboSubscriptionsHandler(remove, httptest.NewRequest(http.MethodDelete, "/api/weibo/subscriptions?id=weibo-likes-42", nil))
	if remove.Code != http.StatusOK || len(store.config.WeiboSubscriptions) != 0 {
		t.Fatalf("delete likes source failed: status=%d feeds=%#v", remove.Code, store.config.WeiboSubscriptions)
	}
}

func TestReconcileMovesWeiboLikesIntoCanonicalDirectory(t *testing.T) {
	tempDir := t.TempDir()
	oldFlowRoot := flowRoot
	flowRoot = filepath.Join(tempDir, "flow")
	t.Cleanup(func() { flowRoot = oldFlowRoot })

	oldName := "错误作者目录"
	oldDirectory := sourceStoragePath(SourceWeibo, oldName)
	if err := os.MkdirAll(oldDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDirectory, "image.jpg"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	content := &Store{file: filepath.Join(tempDir, "content.json"), posts: []Post{{ID: "liked-post", Media: []string{flowPublicPath(SourceWeibo, oldName, "image.jpg")}}}}
	store := &BilibiliStore{
		content: content,
		config: BilibiliConfig{WeiboSubscriptions: []SourceConfig{{
			ID: "weibo-likes-42", Source: SourceWeibo, Name: oldName, StoragePath: oldDirectory,
		}}},
	}

	if err := store.reconcileFlowStorage(); err != nil {
		t.Fatalf("reconcile likes storage: %v", err)
	}
	expectedDirectory := sourceStoragePath(SourceWeibo, "我的点赞")
	if store.config.WeiboSubscriptions[0].Name != "我的点赞" || store.config.WeiboSubscriptions[0].StoragePath != expectedDirectory {
		t.Fatalf("likes source was not canonicalized: %#v", store.config.WeiboSubscriptions[0])
	}
	if _, err := os.Stat(filepath.Join(expectedDirectory, "image.jpg")); err != nil {
		t.Fatalf("liked image was not moved: %v", err)
	}
	expectedMedia := flowPublicPath(SourceWeibo, "我的点赞", "image.jpg")
	if content.posts[0].Media[0] != expectedMedia {
		t.Fatalf("stored media path was not rewritten: %q", content.posts[0].Media[0])
	}
}

func TestLoadStoreFileInitializesEmptyContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.json")
	store, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("initialize content store: %v", err)
	}
	if len(store.posts) != 0 || len(store.feeds) != 0 {
		t.Fatalf("new content store must be empty: posts=%#v feeds=%#v", store.posts, store.feeds)
	}
	reloaded, err := loadStoreFile(path)
	if err != nil {
		t.Fatalf("reload content store: %v", err)
	}
	if len(reloaded.posts) != 0 || len(reloaded.feeds) != 0 {
		t.Fatalf("persisted empty store loaded demo content: posts=%#v feeds=%#v", reloaded.posts, reloaded.feeds)
	}
}

func TestConfigurationBackupExportAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	oldAuthFile, oldBilibiliFile := authFile, bilibiliFile
	authFile, bilibiliFile = filepath.Join(tempDir, "auth.json"), filepath.Join(tempDir, "platform.enc")
	t.Cleanup(func() { authFile, bilibiliFile = oldAuthFile, oldBilibiliFile })
	auth := &AuthConfig{Username: "backup-user", PasswordHash: "backup-hash"}
	platforms := &BilibiliStore{key: make([]byte, 32), config: BilibiliConfig{ProxyURL: "http://127.0.0.1:7890", WeiboSubscriptions: []SourceConfig{{ID: "weibo-42", Source: SourceWeibo, Name: "作者"}}, Subscriptions: []SourceConfig{}}}
	handler := configurationBackupHandler(auth, platforms)
	exportResponse := httptest.NewRecorder()
	handler(exportResponse, httptest.NewRequest(http.MethodGet, "/api/configuration/backup", nil))
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export failed: status=%d body=%s", exportResponse.Code, exportResponse.Body.String())
	}
	var backup ConfigurationBackup
	if err := json.Unmarshal(exportResponse.Body.Bytes(), &backup); err != nil || backup.Version != 1 || backup.Auth.Username != "backup-user" || len(backup.Platforms.WeiboSubscriptions) != 1 {
		t.Fatalf("unexpected backup: err=%v backup=%#v", err, backup)
	}
	auth.Username, auth.PasswordHash = "changed", "changed-hash"
	platforms.config = BilibiliConfig{Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}}
	restoreResponse := httptest.NewRecorder()
	handler(restoreResponse, httptest.NewRequest(http.MethodPut, "/api/configuration/backup", bytes.NewReader(exportResponse.Body.Bytes())))
	if restoreResponse.Code != http.StatusOK || auth.Username != "backup-user" || platforms.config.ProxyURL != "http://127.0.0.1:7890" || len(platforms.config.WeiboSubscriptions) != 1 {
		t.Fatalf("restore failed: status=%d auth=%#v platforms=%#v body=%s", restoreResponse.Code, auth, platforms.config, restoreResponse.Body.String())
	}
}

func TestSignedSessionRemainsValidAcrossStoreRestart(t *testing.T) {
	key := []byte("persistent-session-signing-key")
	first := &SessionStore{tokens: make(map[string]time.Time), key: key}
	token, err := first.create()
	if err != nil {
		t.Fatalf("create signed session: %v", err)
	}
	restarted := &SessionStore{tokens: make(map[string]time.Time), key: key}
	if !restarted.valid(token) {
		t.Fatal("signed session became invalid after store restart")
	}
	if restarted.valid(token + "tampered") {
		t.Fatal("tampered signed session was accepted")
	}
}

func TestSessionHandlerRecognizesPersistentSignedCookie(t *testing.T) {
	key := []byte("persistent-session-signing-key")
	token, err := (&SessionStore{tokens: make(map[string]time.Time), key: key}).create()
	if err != nil {
		t.Fatalf("create signed session: %v", err)
	}

	restarted := &SessionStore{tokens: make(map[string]time.Time), key: key}
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(&http.Cookie{Name: "lumic_session", Value: token})
	response := httptest.NewRecorder()
	sessionHandler(restarted)(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authenticated":true`) {
		t.Fatalf("session was not restored: status=%d body=%s", response.Code, response.Body.String())
	}
}
