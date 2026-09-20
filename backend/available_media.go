package main

import (
	"errors"
	"os"
	"strings"
)

// Only confirmed missing local originals are removed from response copies.
// A permission or transient I/O error must not hide otherwise valid media.
func availablePostImage(source string) bool {
	if !strings.HasPrefix(source, "/flow/") {
		return true
	}
	path, ok := localFlowArchivePath(source)
	if !ok {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	return !info.IsDir()
}

func withAvailablePostImages(post Post) Post {
	media := make([]string, 0, len(post.Media))
	for _, source := range post.Media {
		if availablePostImage(source) {
			media = append(media, source)
		}
	}
	post.Media = media
	return post
}
