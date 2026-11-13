package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func resetRemoteID(post Post) (string, error) {
	prefixes := map[Source][]string{
		SourceBilibili: {"bili-dynamic-", "bili-opus-"},
		SourceWeibo:    {"weibo-status-"},
		SourcePixiv:    {"pixiv-illust-"},
		SourceTwitter:  {"twitter-tweet-"},
	}
	for _, prefix := range prefixes[post.Source] {
		if strings.HasPrefix(post.ID, prefix) {
			id := strings.TrimPrefix(post.ID, prefix)
			if validWeiboUID(id) {
				return id, nil
			}
		}
	}
	return "", errors.New("动态缺少可重新拉取的平台编号")
}

func (b *BilibiliStore) fetchResetPost(stored Post, feed SourceConfig) (Post, error) {
	id, err := resetRemoteID(stored)
	if err != nil {
		return Post{}, err
	}
	b.RLock()
	config := b.config
	b.RUnlock()
	switch stored.Source {
	case SourceWeibo:
		credentials := config.Weibo
		var payload map[string]any
		if credentials.Cookie != "" || hasWeiboPassword(credentials) {
			var status int
			payload, status, err = b.weiboRawRequest("https://weibo.com/ajax/statuses/show?id="+url.QueryEscape(id), &credentials, config.ProxyURL)
			if err != nil {
				if status != http.StatusNotFound && status != http.StatusGone && !isWeiboSessionFailure(err) {
					log.Printf("event=post_reset_detail_failed platform=weibo route=desktop response_status=%d %s", status, syncLogError(err))
					return Post{}, err
				}
				log.Printf("event=post_reset_detail_fallback platform=weibo from=desktop to=mobile response_status=%d %s", status, syncLogError(err))
			}
		}
		if payload == nil {
			payload, _, err = b.weiboPublicRequest("https://m.weibo.cn/statuses/show?id="+url.QueryEscape(id), &credentials, config.ProxyURL)
		}
		if err != nil {
			return Post{}, err
		}
		var posts []Post
		collectWeiboPosts(payload, feed, &posts, make(map[string]bool))
		for _, post := range posts {
			if post.ID == stored.ID {
				return post, nil
			}
		}
	case SourceBilibili:
		if config.Credentials.SESSDATA == "" {
			return Post{}, errors.New("B站未连接")
		}
		if strings.HasPrefix(stored.ID, "bili-opus-") || strings.HasPrefix(stored.OriginalURL, "https://www.bilibili.com/opus/") {
			client, err := externalHTTPClient(config.ProxyURL)
			if err != nil {
				return Post{}, err
			}
			post, err := fetchBilibiliOpusPage(client, config.Credentials, id, feed)
			return post, err
		}
		var payload struct {
			Code int `json:"code"`
			Data struct {
				Item json.RawMessage `json:"item"`
			} `json:"data"`
		}
		err := bilibiliRequest("https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?id="+url.QueryEscape(id)+"&features="+url.QueryEscape(bilibiliDynamicFeatures), config.Credentials, config.ProxyURL, &payload)
		if err != nil {
			return Post{}, err
		}
		if payload.Code != 0 {
			return Post{}, fmt.Errorf("B站详情接口返回 %d", payload.Code)
		}
		return b.parseBilibiliDynamicPost(payload.Data.Item, feed, config.Credentials, config.ProxyURL)
	case SourcePixiv:
		if config.Pixiv.Cookie == "" {
			return Post{}, errors.New("Pixiv 未连接")
		}
		client, err := externalHTTPClient(config.ProxyURL)
		if err != nil {
			return Post{}, err
		}
		var payload map[string]any
		if err := pixivRequestJSON(client, "https://www.pixiv.net/ajax/illust/"+id+"?lang=zh", config.Pixiv, &payload); err != nil {
			return Post{}, err
		}
		body, _ := payload["body"].(map[string]any)
		if jsonValueString(body["id"]) != id {
			return Post{}, errors.New("Pixiv 返回的作品编号不匹配")
		}
		return pixivDetailPost(client, config.Pixiv, id, payload, feed)
	case SourceTwitter:
		if config.Twitter.APIKey == "" {
			return Post{}, errors.New("推特未连接")
		}
		client, err := externalHTTPClient(config.ProxyURL)
		if err != nil {
			return Post{}, err
		}
		var payload map[string]any
		if err := twitterRequest(client, twitterAPIBase+"/twitter/tweets?tweet_ids="+id, config.Twitter, &payload); err != nil {
			return Post{}, err
		}
		for _, tweet := range twitterTweetItems(payload) {
			if firstNonEmptyRemoteText(jsonValueString(tweet["id"]), jsonValueString(tweet["tweetId"])) == id {
				return twitterDetailPost(tweet, id, config.Twitter, feed), nil
			}
		}
	}
	return Post{}, errors.New("平台未返回该动态")
}

