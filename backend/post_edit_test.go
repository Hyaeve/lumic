package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func editRequest(t *testing.T, store *Store, id, caption string, media []string, upload []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("caption", caption)
	data, _ := json.Marshal(media)
	_ = writer.WriteField("media", string(data))
	if upload != nil {
		part, _ := writer.CreateFormFile("images", "image.png")
		_, _ = part.Write(upload)
	}
	writer.Close()
	request := httptest.NewRequest(http.MethodPut, "/api/posts?id="+id, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	result := httptest.NewRecorder()
	store.postsHandler(result, request)
	return result
}

func TestEditPostPersistsAndSurvivesSync(t *testing.T) {
	old := flowRoot
	flowRoot = t.TempDir()
	defer func() { flowRoot = old }()
	store := &Store{file: filepath.Join(t.TempDir(), "content.json"), posts: []Post{{ID: "one", Source: SourcePixiv, Author: "artist", Caption: "before", Media: []string{"/flow/a.png", "/flow/b.png"}, Liked: true}}}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	response := editRequest(t, store, "one", "edited", []string{"/flow/b.png", "upload:0"}, imageData.Bytes())
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	post := store.posts[0]
	if !post.Edited || post.Caption != "edited" || !post.Liked || len(post.Media) != 2 || post.Media[0] != "/flow/b.png" {
		t.Fatal(post)
	}
	path := filepath.Join(flowRoot, filepath.FromSlash(strings.TrimPrefix(post.Media[1], "/flow/")))
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(store.file)
	if err != nil || !bytes.Contains(saved, []byte(`"edited":true`)) {
		t.Fatal("edit not persisted", err)
	}
	if _, err := store.mergePosts([]Post{{ID: "one", Source: SourcePixiv, Author: "artist", Caption: "remote", Media: []string{"/flow/remote.png"}}}); err != nil {
		t.Fatal(err)
	}
	if store.posts[0].Caption != "edited" || store.posts[0].Media[1] != post.Media[1] {
		t.Fatal("sync overwrote local edits")
	}
	response = editRequest(t, store, "one", "", []string{}, nil)
	if response.Code != 200 || store.posts[0].Caption != "" || len(store.posts[0].Media) != 0 {
		t.Fatal("clear text/images failed")
	}
}

func TestEditPostRejectsForeignMediaAndRollsBack(t *testing.T) {
	old := flowRoot
	flowRoot = t.TempDir()
	defer func() { flowRoot = old }()
	store := &Store{posts: []Post{{ID: "one", Caption: "before", Media: []string{"/flow/a.png"}}}}
	for _, media := range [][]string{{"/flow/other.png"}, {"../../secret"}, {"https://example.com/x.png"}, {"upload:0"}} {
		response := editRequest(t, store, "one", "changed", media, []byte("not an image"))
		if response.Code != 400 || store.posts[0].Caption != "before" {
			t.Fatal(response.Code, "invalid edit accepted")
		}
	}
	if result := editRequest(t, store, "missing", "", []string{}, nil); result.Code != 404 {
		t.Fatal("missing post accepted")
	}
	var upload bytes.Buffer
	if err := png.Encode(&upload, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if result := editRequest(t, store, "one", "changed", []string{"upload:0", "/flow/foreign.png"}, upload.Bytes()); result.Code != 400 {
		t.Fatal("foreign image accepted after upload")
	}
	_ = filepath.Walk(flowRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasPrefix(info.Name(), "image-") {
			t.Error("failed edit left uploaded file", path)
		}
		return nil
	})
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	store.file = filepath.Join(blocked, "content.json")
	if result := editRequest(t, store, "one", "changed", []string{}, nil); result.Code != 500 || store.posts[0].Caption != "before" {
		t.Fatal("failed save did not roll back")
	}
}
