package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestAuthorSubscriptionResolvesAnchorNotSameName(t *testing.T) {
	b := &BilibiliStore{config: BilibiliConfig{WeiboSubscriptions: []SourceConfig{
		{ID: "weibo-31", Source: SourceWeibo, Name: "same"},
		{ID: "weibo-likes-51", Source: SourceWeibo, Name: "same"},
	}}}
	b.content = &Store{posts: []Post{
		{ID: "weibo-status-101", Source: SourceWeibo, Author: "same", FeedIDs: []string{"weibo-likes-51"}, OriginalURL: "https://weibo.com/31/101"},
		{ID: "weibo-status-102", Source: SourceWeibo, Author: "same", FeedIDs: []string{"weibo-likes-51"}, OriginalURL: "https://weibo.com/32/102"},
	}}
	if _, ok := b.resolveAuthorSubscription(SourceWeibo, "same", "", ""); ok {
		t.Fatal("same display name with distinct account IDs resolved without anchor")
	}
	if target, ok := b.resolveAuthorSubscription(SourceWeibo, "same", "weibo-status-102", ""); !ok || target.ID != "32" {
		t.Fatalf("anchor resolved to wrong account: %+v, %v", target, ok)
	}
	if target, ok := b.resolveAuthorSubscription(SourceWeibo, "old display name", "", "weibo-31"); !ok || target.ID != "31" || target.Name != "same" {
		t.Fatalf("exact direct feed was not authoritative after rename: %+v, %v", target, ok)
	}
	for _, check := range []struct{ author, postID, feedID string }{
		{"same", "missing-anchor", ""}, {"different", "weibo-status-102", ""},
		{"same", "", "weibo-32"}, {"same", "", "weibo-likes-51"},
	} {
		if target, ok := b.resolveAuthorSubscription(SourceWeibo, check.author, check.postID, check.feedID); ok {
			t.Fatalf("unverified target accepted: %+v", target)
		}
	}
	b.content.posts = append(b.content.posts, Post{ID: "weibo-status-103", Source: SourceWeibo, Author: "same", FeedIDs: []string{"weibo-32"}})
	if target, ok := b.resolveAuthorSubscription(SourceWeibo, "same", "", "weibo-32"); !ok || target.ID != "32" {
		t.Fatalf("archived author feed not resolved after removal: %+v, %v", target, ok)
	}
}

func TestArchivedPostAuthorIDValidation(t *testing.T) {
	for _, test := range []struct {
		post Post
		want string
	}{
		{Post{Source: SourceWeibo, ID: "weibo-status-10", OriginalURL: "https://weibo.com/42/10"}, "42"},
		{Post{Source: SourceWeibo, ID: "weibo-status-10", OriginalURL: "https://weibo.com/42/11", FeedIDs: []string{"weibo-likes-56"}}, ""},
		{Post{Source: SourceWeibo, ID: "weibo-status-10", OriginalURL: "https://weibo.com@example.net/42/10"}, ""},
		{Post{Source: SourceWeibo, ID: "weibo-status-10", AuthorID: "73", FeedIDs: []string{"weibo-56"}}, "73"},
		{Post{Source: SourceBilibili, FeedIDs: []string{bilibiliFavoriteOpusPrefix + "56"}}, ""},
		{Post{Source: SourcePixiv, FeedIDs: []string{"pixiv-00041"}}, "41"},
		{Post{Source: SourcePixiv, FeedIDs: []string{"pixiv-bookmarks-56", "pixiv-00041"}}, ""},
		{Post{Source: SourcePixiv, FeedIDs: []string{"pixiv-41", "pixiv-42"}}, ""},
		{Post{Source: SourceTwitter, ID: "twitter-tweet-10", OriginalURL: "https://x.com/Handle_1/status/10", FeedIDs: []string{"twitter-likes-another"}}, "handle_1"},
		{Post{Source: SourceTwitter, ID: "twitter-tweet-10", OriginalURL: "https://x.com/Handle_1/status/10", FeedIDs: []string{"twitter-likes-handle_1"}}, ""},
		{Post{Source: SourceTwitter, ID: "twitter-tweet-10", OriginalURL: "https://x.com/i/status/10"}, ""},
	} {
		if got := archivedPostAuthorID(test.post); got != test.want {
			t.Fatalf("%+v: got author %q, want %q", test.post, got, test.want)
		}
	}
}