func (s *Store) replaceResetPost(previous, updated Post) (Post, error) {
	s.Lock()
	defer s.Unlock()
	for index, post := range s.posts {
		if post.ID != previous.ID {
			continue
		}
		if !postsEqual(previous, post) {
			return Post{}, errors.New("动态已更改，请刷新后重试")
		}
		updated.Liked, updated.FavoriteExplicit = post.Liked, post.FavoriteExplicit
		updated.FeedIDs = append([]string(nil), post.FeedIDs...)
		updated.Tags = mergeUniqueStrings(post.Tags, updated.Tags)
		updated.Edited = false
		oldPosts := append([]Post(nil), s.posts...)
		s.posts[index] = updated
		archives := map[textArchiveKey]bool{textArchiveKeyForPost(post): true, textArchiveKeyForPost(updated): true}
		if _, err := s.reconcileTextArchivesFor(archives); err != nil {
			s.posts = oldPosts
			_, _ = s.reconcileTextArchivesFor(archives)
			return Post{}, errors.New("无法更新正文归档")
		}
		if err := s.saveLocked(); err != nil {
			s.posts = oldPosts
			_, _ = s.reconcileTextArchivesFor(archives)
			return Post{}, errors.New("无法保存重置结果")
		}
		return s.posts[index], nil
	}
	return Post{}, errors.New("动态已删除")
}

func (b *BilibiliStore) resetPostHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || input.ID == "" || len(input.ID) > 128 {
		writeAPIError(w, 400, "动态编号无效")
		return
	}
	if !b.resetMu.TryLock() {
		writeAPIError(w, 409, "正在重置动态，请稍后重试")
		return
	}
	defer b.resetMu.Unlock()
	updated, status, err := b.resetPost(r.Context(), input.ID)
	if err != nil {
		if r.Context().Err() == nil {
			writeAPIError(w, status, err.Error())
		}
		return
	}
	writeJSON(w, map[string]any{"post": updated, "message": "动态已重新拉取"})
}

