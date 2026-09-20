package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Multipart media entries are either retained URLs owned by this post or
// upload:N references. New files are rolled back if any validation/save fails.
func (s *Store) editPostHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		writeAPIError(w, 400, "图片或正文过大，请缩小后重试")
		return
	}
	defer r.MultipartForm.RemoveAll()
	var media []string
	if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil || len(media) > 50 || len(r.FormValue("caption")) > 1<<20 {
		writeAPIError(w, 400, "编辑内容无效")
		return
	}
	s.Lock()
	defer s.Unlock()
	index := -1
	for i := range s.posts {
		if s.posts[i].ID == r.URL.Query().Get("id") {
			index = i
			break
		}
	}
	if index < 0 {
		writeAPIError(w, 404, "动态不存在或已删除")
		return
	}
	previous := s.posts[index]
	owned := map[string]bool{}
	for _, value := range previous.Media {
		owned[value] = true
	}
	files := r.MultipartForm.File["images"]
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	uploads := map[int]string{}
	for i, value := range media {
		if !strings.HasPrefix(value, "upload:") {
			if !owned[value] {
				writeAPIError(w, 400, "图片不属于当前动态")
				return
			}
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(value, "upload:"))
		if err != nil || n < 0 || n >= len(files) {
			writeAPIError(w, 400, "上传图片无效")
			return
		}
		if saved := uploads[n]; saved != "" {
			media[i] = saved
			continue
		}
		if files[n].Size > 20<<20 {
			writeAPIError(w, 400, "单张图片不能超过20MB")
			return
		}
		file, err := files[n].Open()
		if err != nil {
			writeAPIError(w, 400, "无法读取图片")
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, (20<<20)+1))
		file.Close()
		if err != nil || len(data) > 20<<20 {
			writeAPIError(w, 400, "无法读取图片")
			return
		}
		ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[http.DetectContentType(data)]
		if ext == "" {
			writeAPIError(w, 400, "仅支持JPEG、PNG、GIF、WebP图片")
			return
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 100_000_000 {
			writeAPIError(w, 400, "图片无效或尺寸过大")
			return
		}
		directory := filepath.Join(flowRoot, "edited", sessionDigest(previous.ID))
		if err = os.MkdirAll(directory, 0755); err != nil {
			writeAPIError(w, 500, "无法保存图片")
			return
		}
		output, err := os.CreateTemp(directory, "image-*"+ext)
		if err != nil {
			writeAPIError(w, 500, "无法保存图片")
			return
		}
		created = append(created, output.Name())
		_, err = output.Write(data)
		closeErr := output.Close()
		if err != nil || closeErr != nil {
			writeAPIError(w, 500, "无法保存图片")
			return
		}
		media[i] = fmt.Sprintf("/flow/edited/%s/%s", sessionDigest(previous.ID), filepath.Base(output.Name()))
		uploads[n] = media[i]
	}
	previousPosts := append([]Post(nil), s.posts...)
	s.posts[index].Caption = r.FormValue("caption")
	s.posts[index].Media = media
	s.posts[index].Edited = true
	archives := map[textArchiveKey]bool{textArchiveKeyForPost(previous): true}
	if _, err := s.reconcileTextArchivesFor(archives); err != nil {
		s.posts = previousPosts
		_, _ = s.reconcileTextArchivesFor(archives)
		writeAPIError(w, 500, "无法更新正文存档")
		return
	}
	if err := s.saveLocked(); err != nil {
		s.posts = previousPosts
		_, _ = s.reconcileTextArchivesFor(archives)
		writeAPIError(w, 500, "保存失败，请重试")
		return
	}
	committed = true
	writeJSON(w, s.posts[index])
}
