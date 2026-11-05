package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LivePhotos attach motion to an existing image; they never add a media slot.
type PostLivePhoto struct {
	Image     string `json:"image"`
	Video     string `json:"video"`
	PictureID string `json:"pictureId,omitempty"`
}

func normalizePostLivePhotos(post Post) []PostLivePhoto {
	byImage := make(map[string]PostLivePhoto, len(post.LivePhotos))
	for _, photo := range post.LivePhotos {
		if photo.Image != "" && photo.Video != "" {
			byImage[photo.Image] = photo
		}
	}
	var photos []PostLivePhoto
	for _, image := range post.Media {
		if photo, ok := byImage[image]; ok {
			photos = append(photos, photo)
			delete(byImage, image)
		}
	}
	return photos
}

func rebindLivePhotoImage(post *Post, previous, next string) {
	photos := append([]PostLivePhoto(nil), post.LivePhotos...)
	for index := range photos {
		if photos[index].Image == previous {
			photos[index].Image = next
		}
	}
	post.LivePhotos = photos
}

func weiboLivePhotoVideo(picture map[string]any, fid string) string {
	if direct := usableVideoURL(remoteURLValue(picture["video"]), ""); direct != "" {
		return weiboLivePhotoDirectURL(direct)
	}
	if direct := usableVideoURL(remoteURLValue(picture["video_src"]), ""); direct != "" {
		return weiboLivePhotoDirectURL(direct)
	}
	if fid == "" && jsonValueString(picture["type"]) == "livephoto" {
		fid = jsonValueString(picture["fid"])
	}
	// A fid is a CDN object identifier, not an arbitrary URL or filesystem path.
	if fid == "" || len(fid) > 200 || strings.Trim(fid, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
		return ""
	}
	return "https://video.weibo.com/media/play?" + url.Values{"livephoto": {"//us.sinaimg.cn/" + fid + ".mov"}, "KID": {"unistore,videomovSrc"}}.Encode()
}

func weiboLivePhotoDirectURL(source string) string {
	target, err := url.Parse(source)
	if err != nil {
		return ""
	}
	if target.Hostname() != "video.weibo.com" || target.Path != "/media/play" {
		return source
	}
	// Validate the embedded CDN target without bypassing the supplied dispatch.
	motion, err := url.Parse(normalizeRemoteImage(target.Query().Get("livephoto")))
	if err != nil || motion.User != nil || motion.Port() != "" || motion.Scheme != "https" && motion.Scheme != "http" {
		return ""
	}
	host := strings.ToLower(motion.Hostname())
	if !strings.HasSuffix(host, ".sinaimg.cn") {
		return ""
	}
	// Keep the supplied dispatch entry and all signed query parameters intact.
	return source
}

type livePhotoDownloadError struct {
	host   string
	status int
	kind   string
}

func (e *livePhotoDownloadError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("实况媒体请求失败（域名：%s，HTTP %d）", e.host, e.status)
	}
	switch e.kind {
	case "content_type":
		return "实况媒体未返回视频内容，已保留静态原图"
	case "redirect":
		return "实况媒体重定向地址不受支持，已停止请求"
	case "network":
		return "实况媒体网络请求未完成，请检查代理或网络后重试"
	case "timeout":
		return "实况媒体请求超时，请稍后重试"
	case "local_io":
		return "实况媒体文件写入失败，请检查存储权限与空间"
	default:
		return "实况媒体地址无效或不受支持"
	}
}

func allowedLivePhotoMediaURL(target *url.URL) bool {
	if target == nil || target.User != nil || target.Port() != "" || target.Scheme != "https" && target.Scheme != "http" {
		return false
	}
	host := strings.ToLower(target.Hostname())
	return host == "video.weibo.com" || strings.HasSuffix(host, ".sinaimg.cn") || strings.HasSuffix(host, ".weibocdn.com")
}

type livePhotoTransport struct{ base http.RoundTripper }

func (transport livePhotoTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if !allowedLivePhotoMediaURL(request.URL) {
		return nil, &livePhotoDownloadError{kind: "address"}
	}
	request = request.Clone(request.Context())
	// A Weibo account cookie is not a CDN credential, including on redirects.
	if request.URL.Hostname() != "video.weibo.com" {
		request.Header.Del("Cookie")
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		response.Body.Close()
		return nil, &livePhotoDownloadError{host: request.URL.Hostname(), status: response.StatusCode, kind: "http"}
	}
	if response.StatusCode != 200 && response.StatusCode != 206 && (response.StatusCode < 300 || response.StatusCode >= 400 || response.Header.Get("Location") == "") {
		response.Body.Close()
		return nil, &livePhotoDownloadError{host: request.URL.Hostname(), status: response.StatusCode, kind: "http"}
	}
	if response.StatusCode == 200 || response.StatusCode == 206 {
		contentType := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
		if !strings.HasPrefix(contentType, "video/") && contentType != "application/octet-stream" {
			response.Body.Close()
			return nil, &livePhotoDownloadError{host: request.URL.Hostname(), kind: "content_type"}
		}
	}
	return response, nil
}

