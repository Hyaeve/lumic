package main

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

type authorSubscriptionTarget struct {
	Source           Source
	ID, Name, Avatar string
}

func normalizeSubscriptionAuthorID(source Source, id string) string {
	id = strings.TrimSpace(id)
	switch source {
	case SourceBilibili, SourceWeibo, SourcePixiv:
		if validWeiboUID(id) {
			if source != SourceWeibo {
				if _, err := strconv.ParseUint(id, 10, 64); err != nil {
					return ""
				}
			}
			return strings.TrimLeft(id, "0")
		}
	case SourceTwitter:
		id = strings.ToLower(strings.TrimPrefix(id, "@"))
		if len(id) == 0 || len(id) > 15 {
			return ""
		}
		for _, r := range id {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return ""
			}
		}
		return id
	}
	return ""
}

func sourceAuthorIDFromFeedID(source Source, feedID string) string {
	var prefix string
	switch source {
	case SourceBilibili:
		prefix = "bili-"
	case SourceWeibo:
		prefix = "weibo-"
	case SourcePixiv:
		prefix = "pixiv-"
	case SourceTwitter:
		prefix = "twitter-author-"
	default:
		return ""
	}
	id, ok := strings.CutPrefix(feedID, prefix)
	if !ok || id != strings.TrimSpace(id) {
		return ""
	}
	return normalizeSubscriptionAuthorID(source, id)
}

func archivedPostURLAuthorID(post Post) string {
	if post.Source != SourceWeibo && post.Source != SourceTwitter {
		return ""
	}
	parsed, err := url.Parse(post.OriginalURL)
	if err != nil || parsed.User != nil || parsed.Port() != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	postID, err := resetRemoteID(post)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	host := strings.ToLower(parsed.Hostname())
	if post.Source == SourceWeibo && (host == "weibo.com" || host == "www.weibo.com") && len(parts) == 2 && parts[1] == postID {
		return normalizeSubscriptionAuthorID(post.Source, parts[0])
	}
	if post.Source == SourceTwitter && (host == "x.com" || host == "twitter.com" || host == "www.twitter.com" || host == "www.x.com") &&
		len(parts) == 3 && parts[1] == "status" && parts[2] == postID && !strings.EqualFold(parts[0], "i") {
		id := normalizeSubscriptionAuthorID(post.Source, parts[0])
		for _, feedID := range post.FeedIDs {
			owner, collection := strings.CutPrefix(feedID, "twitter-likes-")
			if collection && normalizeSubscriptionAuthorID(post.Source, owner) == id {
				// Older tweet parsers could synthesize this URL with the account
				// that owned the likes, rather than the actual tweet's author.
				return ""
			}
		}
		return id
	}
	return ""
}

func archivedPostAuthorID(post Post) string {
	if id := normalizeSubscriptionAuthorID(post.Source, post.AuthorID); id != "" {
		return id
	}
	if id := archivedPostURLAuthorID(post); id != "" {
		return id
	}
	var id string
	for _, feedID := range post.FeedIDs {
		if isCollectionFeed(SourceConfig{Source: post.Source, ID: feedID}) {
			// Older archive reconciliation attached direct feeds by display
			// name, so collection membership cannot prove an author's UID.
			return ""
		}
		candidate := sourceAuthorIDFromFeedID(post.Source, feedID)
		if candidate == "" {
			continue
		}
		if id != "" && id != candidate {
			return ""
		}
		id = candidate
	}
	return id
}

func authorTargetFromPost(post Post, id string) authorSubscriptionTarget {
	return authorSubscriptionTarget{Source: post.Source, ID: id, Name: strings.TrimSpace(post.Author), Avatar: post.Avatar}
}

func (b *BilibiliStore) resolveAuthorSubscription(source Source, author, postID, feedID string) (authorSubscriptionTarget, bool) {
	author = strings.TrimSpace(author)
	if author == "" {
		return authorSubscriptionTarget{}, false
	}
	if feedID != "" {
		id := sourceAuthorIDFromFeedID(source, feedID)
		if id == "" {
			return authorSubscriptionTarget{}, false
		}
		if feed, found := b.findSyncSource(feedID, source); found && !isCollectionFeed(feed) {
			return authorSubscriptionTarget{Source: source, ID: id, Name: strings.TrimSpace(feed.Name), Avatar: feed.Avatar}, true
		}
		// The exact source may have been removed while its posts are retained.
		if b.content != nil {
			b.content.RLock()
			defer b.content.RUnlock()
			for _, post := range b.content.posts {
				if post.Source == source && strings.TrimSpace(post.Author) == author && containsString(post.FeedIDs, feedID) && archivedPostAuthorID(post) == id {
					return authorTargetFromPost(post, id), true
				}
			}
		}
		return authorSubscriptionTarget{}, false
	}
	if b.content == nil {
		return authorSubscriptionTarget{}, false
	}
	b.content.RLock()
	defer b.content.RUnlock()
	var target authorSubscriptionTarget
	for _, post := range b.content.posts {
		if post.Source != source || strings.TrimSpace(post.Author) != author || (postID != "" && post.ID != postID) {
			continue
		}
		id := archivedPostAuthorID(post)
		if id == "" || (target.ID != "" && target.ID != id) {
			return authorSubscriptionTarget{}, false
		}
		if target.ID == "" {
			target = authorTargetFromPost(post, id)
		}
		if postID != "" {
			return target, true
		}
	}
	return target, target.ID != ""
}

