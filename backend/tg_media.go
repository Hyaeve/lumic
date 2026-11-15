package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tgPhotoMaxBytes = 10 << 20
const tgAlbumMaxBytes = 32 << 20

func tgMediaHost(host string) bool {
	host = strings.ToLower(host)
	for _, domain := range []string{"pximg.net", "sinaimg.cn", "sinaimg.com", "weibocdn.cn", "weibocdn.com", "hdslb.com", "twimg.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func tgReadPhoto(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, tgPhotoMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > tgPhotoMaxBytes {
		return nil, errors.New("图片不可读取或超过 Telegram 图片大小限制")
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return nil, errors.New("图片格式不可用")
	}
	return data, nil
}

func (bot *tgBot) photoData(ctx context.Context, value string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if strings.HasPrefix(value, "/flow/") {
		parsed, parseErr := url.Parse(value)
		file, valid := localFlowArchivePath(value)
		if parseErr != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !valid {
			return nil, errors.New("归档图片路径无效")
		}
		root, rootErr := filepath.EvalSymlinks(flowRoot)
		canonical, err := filepath.EvalSymlinks(file)
		if rootErr != nil || err != nil {
			return nil, errors.New("归档图片不存在")
		}
		relative, err := filepath.Rel(root, canonical)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, errors.New("归档图片路径无效")
		}
		reader, err := os.Open(canonical)
		if err != nil {
			return nil, errors.New("无法打开归档图片")
		}
		defer reader.Close()
		info, err := reader.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > tgPhotoMaxBytes {
			return nil, errors.New("归档图片不可用或过大")
		}
		return tgReadPhoto(reader)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || len(value) > 8192 || !tgMediaHost(parsed.Hostname()) {
		return nil, errors.New("图片地址不受支持")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return nil, errors.New("图片地址无效")
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "pximg.net" || strings.HasSuffix(host, ".pximg.net"):
		request.Header.Set("Referer", "https://www.pixiv.net/")
	case host == "hdslb.com" || strings.HasSuffix(host, ".hdslb.com"):
		request.Header.Set("Referer", "https://www.bilibili.com/")
	case !strings.HasSuffix(host, "twimg.com"):
		request.Header.Set("Referer", "https://weibo.com/")
	}
	client := *bot.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > 3 || next.URL.Scheme != "https" || next.URL.User != nil || next.URL.Port() != "" || next.URL.Hostname() != parsed.Hostname() {
			return errors.New("图片重定向不受支持")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("图片下载失败，请检查平台或网络")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("图片下载失败，请检查平台或网络")
	}
	return tgReadPhoto(response.Body)
}

func (bot *tgBot) sendPhotos(ctx context.Context, chatID int64, sources []string, caption string) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if len(sources) == 0 {
		return nil, nil
	}
	if len(sources) > 10 {
		return nil, errors.New("单个图片组不能超过十张图片")
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	_ = writer.WriteField("chat_id", fmt.Sprint(chatID))
	media := make([]map[string]string, 0, len(sources))
	totalBytes := 0
	for index, source := range sources {
		data, err := bot.photoData(ctx, source)
		if err != nil {
			return nil, err
		}
		totalBytes += len(data)
		if totalBytes > tgAlbumMaxBytes {
			return nil, errors.New("图片组过大，请减少图片数量")
		}
		field := fmt.Sprintf("image%d", index)
		part, err := writer.CreateFormFile(field, field+".jpg")
		if err != nil {
			return nil, errors.New("无法准备机器人图片")
		}
		if _, err := part.Write(data); err != nil {
			return nil, errors.New("无法准备机器人图片")
		}
		item := map[string]string{"type": "photo", "media": "attach://" + field}
		if index == 0 && caption != "" {
			item["caption"] = caption
		}
		media = append(media, item)
	}
	method := "sendMediaGroup"
	if len(sources) == 1 {
		method = "sendPhoto"
		_ = writer.WriteField("photo", "attach://image0")
		if caption != "" {
			_ = writer.WriteField("caption", caption)
		}
	} else {
		encoded, _ := json.Marshal(media)
		_ = writer.WriteField("media", string(encoded))
	}
	if writer.Close() != nil {
		return nil, errors.New("无法准备机器人图片")
	}
	var messages []tgMessage
	if len(sources) == 1 {
		var message tgMessage
		if err := bot.request(ctx, method, writer.FormDataContentType(), buffer.Bytes(), &message); err != nil {
			return nil, err
		}
		messages = []tgMessage{message}
	} else if err := bot.request(ctx, method, writer.FormDataContentType(), buffer.Bytes(), &messages); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.MessageID)
	}
	return ids, nil
}
