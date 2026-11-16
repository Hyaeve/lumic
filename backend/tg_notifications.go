package main

import (
	"context"
	"errors"
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

func telegramNotificationText(text string, limit int) string {
	if telegramTextUnits(text) <= limit {
		return text
	}
	index := strings.LastIndex(text, "\n\n\n本次更新：")
	if index < 0 {
		return tgClip(text, limit)
	}
	footer := text[index:]
	return strings.TrimRight(tgClip(text[:index], limit-telegramTextUnits(footer)), "\r\n ") + footer
}

func deliverTelegramSyncNotification(ctx context.Context, notification tgSyncNotification, photos func([]string, string) error, message func(string) error) error {
	if notification.Added <= 0 || len(notification.Posts) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	text := telegramSyncText(notification)
	images := notification.Posts[0].Media
	if len(images) == 0 {
		return message(telegramNotificationText(text, 4096))
	}
	// An album creates multiple Telegram messages. Send one preview with its
	// caption so the update and aggregate result always stay together.
	if err := photos(images[:1], telegramNotificationText(text, 1024)); err != nil {
		var uncertain tgPhotoDeliveryUncertainError
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &uncertain) {
			return err
		}
		// Only preparation failures or an explicit API rejection permit a text
		// fallback; an ambiguous network failure might already have delivered.
		if sendErr := message(telegramNotificationText("图片暂未发送成功。\n\n"+text, 4096)); sendErr != nil {
			return sendErr
		}
		return err
	}
	return nil
}

func (bot *tgBot) sendSyncNotification(ctx context.Context, notification tgSyncNotification) error {
	if !bot.currentGeneration() {
		return context.Canceled
	}
	chatID, err := strconv.ParseInt(bot.config.ChatID, 10, 64)
	if err != nil || chatID <= 0 || !bot.config.Notifications {
		return nil
	}
	return deliverTelegramSyncNotification(ctx, notification, func(images []string, caption string) error {
		_, err := bot.sendPreviewPhoto(ctx, chatID, images[0], caption)
		return err
	}, func(text string) error {
		_, err := bot.sendText(ctx, chatID, text, nil)
		return err
	})
}
