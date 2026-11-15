package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

type tgSyncNotification struct {
	Feed   SourceConfig
	Posts  []Post
	Added  int
	Images int
}

func telegramLocalImages(post Post) []string {
	images := make([]string, 0, len(post.Media))
	seen := make(map[string]bool)
	for _, source := range post.Media {
		if !seen[source] && localFlowFileAvailable(source) {
			seen[source] = true
			images = append(images, source)
		}
	}
	return images
}

func telegramSyncNotification(feed SourceConfig, added []Post) (tgSyncNotification, bool) {
	var latest Post
	seen := make(map[string]bool)
	notification := tgSyncNotification{Feed: SourceConfig{ID: feed.ID, Source: feed.Source, Name: feed.Name}}
	for _, post := range added {
		if post.ID == "" || seen[post.ID] {
			continue
		}
		seen[post.ID] = true
		notification.Added++
		notification.Images += len(telegramLocalImages(post))
		if latest.ID == "" || post.Published.After(latest.Published) || (post.Published.Equal(latest.Published) && post.ID < latest.ID) {
			latest = post
		}
	}
	if notification.Added == 0 {
		return tgSyncNotification{}, false
	}
	// Only one post is retained by the bounded runtime queue, not a whole sync.
	notification.Posts = []Post{{ID: latest.ID, Source: latest.Source, Author: latest.Author, Caption: latest.Caption,
		Published: latest.Published, Media: telegramLocalImages(latest)}}
	return notification, true
}

func (s *Store) currentTelegramDelta(added []Post) []Post {
	ids := make(map[string]bool, len(added))
	for _, post := range added {
		ids[post.ID] = true
	}
	s.RLock()
	defer s.RUnlock()
	current := make([]Post, 0, len(ids))
	for _, post := range s.posts {
		if ids[post.ID] {
			post.Media = append([]string(nil), post.Media...)
			current = append(current, post)
		}
	}
	return current
}

func (b *BilibiliStore) queueTelegramNotification(feed SourceConfig, added []Post) {
	if len(added) == 0 {
		return
	}
	b.tgMu.Lock()
	bot := b.tgRuntime
	b.tgMu.Unlock()
	if bot == nil || !bot.config.Enabled || !bot.config.Notifications {
		return
	}
	if isCollectionFeed(feed) && b.content != nil {
		// Collection reconciliation can migrate originals after merge; resolve
		// only the new IDs after that migration, without copying the library.
		added = b.content.currentTelegramDelta(added)
	}
	notification, ok := telegramSyncNotification(feed, added)
	if !ok {
		return
	}
	select {
	case bot.notifications <- notification:
	default:
		log.Printf("event=telegram_notification_dropped reason=queue_full")
	}
}

func telegramSyncText(notification tgSyncNotification) string {
	labels := map[Source]string{SourceBilibili: "哔哩哔哩", SourceWeibo: "微博", SourcePixiv: "Pixiv", SourceTwitter: "推特"}
	label := labels[notification.Feed.Source]
	if label == "" {
		label = "动态"
	}
	headline := strings.TrimSpace(notification.Feed.Name)
	var caption string
	if len(notification.Posts) > 0 {
		post := notification.Posts[0]
		if post.Author != "" && post.Author != headline {
			headline += " · " + post.Author
		}
		caption = strings.TrimSpace(post.Caption)
	}
	text := strings.TrimSpace(label + " · " + headline)
	if caption != "" {
		text += "\n\n" + caption
	}
	return text + fmt.Sprintf("\n\n\n本次更新：新增 %d 条动态，%d 张图片。", notification.Added, notification.Images)
}

// Telegram entity offsets are UTF-16 based. Using the same conservative units
// here also keeps emoji-heavy plain text below caption and message limits.
func telegramTextUnits(text string) int {
	units := 0
	for _, r := range text {
		units++
		if r > 0xffff {
			units++
		}
	}
	return units
}

func splitTelegramText(text string, limit int) []string {
	if text == "" || limit < 2 {
		return nil
	}
	parts := make([]string, 0, telegramTextUnits(text)/limit+1)
	start, units := 0, 0
	for index, r := range text {
		width := 1
		if r > 0xffff {
			width++
		}
		if units+width > limit {
			parts = append(parts, text[start:index])
			start, units = index, 0
		}
		units += width
	}
	return append(parts, text[start:])
}

func telegramNotificationTextParts(text string) []string {
	index := strings.LastIndex(text, "\n\n\n本次更新：")
	if index < 0 {
		return splitTelegramText(text, 4096)
	}
	footer := text[index:]
	parts := splitTelegramText(text[:index], 4096-telegramTextUnits(footer))
	if len(parts) == 0 {
		return []string{footer}
	}
	parts[len(parts)-1] += footer
	return parts
}

func deliverTelegramSyncNotification(ctx context.Context, notification tgSyncNotification, photos func([]string, string) error, message func(string) error) error {
	if notification.Added <= 0 || len(notification.Posts) == 0 {
		return nil
	}
	text := telegramSyncText(notification)
	images := notification.Posts[0].Media
	captionFits := telegramTextUnits(text) <= 1024
	for index := 0; index < len(images); index += 10 {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := index + 10
		if end > len(images) {
			end = len(images)
		}
		caption := ""
		if captionFits && end == len(images) {
			caption = text
		}
		if err := photos(images[index:end], caption); err != nil {
			// Preserve the update and its totals even when a photo was removed
			// after archiving or Telegram declines an otherwise valid original.
			for _, part := range telegramNotificationTextParts("图片暂未发送成功。\n\n" + text) {
				if sendErr := message(part); sendErr != nil {
					return sendErr
				}
			}
			return err
		}
	}
	if !captionFits || len(images) == 0 {
		for _, part := range telegramNotificationTextParts(text) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := message(part); err != nil {
				return err
			}
		}
	}
	return nil
}

func (bot *tgBot) sendSyncNotification(ctx context.Context, notification tgSyncNotification) error {
	chatID, err := strconv.ParseInt(bot.config.ChatID, 10, 64)
	if err != nil || chatID <= 0 || !bot.config.Notifications {
		return nil
	}
	return deliverTelegramSyncNotification(ctx, notification, func(images []string, caption string) error {
		_, err := bot.sendPhotos(ctx, chatID, images, caption)
		return err
	}, func(text string) error {
		_, err := bot.sendText(ctx, chatID, text, nil)
		return err
	})
}