func (b *BilibiliStore) legacyAuthorSubscriptionPost(source Source, author, postID string) (Post, bool) {
	if b.content == nil || postID == "" || (source != SourceBilibili && source != SourcePixiv && source != SourceTwitter && source != SourceWeibo) {
		return Post{}, false
	}
	b.content.RLock()
	defer b.content.RUnlock()
	for _, post := range b.content.posts {
		if post.ID == postID && post.Source == source && strings.TrimSpace(post.Author) == strings.TrimSpace(author) && archivedPostAuthorID(post) == "" {
			_, err := resetRemoteID(post)
			return post, err == nil
		}
	}
	return Post{}, false
}

func (b *BilibiliStore) canResolveLegacyAuthorSubscription(source Source, author, postID string) bool {
	_, ok := b.legacyAuthorSubscriptionPost(source, author, postID)
	return ok
}

func (b *BilibiliStore) resolveLegacyAuthorSubscription(source Source, author, postID string) (authorSubscriptionTarget, bool) {
	stored, ok := b.legacyAuthorSubscriptionPost(source, author, postID)
	if !ok {
		return b.resolveAuthorSubscription(source, author, postID, "")
	}
	id, _ := resetRemoteID(stored)
	resolvedID := b.fetchLegacySubscriptionAuthorID(stored, id)
	if resolvedID == "" {
		return authorSubscriptionTarget{}, false
	}
	b.content.Lock()
	defer b.content.Unlock()
	for index, current := range b.content.posts {
		if current.ID != stored.ID || current.Source != stored.Source || current.Author != stored.Author {
			continue
		}
		if currentID := archivedPostAuthorID(current); currentID != "" {
			if currentID == resolvedID {
				return authorTargetFromPost(current, currentID), true
			}
			return authorSubscriptionTarget{}, false
		}
		b.content.posts[index].AuthorID = resolvedID
		if err := b.content.saveLocked(); err != nil {
			b.content.posts[index].AuthorID = current.AuthorID
			return authorSubscriptionTarget{}, false
		}
		return authorTargetFromPost(current, resolvedID), true
	}
	return authorSubscriptionTarget{}, false
}

func (b *BilibiliStore) fetchLegacySubscriptionAuthorID(stored Post, id string) string {
	b.RLock()
	config := b.config
	b.RUnlock()
	switch stored.Source {
	case SourcePixiv:
		if config.Pixiv.Cookie == "" {
			return ""
		}
		client, err := externalHTTPClient(config.ProxyURL)
		if err != nil {
			return ""
		}
		var payload map[string]any
		if pixivRequestJSON(client, "https://www.pixiv.net/ajax/illust/"+id+"?lang=zh", config.Pixiv, &payload) != nil || pixivPayloadFailed(payload) {
			return ""
		}
		body, _ := payload["body"].(map[string]any)
		if jsonValueString(body["id"]) != id {
			return ""
		}
		return pixivPostAuthorID(body)
	case SourceBilibili:
		if config.Credentials.SESSDATA == "" {
			return ""
		}
		if !strings.HasPrefix(stored.ID, "bili-opus-") && !strings.HasPrefix(stored.OriginalURL, "https://www.bilibili.com/opus/") {
			raw, err := fetchBilibiliDynamicDetail(id, config.Credentials, config.ProxyURL)
			if err != nil {
				return ""
			}
			var detail bilibiliDynamicItem
			if json.Unmarshal(raw, &detail) != nil || detail.ID != id {
				return ""
			}
			return normalizeSubscriptionAuthorID(stored.Source, jsonScalarString(detail.Modules.Author.Mid))
		}
	}
	post, err := b.fetchResetPost(stored, SourceConfig{Source: stored.Source, Name: stored.Author, ContentTypes: []string{"DRAW", "WORD", "ARTICLE", "AV"}})
	if err != nil || post.Source != stored.Source {
		return ""
	}
	if returnedID, err := resetRemoteID(post); err != nil || id != returnedID {
		return ""
	}
	return normalizeSubscriptionAuthorID(stored.Source, post.AuthorID)
}

func pixivPostAuthorID(body map[string]any) string {
	user, _ := body["user"].(map[string]any)
	return normalizeSubscriptionAuthorID(SourcePixiv, firstNonEmptyRemoteText(jsonValueString(body["userId"]), jsonValueString(user["id"])))
}
