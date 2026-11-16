package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tgImageFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 128, G: 64, B: 30, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func tgDetailedImageFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	state := uint32(42)
	for index := 0; index < len(img.Pix); index += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		img.Pix[index], img.Pix[index+1], img.Pix[index+2], img.Pix[index+3] = byte(state), byte(state>>8), byte(state>>16), 255
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestTGMediaLocalPathsAndLimits(t *testing.T) {
	previous := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = previous })
	data := tgImageFixture(t)
	if err := os.WriteFile(filepath.Join(flowRoot, "photo.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	bot := &tgBot{}
	if actual, err := bot.photoData(context.Background(), "/flow/photo.png"); err != nil || !bytes.Equal(actual, data) {
		t.Fatal("valid archived image was not loaded")
	}
	for _, value := range []string{
		"/flow/../photo.png", "/flow/photo.png?token=private", "/flow/photo.png#fragment", "/etc/passwd",
		"https://example.com/photo.png", "http://i.pximg.net/photo.png", "https://user:password@i.pximg.net/photo.png",
		"https://i.pximg.net:8443/photo.png", "https://pximg.net.evil.invalid/photo.png",
	} {
		if _, err := bot.photoData(context.Background(), value); err == nil {
			t.Fatalf("unsupported photo path was accepted: %s", value)
		}
	}
	outside := filepath.Join(t.TempDir(), "private.png")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(flowRoot, "linked.png")); err == nil {
		if _, err := bot.photoData(context.Background(), "/flow/linked.png"); err == nil {
			t.Fatal("symlink escaped the flow root")
		}
	}
	if _, err := tgReadPhoto(strings.NewReader("not-an-image")); err == nil {
		t.Fatal("non-image upload was accepted")
	}
	if _, err := tgReadPhoto(io.LimitReader(zeroTGMediaReader{}, tgPhotoMaxBytes+1)); err == nil {
		t.Fatal("oversized upload was accepted")
	}
}

type zeroTGMediaReader struct{}

func (zeroTGMediaReader) Read(data []byte) (int, error) { clear(data); return len(data), nil }

func TestTGMediaCDNUsesRefererWithoutCredentialsAndRejectsRedirects(t *testing.T) {
	data := tgImageFixture(t)
	var calls atomic.Int32
	bot := &tgBot{store: &BilibiliStore{config: BilibiliConfig{Pixiv: PixivCredentials{Cookie: "private-fixture-cookie"}}}}
	bot.client = &http.Client{Transport: tgTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Referer") != "https://www.pixiv.net/" {
			t.Error("CDN request leaked credentials or lacked the public referer")
		}
		if r.URL.Path == "/redirect" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.com/private"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
	})}
	if actual, err := bot.photoData(context.Background(), "https://i.pximg.net/photo.png"); err != nil || !bytes.Equal(actual, data) {
		t.Fatal("allowlisted CDN image did not load")
	}
	if _, err := bot.photoData(context.Background(), "https://i.pximg.net/redirect"); err == nil || calls.Load() != 2 {
		t.Fatal("cross-host CDN redirect was followed")
	}
}