func downloadWeiboLivePhoto(client *http.Client, source, base, cookie string) (string, error) {
	target, err := url.Parse(source)
	if err != nil || !allowedLivePhotoMediaURL(target) {
		return "", &livePhotoDownloadError{kind: "address"}
	}
	copyClient := *client
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	copyClient.Transport = livePhotoTransport{base: transport}
	copyClient.Jar = nil
	copyClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowedLivePhotoMediaURL(next.URL) || via[len(via)-1].URL.Scheme == "https" && next.URL.Scheme != "https" {
			return &livePhotoDownloadError{kind: "redirect"}
		}
		return nil
	}
	path, err := downloadRemoteVideo(&copyClient, source, base, "https://weibo.com/", cookie)
	if err == nil {
		return path, nil
	}
	var failure *livePhotoDownloadError
	if errors.As(err, &failure) {
		return "", failure
	}
	var network net.Error
	if errors.As(err, &network) {
		kind := "network"
		if network.Timeout() {
			kind = "timeout"
		}
		return "", &livePhotoDownloadError{kind: kind}
	}
	return "", &livePhotoDownloadError{kind: "local_io"}
}

func collectWeiboLivePhotos(mblog map[string]any, indexes map[string]int, media []string) []PostLivePhoto {
	fids := make(map[string]string)
	for _, entry := range strings.Split(jsonValueString(mblog["pic_video"]), ",") {
		if parts := strings.SplitN(entry, ":", 2); len(parts) == 2 {
			fids[parts[0]] = parts[1]
		}
	}
	post := Post{Media: media}
	appendPicture := func(id string, picture map[string]any) {
		index, ok := indexes[id]
		if !ok || (jsonValueString(picture["type"]) != "livephoto" && fids[id] == "") {
			return
		}
		if video := weiboLivePhotoVideo(picture, fids[id]); video != "" {
			post.LivePhotos = append(post.LivePhotos, PostLivePhoto{Image: media[index], Video: video, PictureID: id})
		}
	}
	if pictures, ok := mblog["pics"].([]any); ok {
		for _, raw := range pictures {
			picture, _ := raw.(map[string]any)
			id := firstNonEmptyRemoteText(jsonValueString(picture["pid"]), jsonValueString(picture["pic_id"]), jsonValueString(picture["id"]), jsonValueString(picture["pic_idstr"]))
			appendPicture(id, picture)
		}
	}
	if pictures, ok := mblog["pic_infos"].(map[string]any); ok {
		for id, raw := range pictures {
			picture, _ := raw.(map[string]any)
			appendPicture(id, picture)
		}
	}
	return normalizePostLivePhotos(post)
}

func matchingStoredLivePhoto(photo PostLivePhoto, stored Post) (PostLivePhoto, bool) {
	for _, old := range stored.LivePhotos {
		if photo.PictureID != "" && photo.PictureID == old.PictureID || photo.Image == old.Image {
			return old, true
		}
	}
	return PostLivePhoto{}, false
}

func needsLivePhotoArchive(incoming, stored Post) bool {
	for _, photo := range incoming.LivePhotos {
		old, ok := matchingStoredLivePhoto(photo, stored)
		if !ok || !localFlowFileAvailable(old.Image) || !localFlowFileAvailable(old.Video) {
			return true
		}
	}
	return false
}

func (s *Store) pendingLivePhotoCount(feedID string) int {
	s.RLock()
	defer s.RUnlock()
	count := 0
	for _, post := range s.posts {
		if !containsString(post.FeedIDs, feedID) {
			continue
		}
		for _, photo := range normalizePostLivePhotos(post) {
			if availablePostImage(photo.Image) && !localFlowFileAvailable(photo.Video) {
				count++
			}
		}
	}
	return count
}

// Never pair legacy images by an unverified map iteration position.
func reusePostLivePhotos(incoming, stored Post) []PostLivePhoto {
	if len(incoming.LivePhotos) == 0 {
		return append([]PostLivePhoto(nil), stored.LivePhotos...)
	}
	photos := make([]PostLivePhoto, 0, len(incoming.LivePhotos))
	for _, photo := range incoming.LivePhotos {
		if old, ok := matchingStoredLivePhoto(photo, stored); ok && localFlowFileAvailable(old.Video) {
			photo.Video = old.Video
		}
		photos = append(photos, photo)
	}
	return photos
}