// The caller holds resetMu for this operation, including background batch items.
func (b *BilibiliStore) resetPost(ctx context.Context, id string) (Post, int, error) {
	if err := ctx.Err(); err != nil {
		return Post{}, 0, err
	}
	b.content.RLock()
	var stored Post
	for _, post := range b.content.posts {
		if post.ID == id {
			stored = post
			break
		}
	}
	b.content.RUnlock()
	if stored.ID == "" {
		return Post{}, 404, errors.New("动态不存在或已删除")
	}
	if _, err := resetRemoteID(stored); err != nil {
		return Post{}, 400, err
	}
	if !b.lastPostReset.IsZero() {
		delay := 450 * time.Millisecond
		if stored.Source == SourceBilibili {
			delay = 900 * time.Millisecond
		}
		if wait := delay - time.Since(b.lastPostReset); wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return Post{}, 0, ctx.Err()
			case <-timer.C:
			}
		}
	}
	b.lastPostReset = time.Now()
	feed := SourceConfig{ID: "reset-" + sessionDigest(stored.Author)[:12], Source: stored.Source, Name: stored.Author, Avatar: stored.Avatar, Tags: stored.Tags}
	feeds := b.allSourceConfigs()
	for _, candidate := range feeds {
		if candidate.Source == stored.Source && containsString(stored.FeedIDs, candidate.ID) {
			feed = candidate
			if !isCollectionFeed(candidate) {
				break
			}
		}
	}
	requestFeed := feed
	requestFeed.IncludeVideos = true
	requestFeed.ContentTypes = []string{"DRAW", "WORD", "ARTICLE", "AV"}
	updated, err := b.fetchResetPost(stored, requestFeed)
	if err := ctx.Err(); err != nil {
		return Post{}, 0, err
	}
	if err != nil || updated.ID != stored.ID || updated.Source != stored.Source ||
		(!stored.Edited && ((len(stored.Media) > 0 && len(updated.Media) == 0) ||
			(len(stored.Videos) > 0 && len(updated.Videos) == 0) || (len(stored.LivePhotos) > 0 && len(updated.LivePhotos) == 0))) {
		log.Printf("event=post_reset_failed platform=%s post=%s stage=fetch %s", syncLogPlatform(stored.Source), sessionDigest(stored.ID)[:12], syncLogError(err))
		return Post{}, 502, errors.New("重新拉取失败：请检查平台连接、访问权限或原动态是否仍可用；原归档已保留")
	}
	unlock := b.lockSourceArchive(feed.Source)
	defer unlock()
	// Archiving reads the latest credentials after any detail-request renewal.
	prepared, posts, err := b.archiveSourceContent(feed, []Post{updated})
	if err != nil || prepared.LastSyncWarning != "" {
		log.Printf("event=post_reset_failed platform=%s post=%s stage=archive %s", syncLogPlatform(stored.Source), sessionDigest(stored.ID)[:12], syncLogError(err))
		return Post{}, 502, errors.New("媒体重新归档失败；原动态已保留，请检查网络后重试")
	}
	if err := ctx.Err(); err != nil {
		return Post{}, 0, err
	}
	updated, err = b.content.replaceResetPost(stored, posts[0])
	if err != nil {
		return Post{}, 409, err
	}
	return withAvailablePostImages(updated), http.StatusOK, nil
}

type bilibiliDynamicItem struct {
	ID      string `json:"id_str"`
	Type    string `json:"type"`
	Modules struct {
		Author struct {
			Name  string          `json:"name"`
			Face  string          `json:"face"`
			PubTs json.RawMessage `json:"pub_ts"`
		} `json:"module_author"`
		Dynamic struct {
			Desc *struct {
				Text          string            `json:"text"`
				RichTextNodes []json.RawMessage `json:"rich_text_nodes"`
			} `json:"desc"`
			Major *struct {
				Draw *struct {
					Items []struct {
						Src string `json:"src"`
					} `json:"items"`
				} `json:"draw"`
				Article *struct {
					Title   string   `json:"title"`
					Desc    string   `json:"desc"`
					Content string   `json:"content"`
					Covers  []string `json:"covers"`
				} `json:"article"`
				Opus *struct {
					Title   string          `json:"title"`
					Summary json.RawMessage `json:"summary"`
					Content json.RawMessage `json:"content"`
					Pics    []struct {
						URL string `json:"url"`
					} `json:"pics"`
				} `json:"opus"`
				Archive *struct {
					BVID  string `json:"bvid"`
					Cover string `json:"cover"`
					Title string `json:"title"`
					Desc  string `json:"desc"`
				} `json:"archive"`
			} `json:"major"`
		} `json:"module_dynamic"`
	} `json:"modules"`
}