func TestTGMediaUploadsSingleAndAlbumWithoutPublicArchiveURL(t *testing.T) {
	previous := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = previous })
	data := tgImageFixture(t)
	if err := os.WriteFile(filepath.Join(flowRoot, "photo.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var single, album atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("chat_id") != "42" {
			t.Error("wrong photo recipient")
		}
		reader, _, err := r.FormFile("image0")
		if err != nil {
			t.Error("archive image was not attached")
			return
		}
		actual, _ := io.ReadAll(reader)
		reader.Close()
		if !bytes.Equal(actual, data) {
			t.Error("uploaded file was not the archived image")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			single.Add(1)
			if r.FormValue("photo") != "attach://image0" || r.FormValue("caption") != "fixture caption" {
				t.Error("single image did not use multipart attachment/caption")
			}
			writeJSON(w, map[string]any{"ok": true, "result": map[string]int{"message_id": 12}})
		case strings.HasSuffix(r.URL.Path, "/sendMediaGroup"):
			album.Add(1)
			var media []map[string]string
			if json.Unmarshal([]byte(r.FormValue("media")), &media) != nil || len(media) != 2 || media[0]["caption"] != "fixture caption" || media[1]["caption"] != "" || media[1]["media"] != "attach://image1" {
				t.Error("album payload was not a correctly bounded attachment group")
			}
			writeJSON(w, map[string]any{"ok": true, "result": []map[string]int{{"message_id": 13}, {"message_id": 14}}})
		default:
			t.Error("unexpected photo API route")
		}
	}))
	defer server.Close()
	bot := &tgBot{config: TGBotConfig{Token: tgTestToken}, apiBase: server.URL, client: server.Client()}
	if ids, err := bot.sendPhotos(context.Background(), 42, []string{"/flow/photo.png"}, "fixture caption"); err != nil || len(ids) != 1 || ids[0] != 12 {
		t.Fatalf("single photo upload failed: %v", err)
	}
	if ids, err := bot.sendPhotos(context.Background(), 42, []string{"/flow/photo.png", "/flow/photo.png"}, "fixture caption"); err != nil || len(ids) != 2 || ids[1] != 14 {
		t.Fatalf("album upload failed: %v", err)
	}
	if _, err := bot.sendPhotos(context.Background(), 42, make([]string, 11), ""); err == nil || single.Load() != 1 || album.Load() != 1 {
		t.Fatal("oversized group was accepted or emitted API requests")
	}
}

func TestTGMediaCancellationStopsBatchBeforeUpload(t *testing.T) {
	var downloads atomic.Int32
	bot := &tgBot{client: &http.Client{Transport: tgTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		downloads.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := bot.sendPhotos(ctx, 42, []string{"https://i.pximg.net/one.png", "https://i.pximg.net/two.png"}, "")
	if !errors.Is(err, context.DeadlineExceeded) || downloads.Load() != 1 {
		t.Fatal("canceled batch continued downloading or attempted an upload")
	}
}

func TestTGNotificationPreviewBoundsAndCancellation(t *testing.T) {
	previous := flowRoot
	flowRoot = t.TempDir()
	t.Cleanup(func() { flowRoot = previous })
	if err := os.WriteFile(filepath.Join(flowRoot, "photo.png"), tgImageFixture(t), 0600); err != nil {
		t.Fatal(err)
	}
	bot := &tgBot{}
	for _, source := range []string{"/flow/../photo.png", "/flow/photo.png?private=1", "https://i.pximg.net/photo.png"} {
		if _, err := bot.notificationPhotoData(context.Background(), source); err == nil {
			t.Fatal("notification accepted unsafe or remote photo source", source)
		}
	}
	outside := filepath.Join(t.TempDir(), "private.png")
	if err := os.WriteFile(outside, tgImageFixture(t), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(flowRoot, "linked.png")); err == nil {
		if _, err := bot.notificationPhotoData(context.Background(), "/flow/linked.png"); err == nil {
			t.Fatal("notification preview read an escaped symlink")
		}
	}
	var imageHeader bytes.Buffer
	if err := gif.Encode(&imageHeader, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	data := imageHeader.Bytes()
	binary.LittleEndian.PutUint16(data[6:8], 9000)
	binary.LittleEndian.PutUint16(data[8:10], 9000)
	if err := os.WriteFile(filepath.Join(flowRoot, "huge.gif"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.notificationPhotoData(context.Background(), "/flow/huge.gif"); err == nil || !strings.Contains(err.Error(), "尺寸") {
		t.Fatal("notification preview did not reject a decompression-sized image before decode", err)
	}
	for index := 0; index < cap(mediaPreviewGenerationSlots); index++ {
		mediaPreviewGenerationSlots <- struct{}{}
	}
	defer func() {
		for index := 0; index < cap(mediaPreviewGenerationSlots); index++ {
			<-mediaPreviewGenerationSlots
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := bot.notificationPhotoData(ctx, "/flow/photo.png"); !errors.Is(err, context.DeadlineExceeded) || len(mediaPreviewGenerationSlots) != cap(mediaPreviewGenerationSlots) {
		t.Fatal("preview cancellation ignored the shared slot limit or leaked a slot", err)
	}
}
