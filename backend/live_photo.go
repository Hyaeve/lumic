package main

import (
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
	return "https://us.sinaimg.cn/" + fid + ".mov"
}

func weiboLivePhotoDirectURL(source string) string {
	target, err := url.Parse(source)
	if err != nil {
		return ""
	}
	if target.Hostname() != "video.weibo.com" || target.Path != "/media/play" {
		return source
	}
	// The page wrapper is not the motion file; only unwrap its explicit CDN URL.
	motion, err := url.Parse(normalizeRemoteImage(target.Query().Get("livephoto")))
	if err != nil || motion.User != nil || motion.Port() != "" || motion.Scheme != "https" && motion.Scheme != "http" {
		return ""
	}
	host := strings.ToLower(motion.Hostname())
	if !strings.HasSuffix(host, ".sinaimg.cn") {
		return ""
	}
	return motion.String()
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