func (b *BilibiliStore) parseBilibiliDynamicPost(rawItem json.RawMessage, feed SourceConfig, credentials BilibiliCredentials, proxyURL string) (Post, error) {
	var item bilibiliDynamicItem
	if err := json.Unmarshal(rawItem, &item); err != nil {
		return Post{}, err
	}
	if item.ID == "" || !bilibiliDynamicTypeEnabled(item.Type, feed.ContentTypes, feed.IncludeVideos) {
		return Post{}, errors.New("动态类型不支持")
	}
	captionParts := make([]string, 0, 4)
	if item.Modules.Dynamic.Desc != nil {
		captionParts = append(captionParts, item.Modules.Dynamic.Desc.Text)
		if nodes, err := json.Marshal(item.Modules.Dynamic.Desc.RichTextNodes); err == nil {
			captionParts = append(captionParts, bilibiliRichText(nodes))
		}
	}
	media := make([]string, 0)
	videos := make([]PostVideo, 0, 1)
	if major := item.Modules.Dynamic.Major; major != nil {
		if major.Draw != nil {
			for _, image := range major.Draw.Items {
				media = append(media, normalizeRemoteImage(image.Src))
			}
		}
		if major.Article != nil {
			for _, image := range major.Article.Covers {
				media = append(media, normalizeRemoteImage(image))
			}
		}
		if major.Opus != nil {
			for _, image := range major.Opus.Pics {
				media = append(media, normalizeRemoteImage(image.URL))
			}
		}
		if major.Article != nil {
			captionParts = append(captionParts, major.Article.Title, major.Article.Desc, major.Article.Content)
		}
		if major.Opus != nil {
			captionParts = append(captionParts, major.Opus.Title, bilibiliRichText(major.Opus.Summary), bilibiliRichText(major.Opus.Content))
		}
		if major.Archive != nil {
			captionParts = append(captionParts, major.Archive.Title, major.Archive.Desc)
			if feed.IncludeVideos {
				time.Sleep(260 * time.Millisecond)
				videoURL, videoErr := fetchBilibiliVideoURL(major.Archive.BVID, credentials, proxyURL)
				if videoErr != nil {
					log.Printf("unable to resolve Bilibili video %s: %v", major.Archive.BVID, videoErr)
				} else {
					videos = append(videos, PostVideo{URL: videoURL, Poster: normalizeRemoteImage(major.Archive.Cover)})
				}
			}
		}
	}
	caption := mergeBilibiliCaption(combineRemoteText(captionParts...), bilibiliCaptionFromRaw(rawItem))
	if shouldFetchBilibiliDynamicDetail(item.Type, caption) {
		// 动态详情接口与专栏页面共享 B 站限流策略，避免一批动态连续请求。
		time.Sleep(260 * time.Millisecond)
		detailCaption := b.fetchBilibiliDynamicCaption(item.ID, credentials, proxyURL)
		caption = mergeBilibiliCaption(caption, detailCaption)
	}
	publishedAt := parseRemoteTimestamp(item.Modules.Author.PubTs)
	published := time.Unix(publishedAt, 0)
	if publishedAt == 0 {
		published = time.Now()
	}
	name, avatar := item.Modules.Author.Name, normalizeRemoteImage(item.Modules.Author.Face)
	if name == "" {
		name = feed.Name
	}
	if avatar == "" {
		avatar = feed.Avatar
	}
	return Post{ID: "bili-dynamic-" + item.ID, Source: SourceBilibili, FeedIDs: []string{feed.ID}, Author: name, Avatar: avatar, Caption: caption, Tags: append([]string(nil), feed.Tags...), Media: media, Videos: videos, OriginalURL: "https://t.bilibili.com/" + url.PathEscape(item.ID), Published: published}, nil
}

