package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeiboLivePhotoKeepsOfficialDispatchURL(t *testing.T) {
	source := "https://video.weibo.com/media/play?livephoto=%2F%2Flivephoto.us.sinaimg.cn%2Ffixture.mov&KID=unistore%2CvideomovSrc&token=private-fixture"
	if got := weiboLivePhotoVideo(map[string]any{"video": source}, ""); got != source {
		t.Fatal("official dispatch URL or signed parameters were changed")
	}
	got := weiboLivePhotoVideo(nil, "fixture")
	target, _ := url.Parse(got)
	if target.Hostname() != "video.weibo.com" || target.Path != "/media/play" || target.Query().Get("livephoto") != "//us.sinaimg.cn/fixture.mov" {
		t.Fatal("FID bypassed the official dispatch entry")
	}
}

func TestLivePhotoStaticImagePreservesHEICOrJPEGExtension(t *testing.T) {
	for _, scenario := range []struct{ url, contentType, want string }{
		{"https://wx1.sinaimg.cn/large/fixture.heic", "image/jpeg", ".heic"},
		{"https://wx1.sinaimg.cn/large/fixture.jpg", "image/heic", ".jpg"},
		{"https://wx1.sinaimg.cn/large/fixture", "image/heic", ".heic"},
		{"https://wx1.sinaimg.cn/large/fixture", "image/jpeg", ".jpg"},
	} {
		if got := mediaFileExtension(scenario.url, scenario.contentType); got != scenario.want {
			t.Fatalf("static live-photo extension mismatch: got=%s want=%s", got, scenario.want)
		}
	}
}

func TestLivePhotoDispatchRedirectAndCookieIsolation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/media/play" {
			if r.Header.Get("Cookie") != "SUB=private-fixture" || r.URL.Query().Get("token") != "private-fixture" {
				t.Error("dispatch lost original credentials/parameters")
			}
			http.Redirect(w, r, "https://livephoto.us.sinaimg.cn/fixture.mov?signature=private-fixture", 302)
			return
		}
		if r.URL.Path != "/fixture.mov" || r.Header.Get("Cookie") != "" {
			t.Error("CDN received account credentials or wrong route")
		}
		w.Header().Set("Content-Type", "video/quicktime")
		fmt.Fprint(w, "fixture-motion")
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
	path, err := downloadWeiboLivePhoto(client, "https://video.weibo.com/media/play?livephoto=%2F%2Flivephoto.us.sinaimg.cn%2Ffixture.mov&token=private-fixture", filepath.Join(t.TempDir(), "photo"), "SUB=private-fixture")
	if err != nil || path == "" || calls != 2 {
		t.Fatalf("normal media dispatch failed: calls=%d err=%v", calls, err)
	}
}

func TestLivePhotoDownloadRejectsAndRedactsFailures(t *testing.T) {
	for _, scenario := range []struct {
		name                  string
		status                int
		contentType, location string
	}{
		{"forbidden", 403, "text/html", ""},
		{"limited", 429, "text/html", ""},
		{"html-mov", 200, "text/html", ""},
		{"untrusted-redirect", 302, "", "https://untrusted.invalid/private-fixture?token=private-fixture"},
		{"https-downgrade", 302, "", "http://us.sinaimg.cn/private-fixture.mov"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", scenario.contentType)
				if scenario.location != "" {
					w.Header().Set("Location", scenario.location)
				}
				w.WriteHeader(scenario.status)
				fmt.Fprint(w, "private-fixture")
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			client := &http.Client{Transport: weiboTestTransport{target, server.Client().Transport}}
			directory := t.TempDir()
			_, err := downloadWeiboLivePhoto(client, "https://livephoto.us.sinaimg.cn/private-fixture.mov?token=private-fixture", filepath.Join(directory, "photo"), "SUB=private-fixture")
			if err == nil || calls != 1 {
				t.Fatalf("explicit failure was accepted or retried: calls=%d err=%v", calls, err)
			}
			if strings.Contains(err.Error(), "private-fixture") || strings.Contains(err.Error(), "token=") || strings.Contains(err.Error(), "SUB=") {
				t.Fatal("media failure exposed credentials/URL")
			}
			if matches, _ := filepath.Glob(filepath.Join(directory, "*")); len(matches) != 0 {
				t.Fatal("failed media created an invalid archive file")
			}
		})
	}
}

func TestContainerLogErrorSanitizesNetworkAndUnknownErrors(t *testing.T) {
	for _, err := range []error{
		&url.Error{Op: "Get", URL: "https://user:private-fixture@example.test/private-fixture?token=private-fixture", Err: errors.New("private-fixture")},
		errors.New("HTTP 403 SUB=private-fixture caption=private-fixture"),
		&weiboRequestError{kind: "restricted", message: "HTTP 403 private-fixture"},
	} {
		if fields := syncLogError(err); strings.Contains(fields, "private-fixture") || strings.Contains(fields, "token=") || strings.Contains(fields, "https://") {
			t.Fatal("unsafe container error output")
		}
	}
}

func TestSyncLogsDoNotEchoUpstreamSecrets(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(old) })
	store := newWeiboAuthTestStore(t, WeiboCredentials{})
	feed := SourceConfig{ID: "private-account-fixture", Source: SourceWeibo, Name: "private-name-fixture", Enabled: true}
	store.config.WeiboSubscriptions = []SourceConfig{feed}
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, "SUB=private-cookie-fixture")
	})
	_, err := store.syncSource(feed, false)
	if err == nil {
		t.Fatal("mock rejection did not fail")
	}
	for _, expected := range []string{"event=sync_start", "event=sync_fetch", "event=sync_end", "platform=weibo", "status=failed", "http=403", "duration_ms="} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("missing diagnostic %s: %s", expected, output.String())
		}
	}
	for _, secret := range []string{"private-account-fixture", "private-name-fixture", "private-cookie-fixture", "/api/container", "Cookie"} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("sync log exposed upstream/identity content")
		}
	}
}
