package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func createPostImageFixtures(t *testing.T, posts []Post) {
	t.Helper()
	oldRoot := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = oldRoot })
	for _, post := range posts {
		for _, source := range post.Media {
			path, ok := localFlowArchivePath(source)
			if !ok {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("fixture"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDeletedImagesDisappearFromResponsesWithoutMutatingPosts(t *testing.T) {
	post := Post{ID: "test", Author: "Artist", Media: []string{"/flow/Artist/keep.jpg", "/flow/Artist/%E5%88%A0%E9%99%A4.jpg?v=1", "https://example.test/remote.jpg"}}
	createPostImageFixtures(t, []Post{post})
	path, _ := localFlowArchivePath(post.Media[1])
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	store := &Store{posts: []Post{post}}
	for _, endpoint := range []string{"/api/v1/posts", "/api/v1/posts?author=Artist", "/api/posts"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, endpoint, nil)
		if endpoint == "/api/posts" {
			store.postsHandler(response, request)
		} else {
			apiV1PostsHandler(store)(response, request)
		}
		var media []string
		if endpoint == "/api/posts" {
			var posts []Post
			if err := json.Unmarshal(response.Body.Bytes(), &posts); err != nil {
				t.Fatal(err)
			}
			media = posts[0].Media
		} else {
			var page apiV1PostPage
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			media = page.Items[0].Media
			if len(page.Items[0].PreviewMedia) != 2 {
				t.Fatalf("stale preview in %s: %+v", endpoint, page)
			}
		}
		if len(media) != 2 || media[0] != post.Media[0] || media[1] != post.Media[2] {
			t.Fatalf("wrong available images in %s: %v", endpoint, media)
		}
	}
	if len(apiV1HeaderMedia(store.posts, "seed")) != 2 || len(store.posts[0].Media) != 3 {
		t.Fatal("gallery contains missing image or stored metadata was mutated")
	}
	remaining, _ := localFlowArchivePath(post.Media[0])
	if err := os.Remove(remaining); err != nil {
		t.Fatal(err)
	}
	if len(toAPIV1Post(post).Media) != 1 {
		t.Fatal("subsequent filesystem deletion was cached")
	}
	empty := withAvailablePostImages(Post{Media: post.Media[:2], Caption: "Keep text"})
	if len(empty.Media) != 0 || empty.Caption != "Keep text" {
		t.Fatal("deleted images must leave a text-only post")
	}
}