func pixivDetailPost(client *http.Client, credentials PixivCredentials, id string, detail map[string]any, feed SourceConfig) (Post, error) {
	body, ok := detail["body"].(map[string]any)
	if !ok || pixivPayloadFailed(detail) {
		return Post{}, errors.New("Pixiv 动态详情不可用")
	}
	user, _ := body["user"].(map[string]any)
	author := firstNonEmptyRemoteText(jsonValueString(body["userName"]), jsonValueString(user["name"]), feed.Name)
	avatar := firstNonEmptyRemoteText(jsonValueString(body["profileImageUrl"]), jsonValueString(user["imageBig"]), feed.Avatar)
	avatar = normalizeRemoteImage(avatar)
	if avatar == "" {
		avatar = feed.Avatar
	}
	caption := combineRemoteText(jsonValueString(body["title"]), jsonValueString(body["description"]))
	media := pixivOriginalURLs(client, credentials, id, detail)
	if len(media) == 0 {
		return Post{}, errors.New("Pixiv 未返回原图")
	}
	published := parsePixivTime(jsonValueString(body["createDate"]))
	return Post{ID: "pixiv-illust-" + id, Source: SourcePixiv, FeedIDs: []string{feed.ID}, Author: author, Avatar: avatar, Caption: caption, Tags: append([]string(nil), feed.Tags...), Media: media, OriginalURL: "https://www.pixiv.net/artworks/" + id, Published: published}, nil
}

func twitterDetailPost(tweet map[string]any, tweetID string, credentials TwitterCredentials, feed SourceConfig) Post {
	authorObject, _ := tweet["author"].(map[string]any)
	_, username, authorName, avatar := twitterUserFromValue(authorObject)
	username = firstNonEmptyRemoteText(username, strings.TrimPrefix(credentials.UserName, "@"))
	author := firstNonEmptyRemoteText(authorName, "@"+username, feed.Name)
	post := Post{ID: "twitter-tweet-" + tweetID, Source: SourceTwitter, FeedIDs: []string{feed.ID}, Author: author, Avatar: avatar, Caption: cleanRemoteText(firstNonEmptyRemoteText(jsonValueString(tweet["text"]), jsonValueString(tweet["fullText"]), jsonValueString(tweet["full_text"]))), Tags: append([]string(nil), feed.Tags...), OriginalURL: firstNonEmptyRemoteText(jsonValueString(tweet["url"]), "https://x.com/"+url.PathEscape(username)+"/status/"+url.PathEscape(tweetID)), Published: parseTwitterTime(firstNonEmptyRemoteText(jsonValueString(tweet["createdAt"]), jsonValueString(tweet["created_at"])))}
	for _, item := range twitterMediaItems(tweet) {
		kind := strings.ToLower(firstNonEmptyRemoteText(jsonValueString(item["type"]), jsonValueString(item["mediaType"])))
		mediaURL := normalizeRemoteImage(firstNonEmptyRemoteText(jsonValueString(item["media_url_https"]), jsonValueString(item["mediaUrl"]), jsonValueString(item["media_url"]), jsonValueString(item["originalUrl"]), jsonValueString(item["url"])))
		poster := normalizeRemoteImage(firstNonEmptyRemoteText(jsonValueString(item["thumbnailUrl"]), jsonValueString(item["preview_image_url"]), jsonValueString(item["previewImageUrl"])))
		if kind == "photo" || (kind == "" && mediaURL != "") {
			post.Media = append(post.Media, mediaURL)
			continue
		}
		videoInfo := item["video_info"]
		if videoInfo == nil {
			videoInfo = item["videoInfo"]
		}
		if variants, ok := videoInfo.(map[string]any); ok {
			if list, ok := variants["variants"].([]any); ok {
				values := make([]map[string]any, 0, len(list))
				for _, value := range list {
					if object, ok := value.(map[string]any); ok {
						values = append(values, object)
					}
				}
				mediaURL = twitterVideoURL(values)
			}
		}
		if mediaURL != "" {
			post.Videos = append(post.Videos, PostVideo{URL: mediaURL, Poster: poster})
		}
	}
	return post
}