func TestArchiveParsersRetainVerifiedAuthorIdentity(t *testing.T) {
	b := &BilibiliStore{}
	bili := json.RawMessage("{\"id_str\":\"11\",\"type\":\"DYNAMIC_TYPE_WORD\",\"modules\":{\"module_author\":{\"name\":\"a\",\"mid\":123,\"pub_ts\":1794100800},\"module_dynamic\":{\"desc\":{\"text\":\"title\"}}}}")
	post, err := b.parseBilibiliDynamicPost(bili, SourceConfig{ID: "bili-123", ContentTypes: []string{"WORD"}}, BilibiliCredentials{}, "")
	if err != nil || post.AuthorID != "123" {
		t.Fatalf("Bilibili dynamic identity: %q %v", post.AuthorID, err)
	}
	opus := map[string]any{"detail": map[string]any{"id_str": "12", "modules": []any{
		map[string]any{"module_author": map[string]any{"mid": "124", "name": "b", "pub_ts": 1794100800}},
	}}}
	post, err = bilibiliOpusPost("12", opus, SourceConfig{ID: bilibiliFavoriteOpusPrefix + "77"})
	if err != nil || post.AuthorID != "124" {
		t.Fatalf("Bilibili collection identity: %q %v", post.AuthorID, err)
	}
	pixiv := map[string]any{"body": map[string]any{"userId": "125", "userName": "c", "pageCount": 1, "urls": map[string]any{"original": "https://i.pximg.net/x.jpg"}}}
	post, err = pixivDetailPost(nil, PixivCredentials{}, "13", pixiv, SourceConfig{ID: "pixiv-bookmarks-77"})
	if err != nil || post.AuthorID != "125" {
		t.Fatalf("Pixiv collection identity: %q %v", post.AuthorID, err)
	}
	post = twitterDetailPost(map[string]any{"author": map[string]any{"userName": "A_6"}}, "14", TwitterCredentials{UserName: "owner"}, SourceConfig{ID: "twitter-likes-owner"})
	if post.AuthorID != "a_6" {
		t.Fatalf("Twitter true author identity: %q", post.AuthorID)
	}
	post = twitterDetailPost(map[string]any{}, "15", TwitterCredentials{UserName: "owner"}, SourceConfig{ID: "twitter-likes-owner"})
	if id := archivedPostAuthorID(post); id != "" {
		t.Fatalf("Twitter synthesized collection owner was mistaken for author: %q", id)
	}
}

func TestAuthorSubscriptionLegacyAnchorLookupPersistsIdentity(t *testing.T) {
	b := newWeiboAuthTestStore(t, WeiboCredentials{})
	b.content = &Store{file: filepath.Join(t.TempDir(), "posts.json"), posts: []Post{
		{ID: "weibo-status-101", Source: SourceWeibo, Author: "archived", FeedIDs: []string{"weibo-likes-99"}},
	}}
	var requests atomic.Int32
	mockWeiboAPI(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/statuses/show" || r.URL.Query().Get("id") != "101" {
			t.Errorf("lookup did not use archived anchor detail: %s", r.URL)
		}
		fmt.Fprint(w, "{\"ok\":1,\"data\":{\"id\":\"101\",\"text\":\"example\",\"user\":{\"idstr\":\"42\",\"screen_name\":\"renamed\"}}}")
	})
	if _, ok := b.resolveAuthorSubscription(SourceWeibo, "archived", "weibo-status-101", ""); ok {
		t.Fatal("missing identity unexpectedly resolved locally")
	}
	if !b.canResolveLegacyAuthorSubscription(SourceWeibo, "archived", "weibo-status-101") || requests.Load() != 0 {
		t.Fatal("check did not remain local")
	}
	if target, ok := b.resolveLegacyAuthorSubscription(SourceWeibo, "archived", "weibo-status-101"); !ok || target.ID != "42" || target.Name != "archived" {
		t.Fatalf("archived anchor could not resolve author: %+v, %v", target, ok)
	}
	reloaded, err := loadStoreFile(b.content.file)
	if err != nil || len(reloaded.posts) != 1 || reloaded.posts[0].AuthorID != "42" {
		t.Fatalf("confirmed identity not persisted: %v", err)
	}
	b.content = reloaded
	if target, ok := b.resolveAuthorSubscription(SourceWeibo, "archived", "weibo-status-101", ""); !ok || target.ID != "42" || requests.Load() != 1 {
		t.Fatalf("stored identity was not reusable without another request: %+v, %v, %d", target, ok, requests.Load())
	}
}
