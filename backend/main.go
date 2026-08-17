package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	xdraw "golang.org/x/image/draw"
	xproxy "golang.org/x/net/proxy"
)

type Source string

const (
	SourceWeibo    Source = "weibo"
	SourcePixiv    Source = "pixiv"
	SourceBilibili Source = "bilibili"
	SourceTwitter  Source = "twitter"

	bilibiliFavoriteOpusPrefix = "bili-opus-favorites-"
	bilibiliFavoriteOpusName   = "收藏专栏"
	weiboLikesName             = "我的点赞"
	pixivBookmarksName         = "P站收藏"
	twitterLikesName           = "推特点赞"
)

type mediaPreviewProfile struct {
	Level        int
	MaxDimension int
	JPEGQuality  int
	Suffix       string
}

func mediaPreviewProfileFor(level int, device, network string) mediaPreviewProfile {
	if level < 0 || level > 5 {
		if device == "mobile" {
			level = 3
		} else {
			level = 2
		}
	}
	if level == 5 {
		switch network {
		case "slow":
			level = 4
		case "medium":
			level = 3
		default:
			if device == "mobile" {
				level = 2
			} else {
				level = 1
			}
		}
	}
	profiles := map[int]struct{ dimension, quality int }{
		1: {1600, 93},
		2: {1100, 89},
		3: {720, 80},
		4: {480, 72},
	}
	selected, ok := profiles[level]
	if !ok {
		return mediaPreviewProfile{Level: 0}
	}
	if device != "mobile" {
		device = "desktop"
	}
	return mediaPreviewProfile{
		Level:        level,
		MaxDimension: selected.dimension,
		JPEGQuality:  selected.quality,
		Suffix:       fmt.Sprintf(".q2.%s.%d.jpg", device, level),
	}
}

type Post struct {
	ID               string      `json:"id"`
	Source           Source      `json:"source"`
	FeedIDs          []string    `json:"feedIds,omitempty"`
	Author           string      `json:"author"`
	Avatar           string      `json:"avatar"`
	Caption          string      `json:"caption"`
	Emojis           []PostEmoji `json:"emojis,omitempty"` // Legacy image metadata, removed while loading.
	TextFile         string      `json:"textFile,omitempty"`
	Tags             []string    `json:"tags"`
	Media            []string    `json:"media"`
	Videos           []PostVideo `json:"videos,omitempty"`
	OriginalURL      string      `json:"originalUrl,omitempty"`
	Published        time.Time   `json:"published"`
	Liked            bool        `json:"liked"`
	FavoriteExplicit bool        `json:"favoriteExplicit,omitempty"`
}

type PostVideo struct {
	URL    string `json:"url"`
	Poster string `json:"poster,omitempty"`
}

type PostEmoji struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type SourceConfig struct {
	ID              string    `json:"id"`
	Source          Source    `json:"source"`
	Name            string    `json:"name"`
	Handle          string    `json:"handle"`
	Avatar          string    `json:"avatar,omitempty"`
	Enabled         bool      `json:"enabled"`
	Schedule        string    `json:"schedule"`
	StartDate       string    `json:"startDate,omitempty"`
	ContentTypes    []string  `json:"contentTypes,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	OnlyWithImages  bool      `json:"onlyWithImages,omitempty"`
	IncludeVideos   bool      `json:"includeVideos,omitempty"`
	IncludeKeywords []string  `json:"includeKeywords,omitempty"`
	ExcludeKeywords []string  `json:"excludeKeywords,omitempty"`
	LastSyncedAt    time.Time `json:"lastSyncedAt"`
	LastSyncStatus  string    `json:"lastSyncStatus,omitempty"`
	LastSyncMessage string    `json:"lastSyncMessage,omitempty"`
	LastSyncCount   int       `json:"lastSyncCount,omitempty"`
	StoragePath     string    `json:"storagePath,omitempty"`
}

const sourceStartDateLayout = "2006-01-02"

func normalizeSourceStartDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(sourceStartDateLayout, value)
	if err != nil {
		return "", errors.New("订阅起始日期格式无效，请使用 YYYY-MM-DD")
	}
	return parsed.Format(sourceStartDateLayout), nil
}

func sourceStartBoundary(feed SourceConfig) (time.Time, bool) {
	value, err := normalizeSourceStartDate(feed.StartDate)
	if err != nil || value == "" {
		return time.Time{}, false
	}
	boundary, err := time.ParseInLocation(sourceStartDateLayout, value, time.Local)
	return boundary, err == nil
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool)
	for _, raw := range tags {
		for _, value := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == '#' || r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t'
		}) {
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				seen[value] = true
				result = append(result, value)
			}
		}
	}
	return result
}

func normalizeKeywords(values []string) []string {
	result, seen := make([]string, 0), make(map[string]bool)
	for _, raw := range values {
		for _, value := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t'
		}) {
			value = strings.TrimSpace(value)
			key := strings.ToLower(value)
			if value != "" && !seen[key] {
				seen[key] = true
				result = append(result, value)
			}
		}
	}
	return result
}

func mergeUniqueStrings(groups ...[]string) []string {
	result, seen := make([]string, 0), make(map[string]bool)
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeBilibiliContentTypes(values []string, legacyDefault bool) []string {
	if len(values) == 0 && legacyDefault {
		return []string{"DRAW", "ARTICLE"}
	}
	result := []string{"DRAW"}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "ARTICLE") {
			result = append(result, "ARTICLE")
			break
		}
	}
	return result
}

func bilibiliDynamicTypeEnabled(dynamicType string, contentTypes []string, includeVideos bool) bool {
	if !allowedBilibiliDynamicType(dynamicType) {
		return false
	}
	if dynamicType == "DYNAMIC_TYPE_AV" {
		return includeVideos
	}
	if dynamicType == "DYNAMIC_TYPE_ARTICLE" {
		return containsString(normalizeBilibiliContentTypes(contentTypes, true), "ARTICLE")
	}
	return true
}

func hasWeiboLikesFeed(values []string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, "weibo-likes-") {
			return true
		}
	}
	return false
}

func hasFeedPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func isBilibiliFavoriteOpusFeed(feed SourceConfig) bool {
	return feed.Source == SourceBilibili && strings.HasPrefix(feed.ID, bilibiliFavoriteOpusPrefix)
}

func isCollectionFeed(feed SourceConfig) bool {
	return (feed.Source == SourceWeibo && strings.HasPrefix(feed.ID, "weibo-likes-")) ||
		(feed.Source == SourcePixiv && strings.HasPrefix(feed.ID, "pixiv-bookmarks-")) ||
		(feed.Source == SourceTwitter && strings.HasPrefix(feed.ID, "twitter-likes-")) ||
		isBilibiliFavoriteOpusFeed(feed)
}

// Collection posts retain their collection feed ID for browsing, but share the
// subscribed author's archive whenever that author is also configured directly.
func collectionAuthorSubscription(post Post, feeds []SourceConfig) (SourceConfig, bool) {
	for _, feed := range feeds {
		if feed.Source != post.Source || isCollectionFeed(feed) || !sameSourceAuthor(post.Author, feed.Name) {
			continue
		}
		return feed, true
	}
	return SourceConfig{}, false
}

func postUsesAuthorArchive(post Post) bool {
	switch post.Source {
	case SourceWeibo:
		for _, feedID := range post.FeedIDs {
			if !strings.HasPrefix(feedID, "weibo-likes-") {
				return true
			}
		}
	case SourcePixiv:
		for _, feedID := range post.FeedIDs {
			if !strings.HasPrefix(feedID, "pixiv-bookmarks-") {
				return true
			}
		}
	case SourceBilibili:
		for _, feedID := range post.FeedIDs {
			if !strings.HasPrefix(feedID, bilibiliFavoriteOpusPrefix) {
				return true
			}
		}
	case SourceTwitter:
		for _, feedID := range post.FeedIDs {
			if !strings.HasPrefix(feedID, "twitter-likes-") {
				return true
			}
		}
	}
	return false
}

func filterSourcePosts(posts []Post, feed SourceConfig) []Post {
	include, exclude := normalizeKeywords(feed.IncludeKeywords), normalizeKeywords(feed.ExcludeKeywords)
	startBoundary, hasStartBoundary := sourceStartBoundary(feed)
	filtered := make([]Post, 0, len(posts))
	for _, post := range posts {
		if hasStartBoundary && post.Published.Before(startBoundary) {
			continue
		}
		if len(post.Videos) > 0 && !feed.IncludeVideos {
			continue
		}
		if feed.OnlyWithImages && len(post.Media) == 0 && !(feed.IncludeVideos && len(post.Videos) > 0) {
			continue
		}
		caption := strings.ToLower(post.Caption)
		blocked := false
		for _, keyword := range exclude {
			if strings.Contains(caption, strings.ToLower(keyword)) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		if len(include) > 0 {
			matched := false
			for _, keyword := range include {
				if strings.Contains(caption, strings.ToLower(keyword)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, post)
	}
	return filtered
}

func cronFieldMatches(field string, value, min, max int) bool {
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		step := 1
		if pieces := strings.Split(part, "/"); len(pieces) == 2 {
			parsed, err := strconv.Atoi(pieces[1])
			if err != nil || parsed < 1 {
				return false
			}
			step = parsed
			part = pieces[0]
		}
		start, end := min, max
		if part != "*" {
			if strings.Contains(part, "-") {
				pieces := strings.Split(part, "-")
				if len(pieces) != 2 {
					return false
				}
				var err error
				start, err = strconv.Atoi(pieces[0])
				if err != nil {
					return false
				}
				end, err = strconv.Atoi(pieces[1])
				if err != nil {
					return false
				}
			} else {
				parsed, err := strconv.Atoi(part)
				if err != nil {
					return false
				}
				start, end = parsed, parsed
			}
		}
		if start < min || end > max || start > end {
			return false
		}
		if value >= start && value <= end && (value-start)%step == 0 {
			return true
		}
	}
	return false
}

func cronMatches(expression string, at time.Time) bool {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return false
	}
	return cronFieldMatches(fields[0], at.Minute(), 0, 59) && cronFieldMatches(fields[1], at.Hour(), 0, 23) && cronFieldMatches(fields[2], at.Day(), 1, 31) && cronFieldMatches(fields[3], int(at.Month()), 1, 12) && cronFieldMatches(fields[4], int(at.Weekday()), 0, 6)
}

func normalizeSchedule(value string) string {
	switch strings.TrimSpace(value) {
	case "每 1 小时":
		return "0 * * * *"
	case "每 6 小时":
		return "0 */6 * * *"
	case "每 12 小时":
		return "0 */12 * * *"
	case "每天 20:00":
		return "0 20 * * *"
	case "每天 09:00":
		return "0 9 * * *"
	default:
		if validCron(value) {
			return strings.Join(strings.Fields(value), " ")
		}
		return "0 6 * * *"
	}
}

func validCron(expression string) bool {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return false
	}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for index, field := range fields {
		valid := false
		for value := ranges[index][0]; value <= ranges[index][1]; value++ {
			if cronFieldMatches(field, value, ranges[index][0], ranges[index][1]) {
				valid = true
				break
			}
		}
		if !valid {
			return false
		}
	}
	return true
}

type BilibiliCredentials struct {
	Cookie          string `json:"cookie,omitempty"`
	SESSDATA        string `json:"SESSDATA"`
	BiliJCT         string `json:"bili_jct"`
	Buvid3          string `json:"buvid3"`
	DedeUserID      string `json:"DedeUserID"`
	AccessTimeValue string `json:"ac_time_value,omitempty"`
	Buvid4          string `json:"buvid4,omitempty"`
	DedeUserIDCKMd5 string `json:"DedeUserID__ckMd5,omitempty"`
	UserName        string `json:"userName,omitempty"`
	Avatar          string `json:"avatar,omitempty"`
}

type BilibiliAccount struct {
	Configured bool   `json:"configured"`
	UserID     string `json:"userId,omitempty"`
	UserName   string `json:"userName,omitempty"`
	Avatar     string `json:"avatar,omitempty"`
}

type PixivCredentials struct {
	RefreshToken string `json:"refreshToken,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	UserAgent    string `json:"userAgent,omitempty"`
	Baggage      string `json:"baggage,omitempty"`
	Cookie       string `json:"cookie,omitempty"`
	SentryTrace  string `json:"sentryTrace,omitempty"`
	CSRFToken    string `json:"csrfToken,omitempty"`
	UserID       string `json:"userId,omitempty"`
	UserName     string `json:"userName,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
}

type WeiboCredentials struct {
	Cookie   string `json:"cookie,omitempty"`
	UserID   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

// TwitterCredentials holds an OAuth 2.0 user access token. The token is kept
// only in the encrypted project configuration and is never returned to clients.
type TwitterCredentials struct {
	AccessToken string `json:"accessToken,omitempty"`
	UserID      string `json:"userId,omitempty"`
	UserName    string `json:"userName,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

type BilibiliQRSession struct {
	ID        string    `json:"id"`
	Key       string    `json:"-"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type WeiboQRSession struct {
	ID         string    `json:"id"`
	QRID       string    `json:"-"`
	Image      string    `json:"image,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Exchanging bool      `json:"-"`
}

type BilibiliConfig struct {
	Credentials          BilibiliCredentials `json:"credentials"`
	Pixiv                PixivCredentials    `json:"pixiv"`
	Weibo                WeiboCredentials    `json:"weibo"`
	Twitter              TwitterCredentials  `json:"twitter"`
	Subscriptions        []SourceConfig      `json:"subscriptions"`
	WeiboSubscriptions   []SourceConfig      `json:"weiboSubscriptions"`
	PixivSubscriptions   []SourceConfig      `json:"pixivSubscriptions"`
	TwitterSubscriptions []SourceConfig      `json:"twitterSubscriptions"`
	ProxyURL             string              `json:"proxyUrl,omitempty"`
	PreviewDesktopLevel  *int                `json:"previewDesktopLevel,omitempty"`
	PreviewMobileLevel   *int                `json:"previewMobileLevel,omitempty"`
}

type ProjectSettingsView struct {
	ProxyEnabled        bool   `json:"proxyEnabled"`
	ProxyURL            string `json:"proxyUrl,omitempty"`
	PreviewDesktopLevel int    `json:"previewDesktopLevel"`
	PreviewMobileLevel  int    `json:"previewMobileLevel"`
}

func normalizedPreviewLevel(value *int, fallback int) int {
	if value == nil || *value < 0 || *value > 5 {
		return fallback
	}
	return *value
}

func projectSettingsView(config BilibiliConfig) ProjectSettingsView {
	return ProjectSettingsView{
		ProxyEnabled:        config.ProxyURL != "",
		ProxyURL:            maskedProxyURL(config.ProxyURL),
		PreviewDesktopLevel: normalizedPreviewLevel(config.PreviewDesktopLevel, 2),
		PreviewMobileLevel:  normalizedPreviewLevel(config.PreviewMobileLevel, 3),
	}
}

type BilibiliStore struct {
	sync.RWMutex
	config          BilibiliConfig
	key             []byte
	content         *Store
	bilibiliQR      map[string]BilibiliQRSession
	bilibiliClients map[string]*http.Client
	weiboQR         map[string]WeiboQRSession
	weiboClients    map[string]*http.Client
	syncInProgress  bool
}

type BilibiliUser struct {
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Fans        int64  `json:"fans"`
	Description string `json:"description"`
}

type WeiboUser struct {
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Fans        int64  `json:"fans"`
	Description string `json:"description"`
}

func collectionFeedWithAccountProfile(feed SourceConfig, config BilibiliConfig) SourceConfig {
	avatar := ""
	matched := false
	switch {
	case strings.TrimSpace(config.Credentials.DedeUserID) != "" && feed.ID == bilibiliFavoriteOpusPrefix+strings.TrimSpace(config.Credentials.DedeUserID):
		avatar = config.Credentials.Avatar
		matched = true
	case strings.TrimSpace(config.Weibo.UserID) != "" && feed.ID == "weibo-likes-"+strings.TrimSpace(config.Weibo.UserID):
		avatar = config.Weibo.Avatar
		matched = true
		if strings.TrimSpace(feed.Handle) == "" {
			feed.Handle = config.Weibo.UserName
		}
	case strings.TrimSpace(config.Pixiv.UserID) != "" && feed.ID == "pixiv-bookmarks-"+strings.TrimSpace(config.Pixiv.UserID):
		avatar = config.Pixiv.Avatar
		matched = true
	case strings.TrimSpace(config.Twitter.UserID) != "" && feed.ID == "twitter-likes-"+strings.TrimSpace(config.Twitter.UserID):
		avatar = config.Twitter.Avatar
		matched = true
		if strings.TrimSpace(feed.Handle) == "" {
			feed.Handle = "@" + strings.TrimPrefix(config.Twitter.UserName, "@")
		}
	}
	if strings.TrimSpace(avatar) != "" && !strings.HasPrefix(strings.TrimSpace(feed.Avatar), "/flow/") {
		feed.Avatar = avatar
	}
	if matched {
		feed.Avatar = normalizeRemoteImage(feed.Avatar)
	}
	return feed
}

func applyCollectionAccountProfiles(feeds []SourceConfig, config BilibiliConfig) {
	for index := range feeds {
		feeds[index] = collectionFeedWithAccountProfile(feeds[index], config)
	}
}

func sourceConfigsFromConfig(config BilibiliConfig) []SourceConfig {
	feeds := make([]SourceConfig, 0, len(config.Subscriptions)+len(config.WeiboSubscriptions)+len(config.PixivSubscriptions)+len(config.TwitterSubscriptions))
	feeds = append(feeds, config.Subscriptions...)
	feeds = append(feeds, config.WeiboSubscriptions...)
	feeds = append(feeds, config.PixivSubscriptions...)
	feeds = append(feeds, config.TwitterSubscriptions...)
	applyCollectionAccountProfiles(feeds, config)
	return feeds
}

func (b *BilibiliStore) allSourceConfigs() []SourceConfig {
	b.RLock()
	defer b.RUnlock()
	return sourceConfigsFromConfig(b.config)
}

func (b *BilibiliStore) reconcileCollectionArchives() error {
	if b.content == nil {
		return nil
	}
	feeds := b.allSourceConfigs()
	for _, feed := range feeds {
		if !isCollectionFeed(feed) {
			continue
		}
		if err := b.content.reconcileCollectionMedia(feed, feeds); err != nil {
			return err
		}
	}
	return nil
}

type Store struct {
	sync.RWMutex
	posts []Post
	feeds []SourceConfig
	file  string
}

type ContentData struct {
	Posts []Post         `json:"posts"`
	Feeds []SourceConfig `json:"feeds"`
}

type ConfigurationBackup struct {
	Version    int            `json:"version"`
	ExportedAt time.Time      `json:"exportedAt"`
	Auth       AuthBackup     `json:"auth"`
	Platforms  BilibiliConfig `json:"platforms"`
}

type AuthBackup struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

type SessionStore struct {
	sync.RWMutex
	tokens map[string]time.Time
}

type AuthConfig struct {
	sync.RWMutex
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

var authFile = "/data/auth.json"
var contentFile = "/data/content.json"
var bilibiliFile = "/data/bilibili.enc"
var secretFile = "/data/secret.key"
var flowRoot = "/flow"
var previewRoot = "/previews"

func loadAuthConfig() *AuthConfig {
	if value := os.Getenv("LUMIC_AUTH_FILE"); value != "" {
		authFile = value
	}
	data, err := os.ReadFile(authFile)
	if err == nil {
		var config AuthConfig
		if json.Unmarshal(data, &config) == nil && config.Username != "" && config.PasswordHash != "" {
			return &config
		}
	}
	return &AuthConfig{Username: "Lumic", PasswordHash: hashPassword("Lumic", []byte("lumic-default-salt-v1"))}
}

func (a *AuthConfig) save() error {
	if err := os.MkdirAll(filepath.Dir(authFile), 0700); err != nil {
		return err
	}
	a.RLock()
	data, err := json.Marshal(struct {
		Username     string `json:"username"`
		PasswordHash string `json:"passwordHash"`
	}{a.Username, a.PasswordHash})
	a.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(authFile, data, 0600)
}

func loadOrCreateSecret() ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(secretFile), 0700); err != nil {
		return nil, err
	}
	if key, err := os.ReadFile(secretFile); err == nil && len(key) == 32 {
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(secretFile, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptJSON(key []byte, value any) ([]byte, error) {
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, []byte("lumic-bilibili-v1")), nil
}

func decryptJSON(key, encrypted []byte, value any) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(encrypted) < gcm.NonceSize() {
		return errors.New("invalid encrypted configuration")
	}
	nonce, ciphertext := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte("lumic-bilibili-v1"))
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, value)
}

func loadBilibiliStore() (*BilibiliStore, error) {
	if value := os.Getenv("LUMIC_BILIBILI_FILE"); value != "" {
		bilibiliFile = value
	}
	if value := os.Getenv("LUMIC_SECRET_FILE"); value != "" {
		secretFile = value
	}
	key, err := loadOrCreateSecret()
	if err != nil {
		return nil, err
	}
	store := &BilibiliStore{key: key, config: BilibiliConfig{Subscriptions: []SourceConfig{}, WeiboSubscriptions: []SourceConfig{}, PixivSubscriptions: []SourceConfig{}, TwitterSubscriptions: []SourceConfig{}}, bilibiliQR: make(map[string]BilibiliQRSession), bilibiliClients: make(map[string]*http.Client), weiboQR: make(map[string]WeiboQRSession), weiboClients: make(map[string]*http.Client)}
	if data, err := os.ReadFile(bilibiliFile); err == nil {
		if err := decryptJSON(key, data, &store.config); err != nil {
			return nil, fmt.Errorf("decrypt bilibili configuration: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if store.config.Subscriptions == nil {
		store.config.Subscriptions = []SourceConfig{}
	}
	for index := range store.config.Subscriptions {
		if isBilibiliFavoriteOpusFeed(store.config.Subscriptions[index]) {
			store.config.Subscriptions[index].ContentTypes = []string{"ARTICLE"}
		} else {
			store.config.Subscriptions[index].ContentTypes = normalizeBilibiliContentTypes(store.config.Subscriptions[index].ContentTypes, true)
		}
	}
	if store.config.WeiboSubscriptions == nil {
		store.config.WeiboSubscriptions = []SourceConfig{}
	}
	if store.config.PixivSubscriptions == nil {
		store.config.PixivSubscriptions = []SourceConfig{}
	}
	if store.config.TwitterSubscriptions == nil {
		store.config.TwitterSubscriptions = []SourceConfig{}
	}
	return store, nil
}

func initializeFlowStorage() error {
	if value := strings.TrimSpace(os.Getenv("LUMIC_FLOW_ROOT")); value != "" {
		flowRoot = value
	}
	if value := strings.TrimSpace(os.Getenv("LUMIC_PREVIEW_ROOT")); value != "" {
		previewRoot = value
	}
	for _, source := range []Source{SourceBilibili, SourcePixiv, SourceWeibo, SourceTwitter} {
		if err := os.MkdirAll(filepath.Join(flowRoot, string(source)), 0755); err != nil {
			return fmt.Errorf("create %s flow directory: %w", source, err)
		}
	}
	if err := os.MkdirAll(previewRoot, 0755); err != nil {
		return fmt.Errorf("create preview directory: %w", err)
	}
	legacyPreviewRoot, legacyErr := filepath.Abs(filepath.Join(flowRoot, ".previews"))
	currentPreviewRoot, currentErr := filepath.Abs(previewRoot)
	if legacyErr == nil && currentErr == nil && legacyPreviewRoot != currentPreviewRoot {
		if err := os.RemoveAll(legacyPreviewRoot); err != nil {
			return fmt.Errorf("remove legacy preview directory: %w", err)
		}
	}
	return nil
}

func safeFlowDirectoryName(name string) string {
	name = strings.TrimSpace(name)
	var builder strings.Builder
	for _, char := range name {
		if char < 32 || strings.ContainsRune(`<>:"/\\|?*`, char) {
			builder.WriteRune('_')
		} else {
			builder.WriteRune(char)
		}
	}
	name = strings.Trim(builder.String(), " .")
	if name == "" || name == "." || name == ".." {
		name = "unnamed"
	}
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true, "LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}
	if reserved[strings.ToUpper(name)] {
		name = "_" + name
	}
	return name
}

func sourceStoragePath(source Source, name string) string {
	return filepath.Join(flowRoot, string(source), safeFlowDirectoryName(name))
}

func canonicalSourceName(feed SourceConfig) string {
	if feed.Source == SourceWeibo && strings.HasPrefix(feed.ID, "weibo-likes-") {
		return weiboLikesName
	}
	if feed.Source == SourcePixiv && strings.HasPrefix(feed.ID, "pixiv-bookmarks-") {
		return pixivBookmarksName
	}
	if isBilibiliFavoriteOpusFeed(feed) {
		return bilibiliFavoriteOpusName
	}
	return feed.Name
}

func deleteSourceStorage(source Source, author string) error {
	root, err := filepath.Abs(flowRoot)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(sourceStoragePath(source, author))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("作者内容目录不在 flow 存储范围内")
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return nil
}

func flowPublicPath(source Source, author, name string) string {
	return "/flow/" + url.PathEscape(string(source)) + "/" + url.PathEscape(safeFlowDirectoryName(author)) + "/" + url.PathEscape(name)
}

// Cache account avatars locally so mobile WebViews do not depend on third-party
// avatar hosts, which may reject hotlinking or fail under their network policy.
func cacheAccountAvatar(source Source, storageName, avatar, proxyURL, referer, cookie string) string {
	avatar = normalizeRemoteImage(avatar)
	if avatar == "" || strings.HasPrefix(avatar, "/flow/") {
		return avatar
	}
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return avatar
	}
	directory := sourceStoragePath(source, storageName)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return avatar
	}
	path, err := downloadRemoteImage(client, avatar, filepath.Join(directory, "account-avatar"), referer, cookie)
	if err != nil {
		return avatar
	}
	return flowPublicPath(source, storageName, filepath.Base(path))
}

func prepareSourceStorage(feed SourceConfig) (SourceConfig, error) {
	feed.Name = canonicalSourceName(feed)
	feed.StoragePath = sourceStoragePath(feed.Source, feed.Name)
	if err := os.MkdirAll(feed.StoragePath, 0755); err != nil {
		return feed, err
	}
	metadata, err := json.MarshalIndent(struct {
		Source          Source   `json:"source"`
		ID              string   `json:"id"`
		Name            string   `json:"name"`
		Handle          string   `json:"handle"`
		Avatar          string   `json:"avatar,omitempty"`
		StartDate       string   `json:"startDate,omitempty"`
		ContentTypes    []string `json:"contentTypes,omitempty"`
		Tags            []string `json:"tags,omitempty"`
		OnlyWithImages  bool     `json:"onlyWithImages,omitempty"`
		IncludeVideos   bool     `json:"includeVideos,omitempty"`
		IncludeKeywords []string `json:"includeKeywords,omitempty"`
		ExcludeKeywords []string `json:"excludeKeywords,omitempty"`
	}{feed.Source, feed.ID, feed.Name, feed.Handle, feed.Avatar, feed.StartDate, feed.ContentTypes, feed.Tags, feed.OnlyWithImages, feed.IncludeVideos, feed.IncludeKeywords, feed.ExcludeKeywords}, "", "  ")
	if err != nil {
		return feed, err
	}
	temporary := filepath.Join(feed.StoragePath, ".source.json.tmp")
	if err := os.WriteFile(temporary, metadata, 0644); err != nil {
		return feed, err
	}
	if err := os.Rename(temporary, filepath.Join(feed.StoragePath, "source.json")); err != nil {
		_ = os.Remove(temporary)
		return feed, err
	}
	return feed, nil
}

func (b *BilibiliStore) reconcileFlowStorage() error {
	b.Lock()
	defer b.Unlock()
	lists := []*[]SourceConfig{&b.config.Subscriptions, &b.config.WeiboSubscriptions, &b.config.PixivSubscriptions, &b.config.TwitterSubscriptions}
	for _, list := range lists {
		for index, feed := range *list {
			oldStoragePath := feed.StoragePath
			oldName := feed.Name
			if oldStoragePath == "" {
				oldStoragePath = sourceStoragePath(feed.Source, oldName)
			}
			feed.Name = canonicalSourceName(feed)
			if oldStoragePath != "" && oldStoragePath != sourceStoragePath(feed.Source, feed.Name) {
				if err := migrateSourceStorage(oldStoragePath, sourceStoragePath(feed.Source, feed.Name)); err != nil {
					return fmt.Errorf("migrate flow storage for %s: %w", feed.Name, err)
				}
				if b.content != nil {
					if err := b.content.rewriteMediaPrefix(flowPublicPath(feed.Source, oldName, ""), flowPublicPath(feed.Source, feed.Name, "")); err != nil {
						return fmt.Errorf("rewrite flow paths for %s: %w", feed.Name, err)
					}
				}
			}
			prepared, err := prepareSourceStorage(feed)
			if err != nil {
				return fmt.Errorf("prepare flow storage for %s: %w", feed.Name, err)
			}
			(*list)[index] = prepared
			if b.content != nil && strings.HasPrefix(prepared.ID, "weibo-likes-") {
				if err := b.content.reconcileWeiboLikesPosts(prepared); err != nil {
					return fmt.Errorf("reconcile weibo likes posts: %w", err)
				}
			}
			if b.content != nil && isCollectionFeed(prepared) {
				if err := b.content.reconcileCollectionMedia(prepared, sourceConfigsFromConfig(b.config)); err != nil {
					return fmt.Errorf("reconcile collection media for %s: %w", prepared.Name, err)
				}
			}
		}
	}
	return nil
}

func migrateSourceStorage(from, to string) error {
	if from == "" || from == to {
		return nil
	}
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		source := filepath.Join(from, entry.Name())
		target := filepath.Join(to, entry.Name())
		if _, err := os.Stat(target); err == nil {
			target = availableMediaTargetBase(to, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))) + filepath.Ext(entry.Name())
		}
		if err := os.Rename(source, target); err != nil {
			return err
		}
	}
	return os.Remove(from)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lumic-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err == nil {
		return nil
	}
	// Windows 不允许 Rename 覆盖已有文件；数据已完整同步到临时文件后再替换。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryName, path)
}

func (b *BilibiliStore) save() error {
	b.RLock()
	data, err := encryptJSON(b.key, b.config)
	b.RUnlock()
	if err != nil {
		return err
	}
	return atomicWriteFile(bilibiliFile, data, 0600)
}

func configurationBackupHandler(auth *AuthConfig, platforms *BilibiliStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			auth.RLock()
			authBackup := AuthBackup{Username: auth.Username, PasswordHash: auth.PasswordHash}
			auth.RUnlock()
			platforms.RLock()
			platformBackup := platforms.config
			platforms.RUnlock()
			w.Header().Set("Content-Disposition", `attachment; filename="lumic-config-backup.json"`)
			writeJSON(w, ConfigurationBackup{Version: 1, ExportedAt: time.Now().UTC(), Auth: authBackup, Platforms: platformBackup})
		case http.MethodPut:
			var backup ConfigurationBackup
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
			if decoder.Decode(&backup) != nil || backup.Version != 1 || strings.TrimSpace(backup.Auth.Username) == "" || strings.TrimSpace(backup.Auth.PasswordHash) == "" {
				writeAPIError(w, http.StatusBadRequest, "备份文件无效或版本不受支持")
				return
			}
			if backup.Platforms.Subscriptions == nil {
				backup.Platforms.Subscriptions = []SourceConfig{}
			}
			if backup.Platforms.WeiboSubscriptions == nil {
				backup.Platforms.WeiboSubscriptions = []SourceConfig{}
			}
			if backup.Platforms.PixivSubscriptions == nil {
				backup.Platforms.PixivSubscriptions = []SourceConfig{}
			}
			if backup.Platforms.TwitterSubscriptions == nil {
				backup.Platforms.TwitterSubscriptions = []SourceConfig{}
			}
			auth.Lock()
			previousUsername, previousHash := auth.Username, auth.PasswordHash
			auth.Username, auth.PasswordHash = strings.TrimSpace(backup.Auth.Username), backup.Auth.PasswordHash
			auth.Unlock()
			platforms.Lock()
			previousPlatforms := platforms.config
			platforms.config = backup.Platforms
			platforms.Unlock()
			if err := auth.save(); err != nil {
				auth.Lock()
				auth.Username, auth.PasswordHash = previousUsername, previousHash
				auth.Unlock()
				platforms.Lock()
				platforms.config = previousPlatforms
				platforms.Unlock()
				writeAPIError(w, http.StatusInternalServerError, "无法恢复登录配置")
				return
			}
			if err := platforms.save(); err != nil {
				auth.Lock()
				auth.Username, auth.PasswordHash = previousUsername, previousHash
				auth.Unlock()
				platforms.Lock()
				platforms.config = previousPlatforms
				platforms.Unlock()
				_ = auth.save()
				writeAPIError(w, http.StatusInternalServerError, "无法恢复平台配置")
				return
			}
			writeJSON(w, map[string]string{"status": "restored", "message": "配置已恢复"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (s *SessionStore) create() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	s.Lock()
	if s.tokens == nil {
		s.tokens = make(map[string]time.Time)
	}
	now := time.Now()
	for existing, expiresAt := range s.tokens {
		if !expiresAt.After(now) {
			delete(s.tokens, existing)
		}
	}
	s.tokens[token] = now.Add(sessionLifetime)
	s.Unlock()
	return token, nil
}

func (s *SessionStore) valid(token string) bool {
	s.Lock()
	defer s.Unlock()
	expires, exists := s.tokens[token]
	if !exists {
		return false
	}
	if time.Now().After(expires) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *SessionStore) revoke(token string) {
	s.Lock()
	delete(s.tokens, token)
	s.Unlock()
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "lumic_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("LUMIC_COOKIE_SECURE") == "true", MaxAge: -1, Expires: time.Unix(1, 0)})
}

func hashPassword(password string, salt []byte) string {
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(hash)
}

func passwordMatches(password, encoded string) bool {
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return false
	}
	salt := []byte("lumic-default-salt-v1")
	actual := hashPassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(encoded)) == 1
}

func loginHandler(sessions *SessionStore, auth *AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		auth.RLock()
		username, storedHash := auth.Username, auth.PasswordHash
		auth.RUnlock()
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || subtle.ConstantTimeCompare([]byte(input.Username), []byte(username)) != 1 || !passwordMatches(input.Password, storedHash) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		token, err := sessions.create()
		if err != nil {
			http.Error(w, "unable to create session", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "lumic_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("LUMIC_COOKIE_SECURE") == "true", MaxAge: int(sessionLifetime.Seconds()), Expires: time.Now().Add(sessionLifetime)})
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func sessionHandler(sessions *SessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := requestSessionToken(r)
		if token == "" || !sessions.valid(token) {
			clearSessionCookie(w)
			writeJSON(w, map[string]bool{"authenticated": false})
			return
		}
		writeJSON(w, map[string]bool{"authenticated": true})
	}
}

func logoutHandler(sessions *SessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if token := requestSessionToken(r); token != "" {
			sessions.revoke(token)
		}
		clearSessionCookie(w)
		writeJSON(w, map[string]string{"status": "logged_out"})
	}
}

func settingsHandler(auth *AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			auth.RLock()
			writeJSON(w, map[string]string{"username": auth.Username})
			auth.RUnlock()
			return
		}
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Username    string `json:"username"`
			NewPassword string `json:"newPassword"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || len(input.Username) < 3 || len(input.NewPassword) < 8 {
			http.Error(w, "invalid settings", http.StatusBadRequest)
			return
		}
		auth.Lock()
		auth.Username = input.Username
		auth.PasswordHash = hashPassword(input.NewPassword, []byte("lumic-default-salt-v1"))
		auth.Unlock()
		if err := auth.save(); err != nil {
			http.Error(w, "unable to save settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func passwordHash() string {
	password := os.Getenv("LUMIC_PASSWORD")
	if password == "" {
		password = "Lumic"
	}
	return hashPassword(password, []byte("lumic-default-salt-v1"))
}

func authMiddleware(sessions *SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		token := requestSessionToken(r)
		if token == "" || !sessions.valid(token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func demoStore() *Store {
	now := time.Now()
	return &Store{
		posts: []Post{
			{ID: "wb-001", Source: SourceWeibo, Author: "林间拾光", Avatar: "https://i.pravatar.cc/96?img=47", Caption: "把黄昏收藏进今天的相册里。风经过树梢，带来一点夏天的回声。", Tags: []string{"日常", "摄影"}, Published: now.Add(-42 * time.Minute), Liked: true},
			{ID: "px-002", Source: SourcePixiv, Author: "Aoi Sora", Avatar: "https://i.pravatar.cc/96?img=32", Caption: "新作｜雨后的玻璃温室，想画出潮湿空气里柔软的光。", Tags: []string{"原创", "插画", "光影"}, Published: now.Add(-2 * time.Hour), Media: []string{"https://images.unsplash.com/photo-1518709268805-4e9042af9f23?w=900&q=80"}},
			{ID: "bl-003", Source: SourceBilibili, Author: "慢慢生活研究所", Avatar: "https://i.pravatar.cc/96?img=12", Caption: "一期关于城市散步的记录，和你分享最近发现的三家小店。", Tags: []string{"VLOG", "城市漫游"}, Published: now.Add(-5 * time.Hour)},
			{ID: "wb-004", Source: SourceWeibo, Author: "山茶花开时", Avatar: "https://i.pravatar.cc/96?img=5", Caption: "今日份蓝色时刻。愿我们都能留住一些不被打扰的浪漫。", Tags: []string{"随手拍", "生活"}, Published: now.Add(-8 * time.Hour), Liked: true},
			{ID: "px-005", Source: SourcePixiv, Author: "Mori", Avatar: "https://i.pravatar.cc/96?img=20", Caption: "夏日习作 #sketch #summer", Tags: []string{"sketch", "summer"}, Published: now.Add(-24 * time.Hour)},
		},
		feeds: []SourceConfig{
			{ID: "feed-2", Source: SourceWeibo, Name: "建筑可视化研究所", Handle: "@arch_visual", Enabled: true, Schedule: "每天 09:00", LastSyncedAt: now.Add(-3 * time.Hour)},
			{ID: "feed-3", Source: SourcePixiv, Name: "Aoi Sora", Handle: "@aoisora", Enabled: true, Schedule: "每 12 小时", LastSyncedAt: now.Add(-2 * time.Hour)},
			{ID: "feed-4", Source: SourceBilibili, Name: "慢慢生活研究所", Handle: "UID 184729", Enabled: false, Schedule: "每天 20:00", LastSyncedAt: now.Add(-24 * time.Hour)},
		},
	}
}

func emptyStore() *Store {
	return &Store{posts: []Post{}, feeds: []SourceConfig{}}
}

func loadStore() (*Store, error) {
	if value := strings.TrimSpace(os.Getenv("LUMIC_CONTENT_FILE")); value != "" {
		contentFile = value
	}
	return loadStoreFile(contentFile)
}

func loadStoreFile(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var content ContentData
		if err := json.Unmarshal(data, &content); err != nil {
			return nil, fmt.Errorf("decode content data: %w", err)
		}
		if content.Posts == nil {
			content.Posts = []Post{}
		}
		if content.Feeds == nil {
			content.Feeds = []SourceConfig{}
		}
		store := &Store{posts: content.Posts, feeds: content.Feeds, file: path}
		changed, cleanupErr := store.removeLegacyEmojiImages()
		if cleanupErr != nil {
			return nil, fmt.Errorf("remove legacy emoji images: %w", cleanupErr)
		}
		videoChanged, videoErr := store.normalizeStoredPostVideos()
		if videoErr != nil {
			return nil, fmt.Errorf("normalize stored post videos: %w", videoErr)
		}
		changed = changed || videoChanged
		textChanged, textErr := store.reconcileTextArchives()
		if textErr != nil {
			return nil, fmt.Errorf("archive post text: %w", textErr)
		}
		changed = changed || textChanged
		if changed {
			store.Lock()
			cleanupErr = store.saveLocked()
			store.Unlock()
			if cleanupErr != nil {
				return nil, fmt.Errorf("persist text-only emoji migration: %w", cleanupErr)
			}
		}
		return store, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	store := emptyStore()
	store.file = path
	store.Lock()
	err = store.saveLocked()
	store.Unlock()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) removeLegacyEmojiImages() (bool, error) {
	changed := false
	for index := range s.posts {
		if len(s.posts[index].Emojis) == 0 {
			continue
		}
		for _, emoji := range s.posts[index].Emojis {
			if err := deletePostMedia([]string{emoji.URL}); err != nil {
				return false, err
			}
		}
		s.posts[index].Emojis = nil
		changed = true
	}
	return changed, nil
}

func videoQualityScore(value string) int {
	lower := strings.ToLower(value)
	score := 0
	for _, quality := range []struct {
		label  string
		weight int
	}{{"2160", 600}, {"1440", 500}, {"1080", 400}, {"720", 300}, {"480", 200}, {"360", 100}} {
		if strings.Contains(lower, quality.label) {
			score = quality.weight
			break
		}
	}
	if strings.Contains(lower, "_hd") || strings.Contains(lower, "-hd") || strings.Contains(lower, "high") {
		score += 40
	}
	if strings.Contains(lower, ".mp4") {
		score += 10
	}
	return score
}

func normalizePostVideos(videos []PostVideo) []PostVideo {
	bestIndex := -1
	bestScore := -1
	poster := ""
	seen := make(map[string]bool, len(videos))
	for index, video := range videos {
		video.URL = strings.TrimSpace(video.URL)
		video.Poster = strings.TrimSpace(video.Poster)
		if video.URL == "" || seen[video.URL] {
			continue
		}
		seen[video.URL] = true
		if poster == "" && video.Poster != "" {
			poster = video.Poster
		}
		score := videoQualityScore(video.URL)
		if bestIndex < 0 || score > bestScore {
			bestIndex = index
			bestScore = score
		}
	}
	if bestIndex < 0 {
		return nil
	}
	selected := videos[bestIndex]
	selected.URL = strings.TrimSpace(selected.URL)
	selected.Poster = strings.TrimSpace(selected.Poster)
	if selected.Poster == "" {
		selected.Poster = poster
	}
	return []PostVideo{selected}
}

func (s *Store) normalizeStoredPostVideos() (bool, error) {
	changed := false
	for index := range s.posts {
		videos := s.posts[index].Videos
		normalized := normalizePostVideos(videos)
		if len(videos) == len(normalized) {
			if len(videos) == 0 || (videos[0].URL == normalized[0].URL && videos[0].Poster == normalized[0].Poster) {
				continue
			}
		}
		retained := make(map[string]bool, 2)
		for _, video := range normalized {
			retained[video.URL] = true
			retained[video.Poster] = true
		}
		removed := make([]string, 0, len(videos)*2)
		for _, video := range videos {
			if video.URL != "" && !retained[video.URL] {
				removed = append(removed, video.URL)
			}
			if video.Poster != "" && !retained[video.Poster] {
				removed = append(removed, video.Poster)
			}
		}
		if err := deletePostMedia(removed); err != nil {
			return false, err
		}
		s.posts[index].Videos = normalized
		changed = true
	}
	return changed, nil
}

func (s *Store) reconcileTextArchives() (bool, error) {
	return s.reconcileTextArchivesFor(nil)
}

func (s *Store) saveLocked() error {
	if s.file == "" {
		return nil
	}
	data, err := json.Marshal(ContentData{Posts: s.posts, Feeds: s.feeds})
	if err != nil {
		return err
	}
	return atomicWriteFile(s.file, data, 0600)
}

func (s *Store) mergePosts(incoming []Post) (int, error) {
	if len(incoming) == 0 {
		return 0, nil
	}
	s.Lock()
	defer s.Unlock()
	indexes := make(map[string]int, len(s.posts)+len(incoming))
	for index, post := range s.posts {
		indexes[post.ID] = index
	}
	previous := append([]Post(nil), s.posts...)
	affectedArchives := make(map[textArchiveKey]bool)
	added, changed := 0, false
	for _, post := range incoming {
		post.Videos = normalizePostVideos(post.Videos)
		if post.ID == "" {
			continue
		}
		affectedArchives[textArchiveKeyForPost(post)] = true
		post.Emojis = nil
		if index, exists := indexes[post.ID]; exists {
			stored := s.posts[index]
			affectedArchives[textArchiveKeyForPost(stored)] = true
			if post.TextFile == "" {
				post.TextFile = stored.TextFile
			}
			post.FeedIDs = mergeUniqueStrings(stored.FeedIDs, post.FeedIDs)
			post.Tags = mergeUniqueStrings(stored.Tags, post.Tags)
			post.FavoriteExplicit = stored.FavoriteExplicit
			if hasWeiboLikesFeed(post.FeedIDs) && !stored.FavoriteExplicit {
				post.Liked = false
			} else {
				post.Liked = stored.Liked
			}
			if !postsEqual(stored, post) {
				s.posts[index] = post
				changed = true
			}
			continue
		}
		indexes[post.ID] = len(s.posts)
		s.posts = append(s.posts, post)
		added++
		changed = true
	}
	if !changed {
		return 0, nil
	}
	sort.SliceStable(s.posts, func(i, j int) bool { return s.posts[i].Published.After(s.posts[j].Published) })
	if _, err := s.reconcileTextArchivesFor(affectedArchives); err != nil {
		s.posts = previous
		return 0, err
	}
	if err := s.saveLocked(); err != nil {
		s.posts = previous
		return 0, err
	}
	return added, nil
}

func (s *Store) rewriteMediaPrefix(oldPrefix, newPrefix string) error {
	if oldPrefix == newPrefix {
		return nil
	}
	s.Lock()
	defer s.Unlock()
	changed := false
	for postIndex := range s.posts {
		for mediaIndex, media := range s.posts[postIndex].Media {
			if strings.HasPrefix(media, oldPrefix) {
				s.posts[postIndex].Media[mediaIndex] = newPrefix + strings.TrimPrefix(media, oldPrefix)
				changed = true
			}
		}
		for videoIndex := range s.posts[postIndex].Videos {
			if strings.HasPrefix(s.posts[postIndex].Videos[videoIndex].URL, oldPrefix) {
				s.posts[postIndex].Videos[videoIndex].URL = newPrefix + strings.TrimPrefix(s.posts[postIndex].Videos[videoIndex].URL, oldPrefix)
				changed = true
			}
			if strings.HasPrefix(s.posts[postIndex].Videos[videoIndex].Poster, oldPrefix) {
				s.posts[postIndex].Videos[videoIndex].Poster = newPrefix + strings.TrimPrefix(s.posts[postIndex].Videos[videoIndex].Poster, oldPrefix)
				changed = true
			}
		}
		if strings.HasPrefix(s.posts[postIndex].TextFile, oldPrefix) {
			s.posts[postIndex].TextFile = newPrefix + strings.TrimPrefix(s.posts[postIndex].TextFile, oldPrefix)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) reconcileCollectionMedia(feed SourceConfig, configuredFeeds []SourceConfig) error {
	if !isCollectionFeed(feed) {
		return nil
	}
	s.Lock()
	defer s.Unlock()
	previous := append([]Post(nil), s.posts...)
	changed := false
	type mediaMove struct {
		postIndex  int
		mediaIndex int
		mediaKind  string
		oldPublic  string
		oldLocal   string
		newLocal   string
	}
	moves := make([]mediaMove, 0)
	rollback := func() {
		for index := len(moves) - 1; index >= 0; index-- {
			move := moves[index]
			_ = os.MkdirAll(filepath.Dir(move.oldLocal), 0755)
			_ = os.Rename(move.newLocal, move.oldLocal)
			switch move.mediaKind {
			case "video":
				s.posts[move.postIndex].Videos[move.mediaIndex].URL = move.oldPublic
			case "poster":
				s.posts[move.postIndex].Videos[move.mediaIndex].Poster = move.oldPublic
			default:
				s.posts[move.postIndex].Media[move.mediaIndex] = move.oldPublic
			}
		}
	}
	moveMedia := func(postIndex, mediaIndex int, mediaKind, media, targetName string) error {
		localPath, ok := localFlowArchivePath(media)
		if !ok {
			return nil
		}
		info, statErr := os.Stat(localPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return nil
			}
			return statErr
		}
		if info.IsDir() {
			return nil
		}
		targetDirectory := sourceStoragePath(feed.Source, targetName)
		if err := os.MkdirAll(targetDirectory, 0755); err != nil {
			return err
		}
		targetDirectoryAbsolute, err := filepath.Abs(targetDirectory)
		if err != nil {
			return err
		}
		currentDirectory, pathErr := filepath.Abs(filepath.Dir(localPath))
		if pathErr != nil {
			return pathErr
		}
		if strings.EqualFold(currentDirectory, targetDirectoryAbsolute) {
			return nil
		}
		extension := filepath.Ext(localPath)
		base := strings.TrimSuffix(filepath.Base(localPath), extension)
		targetBase := availableMediaTargetBase(targetDirectory, base)
		targetPath := targetBase + extension
		if err := os.Rename(localPath, targetPath); err != nil {
			return err
		}
		updated := flowPublicPath(feed.Source, targetName, filepath.Base(targetPath))
		moves = append(moves, mediaMove{postIndex: postIndex, mediaIndex: mediaIndex, mediaKind: mediaKind, oldPublic: media, oldLocal: localPath, newLocal: targetPath})
		changed = true
		switch mediaKind {
		case "video":
			s.posts[postIndex].Videos[mediaIndex].URL = updated
		case "poster":
			s.posts[postIndex].Videos[mediaIndex].Poster = updated
		default:
			s.posts[postIndex].Media[mediaIndex] = updated
		}
		return deletePostMedia([]string{media})
	}
	for postIndex := range s.posts {
		if !containsString(s.posts[postIndex].FeedIDs, feed.ID) {
			continue
		}
		targetName := canonicalSourceName(feed)
		if authorFeed, ok := collectionAuthorSubscription(s.posts[postIndex], configuredFeeds); ok {
			targetName = authorFeed.Name
			updatedFeedIDs := mergeUniqueStrings(s.posts[postIndex].FeedIDs, []string{authorFeed.ID})
			if !stringSlicesEqual(s.posts[postIndex].FeedIDs, updatedFeedIDs) {
				s.posts[postIndex].FeedIDs = updatedFeedIDs
				changed = true
			}
		}
		for mediaIndex, media := range s.posts[postIndex].Media {
			if err := moveMedia(postIndex, mediaIndex, "image", media, targetName); err != nil {
				rollback()
				s.posts = previous
				return err
			}
		}
		for videoIndex, video := range s.posts[postIndex].Videos {
			if err := moveMedia(postIndex, videoIndex, "video", video.URL, targetName); err != nil {
				rollback()
				s.posts = previous
				return err
			}
			if err := moveMedia(postIndex, videoIndex, "poster", video.Poster, targetName); err != nil {
				rollback()
				s.posts = previous
				return err
			}
		}
	}
	if len(moves) == 0 && !changed {
		return nil
	}
	textChanged, err := s.reconcileTextArchivesFor(nil)
	if err != nil {
		rollback()
		s.posts = previous
		return err
	}
	changed = changed || textChanged
	if !changed {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		rollback()
		s.posts = previous
		return err
	}
	return nil
}

func (s *Store) reconcileWeiboLikesPosts(feed SourceConfig) error {
	s.Lock()
	defer s.Unlock()
	prefix := flowPublicPath(SourceWeibo, canonicalSourceName(feed), "")
	previous := append([]Post(nil), s.posts...)
	changed := false
	for index := range s.posts {
		post := &s.posts[index]
		belongsToSource := containsString(post.FeedIDs, feed.ID)
		if !belongsToSource && strings.HasPrefix(post.TextFile, prefix) {
			belongsToSource = true
		}
		if !belongsToSource && postHasMediaPrefix(post, prefix) {
			belongsToSource = true
		}
		if !belongsToSource {
			continue
		}
		updatedFeedIDs := mergeUniqueStrings(post.FeedIDs, []string{feed.ID})
		if !stringSlicesEqual(post.FeedIDs, updatedFeedIDs) {
			post.FeedIDs = updatedFeedIDs
			changed = true
		}
		if post.Liked && !post.FavoriteExplicit {
			post.Liked = false
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		s.posts = previous
		return err
	}
	return nil
}

func (s *Store) setPostLiked(id string, liked bool) (Post, error) {
	s.Lock()
	defer s.Unlock()
	for index := range s.posts {
		if s.posts[index].ID != id {
			continue
		}
		previous := s.posts[index].Liked
		previousExplicit := s.posts[index].FavoriteExplicit
		s.posts[index].Liked = liked
		s.posts[index].FavoriteExplicit = true
		if err := s.saveLocked(); err != nil {
			s.posts[index].Liked = previous
			s.posts[index].FavoriteExplicit = previousExplicit
			return Post{}, err
		}
		return s.posts[index], nil
	}
	return Post{}, os.ErrNotExist
}

func (s *Store) setPostsLiked(ids []string, liked bool) (int, error) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			idSet[id] = true
		}
	}
	if len(idSet) == 0 {
		return 0, errors.New("至少选择一条动态")
	}
	s.Lock()
	defer s.Unlock()
	previous := append([]Post(nil), s.posts...)
	updated := 0
	for index := range s.posts {
		if !idSet[s.posts[index].ID] {
			continue
		}
		s.posts[index].Liked = liked
		s.posts[index].FavoriteExplicit = true
		updated++
	}
	if updated == 0 {
		return 0, os.ErrNotExist
	}
	if err := s.saveLocked(); err != nil {
		s.posts = previous
		return 0, err
	}
	return updated, nil
}

func (s *Store) setSourceTags(feed SourceConfig, feeds []SourceConfig) error {
	s.Lock()
	defer s.Unlock()
	previous := append([]Post(nil), s.posts...)
	tagsByFeed := make(map[string][]string, len(feeds))
	for _, configured := range feeds {
		tagsByFeed[configured.ID] = append([]string(nil), configured.Tags...)
	}
	changed := false
	for index := range s.posts {
		post := &s.posts[index]
		hasFeedID := containsString(post.FeedIDs, feed.ID)
		belongsToSource := postBelongsToSource(post, feed)
		if !hasFeedID && belongsToSource {
			post.FeedIDs = mergeUniqueStrings(post.FeedIDs, []string{feed.ID})
			changed = true
		}
		if !belongsToSource {
			continue
		}
		updatedTags := make([]string, 0)
		for _, feedID := range post.FeedIDs {
			updatedTags = mergeUniqueStrings(updatedTags, tagsByFeed[feedID])
		}
		if !stringSlicesEqual(post.Tags, updatedTags) {
			post.Tags = updatedTags
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		s.posts = previous
		return err
	}
	return nil
}

func sameSourceAuthor(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && (left == right || strings.EqualFold(left, right) || safeFlowDirectoryName(left) == safeFlowDirectoryName(right))
}

func postMediaPaths(post Post) []string {
	paths := append([]string(nil), post.Media...)
	for _, video := range post.Videos {
		paths = append(paths, video.URL, video.Poster)
	}
	return paths
}

func postHasMediaPrefix(post *Post, prefix string) bool {
	if post == nil {
		return false
	}
	for _, media := range postMediaPaths(*post) {
		if strings.HasPrefix(media, prefix) {
			return true
		}
	}
	return false
}

func postBelongsToSource(post *Post, feed SourceConfig) bool {
	if post == nil {
		return false
	}
	if containsString(post.FeedIDs, feed.ID) {
		return true
	}
	return postBelongsToSourcePath(post, feed)
}

func postBelongsToSourcePath(post *Post, feed SourceConfig) bool {
	if post == nil {
		return false
	}
	if strings.HasPrefix(feed.ID, "weibo-likes-") {
		prefix := flowPublicPath(SourceWeibo, canonicalSourceName(feed), "")
		if strings.HasPrefix(post.TextFile, prefix) {
			return true
		}
		return postHasMediaPrefix(post, prefix)
	}
	if post.Source != feed.Source {
		return false
	}
	if sameSourceAuthor(post.Author, feed.Name) {
		return true
	}
	prefix := flowPublicPath(feed.Source, feed.Name, "")
	if strings.HasPrefix(post.TextFile, prefix) {
		return true
	}
	return postHasMediaPrefix(post, prefix)
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *Store) deletePosts(ids []string, source Source, author string) (int, error) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			idSet[id] = true
		}
	}
	author = strings.TrimSpace(author)
	if len(idSet) == 0 && author == "" {
		return 0, errors.New("未指定要删除的动态")
	}
	s.Lock()
	defer s.Unlock()
	kept := make([]Post, 0, len(s.posts))
	removed := make([]Post, 0)
	for _, post := range s.posts {
		matched := idSet[post.ID] || (author != "" && post.Source == source && post.Author == author)
		if matched {
			removed = append(removed, post)
			continue
		}
		kept = append(kept, post)
	}
	if len(removed) == 0 {
		if author != "" {
			if err := deleteSourceStorage(source, author); err != nil {
				return 0, err
			}
			return 0, nil
		}
		return 0, os.ErrNotExist
	}
	affectedArchives := make(map[textArchiveKey]bool)
	for _, post := range removed {
		media := postMediaPaths(post)
		affectedArchives[textArchiveKeyForPost(post)] = true
		for _, emoji := range post.Emojis {
			media = append(media, emoji.URL)
		}
		if err := deletePostMedia(media); err != nil {
			return 0, err
		}
	}
	previous := s.posts
	s.posts = kept
	if author == "" {
		if _, err := s.reconcileTextArchivesFor(affectedArchives); err != nil {
			s.posts = previous
			return 0, err
		}
	}
	if err := s.saveLocked(); err != nil {
		s.posts = previous
		return 0, err
	}
	if author != "" {
		if err := deleteSourceStorage(source, author); err != nil {
			return len(removed), err
		}
	}
	return len(removed), nil
}

func postsEqual(left, right Post) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func deletePostMedia(media []string) error {
	root, err := filepath.Abs(flowRoot)
	if err != nil {
		return err
	}
	for _, raw := range media {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		parsed, parseErr := url.Parse(value)
		if parseErr == nil && parsed.Scheme != "" && parsed.Scheme != "file" {
			continue
		}
		candidate := value
		if parseErr == nil {
			candidate = parsed.Path
			if parsed.Scheme == "file" && parsed.Host != "" {
				candidate = "//" + parsed.Host + parsed.Path
			}
		}
		if strings.HasPrefix(filepath.ToSlash(candidate), "/flow/") {
			candidate = filepath.Join(flowRoot, strings.TrimPrefix(filepath.ToSlash(candidate), "/flow/"))
		}
		absolute, err := filepath.Abs(filepath.FromSlash(candidate))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		previewPaths := mediaPreviewCachePaths(root, absolute)
		if info, err := os.Stat(absolute); err == nil {
			if info.IsDir() {
				continue
			}
			if err := os.Remove(absolute); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		for _, previewPath := range previewPaths {
			if _, _, err := removeMediaPreviewCacheFile(previewPath); err != nil {
				return err
			}
		}
	}
	return nil
}

const (
	mediaPreviewMaxDimension       = 900
	mediaPreviewJPEGQuality        = 88
	mediaPreviewCacheSuffix        = ".v4.jpg"
	mediaPreviewMobileMaxDimension = 720
	mediaPreviewMobileJPEGQuality  = 80
	mediaPreviewMobileCacheSuffix  = ".mobile.v1.jpg"
	mediaPreviewMaxAge             = 30 * 24 * time.Hour
	mediaPreviewTouchInterval      = 24 * time.Hour
	mediaPreviewCleanupPeriod      = 12 * time.Hour
	mediaPreviewTemporaryMaxAge    = time.Hour
)

type mediaPreviewLock struct {
	mutex      sync.Mutex
	references int
}

var mediaPreviewLocks = struct {
	sync.Mutex
	entries map[string]*mediaPreviewLock
}{entries: make(map[string]*mediaPreviewLock)}
var mediaPreviewGenerationSlots = make(chan struct{}, 2)
var mediaPreviewQualitySuffixPattern = regexp.MustCompile(`\.q2\.(?:desktop|mobile)\.[1-4]\.jpg$`)

func acquireMediaPreviewLock(previewPath string) func() {
	mediaPreviewLocks.Lock()
	entry := mediaPreviewLocks.entries[previewPath]
	if entry == nil {
		entry = &mediaPreviewLock{}
		mediaPreviewLocks.entries[previewPath] = entry
	}
	entry.references++
	mediaPreviewLocks.Unlock()

	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		mediaPreviewLocks.Lock()
		entry.references--
		if entry.references == 0 && mediaPreviewLocks.entries[previewPath] == entry {
			delete(mediaPreviewLocks.entries, previewPath)
		}
		mediaPreviewLocks.Unlock()
	}
}

func mediaPreviewCachePath(root, source string) (string, bool) {
	return mediaPreviewCachePathWithSuffix(root, source, mediaPreviewCacheSuffix)
}

func mediaPreviewMobileCachePath(root, source string) (string, bool) {
	return mediaPreviewCachePathWithSuffix(root, source, mediaPreviewMobileCacheSuffix)
}

func mediaPreviewCachePathWithSuffix(root, source, suffix string) (string, bool) {
	relative, err := filepath.Rel(root, source)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	previewBase, err := filepath.Abs(previewRoot)
	if err != nil {
		return "", false
	}
	return filepath.Join(previewBase, relative) + suffix, true
}

func obsoleteMediaPreviewCachePaths(root, source string) []string {
	relative, err := filepath.Rel(root, source)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	previewBase, err := filepath.Abs(previewRoot)
	if err != nil {
		return nil
	}
	base := filepath.Join(previewBase, relative)
	return []string{base + ".v3.jpg", base + ".v2.jpg", base + ".jpg"}
}

func mediaPreviewCachePaths(root, source string) []string {
	paths := obsoleteMediaPreviewCachePaths(root, source)
	for _, suffix := range []string{mediaPreviewCacheSuffix, mediaPreviewMobileCacheSuffix} {
		if path, ok := mediaPreviewCachePathWithSuffix(root, source, suffix); ok {
			paths = append(paths, path)
		}
	}
	for _, device := range []string{"desktop", "mobile"} {
		for level := 1; level <= 4; level++ {
			if path, ok := mediaPreviewCachePathWithSuffix(root, source, fmt.Sprintf(".q2.%s.%d.jpg", device, level)); ok {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func resizeMediaPreview(source image.Image, width, height int) *image.RGBA {
	preview := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{R: 242, G: 245, B: 242, A: 255}
	draw.Draw(preview, preview.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
	xdraw.ApproxBiLinear.Scale(preview, preview.Bounds(), source, source.Bounds(), draw.Over, nil)
	return preview
}

func generateMediaPreview(sourcePath, previewPath string, maxDimension, jpegQuality int) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	decoded, _, err := image.Decode(sourceFile)
	if err != nil {
		return err
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return errors.New("invalid image dimensions")
	}
	if width > maxDimension || height > maxDimension {
		if width >= height {
			height = max(1, height*maxDimension/width)
			width = maxDimension
		} else {
			width = max(1, width*maxDimension/height)
			height = maxDimension
		}
	}
	if err := os.MkdirAll(filepath.Dir(previewPath), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(previewPath), ".lumic-preview-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := jpeg.Encode(temporary, resizeMediaPreview(decoded, width, height), &jpeg.Options{Quality: jpegQuality}); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replaceFile(temporaryName, previewPath)
}

func removeMediaPreviewCacheFile(previewPath string) (int64, bool, error) {
	release := acquireMediaPreviewLock(previewPath)
	defer release()
	info, err := os.Stat(previewPath)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if info.IsDir() {
		return 0, false, nil
	}
	if err := os.Remove(previewPath); err != nil && !os.IsNotExist(err) {
		return 0, false, err
	}
	return info.Size(), true, nil
}

func mediaPreviewSourceRelative(relative string) (string, bool) {
	for _, suffix := range []string{mediaPreviewCacheSuffix, mediaPreviewMobileCacheSuffix} {
		if strings.HasSuffix(relative, suffix) {
			return strings.TrimSuffix(relative, suffix), true
		}
	}
	if match := mediaPreviewQualitySuffixPattern.FindString(relative); match != "" {
		return strings.TrimSuffix(relative, match), true
	}
	return "", false
}

func cleanupMediaPreviewCache(now time.Time) (int, int64, error) {
	previewBase, err := filepath.Abs(previewRoot)
	if err != nil {
		return 0, 0, err
	}
	flowBase, err := filepath.Abs(flowRoot)
	if err != nil {
		return 0, 0, err
	}
	if err := os.MkdirAll(previewBase, 0755); err != nil {
		return 0, 0, err
	}
	directories := make([]string, 0)
	removed := 0
	var reclaimed int64
	var firstErr error
	err = filepath.Walk(previewBase, func(currentPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if firstErr == nil {
				firstErr = walkErr
			}
			return nil
		}
		if info.IsDir() {
			if currentPath != previewBase {
				directories = append(directories, currentPath)
			}
			return nil
		}
		relative, relativeErr := filepath.Rel(previewBase, currentPath)
		if relativeErr != nil {
			if firstErr == nil {
				firstErr = relativeErr
			}
			return nil
		}
		name := info.Name()
		remove := false
		switch {
		case strings.HasPrefix(name, ".lumic-preview-") && strings.HasSuffix(name, ".tmp"):
			remove = now.Sub(info.ModTime()) >= mediaPreviewTemporaryMaxAge
		default:
			sourceRelative, supported := mediaPreviewSourceRelative(relative)
			if !supported {
				remove = true
				break
			}
			sourcePath := filepath.Join(flowBase, sourceRelative)
			sourceInfo, sourceErr := os.Stat(sourcePath)
			if os.IsNotExist(sourceErr) || (sourceErr == nil && sourceInfo.IsDir()) {
				remove = true
			} else if sourceErr != nil {
				if firstErr == nil {
					firstErr = sourceErr
				}
				return nil
			} else if now.Sub(info.ModTime()) >= mediaPreviewMaxAge {
				remove = true
			}
		}
		if !remove {
			return nil
		}
		size, didRemove, removeErr := removeMediaPreviewCacheFile(currentPath)
		if removeErr != nil {
			if firstErr == nil {
				firstErr = removeErr
			}
			return nil
		}
		if didRemove {
			removed++
			reclaimed += size
		}
		return nil
	})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		_ = os.Remove(directories[index])
	}
	return removed, reclaimed, firstErr
}

func startMediaPreviewCleanup() {
	run := func() {
		removed, reclaimed, err := cleanupMediaPreviewCache(time.Now())
		if err != nil {
			log.Printf("preview cache cleanup failed: %v", err)
			return
		}
		if removed > 0 {
			log.Printf("preview cache cleanup removed %d files and reclaimed %.1f MiB", removed, float64(reclaimed)/(1024*1024))
		}
	}
	run()
	go func() {
		ticker := time.NewTicker(mediaPreviewCleanupPeriod)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}

func mediaPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := filepath.Abs(flowRoot)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	relative := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/preview/")))
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	sourcePath, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	device := strings.TrimSpace(r.URL.Query().Get("device"))
	legacyMobile := device == "" && r.URL.Query().Get("mobile") == "1"
	rawLevel := strings.TrimSpace(r.URL.Query().Get("level"))
	profile := mediaPreviewProfile{}
	if rawLevel == "" {
		if legacyMobile {
			profile = mediaPreviewProfile{Level: 3, MaxDimension: mediaPreviewMobileMaxDimension, JPEGQuality: mediaPreviewMobileJPEGQuality, Suffix: mediaPreviewMobileCacheSuffix}
		} else {
			profile = mediaPreviewProfile{Level: 2, MaxDimension: mediaPreviewMaxDimension, JPEGQuality: mediaPreviewJPEGQuality, Suffix: mediaPreviewCacheSuffix}
		}
	} else {
		level, _ := strconv.Atoi(rawLevel)
		profile = mediaPreviewProfileFor(level, device, strings.TrimSpace(r.URL.Query().Get("network")))
	}
	if profile.Level == 0 {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, sourcePath)
		return
	}
	previewPath, ok := mediaPreviewCachePathWithSuffix(root, sourcePath, profile.Suffix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil || sourceInfo.IsDir() {
		http.NotFound(w, r)
		return
	}
	release := acquireMediaPreviewLock(previewPath)
	defer release()
	previewInfo, previewErr := os.Stat(previewPath)
	if previewErr != nil || previewInfo.Size() == 0 || previewInfo.ModTime().Before(sourceInfo.ModTime()) {
		mediaPreviewGenerationSlots <- struct{}{}
		generationErr := func() error {
			defer func() { <-mediaPreviewGenerationSlots }()
			return generateMediaPreview(sourcePath, previewPath, profile.MaxDimension, profile.JPEGQuality)
		}()
		if generationErr != nil {
			w.Header().Set("Cache-Control", "public, max-age=3600")
			http.ServeFile(w, r, sourcePath)
			return
		}
		for _, obsoletePreview := range obsoleteMediaPreviewCachePaths(root, sourcePath) {
			_ = os.Remove(obsoletePreview)
		}
	} else if time.Since(previewInfo.ModTime()) >= mediaPreviewTouchInterval {
		now := time.Now()
		_ = os.Chtimes(previewPath, now, now)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, previewPath)
}

func (s *Store) postsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPatch {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		var input struct {
			IDs   []string `json:"ids"`
			Liked *bool    `json:"liked"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil || input.Liked == nil {
			writeAPIError(w, http.StatusBadRequest, "请提供有效的收藏状态")
			return
		}
		if id == "" {
			count, err := s.setPostsLiked(input.IDs, *input.Liked)
			if errors.Is(err, os.ErrNotExist) {
				writeAPIError(w, http.StatusNotFound, "动态不存在或已删除")
				return
			}
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, map[string]any{"status": "updated", "count": count})
			return
		}
		post, err := s.setPostLiked(id, *input.Liked)
		if errors.Is(err, os.ErrNotExist) {
			writeAPIError(w, http.StatusNotFound, "动态不存在或已删除")
			return
		}
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法持久化收藏状态")
			return
		}
		writeJSON(w, post)
		return
	}
	if r.Method == http.MethodDelete {
		ids := r.URL.Query()["id"]
		source := Source(strings.TrimSpace(r.URL.Query().Get("source")))
		author := strings.TrimSpace(r.URL.Query().Get("author"))
		if r.Body != nil && r.ContentLength != 0 {
			var input struct {
				IDs    []string `json:"ids"`
				Source Source   `json:"source"`
				Author string   `json:"author"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
				writeAPIError(w, http.StatusBadRequest, "删除请求格式无效")
				return
			}
			ids, source, author = append(ids, input.IDs...), input.Source, input.Author
		}
		count, err := s.deletePosts(ids, source, author)
		if errors.Is(err, os.ErrNotExist) {
			writeAPIError(w, http.StatusNotFound, "动态不存在或已删除")
			return
		}
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"status": "deleted", "count": count, "message": "动态及关联媒体文件已永久删除"})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.RLock()
	result := append([]Post(nil), s.posts...)
	s.RUnlock()
	if source := r.URL.Query().Get("source"); source != "" && source != "all" {
		filtered := result[:0]
		for _, post := range result {
			if string(post.Source) == source {
				filtered = append(filtered, post)
			}
		}
		result = filtered
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Published.After(result[j].Published) })
	writeJSON(w, result)
}

func (s *Store) feedsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		s.Lock()
		for index, feed := range s.feeds {
			if feed.ID == id {
				previous := s.feeds
				s.feeds = append(append([]SourceConfig(nil), s.feeds[:index]...), s.feeds[index+1:]...)
				if err := s.saveLocked(); err != nil {
					s.feeds = previous
					s.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化订阅删除")
					return
				}
				s.Unlock()
				writeJSON(w, map[string]string{"status": "deleted", "message": "订阅已永久删除，已采集文件予以保留"})
				return
			}
		}
		s.Unlock()
		writeAPIError(w, http.StatusNotFound, "来源不存在")
		return
	}
	if r.Method == http.MethodPost && r.URL.Query().Get("action") == "sync" {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		s.Lock()
		for index := range s.feeds {
			if s.feeds[index].ID == id {
				s.feeds[index].LastSyncedAt = time.Now()
				feed := s.feeds[index]
				if err := s.saveLocked(); err != nil {
					s.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法保存同步状态")
					return
				}
				s.Unlock()
				writeJSON(w, map[string]any{"status": "completed", "message": "来源状态已更新；该来源暂无在线采集器", "source": feed})
				return
			}
		}
		s.Unlock()
		writeAPIError(w, http.StatusNotFound, "来源不存在")
		return
	}
	if r.Method == http.MethodPost {
		var feed SourceConfig
		if err := json.NewDecoder(r.Body).Decode(&feed); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		feed.ID = "feed-" + time.Now().Format("150405")
		feed.Enabled = true
		s.Lock()
		s.feeds = append(s.feeds, feed)
		if err := s.saveLocked(); err != nil {
			s.feeds = s.feeds[:len(s.feeds)-1]
			s.Unlock()
			writeAPIError(w, http.StatusInternalServerError, "无法保存新来源")
			return
		}
		s.Unlock()
		writeJSON(w, feed)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.RLock()
	result := append([]SourceConfig(nil), s.feeds...)
	s.RUnlock()
	writeJSON(w, result)
}

func cleanCookieValue(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, "\"'")
}

func cookieValue(rawCookie, name string) string {
	for _, part := range strings.Split(rawCookie, ";") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) == 2 && strings.EqualFold(strings.TrimSpace(pair[0]), name) {
			return cleanCookieValue(pair[1])
		}
	}
	return ""
}

func normalizeBilibiliCredentials(credentials BilibiliCredentials) BilibiliCredentials {
	credentials.Cookie = strings.TrimSpace(credentials.Cookie)
	credentials.SESSDATA = cleanCookieValue(credentials.SESSDATA)
	credentials.BiliJCT = cleanCookieValue(credentials.BiliJCT)
	credentials.Buvid3 = cleanCookieValue(credentials.Buvid3)
	credentials.DedeUserID = cleanCookieValue(credentials.DedeUserID)
	credentials.AccessTimeValue = cleanCookieValue(credentials.AccessTimeValue)
	credentials.Buvid4 = cleanCookieValue(credentials.Buvid4)
	credentials.DedeUserIDCKMd5 = cleanCookieValue(credentials.DedeUserIDCKMd5)
	if credentials.Cookie != "" {
		credentials.SESSDATA = cookieValue(credentials.Cookie, "SESSDATA")
		credentials.BiliJCT = cookieValue(credentials.Cookie, "bili_jct")
		credentials.Buvid3 = cookieValue(credentials.Cookie, "buvid3")
		credentials.DedeUserID = cookieValue(credentials.Cookie, "DedeUserID")
		credentials.AccessTimeValue = cookieValue(credentials.Cookie, "ac_time_value")
		credentials.Buvid4 = cookieValue(credentials.Cookie, "buvid4")
		credentials.DedeUserIDCKMd5 = cookieValue(credentials.Cookie, "DedeUserID__ckMd5")
	}
	return credentials
}

func bilibiliCookie(credentials BilibiliCredentials) string {
	if strings.TrimSpace(credentials.Cookie) != "" {
		return strings.TrimSpace(credentials.Cookie)
	}
	values := [][2]string{{"SESSDATA", credentials.SESSDATA}, {"bili_jct", credentials.BiliJCT}, {"buvid3", credentials.Buvid3}, {"DedeUserID", credentials.DedeUserID}, {"ac_time_value", credentials.AccessTimeValue}, {"buvid4", credentials.Buvid4}, {"DedeUserID__ckMd5", credentials.DedeUserIDCKMd5}}
	parts := make([]string, 0, len(values))
	for _, item := range values {
		if value := cleanCookieValue(item[1]); value != "" {
			parts = append(parts, item[0]+"="+value)
		}
	}
	return strings.Join(parts, "; ")
}

func validateProxyURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("代理地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h" {
		return nil, errors.New("代理仅支持 http、https、socks5 或 socks5h")
	}
	return parsed, nil
}

func externalHTTPClient(proxyURL string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	if strings.TrimSpace(proxyURL) != "" {
		parsed, err := validateProxyURL(proxyURL)
		if err != nil {
			return nil, err
		}
		if parsed.Scheme == "socks5" || parsed.Scheme == "socks5h" {
			var auth *xproxy.Auth
			if parsed.User != nil {
				password, _ := parsed.User.Password()
				auth = &xproxy.Auth{User: parsed.User.Username(), Password: password}
			}
			dialer, err := xproxy.SOCKS5("tcp", parsed.Host, auth, xproxy.Direct)
			if err != nil {
				return nil, err
			}
			transport.Proxy = nil
			transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		} else {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}, nil
}

func maskedProxyURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "••••••")
	}
	return parsed.String()
}

func bilibiliRequest(endpoint string, credentials BilibiliCredentials, proxyURL string, target any) error {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Origin", "https://www.bilibili.com")
	request.Header.Set("Referer", "https://www.bilibili.com/")
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.Header.Set("Cookie", bilibiliCookie(credentials))
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bilibili returned status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func verifyBilibiliCredentials(credentials BilibiliCredentials, proxyURL string) error {
	credentials = normalizeBilibiliCredentials(credentials)
	if credentials.SESSDATA == "" || credentials.DedeUserID == "" {
		return errors.New("完整 Cookie 中缺少 SESSDATA 或 DedeUserID")
	}
	if _, err := strconv.ParseInt(credentials.DedeUserID, 10, 64); err != nil {
		return errors.New("invalid DedeUserID")
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			IsLogin bool  `json:"isLogin"`
			Mid     int64 `json:"mid"`
		} `json:"data"`
	}
	if err := bilibiliRequest("https://api.bilibili.com/x/web-interface/nav", credentials, proxyURL, &payload); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return errors.New("连接 B 站超时，请检查项目代理或容器网络")
		}
		if proxyURL != "" {
			return errors.New("无法通过项目代理连接 B 站，请检查代理地址、认证信息及容器可达性")
		}
		return errors.New("无法直连 B 站，请在设置中配置项目代理后重试")
	}
	if payload.Code != 0 {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = "未知原因"
		}
		if payload.Code == -101 {
			return fmt.Errorf("B 站未识别登录态（代码 -101：%s）。请改用浏览器 Network 请求头中的完整 Cookie，并确保容器与浏览器使用相同出口 IP", message)
		}
		return fmt.Errorf("B 站接口拒绝验证（代码 %d：%s）", payload.Code, message)
	}
	if !payload.Data.IsLogin {
		return errors.New("SESSDATA 登录态已失效或 Cookie 不完整")
	}
	if strconv.FormatInt(payload.Data.Mid, 10) != credentials.DedeUserID {
		return fmt.Errorf("DedeUserID 不匹配，当前 Cookie 对应 UID %d", payload.Data.Mid)
	}
	return nil
}

func bilibiliAccountProfile(credentials BilibiliCredentials, proxyURL string) BilibiliAccount {
	account := BilibiliAccount{Configured: credentials.SESSDATA != "", UserID: credentials.DedeUserID, UserName: credentials.UserName, Avatar: credentials.Avatar}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Mid   int64  `json:"mid"`
			Uname string `json:"uname"`
			Face  string `json:"face"`
		} `json:"data"`
	}
	if err := bilibiliRequest("https://api.bilibili.com/x/web-interface/nav", credentials, proxyURL, &payload); err != nil || payload.Code != 0 {
		return account
	}
	if payload.Data.Mid > 0 {
		account.UserID = strconv.FormatInt(payload.Data.Mid, 10)
	}
	account.UserName = strings.TrimSpace(payload.Data.Uname)
	account.Avatar = normalizeRemoteImage(payload.Data.Face)
	return account
}

func (b *BilibiliStore) accountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		credentials, proxyURL := b.config.Credentials, b.config.ProxyURL
		b.RUnlock()
		account := BilibiliAccount{Configured: credentials.SESSDATA != "", UserID: credentials.DedeUserID, UserName: credentials.UserName, Avatar: credentials.Avatar}
		account.Avatar = normalizeRemoteImage(account.Avatar)
		if account.Configured && (account.UserName == "" || account.Avatar == "") {
			enriched := bilibiliAccountProfile(credentials, proxyURL)
			if enriched.UserName != "" || enriched.Avatar != "" {
				credentials.UserName, credentials.Avatar = enriched.UserName, enriched.Avatar
				b.Lock()
				b.config.Credentials = credentials
				b.Unlock()
				_ = b.save()
				account = enriched
			}
		}
		if account.Configured && account.Avatar != "" {
			cachedAvatar := cacheAccountAvatar(SourceBilibili, bilibiliFavoriteOpusName, account.Avatar, proxyURL, "https://www.bilibili.com/", bilibiliCookie(credentials))
			if cachedAvatar != account.Avatar {
				account.Avatar = cachedAvatar
				credentials.Avatar = cachedAvatar
				b.Lock()
				b.config.Credentials = credentials
				b.Unlock()
				_ = b.save()
			}
		}
		writeJSON(w, account)
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var credentials BilibiliCredentials
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&credentials) != nil {
		http.Error(w, "invalid credentials", http.StatusBadRequest)
		return
	}
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	credentials = normalizeBilibiliCredentials(credentials)
	if err := verifyBilibiliCredentials(credentials, proxyURL); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	account := bilibiliAccountProfile(credentials, proxyURL)
	credentials.UserName, credentials.Avatar = account.UserName, account.Avatar
	b.Lock()
	b.config.Credentials = credentials
	b.Unlock()
	if err := b.save(); err != nil {
		http.Error(w, "unable to save bilibili account", http.StatusInternalServerError)
		return
	}
	writeJSON(w, account)
}

func plainBilibiliText(value string) string {
	return strings.TrimSpace(strings.NewReplacer("<em class=\"keyword\">", "", "</em>", "", "<em>", "").Replace(value))
}

func (b *BilibiliStore) searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if len([]rune(keyword)) < 1 || len([]rune(keyword)) > 40 {
		http.Error(w, "invalid keyword", http.StatusBadRequest)
		return
	}
	b.RLock()
	credentials := b.config.Credentials
	b.RUnlock()
	if credentials.SESSDATA == "" {
		http.Error(w, "bilibili account is not configured", http.StatusPreconditionFailed)
		return
	}
	endpoint := "https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&page=1&page_size=20&keyword=" + url.QueryEscape(keyword)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				Mid   json.Number `json:"mid"`
				Uname string      `json:"uname"`
				Upic  string      `json:"upic"`
				Fans  int64       `json:"fans"`
				Usign string      `json:"usign"`
			} `json:"result"`
		} `json:"data"`
	}
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil || payload.Code != 0 {
		http.Error(w, "bilibili search is temporarily unavailable", http.StatusBadGateway)
		return
	}
	users := make([]BilibiliUser, 0, len(payload.Data.Result))
	for _, result := range payload.Data.Result {
		avatar := result.Upic
		if strings.HasPrefix(avatar, "//") {
			avatar = "https:" + avatar
		}
		users = append(users, BilibiliUser{UserID: result.Mid.String(), Name: plainBilibiliText(result.Uname), Avatar: avatar, Fans: result.Fans, Description: result.Usign})
	}
	writeJSON(w, users)
}

func (b *BilibiliStore) subscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig{}, b.config.Subscriptions...)
		applyCollectionAccountProfiles(result, b.config)
		b.RUnlock()
		writeJSON(w, result)
		return
	}
	if r.Method == http.MethodDelete {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.Lock()
		for index, feed := range b.config.Subscriptions {
			if feed.ID == id {
				previous := b.config.Subscriptions
				b.config.Subscriptions = append(append([]SourceConfig(nil), b.config.Subscriptions[:index]...), b.config.Subscriptions[index+1:]...)
				b.Unlock()
				if err := b.save(); err != nil {
					b.Lock()
					b.config.Subscriptions = previous
					b.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化订阅删除")
					return
				}
				writeJSON(w, map[string]string{"status": "deleted", "message": "订阅已永久删除，内容目录和已采集文件予以保留"})
				return
			}
		}
		b.Unlock()
		writeAPIError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	if r.Method == http.MethodPost && (r.URL.Query().Get("action") == "sync" || r.URL.Query().Get("action") == "resync") {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.RLock()
		var feed SourceConfig
		for _, candidate := range b.config.Subscriptions {
			if candidate.ID == id {
				feed = candidate
				break
			}
		}
		b.RUnlock()
		if feed.ID == "" {
			writeAPIError(w, http.StatusNotFound, "订阅不存在")
			return
		}
		updated, err := b.syncSource(feed, r.URL.Query().Get("action") == "resync")
		if err != nil {
			writeJSON(w, map[string]any{"status": "failed", "message": err.Error(), "source": updated})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "message": updated.LastSyncMessage, "source": updated})
		return
	}
	if r.Method == http.MethodPut {
		var input SourceConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || !strings.HasPrefix(input.ID, "bili-") {
			writeAPIError(w, http.StatusBadRequest, "来源设置无效")
			return
		}
		input.Schedule = normalizeSchedule(input.Schedule)
		if !validCron(input.Schedule) {
			writeAPIError(w, http.StatusBadRequest, "Cron 表达式无效，请使用五段式格式")
			return
		}
		startDate, startDateErr := normalizeSourceStartDate(input.StartDate)
		if startDateErr != nil {
			writeAPIError(w, http.StatusBadRequest, startDateErr.Error())
			return
		}
		input.StartDate = startDate
		b.Lock()
		found := false
		for index := range b.config.Subscriptions {
			if b.config.Subscriptions[index].ID == input.ID {
				b.config.Subscriptions[index].Enabled = input.Enabled
				b.config.Subscriptions[index].Schedule = input.Schedule
				b.config.Subscriptions[index].StartDate = input.StartDate
				if isBilibiliFavoriteOpusFeed(b.config.Subscriptions[index]) {
					b.config.Subscriptions[index].ContentTypes = []string{"ARTICLE"}
				} else {
					b.config.Subscriptions[index].ContentTypes = normalizeBilibiliContentTypes(input.ContentTypes, false)
				}
				b.config.Subscriptions[index].Tags = normalizeTags(input.Tags)
				b.config.Subscriptions[index].OnlyWithImages = input.OnlyWithImages
				b.config.Subscriptions[index].IncludeVideos = input.IncludeVideos
				b.config.Subscriptions[index].IncludeKeywords = normalizeKeywords(input.IncludeKeywords)
				b.config.Subscriptions[index].ExcludeKeywords = normalizeKeywords(input.ExcludeKeywords)
				input = b.config.Subscriptions[index]
				found = true
				break
			}
		}
		b.Unlock()
		if !found {
			writeAPIError(w, http.StatusNotFound, "来源不存在")
			return
		}
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存来源设置")
			return
		}
		if b.content != nil {
			if err := b.content.setSourceTags(input, b.allSourceConfigs()); err != nil {
				writeAPIError(w, http.StatusInternalServerError, "无法将标签应用到来源动态")
				return
			}
		}
		writeJSON(w, input)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("type") == "favorite-opus" {
		b.RLock()
		credentials := b.config.Credentials
		b.RUnlock()
		if credentials.SESSDATA == "" || credentials.DedeUserID == "" {
			writeAPIError(w, http.StatusPreconditionFailed, "请先连接哔哩哔哩账号")
			return
		}
		feed := SourceConfig{
			ID:             bilibiliFavoriteOpusPrefix + credentials.DedeUserID,
			Source:         SourceBilibili,
			Name:           bilibiliFavoriteOpusName,
			Handle:         "UID " + credentials.DedeUserID,
			Avatar:         credentials.Avatar,
			Enabled:        true,
			Schedule:       "0 6 * * *",
			ContentTypes:   []string{"ARTICLE"},
			Tags:           []string{"B站收藏"},
			OnlyWithImages: false,
		}
		prepared, err := prepareSourceStorage(feed)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法创建哔哩哔哩收藏专栏目录")
			return
		}
		feed = prepared
		b.Lock()
		for _, existing := range b.config.Subscriptions {
			if existing.ID == feed.ID {
				b.Unlock()
				writeAPIError(w, http.StatusConflict, "收藏专栏来源已经存在")
				return
			}
		}
		b.config.Subscriptions = append(b.config.Subscriptions, feed)
		b.Unlock()
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存收藏专栏来源")
			return
		}
		writeJSON(w, feed)
		return
	}
	var input struct {
		UserID   string   `json:"userId"`
		Name     string   `json:"name"`
		Avatar   string   `json:"avatar"`
		Schedule string   `json:"schedule"`
		Tags     []string `json:"tags"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Name) == "" {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if input.Schedule == "" {
		input.Schedule = "0 6 * * *"
	}
	userID := strings.TrimSpace(input.UserID)
	if _, err := strconv.ParseUint(userID, 10, 64); err != nil {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	feed := SourceConfig{ID: "bili-" + userID, Source: SourceBilibili, Name: strings.TrimSpace(input.Name), Handle: "UID " + userID, Avatar: strings.TrimSpace(input.Avatar), Enabled: true, Schedule: input.Schedule, ContentTypes: []string{"DRAW"}, Tags: normalizeTags(input.Tags), OnlyWithImages: true}
	var storageErr error
	feed, storageErr = prepareSourceStorage(feed)
	if storageErr != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法创建 UP 主内容目录")
		return
	}
	b.Lock()
	for _, existing := range b.config.Subscriptions {
		if existing.ID == feed.ID {
			b.Unlock()
			http.Error(w, "already subscribed", http.StatusConflict)
			return
		}
	}
	b.config.Subscriptions = append(b.config.Subscriptions, feed)
	b.Unlock()
	if err := b.save(); err != nil {
		http.Error(w, "unable to save subscription", http.StatusInternalServerError)
		return
	}
	if err := b.reconcileCollectionArchives(); err != nil {
		http.Error(w, "unable to reconcile collection archives", http.StatusInternalServerError)
		return
	}
	writeJSON(w, feed)
}

func (b *BilibiliStore) projectSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		view := projectSettingsView(b.config)
		b.RUnlock()
		writeJSON(w, view)
		return
	}
	var input struct {
		ProxyURL            *string `json:"proxyUrl"`
		PreviewDesktopLevel *int    `json:"previewDesktopLevel"`
		PreviewMobileLevel  *int    `json:"previewMobileLevel"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "代理设置格式无效")
		return
	}
	proxyURL := ""
	if input.ProxyURL != nil {
		proxyURL = strings.TrimSpace(*input.ProxyURL)
		if proxyURL != "" {
			if _, err := validateProxyURL(proxyURL); err != nil {
				writeAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
	}
	for _, level := range []*int{input.PreviewDesktopLevel, input.PreviewMobileLevel} {
		if level != nil && (*level < 0 || *level > 5) {
			writeAPIError(w, http.StatusBadRequest, "预览画质档位必须在 0 到 5 之间")
			return
		}
	}
	if r.Method == http.MethodPost {
		if input.ProxyURL == nil {
			writeAPIError(w, http.StatusBadRequest, "请提供代理地址")
			return
		}
		client, err := externalHTTPClient(proxyURL)
		if err == nil {
			request, requestErr := http.NewRequest(http.MethodGet, "https://www.pixiv.net/robots.txt", nil)
			if requestErr == nil {
				request.Header.Set("User-Agent", "Lumic/1.0")
				response, requestErr := client.Do(request)
				if requestErr == nil {
					response.Body.Close()
					if response.StatusCode < 500 {
						writeJSON(w, map[string]string{"status": "ok", "message": "代理可以访问 pixiv"})
						return
					}
				}
			}
		}
		writeAPIError(w, http.StatusBadGateway, "代理测试失败，请检查地址、认证信息和 Docker 容器网络")
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.Lock()
	if input.ProxyURL != nil {
		b.config.ProxyURL = proxyURL
	}
	if input.PreviewDesktopLevel != nil {
		value := *input.PreviewDesktopLevel
		b.config.PreviewDesktopLevel = &value
	}
	if input.PreviewMobileLevel != nil {
		value := *input.PreviewMobileLevel
		b.config.PreviewMobileLevel = &value
	}
	view := projectSettingsView(b.config)
	b.Unlock()
	if err := b.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存项目设置")
		return
	}
	writeJSON(w, view)
}

// Bilibili sync accepts text, image-text, opus and article cards. Video cards are opt-in; forwarded cards remain excluded.
const bilibiliDynamicFeatures = "itemOpusStyle,opusBigCover,onlyfansVote,decorationCard,forwardListHidden,ugcDelete,onlyfansAssetsV2,ugcSeason,onlyfansQaCard"

func allowedBilibiliDynamicType(dynamicType string) bool {
	switch dynamicType {
	case "DYNAMIC_TYPE_WORD", "DYNAMIC_TYPE_DRAW", "DYNAMIC_TYPE_ARTICLE", "DYNAMIC_TYPE_OPUS", "DYNAMIC_TYPE_AV":
		return true
	default:
		return false
	}
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
var htmlLineBreakPattern = regexp.MustCompile(`(?i)<br\s*/?>|</p\s*>|</div\s*>|</li\s*>`)
var htmlImagePattern = regexp.MustCompile(`(?is)<img\b[^>]*>`)
var htmlImageAltPattern = regexp.MustCompile(`(?is)\b(?:alt|title|data-alt)\s*=\s*[\"']([^\"']*)[\"']`)

func normalizeRemoteImage(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	return value
}

func normalizeWeiboOriginalImage(value string) string {
	value = normalizeRemoteImage(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return value
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, ".sinaimg.cn") && host != "sinaimg.cn" {
		return value
	}
	segments := strings.Split(parsed.Path, "/")
	if len(segments) > 2 && segments[1] != "" {
		segments[1] = "original"
		parsed.Path = strings.Join(segments, "/")
		parsed.RawPath = ""
	}
	return parsed.String()
}

func cleanRemoteText(value string) string {
	value = htmlLineBreakPattern.ReplaceAllString(value, "\n")
	// Preserve platform emoji labels from HTML image nodes instead of dropping them with other tags.
	value = htmlImagePattern.ReplaceAllStringFunc(value, func(tag string) string {
		matches := htmlImageAltPattern.FindStringSubmatch(tag)
		if len(matches) > 1 {
			return matches[1]
		}
		return ""
	})
	value = stdhtml.UnescapeString(htmlTagPattern.ReplaceAllString(value, ""))
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" && (len(cleaned) == 0 || cleaned[len(cleaned)-1] == "") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func firstNonEmptyRemoteText(values ...string) string {
	for _, value := range values {
		value = cleanRemoteText(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func combineRemoteText(values ...string) string {
	parts, seen := make([]string, 0, len(values)), make(map[string]bool)
	for _, value := range values {
		value = cleanRemoteText(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		parts = append(parts, value)
	}
	return strings.Join(parts, "\n\n")
}

func mergeDetailedRemoteText(values ...string) string {
	result := ""
	for _, value := range values {
		value = cleanRemoteText(value)
		if value == "" || value == result {
			continue
		}
		if result == "" || strings.Contains(value, result) {
			result = value
			continue
		}
		if strings.Contains(result, value) {
			continue
		}
		result = combineRemoteText(result, value)
	}
	return result
}

func bilibiliDirectText(value map[string]any) string {
	for _, key := range []string{"text", "words", "orig_text", "raw_text", "text_raw"} {
		if text, ok := value[key].(string); ok {
			if text = cleanRemoteText(text); text != "" {
				return text
			}
		}
	}
	typeName := strings.ToUpper(jsonValueString(value["type"]))
	if strings.Contains(typeName, "EMOJI") {
		for _, key := range []string{"emoji_text", "emoji", "name", "label", "alt_text"} {
			if text, ok := value[key].(string); ok {
				if text = cleanRemoteText(text); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func bilibiliInlineText(value any) string {
	switch typed := value.(type) {
	case string:
		return cleanRemoteText(typed)
	case []any:
		var text strings.Builder
		for _, item := range typed {
			text.WriteString(bilibiliInlineText(item))
		}
		return cleanRemoteText(text.String())
	case map[string]any:
		if text := bilibiliDirectText(typed); text != "" {
			return text
		}
		for _, key := range []string{"word", "rich", "nodes", "text", "heading", "blockquote"} {
			if nested, exists := typed[key]; exists {
				if text := bilibiliInlineText(nested); text != "" {
					return text
				}
			}
		}
		return ""
	default:
		return ""
	}
}

func collectBilibiliRichText(value any, parts *[]string) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var nested any
			decoder := json.NewDecoder(strings.NewReader(trimmed))
			decoder.UseNumber()
			if decoder.Decode(&nested) == nil {
				collectBilibiliRichText(nested, parts)
				return
			}
		}
		if text := cleanRemoteText(typed); text != "" {
			*parts = append(*parts, text)
		}
	case []any:
		for _, item := range typed {
			collectBilibiliRichText(item, parts)
		}
	case map[string]any:
		directText := bilibiliDirectText(typed)
		if directText != "" {
			*parts = append(*parts, directText)
		}
		if directText == "" {
			for _, key := range []string{"rich_text_nodes", "nodes"} {
				if nested, exists := typed[key]; exists {
					if text := bilibiliInlineText(nested); text != "" {
						*parts = append(*parts, text)
					}
				}
			}
		}
		for _, key := range []string{"paragraphs", "content", "summary", "desc", "description", "title", "text_content", "text", "word", "heading", "blockquote", "list", "opus", "draw", "article", "major", "modules", "module_content", "module_dynamic", "item", "items"} {
			if nested, exists := typed[key]; exists {
				collectBilibiliRichText(nested, parts)
			}
		}
	}
}

func bilibiliObjectText(value any) string {
	parts := make([]string, 0)
	collectBilibiliRichText(value, &parts)
	return combineRemoteText(parts...)
}

func bilibiliRichText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	parts := make([]string, 0)
	collectBilibiliRichText(value, &parts)
	return combineRemoteText(parts...)
}

func bilibiliCaptionFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	return bilibiliObjectText(value)
}

func (b *BilibiliStore) fetchBilibiliDynamicCaption(dynamicID string, credentials BilibiliCredentials, proxyURL string) string {
	endpoints := []string{
		"https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?id=" + url.QueryEscape(dynamicID) + "&features=" + url.QueryEscape(bilibiliDynamicFeatures) + "&timezone_offset=-480",
		"https://api.bilibili.com/x/polymer/web-dynamic/v1/opus/detail?id=" + url.QueryEscape(dynamicID) + "&features=" + url.QueryEscape(bilibiliDynamicFeatures),
		"https://api.bilibili.com/x/polymer/web-dynamic/v1/opus/detail?opus_id=" + url.QueryEscape(dynamicID) + "&features=" + url.QueryEscape(bilibiliDynamicFeatures),
	}
	captions := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		var payload map[string]any
		if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil {
			continue
		}
		code := jsonValueString(payload["code"])
		if code != "" && code != "0" {
			continue
		}
		if caption := bilibiliObjectText(payload["data"]); caption != "" {
			captions = append(captions, caption)
		}
	}
	return mergeDetailedRemoteText(captions...)
}

func bilibiliInitialState(body []byte) (map[string]any, error) {
	marker := []byte("window.__INITIAL_STATE__=")
	start := bytes.Index(body, marker)
	if start < 0 {
		return nil, errors.New("专栏页面缺少初始化数据")
	}
	decoder := json.NewDecoder(bytes.NewReader(body[start+len(marker):]))
	decoder.UseNumber()
	var state map[string]any
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("解析专栏页面失败: %w", err)
	}
	return state, nil
}

func collectBilibiliOpusImages(value any, images *[]string, seen map[string]bool) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectBilibiliOpusImages(item, images, seen)
		}
	case map[string]any:
		for key, nested := range typed {
			if (key == "url" || key == "src") && nested != nil {
				image := normalizeRemoteImage(jsonValueString(nested))
				if image != "" && strings.Contains(strings.ToLower(image), "hdslb.com/bfs/") && !seen[image] {
					seen[image] = true
					*images = append(*images, image)
				}
			}
			collectBilibiliOpusImages(nested, images, seen)
		}
	}
}

func bilibiliOpusPost(opusID string, state map[string]any, feed SourceConfig) (Post, error) {
	detail, ok := state["detail"].(map[string]any)
	if !ok {
		return Post{}, errors.New("专栏详情不存在")
	}
	if id := jsonValueString(detail["id_str"]); id != "" {
		opusID = id
	}
	modules, _ := detail["modules"].([]any)
	captionParts := make([]string, 0)
	media := make([]string, 0)
	seenMedia := make(map[string]bool)
	author, avatar := "", ""
	var published time.Time
	for _, rawModule := range modules {
		module, _ := rawModule.(map[string]any)
		if title, ok := module["module_title"].(map[string]any); ok {
			captionParts = append(captionParts, jsonValueString(title["text"]))
		}
		if moduleAuthor, ok := module["module_author"].(map[string]any); ok {
			author = firstNonEmptyRemoteText(author, jsonValueString(moduleAuthor["name"]))
			avatar = firstNonEmptyRemoteText(avatar, normalizeRemoteImage(jsonValueString(moduleAuthor["face"])))
			if timestamp, err := strconv.ParseInt(jsonValueString(moduleAuthor["pub_ts"]), 10, 64); err == nil && timestamp > 0 {
				published = time.Unix(timestamp, 0)
			}
		}
		if content, ok := module["module_content"].(map[string]any); ok {
			captionParts = append(captionParts, bilibiliObjectText(content))
			collectBilibiliOpusImages(content, &media, seenMedia)
		}
		if top, ok := module["module_top"].(map[string]any); ok {
			collectBilibiliOpusImages(top, &media, seenMedia)
		}
	}
	if author == "" {
		author = feed.Name
	}
	if avatar == "" {
		avatar = feed.Avatar
	}
	if published.IsZero() {
		published = time.Now()
	}
	if opusID == "" {
		return Post{}, errors.New("专栏 ID 为空")
	}
	return Post{
		ID:          "bili-dynamic-" + opusID,
		Source:      SourceBilibili,
		FeedIDs:     []string{feed.ID},
		Author:      author,
		Avatar:      avatar,
		Caption:     combineRemoteText(captionParts...),
		Tags:        append([]string(nil), feed.Tags...),
		Media:       media,
		OriginalURL: "https://www.bilibili.com/opus/" + url.PathEscape(opusID),
		Published:   published,
	}, nil
}

func fetchBilibiliOpusPage(client *http.Client, credentials BilibiliCredentials, opusID string, feed SourceConfig) (Post, error) {
	request, err := http.NewRequest(http.MethodGet, "https://www.bilibili.com/opus/"+url.PathEscape(opusID), nil)
	if err != nil {
		return Post{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Referer", "https://space.bilibili.com/")
	request.Header.Set("Cookie", bilibiliCookie(credentials))
	response, err := client.Do(request)
	if err != nil {
		return Post{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Post{}, fmt.Errorf("专栏页面返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 12<<20))
	if err != nil {
		return Post{}, err
	}
	state, err := bilibiliInitialState(body)
	if err != nil {
		return Post{}, err
	}
	return bilibiliOpusPost(opusID, state, feed)
}

func (b *BilibiliStore) fetchBilibiliFavoriteOpusPosts(feed SourceConfig, full bool) ([]Post, error) {
	b.RLock()
	credentials, proxyURL := b.config.Credentials, b.config.ProxyURL
	b.RUnlock()
	if credentials.SESSDATA == "" || credentials.DedeUserID == "" {
		return nil, errors.New("B 站账号未连接")
	}
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	pageLimit := 1
	if full {
		pageLimit = 100
	}
	offset := ""
	posts := make([]Post, 0)
	seen := make(map[string]bool)
	var lastDetailError error
	for page := 0; page < pageLimit; page++ {
		query := url.Values{"page_size": {"20"}}
		if offset != "" {
			query.Set("offset", offset)
		}
		var payload struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Items  []map[string]any `json:"items"`
				Offset string           `json:"offset"`
			} `json:"data"`
		}
		endpoint := "https://api.bilibili.com/x/polymer/web-dynamic/v1/opus/feed/fav?" + query.Encode()
		if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil {
			return nil, fmt.Errorf("B 站收藏专栏请求失败: %w", err)
		}
		if payload.Code != 0 {
			return nil, fmt.Errorf("B 站收藏专栏接口拒绝访问: %s", payload.Message)
		}
		for _, item := range payload.Data.Items {
			opusID := jsonValueString(item["opus_id"])
			if opusID == "" || seen[opusID] {
				continue
			}
			seen[opusID] = true
			post, detailErr := fetchBilibiliOpusPage(client, credentials, opusID, feed)
			if detailErr != nil {
				lastDetailError = detailErr
				continue
			}
			posts = append(posts, post)
			time.Sleep(350 * time.Millisecond)
		}
		if len(payload.Data.Items) == 0 || payload.Data.Offset == "" || payload.Data.Offset == offset {
			break
		}
		offset = payload.Data.Offset
		time.Sleep(500 * time.Millisecond)
	}
	if len(posts) == 0 && lastDetailError != nil {
		return nil, fmt.Errorf("无法读取收藏专栏详情: %w", lastDetailError)
	}
	return posts, nil
}

func shouldFetchBilibiliDynamicDetail(dynamicType, caption string) bool {
	if strings.TrimSpace(caption) == "" {
		return true
	}
	return dynamicType == "DYNAMIC_TYPE_OPUS" || dynamicType == "DYNAMIC_TYPE_ARTICLE"
}

func mediaFileExtension(remoteURL, contentType string) string {
	extension := strings.ToLower(filepath.Ext(path.Base(strings.Split(remoteURL, "?")[0])))
	if matched, _ := regexp.MatchString(`^\.(jpe?g|png|gif|webp|avif)$`, extension); matched {
		return extension
	}
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])
	switch contentType {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	default:
		return ".jpg"
	}
}

func videoFileExtension(remoteURL, contentType string) string {
	extension := strings.ToLower(filepath.Ext(path.Base(strings.Split(remoteURL, "?")[0])))
	if matched, _ := regexp.MatchString(`^\.(mp4|webm|mov|m4v|ogv)$`, extension); matched {
		return extension
	}
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])
	switch contentType {
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "video/ogg", "application/ogg":
		return ".ogv"
	default:
		return ".mp4"
	}
}

func downloadRemoteImage(client *http.Client, remoteURL, targetBase, referer, cookie string) (string, error) {
	remoteURL = normalizeRemoteImage(remoteURL)
	if remoteURL == "" || strings.HasPrefix(remoteURL, "/flow/") {
		return remoteURL, nil
	}
	request, err := http.NewRequest(http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return "", fmt.Errorf("unexpected content type %s", contentType)
	}
	target := targetBase + mediaFileExtension(remoteURL, contentType)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".lumic-image-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	written, copyErr := io.CopyBuffer(temporary, response.Body, make([]byte, 64<<10))
	if copyErr != nil || written == 0 {
		temporary.Close()
		if copyErr != nil {
			return "", copyErr
		}
		return "", errors.New("empty image")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := replaceFile(temporaryName, target); err != nil {
		return "", err
	}
	return target, nil
}

func downloadRemoteVideo(client *http.Client, remoteURL, targetBase, referer, cookie string) (string, error) {
	remoteURL = normalizeRemoteImage(stdhtml.UnescapeString(remoteURL))
	if remoteURL == "" || strings.HasPrefix(remoteURL, "/flow/") {
		return remoteURL, nil
	}
	request, err := http.NewRequest(http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "video/mp4,video/webm,video/*;q=0.9,application/octet-stream;q=0.7,*/*;q=0.5")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	remoteExtension := strings.ToLower(filepath.Ext(path.Base(strings.Split(remoteURL, "?")[0])))
	knownExtension, _ := regexp.MatchString(`^\.(mp4|webm|mov|m4v|ogv)$`, remoteExtension)
	if !strings.HasPrefix(contentType, "video/") && !strings.HasPrefix(contentType, "application/octet-stream") && !knownExtension {
		return "", fmt.Errorf("unexpected content type %s", contentType)
	}
	target := targetBase + videoFileExtension(remoteURL, contentType)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".lumic-video-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	written, copyErr := io.CopyBuffer(temporary, response.Body, make([]byte, 256<<10))
	if copyErr != nil || written == 0 {
		temporary.Close()
		if copyErr != nil {
			return "", copyErr
		}
		return "", errors.New("empty video")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := replaceFile(temporaryName, target); err != nil {
		return "", err
	}
	return target, nil
}

func replaceFile(temporaryName, target string) error {
	if err := os.Rename(temporaryName, target); err == nil {
		return nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryName, target)
}

func imageDownloadDelay(source Source, sequence int) time.Duration {
	delay := 180 * time.Millisecond
	if source == SourceBilibili {
		delay = 1500*time.Millisecond + time.Duration(sequence%4)*150*time.Millisecond
	}
	if source == SourcePixiv {
		delay = 520*time.Millisecond + time.Duration(sequence%3)*110*time.Millisecond
	}
	if raw := strings.TrimSpace(os.Getenv("LUMIC_IMAGE_DOWNLOAD_DELAY_MS")); raw != "" {
		if milliseconds, err := strconv.Atoi(raw); err == nil && milliseconds >= 0 && milliseconds <= 5000 {
			delay = time.Duration(milliseconds) * time.Millisecond
		}
	}
	return delay
}

func videoDownloadDelay(source Source, sequence int) time.Duration {
	delay := 500 * time.Millisecond
	if source == SourceBilibili {
		delay = 2200*time.Millisecond + time.Duration(sequence%3)*250*time.Millisecond
	}
	return delay
}

func postMediaBaseName(post Post, mediaIndex int) string {
	author := safeFlowDirectoryName(post.Author)
	if author == "unnamed" {
		author = "作者"
	}
	published := post.Published
	if published.IsZero() {
		published = time.Now()
	}
	base := author + "-" + published.Format("20060102")
	if len(post.Media) > 1 {
		base += "·" + strconv.Itoa(mediaIndex+1)
	}
	return base
}

func postVideoBaseName(post Post, videoIndex int, poster bool) string {
	author := safeFlowDirectoryName(post.Author)
	if author == "unnamed" {
		author = "作者"
	}
	published := post.Published
	if published.IsZero() {
		published = time.Now()
	}
	base := author + "-" + published.Format("20060102") + "-视频"
	if len(post.Videos) > 1 {
		base += "·" + strconv.Itoa(videoIndex+1)
	}
	if poster {
		base += "-封面"
	}
	return base
}

type textArchiveKey struct {
	Source          Source
	AuthorDirectory string
}

func textArchiveKeyForPost(post Post) textArchiveKey {
	archiveName := post.Author
	switch post.Source {
	case SourceWeibo:
		if hasFeedPrefix(post.FeedIDs, "weibo-likes-") && !postUsesAuthorArchive(post) {
			archiveName = weiboLikesName
		}
	case SourcePixiv:
		if hasFeedPrefix(post.FeedIDs, "pixiv-bookmarks-") && !postUsesAuthorArchive(post) {
			archiveName = pixivBookmarksName
		}
	case SourceBilibili:
		if hasFeedPrefix(post.FeedIDs, bilibiliFavoriteOpusPrefix) && !postUsesAuthorArchive(post) {
			archiveName = bilibiliFavoriteOpusName
		}
	case SourceTwitter:
		if hasFeedPrefix(post.FeedIDs, "twitter-likes-") && !postUsesAuthorArchive(post) {
			archiveName = twitterLikesName
		}
	}
	return textArchiveKey{Source: post.Source, AuthorDirectory: safeFlowDirectoryName(archiveName)}
}

func textArchivePaths(key textArchiveKey) (string, string) {
	fileName := "post_contents.txt"
	localPath := filepath.Join(flowRoot, string(key.Source), key.AuthorDirectory, fileName)
	publicPath := flowPublicPath(key.Source, key.AuthorDirectory, fileName)
	return localPath, publicPath
}

func localFlowArchivePath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "" && parsed.Scheme != "file") {
		return "", false
	}
	candidate := parsed.Path
	if strings.HasPrefix(filepath.ToSlash(candidate), "/flow/") {
		candidate = filepath.Join(flowRoot, strings.TrimPrefix(filepath.ToSlash(candidate), "/flow/"))
	}
	root, rootErr := filepath.Abs(flowRoot)
	absolute, pathErr := filepath.Abs(filepath.FromSlash(candidate))
	if rootErr != nil || pathErr != nil {
		return "", false
	}
	relative, relErr := filepath.Rel(root, absolute)
	if relErr != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return absolute, true
}

func sameLocalFlowArchivePath(left, right string) bool {
	leftPath, leftOK := localFlowArchivePath(left)
	rightPath, rightOK := localFlowArchivePath(right)
	return leftOK && rightOK && strings.EqualFold(filepath.Clean(leftPath), filepath.Clean(rightPath))
}

func renderAuthorTextArchive(posts []Post) []byte {
	ordered := append([]Post(nil), posts...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Published.Before(ordered[j].Published) })
	var content strings.Builder
	for _, post := range ordered {
		caption := cleanRemoteText(post.Caption)
		if caption == "" {
			continue
		}
		if content.Len() > 0 {
			content.WriteString("\n\n")
		}
		if post.Published.IsZero() {
			content.WriteString("[未记录时间]")
		} else {
			content.WriteString("[")
			content.WriteString(post.Published.Format("2006-01-02 15:04:05"))
			content.WriteString("]")
		}
		if author := cleanRemoteText(post.Author); author != "" {
			content.WriteString(" ")
			content.WriteString(author)
		}
		content.WriteString("\n")
		content.WriteString(caption)
	}
	if content.Len() > 0 {
		content.WriteString("\n")
	}
	return []byte(content.String())
}

func (s *Store) reconcileTextArchivesFor(selected map[textArchiveKey]bool) (bool, error) {
	keys := make(map[textArchiveKey]bool)
	groups := make(map[textArchiveKey][]int)
	previousPaths := make(map[textArchiveKey]map[string]bool)
	protectedPaths := make(map[string]bool)
	changed := false
	if selected != nil {
		for key := range selected {
			keys[key] = true
		}
	}
	for _, post := range s.posts {
		if cleanRemoteText(post.Caption) == "" {
			continue
		}
		_, publicPath := textArchivePaths(textArchiveKeyForPost(post))
		if localPath, ok := localFlowArchivePath(publicPath); ok {
			protectedPaths[strings.ToLower(filepath.Clean(localPath))] = true
		}
	}
	for index := range s.posts {
		key := textArchiveKeyForPost(s.posts[index])
		if selected != nil && !selected[key] {
			continue
		}
		keys[key] = true
		if s.posts[index].TextFile != "" {
			if previousPaths[key] == nil {
				previousPaths[key] = make(map[string]bool)
			}
			previousPaths[key][s.posts[index].TextFile] = true
		}
		caption := cleanRemoteText(s.posts[index].Caption)
		if caption != s.posts[index].Caption {
			s.posts[index].Caption = caption
			changed = true
		}
		if caption == "" {
			if s.posts[index].TextFile != "" {
				s.posts[index].TextFile = ""
				changed = true
			}
			continue
		}
		groups[key] = append(groups[key], index)
	}
	for key := range keys {
		localPath, publicPath := textArchivePaths(key)
		indexes := groups[key]
		if len(indexes) == 0 {
			if err := deletePostMedia([]string{publicPath}); err != nil {
				return false, err
			}
		} else {
			posts := make([]Post, 0, len(indexes))
			for _, index := range indexes {
				posts = append(posts, s.posts[index])
			}
			if err := atomicWriteFile(localPath, renderAuthorTextArchive(posts), 0644); err != nil {
				return false, err
			}
			for _, index := range indexes {
				if s.posts[index].TextFile != publicPath {
					s.posts[index].TextFile = publicPath
					changed = true
				}
			}
		}
		for previousPath := range previousPaths[key] {
			if sameLocalFlowArchivePath(previousPath, publicPath) {
				continue
			}
			if localPath, ok := localFlowArchivePath(previousPath); ok && protectedPaths[strings.ToLower(filepath.Clean(localPath))] {
				continue
			}
			if err := deletePostMedia([]string{previousPath}); err != nil {
				return false, err
			}
		}
	}
	return changed, nil
}

func availableMediaTargetBase(directory, base string) string {
	target := filepath.Join(directory, base)
	for sequence := 2; ; sequence++ {
		matches, _ := filepath.Glob(target + ".*")
		if len(matches) == 0 {
			return target
		}
		target = filepath.Join(directory, base+"-"+strconv.Itoa(sequence))
	}
}

func (b *BilibiliStore) archiveSourceContent(feed SourceConfig, posts []Post) (SourceConfig, []Post, error) {
	prepared, err := prepareSourceStorage(feed)
	if err != nil {
		return feed, posts, err
	}
	client, err := externalHTTPClient(b.config.ProxyURL)
	if err != nil {
		return feed, posts, err
	}
	referer, cookie := "https://www.bilibili.com/", bilibiliCookie(b.config.Credentials)
	if feed.Source == SourceWeibo {
		referer, cookie = "https://weibo.com/", b.config.Weibo.Cookie
	} else if feed.Source == SourcePixiv {
		referer, cookie = "https://www.pixiv.net/", b.config.Pixiv.Cookie
	} else if feed.Source == SourceTwitter {
		referer, cookie = "https://x.com/", ""
	}
	if prepared.Avatar != "" && !strings.HasPrefix(prepared.Avatar, "/flow/") {
		avatarPath, downloadErr := downloadRemoteImage(client, prepared.Avatar, filepath.Join(prepared.StoragePath, "avatar"), referer, cookie)
		if downloadErr == nil {
			prepared.Avatar = flowPublicPath(prepared.Source, prepared.Name, filepath.Base(avatarPath))
		}
	}
	configuredFeeds := make([]SourceConfig, 0, len(b.config.Subscriptions)+len(b.config.WeiboSubscriptions)+len(b.config.PixivSubscriptions)+len(b.config.TwitterSubscriptions))
	configuredFeeds = append(configuredFeeds, b.config.Subscriptions...)
	configuredFeeds = append(configuredFeeds, b.config.WeiboSubscriptions...)
	configuredFeeds = append(configuredFeeds, b.config.PixivSubscriptions...)
	configuredFeeds = append(configuredFeeds, b.config.TwitterSubscriptions...)
	for postIndex := range posts {
		posts[postIndex].Videos = normalizePostVideos(posts[postIndex].Videos)
		target := prepared
		if isCollectionFeed(prepared) {
			if authorFeed, ok := collectionAuthorSubscription(posts[postIndex], configuredFeeds); ok {
				var prepareErr error
				target, prepareErr = prepareSourceStorage(authorFeed)
				if prepareErr != nil {
					return feed, posts, prepareErr
				}
				posts[postIndex].FeedIDs = mergeUniqueStrings(posts[postIndex].FeedIDs, []string{prepared.ID, target.ID})
			}
		}
		if !strings.HasPrefix(prepared.ID, "weibo-likes-") && !strings.HasPrefix(prepared.ID, "pixiv-bookmarks-") && !strings.HasPrefix(prepared.ID, "twitter-likes-") && !isBilibiliFavoriteOpusFeed(prepared) {
			posts[postIndex].Avatar = prepared.Avatar
		}
		localMedia := make([]string, 0, len(posts[postIndex].Media))
		for mediaIndex, remoteURL := range posts[postIndex].Media {
			if strings.HasPrefix(remoteURL, "/flow/") {
				localMedia = append(localMedia, remoteURL)
			} else {
				base := availableMediaTargetBase(target.StoragePath, postMediaBaseName(posts[postIndex], mediaIndex))
				localPath, downloadErr := downloadRemoteImage(client, remoteURL, base, referer, cookie)
				if downloadErr != nil {
					return feed, posts, fmt.Errorf("下载动态 %s 的第 %d 张图片失败: %w", posts[postIndex].ID, mediaIndex+1, downloadErr)
				}
				localMedia = append(localMedia, flowPublicPath(target.Source, target.Name, filepath.Base(localPath)))
				if delay := imageDownloadDelay(feed.Source, postIndex+mediaIndex); delay > 0 {
					time.Sleep(delay)
				}
			}
		}
		posts[postIndex].Media = localMedia
		localVideos := make([]PostVideo, 0, len(posts[postIndex].Videos))
		for videoIndex, video := range posts[postIndex].Videos {
			if strings.TrimSpace(video.URL) == "" {
				continue
			}
			localVideo := video
			if video.Poster != "" && !strings.HasPrefix(video.Poster, "/flow/") {
				posterBase := availableMediaTargetBase(target.StoragePath, postVideoBaseName(posts[postIndex], videoIndex, true))
				posterPath, downloadErr := downloadRemoteImage(client, video.Poster, posterBase, referer, cookie)
				if downloadErr != nil {
					return feed, posts, fmt.Errorf("下载动态 %s 的第 %d 个视频封面失败: %w", posts[postIndex].ID, videoIndex+1, downloadErr)
				}
				localVideo.Poster = flowPublicPath(target.Source, target.Name, filepath.Base(posterPath))
			}
			if !strings.HasPrefix(video.URL, "/flow/") {
				videoBase := availableMediaTargetBase(target.StoragePath, postVideoBaseName(posts[postIndex], videoIndex, false))
				videoPath, downloadErr := downloadRemoteVideo(client, video.URL, videoBase, referer, cookie)
				if downloadErr != nil {
					return feed, posts, fmt.Errorf("下载动态 %s 的第 %d 个视频失败: %w", posts[postIndex].ID, videoIndex+1, downloadErr)
				}
				localVideo.URL = flowPublicPath(target.Source, target.Name, filepath.Base(videoPath))
				if delay := videoDownloadDelay(feed.Source, postIndex+videoIndex); delay > 0 {
					time.Sleep(delay)
				}
			}
			localVideos = append(localVideos, localVideo)
		}
		posts[postIndex].Videos = localVideos
		posts[postIndex].Emojis = nil
	}
	prepared, err = prepareSourceStorage(prepared)
	return prepared, posts, err
}

func parseRemoteTimestamp(raw json.RawMessage) int64 {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" || value == "null" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func fetchBilibiliVideoURL(bvid string, credentials BilibiliCredentials, proxyURL string) (string, error) {
	bvid = strings.TrimSpace(bvid)
	if bvid == "" {
		return "", errors.New("BVID 为空")
	}
	var view struct {
		Code int `json:"code"`
		Data struct {
			CID   int64 `json:"cid"`
			Pages []struct {
				CID int64 `json:"cid"`
			} `json:"pages"`
		} `json:"data"`
	}
	if err := bilibiliRequest("https://api.bilibili.com/x/web-interface/view?bvid="+url.QueryEscape(bvid), credentials, proxyURL, &view); err != nil {
		return "", err
	}
	if view.Code != 0 {
		return "", fmt.Errorf("视频详情接口返回 %d", view.Code)
	}
	cid := view.Data.CID
	if cid == 0 && len(view.Data.Pages) > 0 {
		cid = view.Data.Pages[0].CID
	}
	if cid == 0 {
		return "", errors.New("视频 CID 为空")
	}
	query := url.Values{
		"bvid":         {bvid},
		"cid":          {strconv.FormatInt(cid, 10)},
		"qn":           {"80"},
		"fnver":        {"0"},
		"fnval":        {"0"},
		"fourk":        {"1"},
		"platform":     {"html5"},
		"high_quality": {"1"},
	}
	var play struct {
		Code int `json:"code"`
		Data struct {
			DURL []struct {
				URL       string   `json:"url"`
				BackupURL []string `json:"backup_url"`
			} `json:"durl"`
		} `json:"data"`
	}
	if err := bilibiliRequest("https://api.bilibili.com/x/player/playurl?"+query.Encode(), credentials, proxyURL, &play); err != nil {
		return "", err
	}
	if play.Code != 0 || len(play.Data.DURL) == 0 {
		return "", fmt.Errorf("视频播放接口返回 %d", play.Code)
	}
	videoURL := normalizeRemoteImage(play.Data.DURL[0].URL)
	if videoURL == "" && len(play.Data.DURL[0].BackupURL) > 0 {
		videoURL = normalizeRemoteImage(play.Data.DURL[0].BackupURL[0])
	}
	if videoURL == "" {
		return "", errors.New("视频播放地址为空")
	}
	return videoURL, nil
}

func (b *BilibiliStore) findBilibiliUser(name string) (BilibiliUser, error) {
	b.RLock()
	credentials, proxyURL := b.config.Credentials, b.config.ProxyURL
	b.RUnlock()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				Mid   json.Number `json:"mid"`
				Uname string      `json:"uname"`
				Upic  string      `json:"upic"`
				Fans  int64       `json:"fans"`
				Usign string      `json:"usign"`
			} `json:"result"`
		} `json:"data"`
	}
	endpoint := "https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&page=1&page_size=20&keyword=" + url.QueryEscape(name)
	if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil || payload.Code != 0 {
		return BilibiliUser{}, errors.New("无法校正 UP 主资料")
	}
	for _, result := range payload.Data.Result {
		if plainBilibiliText(result.Uname) == name {
			return BilibiliUser{UserID: result.Mid.String(), Name: name, Avatar: normalizeRemoteImage(result.Upic), Fans: result.Fans, Description: result.Usign}, nil
		}
	}
	return BilibiliUser{}, errors.New("无法按昵称找到原 UP 主，请删除后重新订阅")
}

func (b *BilibiliStore) fetchBilibiliPosts(feed SourceConfig, full bool) ([]Post, error) {
	userID := strings.TrimSpace(strings.TrimPrefix(feed.ID, "bili-"))
	b.RLock()
	credentials, proxyURL := b.config.Credentials, b.config.ProxyURL
	b.RUnlock()
	if credentials.SESSDATA == "" {
		return nil, errors.New("B 站账号未连接")
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items   []json.RawMessage `json:"items"`
			Offset  string            `json:"offset"`
			HasMore bool              `json:"has_more"`
		} `json:"data"`
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
	posts := make([]Post, 0)
	offset := ""
	for page := 0; page < 100; page++ {
		query := url.Values{}
		query.Set("host_mid", userID)
		query.Set("features", bilibiliDynamicFeatures)
		query.Set("timezone_offset", "-480")
		if offset != "" {
			query.Set("offset", offset)
		}
		endpoint := "https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/space?" + query.Encode()
		payload.Data.Items = nil
		if err := bilibiliRequest(endpoint, credentials, proxyURL, &payload); err != nil {
			return nil, fmt.Errorf("B 站请求失败: %w", err)
		}
		if payload.Code != 0 {
			return nil, fmt.Errorf("B 站接口拒绝拉取: %s", payload.Message)
		}
		reachedBoundary := false
		for _, rawItem := range payload.Data.Items {
			var item bilibiliDynamicItem
			if json.Unmarshal(rawItem, &item) != nil {
				continue
			}
			if item.ID == "" || !bilibiliDynamicTypeEnabled(item.Type, feed.ContentTypes, feed.IncludeVideos) {
				continue
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
						videoURL, videoErr := fetchBilibiliVideoURL(major.Archive.BVID, credentials, proxyURL)
						if videoErr != nil {
							log.Printf("unable to resolve Bilibili video %s: %v", major.Archive.BVID, videoErr)
						} else {
							videos = append(videos, PostVideo{URL: videoURL, Poster: normalizeRemoteImage(major.Archive.Cover)})
						}
					}
				}
			}
			captionParts = append(captionParts, bilibiliCaptionFromRaw(rawItem))
			caption := combineRemoteText(captionParts...)
			if shouldFetchBilibiliDynamicDetail(item.Type, caption) {
				detailCaption := b.fetchBilibiliDynamicCaption(item.ID, credentials, proxyURL)
				caption = mergeDetailedRemoteText(caption, detailCaption)
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
			posts = append(posts, Post{ID: "bili-dynamic-" + item.ID, Source: SourceBilibili, FeedIDs: []string{feed.ID}, Author: name, Avatar: avatar, Caption: caption, Tags: append([]string(nil), feed.Tags...), Media: media, Videos: videos, OriginalURL: "https://t.bilibili.com/" + url.PathEscape(item.ID), Published: published})
			if !full && !feed.LastSyncedAt.IsZero() && !published.After(feed.LastSyncedAt) {
				reachedBoundary = true
			}
		}
		if len(payload.Data.Items) == 0 || !payload.Data.HasMore || payload.Data.Offset == "" || payload.Data.Offset == offset || reachedBoundary {
			break
		}
		if !full && feed.LastSyncedAt.IsZero() {
			break
		}
		offset = payload.Data.Offset
	}
	return posts, nil
}

func remoteURLValue(value any) string {
	if object, ok := value.(map[string]any); ok {
		return firstNonEmptyRemoteText(jsonValueString(object["url"]), jsonValueString(object["src"]))
	}
	return jsonValueString(value)
}

func usableVideoURL(value, mime string) string {
	value = normalizeRemoteImage(stdhtml.UnescapeString(strings.TrimSpace(value)))
	if value == "" || value == "<nil>" || strings.Contains(strings.ToLower(value), ".m3u8") {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	if mime != "" && !strings.Contains(strings.ToLower(mime), "video") && !strings.Contains(strings.ToLower(mime), "mp4") {
		return ""
	}
	return value
}

func weiboDirectVideoURL(object map[string]any) string {
	for _, key := range []string{"mp4_1080p_mp4", "mp4_720p_mp4", "mp4_hd_url", "mp4_sd_url", "stream_url_hd", "stream_url"} {
		if videoURL := usableVideoURL(remoteURLValue(object[key]), ""); videoURL != "" {
			return videoURL
		}
	}
	mime := firstNonEmptyRemoteText(jsonValueString(object["mime"]), jsonValueString(object["mime_type"]), jsonValueString(object["type"]), jsonValueString(object["format"]))
	if strings.Contains(strings.ToLower(mime), "video") || strings.Contains(strings.ToLower(mime), "mp4") {
		return usableVideoURL(remoteURLValue(object["url"]), mime)
	}
	return ""
}

func weiboDirectVideoPoster(object map[string]any) string {
	for _, key := range []string{"poster_url", "poster", "page_pic", "cover", "cover_image", "cover_url", "video_poster", "thumbnail"} {
		if poster := weiboPosterValue(object[key]); poster != "" {
			return poster
		}
	}
	return ""
}

func weiboPosterValue(value any) string {
	switch typed := value.(type) {
	case string:
		poster := normalizeRemoteImage(typed)
		if poster != "" && poster != "<nil>" {
			return poster
		}
	case map[string]any:
		for _, key := range []string{"url", "src", "pic_big", "large", "original", "image_url"} {
			if poster := weiboPosterValue(typed[key]); poster != "" {
				return poster
			}
		}
	}
	return ""
}

func findWeiboVideoPoster(value any) string {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if poster := findWeiboVideoPoster(item); poster != "" {
				return poster
			}
		}
	case map[string]any:
		if poster := weiboDirectVideoPoster(typed); poster != "" {
			return poster
		}
		for _, key := range []string{"page_info", "media_info", "video", "playback_list", "play_info"} {
			if poster := findWeiboVideoPoster(typed[key]); poster != "" {
				return poster
			}
		}
		for key, child := range typed {
			switch key {
			case "user", "pics", "pic_infos", "retweeted_status", "page_info", "media_info", "video", "playback_list", "play_info":
				continue
			}
			if poster := findWeiboVideoPoster(child); poster != "" {
				return poster
			}
		}
	}
	return ""
}

func collectWeiboVideos(value any, videos *[]PostVideo, indexes map[string]int) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectWeiboVideos(item, videos, indexes)
		}
	case map[string]any:
		videoURL := weiboDirectVideoURL(typed)
		if videoURL != "" {
			poster := weiboDirectVideoPoster(typed)
			if index, exists := indexes[videoURL]; exists {
				if (*videos)[index].Poster == "" && poster != "" {
					(*videos)[index].Poster = poster
				}
			} else {
				indexes[videoURL] = len(*videos)
				*videos = append(*videos, PostVideo{URL: videoURL, Poster: poster})
			}
		}
		for _, child := range typed {
			collectWeiboVideos(child, videos, indexes)
		}
	}
}

func collectWeiboPosts(value any, feed SourceConfig, posts *[]Post, seen map[string]bool) {
	if object, ok := value.(map[string]any); ok {
		mblog := object
		if nested, exists := object["mblog"].(map[string]any); exists {
			mblog = nested
		}
		user, hasUser := mblog["user"].(map[string]any)
		id := jsonValueString(mblog["id"])
		if hasUser && id != "" && id != "<nil>" && !seen[id] {
			seen[id] = true
			name := jsonValueString(user["screen_name"])
			avatar := normalizeRemoteImage(jsonValueString(user["avatar_hd"]))
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(user["avatar_large"]))
			}
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(user["profile_image_url"]))
			}
			if name == "" {
				name = feed.Name
			}
			if avatar == "" {
				avatar = feed.Avatar
			}
			published, parseErr := time.Parse("Mon Jan 02 15:04:05 -0700 2006", fmt.Sprint(mblog["created_at"]))
			if parseErr != nil {
				published = time.Now()
			}
			media := make([]string, 0)
			mediaIndexes := make(map[string]int)
			appendMedia := func(pictureID, image string) {
				image = normalizeWeiboOriginalImage(image)
				if image == "" || image == "<nil>" {
					return
				}
				key := strings.TrimSpace(pictureID)
				if key == "" {
					key = image
				}
				if _, exists := mediaIndexes[key]; exists {
					return
				}
				mediaIndexes[key] = len(media)
				media = append(media, image)
			}
			if pictures, ok := mblog["pics"].([]any); ok {
				for _, rawPicture := range pictures {
					picture, _ := rawPicture.(map[string]any)
					pictureID := firstNonEmptyRemoteText(jsonValueString(picture["pid"]), jsonValueString(picture["pic_id"]), jsonValueString(picture["id"]), jsonValueString(picture["pic_idstr"]))
					original, _ := picture["original"].(map[string]any)
					image := normalizeWeiboOriginalImage(jsonValueString(original["url"]))
					large, _ := picture["large"].(map[string]any)
					if image == "" {
						image = normalizeWeiboOriginalImage(jsonValueString(large["url"]))
					}
					if image == "" {
						image = normalizeWeiboOriginalImage(jsonValueString(picture["url"]))
					}
					appendMedia(pictureID, image)
				}
			}
			if picInfos, ok := mblog["pic_infos"].(map[string]any); ok {
				for pictureID, rawPicture := range picInfos {
					picture, _ := rawPicture.(map[string]any)
					original, _ := picture["original"].(map[string]any)
					image := normalizeWeiboOriginalImage(jsonValueString(original["url"]))
					if image == "" {
						large, _ := picture["large"].(map[string]any)
						image = normalizeWeiboOriginalImage(jsonValueString(large["url"]))
					}
					appendMedia(pictureID, image)
				}
			}
			videos := make([]PostVideo, 0, 1)
			collectWeiboVideos(mblog, &videos, make(map[string]int))
			videos = normalizePostVideos(videos)
			videoPoster := findWeiboVideoPoster(mblog)
			if videoPoster == "" && len(media) > 0 {
				videoPoster = media[0]
			}
			for index := range videos {
				if videos[index].Poster == "" {
					videos[index].Poster = videoPoster
				}
			}
			rawCaption := jsonValueString(mblog["text"])
			caption := jsonValueString(mblog["text_raw"])
			if caption == "" {
				caption = cleanRemoteText(rawCaption)
			}
			profileID := jsonValueString(user["idstr"])
			if profileID == "" {
				profileID = jsonValueString(user["id"])
			}
			originalURL := ""
			if profileID != "" {
				originalURL = "https://weibo.com/" + url.PathEscape(profileID) + "/" + url.PathEscape(id)
			}
			*posts = append(*posts, Post{ID: "weibo-status-" + id, Source: SourceWeibo, FeedIDs: []string{feed.ID}, Author: name, Avatar: avatar, Caption: caption, Tags: append([]string(nil), feed.Tags...), Media: media, Videos: videos, OriginalURL: originalURL, Published: published})
		}
		for _, child := range object {
			collectWeiboPosts(child, feed, posts, seen)
		}
	} else if list, ok := value.([]any); ok {
		for _, child := range list {
			collectWeiboPosts(child, feed, posts, seen)
		}
	}
}

func (b *BilibiliStore) fetchWeiboPosts(feed SourceConfig, full bool) ([]Post, error) {
	userID := strings.TrimSpace(strings.TrimPrefix(feed.ID, "weibo-"))
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" {
		return nil, errors.New("微博账号未连接")
	}
	endpointForPage := []func(int) string{
		func(page int) string {
			return fmt.Sprintf("https://weibo.com/ajax/statuses/mymblog?uid=%s&page=%d&feature=0", url.QueryEscape(userID), page)
		},
		func(page int) string {
			return fmt.Sprintf("https://weibo.com/ajax/statuses/mymblog?uid=%s&page=%d&feature=0&profile_ftype=1", url.QueryEscape(userID), page)
		},
		func(page int) string {
			return fmt.Sprintf("https://m.weibo.cn/api/container/getIndex?type=uid&value=%s&containerid=%s&page=%d", url.QueryEscape(userID), url.QueryEscape("107603"+userID), page)
		},
	}
	var lastErr error
	for _, makeEndpoint := range endpointForPage {
		posts := make([]Post, 0)
		seen := make(map[string]bool)
		for page := 1; page <= 100; page++ {
			payload, _, err := weiboRawRequest(makeEndpoint(page), credentials, proxyURL)
			if err != nil {
				lastErr = err
				break
			}
			pagePosts := make([]Post, 0)
			collectWeiboPosts(payload, feed, &pagePosts, seen)
			if len(pagePosts) == 0 {
				break
			}
			posts = append(posts, pagePosts...)
			if !full && feed.LastSyncedAt.IsZero() {
				break
			}
			if !full && !feed.LastSyncedAt.IsZero() {
				reachedBoundary := false
				for _, post := range pagePosts {
					if !post.Published.After(feed.LastSyncedAt) {
						reachedBoundary = true
						break
					}
				}
				if reachedBoundary {
					break
				}
			}
		}
		if len(posts) > 0 {
			return posts, nil
		}
		if lastErr == nil {
			lastErr = errors.New("接口未返回该博主的动态")
		}
	}
	return nil, fmt.Errorf("微博拉取失败：%w", lastErr)
}

func (b *BilibiliStore) fetchWeiboLikedPosts(feed SourceConfig, full bool) ([]Post, error) {
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" || credentials.UserID == "" {
		return nil, errors.New("微博账号未连接")
	}
	if feed.ID == "" {
		feed = SourceConfig{ID: "weibo-likes-" + credentials.UserID, Source: SourceWeibo, Name: "我的点赞", Handle: credentials.UserName, Avatar: credentials.Avatar}
	}
	endpoints := []func(int) string{
		func(page int) string {
			return fmt.Sprintf("https://weibo.com/ajax/statuses/likelist?uid=%s&page=%d&relate=fans", url.QueryEscape(credentials.UserID), page)
		},
		func(page int) string {
			return fmt.Sprintf("https://weibo.com/ajax/profile/likelist?uid=%s&page=%d", url.QueryEscape(credentials.UserID), page)
		},
		func(page int) string {
			return fmt.Sprintf("https://m.weibo.cn/api/container/getIndex?containerid=%s&page=%d", url.QueryEscape("230869"+credentials.UserID), page)
		},
	}
	var lastErr error
	pageLimit := 1
	if full {
		pageLimit = 50
	}
	for _, endpoint := range endpoints {
		posts, seen := make([]Post, 0), make(map[string]bool)
		for page := 1; page <= pageLimit; page++ {
			payload, _, err := weiboRawRequest(endpoint(page), credentials, proxyURL)
			if err != nil {
				lastErr = err
				break
			}
			pagePosts := make([]Post, 0)
			collectWeiboPosts(payload, feed, &pagePosts, seen)
			if len(pagePosts) == 0 {
				break
			}
			posts = append(posts, pagePosts...)
		}
		if len(posts) > 0 {
			return posts, nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("微博接口未返回点赞动态，请确认当前 Cookie 仍有效")
	}
	return nil, fmt.Errorf("微博点赞拉取失败：%w", lastErr)
}

func (b *BilibiliStore) weiboLikesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	posts, err := b.fetchWeiboLikedPosts(SourceConfig{}, false)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	added, err := b.content.mergePosts(posts)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存微博点赞动态")
		return
	}
	writeJSON(w, map[string]any{"status": "success", "added": added, "count": len(posts), "message": fmt.Sprintf("已同步 %d 条微博点赞动态", len(posts))})
}

func postsAfter(posts []Post, boundary time.Time) []Post {
	if boundary.IsZero() {
		return posts
	}
	filtered := make([]Post, 0, len(posts))
	for _, post := range posts {
		if post.Published.After(boundary) {
			filtered = append(filtered, post)
		}
	}
	return filtered
}

func (s *Store) postsAfterOrCaptionRepair(posts []Post, boundary time.Time) []Post {
	if boundary.IsZero() {
		return posts
	}
	s.RLock()
	existing := make(map[string]Post, len(s.posts))
	for _, post := range s.posts {
		existing[post.ID] = post
	}
	s.RUnlock()
	filtered := make([]Post, 0, len(posts))
	for _, post := range posts {
		if post.Published.After(boundary) {
			filtered = append(filtered, post)
			continue
		}
		stored, ok := existing[post.ID]
		incomingCaption := cleanRemoteText(post.Caption)
		storedCaption := cleanRemoteText(stored.Caption)
		if !ok || incomingCaption == "" || incomingCaption == storedCaption || (storedCaption != "" && len([]rune(incomingCaption)) <= len([]rune(storedCaption))) {
			continue
		}
		if len(stored.Media) > 0 {
			post.Media = append([]string(nil), stored.Media...)
		}
		if len(stored.Videos) > 0 {
			post.Videos = append([]PostVideo(nil), stored.Videos...)
		}
		post.TextFile = stored.TextFile
		post.Emojis = nil
		if strings.HasPrefix(stored.Avatar, "/flow/") {
			post.Avatar = stored.Avatar
		}
		post.Liked = stored.Liked
		post.FavoriteExplicit = stored.FavoriteExplicit
		post.FeedIDs = mergeUniqueStrings(stored.FeedIDs, post.FeedIDs)
		filtered = append(filtered, post)
	}
	return filtered
}

func (s *Store) postsAfterOrSourceMembership(posts []Post, boundary time.Time, feedID string) []Post {
	if boundary.IsZero() {
		return posts
	}
	s.RLock()
	existing := make(map[string]Post, len(s.posts))
	for _, post := range s.posts {
		existing[post.ID] = post
	}
	s.RUnlock()
	filtered := make([]Post, 0, len(posts))
	for _, post := range posts {
		if post.Published.After(boundary) {
			filtered = append(filtered, post)
			continue
		}
		stored, ok := existing[post.ID]
		if !ok {
			post.FeedIDs = mergeUniqueStrings(post.FeedIDs, []string{feedID})
			filtered = append(filtered, post)
			continue
		}
		if containsString(stored.FeedIDs, feedID) {
			continue
		}
		if len(stored.Media) > 0 {
			post.Media = append([]string(nil), stored.Media...)
		}
		if len(stored.Videos) > 0 {
			post.Videos = append([]PostVideo(nil), stored.Videos...)
		}
		post.TextFile = stored.TextFile
		post.Emojis = nil
		if strings.HasPrefix(stored.Avatar, "/flow/") {
			post.Avatar = stored.Avatar
		}
		post.Liked = stored.Liked
		post.FavoriteExplicit = stored.FavoriteExplicit
		post.FeedIDs = mergeUniqueStrings(stored.FeedIDs, post.FeedIDs, []string{feedID})
		filtered = append(filtered, post)
	}
	return filtered
}

func localFlowFileAvailable(value string) bool {
	localPath, ok := localFlowArchivePath(value)
	if !ok {
		return false
	}
	info, err := os.Stat(localPath)
	return err == nil && !info.IsDir()
}

func reuseAvailablePostArchive(incoming, stored Post) (Post, bool) {
	prepared := incoming
	complete := true
	if len(incoming.Media) == 0 && len(stored.Media) > 0 {
		prepared.Media = append([]string(nil), stored.Media...)
		for _, media := range prepared.Media {
			if !localFlowFileAvailable(media) {
				complete = false
			}
		}
	} else {
		for index := range prepared.Media {
			if index < len(stored.Media) && localFlowFileAvailable(stored.Media[index]) {
				prepared.Media[index] = stored.Media[index]
			} else {
				complete = false
			}
		}
		for index := len(prepared.Media); index < len(stored.Media); index++ {
			if localFlowFileAvailable(stored.Media[index]) {
				prepared.Media = append(prepared.Media, stored.Media[index])
			}
		}
	}
	if len(incoming.Videos) == 0 && len(stored.Videos) > 0 {
		prepared.Videos = append([]PostVideo(nil), stored.Videos...)
		for _, video := range prepared.Videos {
			if !localFlowFileAvailable(video.URL) || (video.Poster != "" && !localFlowFileAvailable(video.Poster)) {
				complete = false
			}
		}
	} else {
		for index := range prepared.Videos {
			if index >= len(stored.Videos) {
				complete = false
				continue
			}
			storedVideo := stored.Videos[index]
			if localFlowFileAvailable(storedVideo.URL) {
				prepared.Videos[index].URL = storedVideo.URL
			} else {
				complete = false
			}
			if prepared.Videos[index].Poster != "" {
				if localFlowFileAvailable(storedVideo.Poster) {
					prepared.Videos[index].Poster = storedVideo.Poster
				} else {
					complete = false
				}
			}
		}
		for index := len(prepared.Videos); index < len(stored.Videos); index++ {
			storedVideo := stored.Videos[index]
			if localFlowFileAvailable(storedVideo.URL) && (storedVideo.Poster == "" || localFlowFileAvailable(storedVideo.Poster)) {
				prepared.Videos = append(prepared.Videos, storedVideo)
			}
		}
	}
	if stored.TextFile != "" && localFlowFileAvailable(stored.TextFile) {
		prepared.TextFile = stored.TextFile
	} else if cleanRemoteText(incoming.Caption) != "" {
		complete = false
	}
	if strings.HasPrefix(stored.Avatar, "/flow/") && localFlowFileAvailable(stored.Avatar) {
		prepared.Avatar = stored.Avatar
	}
	if cleanRemoteText(stored.Caption) != "" && len([]rune(cleanRemoteText(stored.Caption))) >= len([]rune(cleanRemoteText(incoming.Caption))) {
		prepared.Caption = stored.Caption
	}
	if prepared.OriginalURL == "" {
		prepared.OriginalURL = stored.OriginalURL
	}
	prepared.Liked = stored.Liked
	prepared.FavoriteExplicit = stored.FavoriteExplicit
	prepared.FeedIDs = mergeUniqueStrings(stored.FeedIDs, incoming.FeedIDs)
	return prepared, complete
}

func historyPostNeedsMetadataUpdate(stored, incoming Post, feed SourceConfig) bool {
	if !containsString(stored.FeedIDs, feed.ID) {
		return true
	}
	storedCaption, incomingCaption := cleanRemoteText(stored.Caption), cleanRemoteText(incoming.Caption)
	if incomingCaption != "" && len([]rune(incomingCaption)) > len([]rune(storedCaption)) {
		return true
	}
	if stored.OriginalURL == "" && incoming.OriginalURL != "" {
		return true
	}
	if stored.Avatar == "" && incoming.Avatar != "" {
		return true
	}
	for _, tag := range feed.Tags {
		if !containsString(stored.Tags, tag) {
			return true
		}
	}
	return false
}

func (s *Store) postsForHistoryBackfill(posts []Post, feed SourceConfig) ([]Post, int) {
	s.RLock()
	existing := make(map[string]Post, len(s.posts))
	for _, post := range s.posts {
		existing[post.ID] = post
	}
	s.RUnlock()

	pending := make([]Post, 0, len(posts))
	skipped := 0
	for _, incoming := range posts {
		stored, ok := existing[incoming.ID]
		if !ok {
			pending = append(pending, incoming)
			continue
		}
		prepared, archiveComplete := reuseAvailablePostArchive(incoming, stored)
		if postBelongsToSource(&stored, feed) && archiveComplete && !historyPostNeedsMetadataUpdate(stored, incoming, feed) {
			skipped++
			continue
		}
		pending = append(pending, prepared)
	}
	return pending, skipped
}

func (b *BilibiliStore) syncSource(feed SourceConfig, full bool) (SourceConfig, error) {
	originalID := feed.ID
	b.RLock()
	feed = collectionFeedWithAccountProfile(feed, b.config)
	b.RUnlock()
	if feed.Source == SourceBilibili && !isBilibiliFavoriteOpusFeed(feed) {
		userID := strings.TrimPrefix(feed.ID, "bili-")
		if _, err := strconv.ParseUint(userID, 10, 64); err != nil || len(userID) > 15 || strings.TrimSpace(feed.Avatar) == "" {
			if repaired, repairErr := b.findBilibiliUser(feed.Name); repairErr == nil {
				feed.ID = "bili-" + repaired.UserID
				feed.Handle = "UID " + repaired.UserID
				feed.Avatar = repaired.Avatar
			}
		}
	}
	if feed.Source == SourceWeibo && !strings.HasPrefix(feed.ID, "weibo-likes-") {
		userID := strings.TrimSpace(strings.TrimPrefix(feed.ID, "weibo-"))
		b.RLock()
		credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
		b.RUnlock()
		if userID != "" && credentials.Cookie != "" {
			if user, repairErr := fetchWeiboUser(userID, credentials, proxyURL); repairErr == nil {
				if strings.TrimSpace(user.Name) != "" {
					feed.Name = user.Name
				}
				if strings.TrimSpace(user.Avatar) != "" {
					feed.Avatar = user.Avatar
				}
			}
		}
	}
	var posts []Post
	var err error
	switch feed.Source {
	case SourceBilibili:
		if isBilibiliFavoriteOpusFeed(feed) {
			posts, err = b.fetchBilibiliFavoriteOpusPosts(feed, full)
		} else {
			posts, err = b.fetchBilibiliPosts(feed, full)
		}
	case SourceWeibo:
		if strings.HasPrefix(feed.ID, "weibo-likes-") {
			posts, err = b.fetchWeiboLikedPosts(feed, full)
		} else {
			posts, err = b.fetchWeiboPosts(feed, full)
		}
	case SourcePixiv:
		posts, err = b.fetchPixivPosts(feed, full)
	case SourceTwitter:
		if strings.HasPrefix(feed.ID, "twitter-likes-") {
			posts, err = b.fetchTwitterLikedPosts(feed, full)
		} else {
			err = errors.New("当前仅支持拉取登录账号点赞的推特动态")
		}
	default:
		err = errors.New("该来源尚未配置采集器")
	}
	added, historyEligible, historyPending, historySkipped := 0, 0, 0, 0
	if err == nil {
		if !full {
			if isBilibiliFavoriteOpusFeed(feed) && b.content != nil {
				posts = b.content.postsAfterOrSourceMembership(posts, feed.LastSyncedAt, feed.ID)
			} else if feed.Source == SourceBilibili && b.content != nil {
				posts = b.content.postsAfterOrCaptionRepair(posts, feed.LastSyncedAt)
			} else if isCollectionFeed(feed) && b.content != nil {
				posts = b.content.postsAfterOrSourceMembership(posts, feed.LastSyncedAt, feed.ID)
			} else {
				posts = postsAfter(posts, feed.LastSyncedAt)
			}
		}
		posts = filterSourcePosts(posts, feed)
		if full {
			historyEligible = len(posts)
			if b.content != nil {
				posts, historySkipped = b.content.postsForHistoryBackfill(posts, feed)
			}
			historyPending = len(posts)
		}
		if len(posts) > 0 && !strings.HasPrefix(feed.ID, "weibo-likes-") && !strings.HasPrefix(feed.ID, "pixiv-bookmarks-") && !strings.HasPrefix(feed.ID, "twitter-likes-") && !isBilibiliFavoriteOpusFeed(feed) {
			if strings.TrimSpace(posts[0].Author) != "" {
				feed.Name = posts[0].Author
			}
			if strings.TrimSpace(posts[0].Avatar) != "" {
				feed.Avatar = posts[0].Avatar
			}
		}
		feed.Name = canonicalSourceName(feed)
		b.RLock()
		feed, posts, err = b.archiveSourceContent(feed, posts)
		b.RUnlock()
		if err == nil && b.content == nil {
			err = errors.New("动态存储未初始化")
		} else if err == nil {
			added, err = b.content.mergePosts(posts)
			if err == nil && isCollectionFeed(feed) {
				err = b.content.reconcileCollectionMedia(feed, b.allSourceConfigs())
			}
		}
	}
	now := time.Now()
	b.Lock()
	lists := []*[]SourceConfig{&b.config.Subscriptions, &b.config.WeiboSubscriptions, &b.config.PixivSubscriptions, &b.config.TwitterSubscriptions}
	for _, list := range lists {
		for index := range *list {
			if (*list)[index].ID != feed.ID && (*list)[index].ID != originalID {
				continue
			}
			(*list)[index].ID = feed.ID
			(*list)[index].Name = feed.Name
			(*list)[index].Handle = feed.Handle
			if feed.Avatar != "" {
				(*list)[index].Avatar = feed.Avatar
			}
			(*list)[index].LastSyncedAt = now
			(*list)[index].LastSyncCount = added
			if err != nil {
				(*list)[index].LastSyncStatus = "failed"
				(*list)[index].LastSyncMessage = err.Error()
			} else {
				(*list)[index].LastSyncStatus = "success"
				if full {
					repaired := historyPending - added
					if repaired < 0 {
						repaired = 0
					}
					(*list)[index].LastSyncMessage = fmt.Sprintf("历史动态获取完成：符合设置 %d 条，新增 %d 条，修复或补充 %d 条，跳过 %d 条本地已完整归档动态", historyEligible, added, repaired, historySkipped)
				} else {
					(*list)[index].LastSyncMessage = fmt.Sprintf("增量拉取完成，新增 %d 条动态", added)
				}
			}
			feed = (*list)[index]
		}
	}
	b.Unlock()
	if saveErr := b.save(); saveErr != nil && err == nil {
		err = saveErr
	}
	return feed, err
}

func (b *BilibiliStore) syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.Lock()
	if b.syncInProgress {
		b.Unlock()
		writeJSON(w, map[string]any{"status": "running", "message": "正在后台拉取最新动态，请稍后刷新查看。"})
		return
	}
	b.syncInProgress = true
	b.Unlock()

	go func() {
		defer func() {
			b.Lock()
			b.syncInProgress = false
			b.Unlock()
		}()
		b.RLock()
		feeds := append(append(append(append([]SourceConfig{}, b.config.Subscriptions...), b.config.WeiboSubscriptions...), b.config.PixivSubscriptions...), b.config.TwitterSubscriptions...)
		b.RUnlock()
		for _, feed := range feeds {
			if feed.Enabled {
				_, _ = b.syncSource(feed, false)
			}
		}
	}()
	writeJSON(w, map[string]any{"status": "started", "message": "已开始在后台拉取最新动态。"})
}

func (b *BilibiliStore) runScheduledSync(at time.Time) {
	b.RLock()
	feeds := append(append(append(append([]SourceConfig{}, b.config.Subscriptions...), b.config.WeiboSubscriptions...), b.config.PixivSubscriptions...), b.config.TwitterSubscriptions...)
	b.RUnlock()
	for _, feed := range feeds {
		if !feed.Enabled || !cronMatches(normalizeSchedule(feed.Schedule), at) {
			continue
		}
		if !feed.LastSyncedAt.IsZero() && feed.LastSyncedAt.Year() == at.Year() && feed.LastSyncedAt.YearDay() == at.YearDay() && feed.LastSyncedAt.Hour() == at.Hour() && feed.LastSyncedAt.Minute() == at.Minute() {
			continue
		}
		_, _ = b.syncSource(feed, false)
	}
}

func (b *BilibiliStore) startScheduler() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for at := range ticker.C {
			b.runScheduledSync(at)
		}
	}()
}

func setPixivBrowserHeaders(request *http.Request, credentials PixivCredentials) {
	request.Header.Set("User-Agent", strings.TrimSpace(credentials.UserAgent))
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Referer", "https://www.pixiv.net/users/"+url.PathEscape(strings.TrimSpace(credentials.UserID)))
	request.Header.Set("Cookie", strings.TrimSpace(credentials.Cookie))
	if value := strings.TrimSpace(credentials.Baggage); value != "" {
		request.Header.Set("Baggage", value)
	}
	if value := strings.TrimSpace(credentials.SentryTrace); value != "" {
		request.Header.Set("Sentry-Trace", value)
	}
	if value := strings.TrimSpace(credentials.CSRFToken); value != "" {
		request.Header.Set("X-CSRF-Token", value)
	}
}

func pixivBrowserCredentialsRequest(credentials PixivCredentials, proxyURL, baseURL string) (PixivCredentials, error) {
	credentials.UserAgent = strings.TrimSpace(credentials.UserAgent)
	credentials.Baggage = strings.TrimSpace(credentials.Baggage)
	credentials.Cookie = strings.TrimSpace(credentials.Cookie)
	credentials.SentryTrace = strings.TrimSpace(credentials.SentryTrace)
	credentials.CSRFToken = strings.TrimSpace(credentials.CSRFToken)
	credentials.UserID = strings.TrimSpace(credentials.UserID)
	if credentials.UserAgent == "" || credentials.Cookie == "" || credentials.UserID == "" {
		return PixivCredentials{}, errors.New("请填写浏览器 UA、Cookie 和用户 ID")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/ajax/user/" + url.PathEscape(credentials.UserID) + "?full=1&lang=zh"
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return PixivCredentials{}, err
	}
	setPixivBrowserHeaders(request, credentials)
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return PixivCredentials{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return PixivCredentials{}, fmt.Errorf("无法连接 Pixiv：%w", err)
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return PixivCredentials{}, errors.New("Pixiv 用户接口响应无法解析")
	}
	if response.StatusCode != http.StatusOK {
		return PixivCredentials{}, errors.New("Pixiv 浏览器凭证无效或已过期")
	}
	if failed, _ := payload["error"].(bool); failed {
		message := strings.TrimSpace(jsonValueString(payload["message"]))
		if message == "" {
			message = "Pixiv 浏览器凭证无效或已过期"
		}
		return PixivCredentials{}, errors.New(message)
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		return PixivCredentials{}, errors.New("Pixiv 用户接口未返回账号资料")
	}
	userID := strings.TrimSpace(jsonValueString(body["userId"]))
	if userID == "" {
		return PixivCredentials{}, errors.New("Pixiv 用户接口未返回用户 ID")
	}
	if userID != credentials.UserID {
		return PixivCredentials{}, errors.New("Pixiv 用户 ID 与浏览器登录账号不一致")
	}
	credentials.UserName = strings.TrimSpace(jsonValueString(body["name"]))
	credentials.Avatar = normalizeRemoteImage(jsonValueString(body["imageBig"]))
	if credentials.Avatar == "" {
		credentials.Avatar = normalizeRemoteImage(jsonValueString(body["image"]))
	}
	credentials.RefreshToken, credentials.ClientID, credentials.ClientSecret = "", "", ""
	return credentials, nil
}

func (b *BilibiliStore) pixivHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	proxyURL, current := b.config.ProxyURL, b.config.Pixiv
	b.RUnlock()
	if r.Method == http.MethodGet {
		current.Avatar = normalizeRemoteImage(current.Avatar)
		if current.Cookie != "" && current.UserID != "" && (current.UserName == "" || current.Avatar == "") {
			// 账号资料补全可能受 Pixiv 网络或代理影响，不能阻塞打开订阅页面。
			go func(credentials PixivCredentials, proxy string) {
				enriched, err := pixivBrowserCredentialsRequest(credentials, proxy, "https://www.pixiv.net")
				if err != nil {
					return
				}
				b.Lock()
				if b.config.Pixiv.Cookie == credentials.Cookie && b.config.Pixiv.UserID == credentials.UserID {
					b.config.Pixiv = enriched
				}
				b.Unlock()
				_ = b.save()
			}(current, proxyURL)
		}
		if current.Avatar != "" {
			cachedAvatar := cacheAccountAvatar(SourcePixiv, pixivBookmarksName, current.Avatar, proxyURL, "https://www.pixiv.net/", current.Cookie)
			if cachedAvatar != current.Avatar {
				current.Avatar = cachedAvatar
				b.Lock()
				if b.config.Pixiv.Cookie == current.Cookie && b.config.Pixiv.UserID == current.UserID {
					b.config.Pixiv.Avatar = cachedAvatar
				}
				b.Unlock()
				_ = b.save()
			}
		}
		writeJSON(w, map[string]any{"configured": current.Cookie != "" && current.UserID != "", "userId": current.UserID, "userName": current.UserName, "avatar": current.Avatar})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input PixivCredentials
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "Pixiv 凭证格式无效")
		return
	}
	credentials, err := pixivBrowserCredentialsRequest(input, proxyURL, "https://www.pixiv.net")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.Lock()
	b.config.Pixiv = credentials
	b.Unlock()
	if err := b.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存 Pixiv 凭证")
		return
	}
	writeJSON(w, map[string]any{"configured": true, "userId": credentials.UserID, "userName": credentials.UserName, "avatar": credentials.Avatar})
}

func pixivRequestJSON(client *http.Client, endpoint string, credentials PixivCredentials, target any) error {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	setPixivBrowserHeaders(request, credentials)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Pixiv HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(target); err != nil {
		return err
	}
	return nil
}

func fetchPixivUserProfile(client *http.Client, credentials PixivCredentials, userID string) (BilibiliUser, error) {
	var payload map[string]any
	endpoint := "https://www.pixiv.net/ajax/user/" + url.PathEscape(userID) + "?full=1&lang=zh"
	if err := pixivRequestJSON(client, endpoint, credentials, &payload); err != nil {
		return BilibiliUser{}, err
	}
	if pixivPayloadFailed(payload) {
		return BilibiliUser{}, errors.New("Pixiv 用户接口拒绝访问")
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		return BilibiliUser{}, errors.New("Pixiv 用户资料无效")
	}
	name := firstNonEmptyRemoteText(jsonValueString(body["name"]), "Pixiv 画师 "+userID)
	avatar := firstNonEmptyRemoteText(jsonValueString(body["imageBig"]), jsonValueString(body["image"]))
	return BilibiliUser{UserID: userID, Name: name, Avatar: normalizeRemoteImage(avatar), Description: cleanRemoteText(jsonValueString(body["comment"]))}, nil
}

func pixivNumericID(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func collectPixivIDs(value any, ids *[]string, seen map[string]bool) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectPixivIDs(item, ids, seen)
		}
	case map[string]any:
		for key, item := range typed {
			if key == "illusts" || key == "manga" {
				if grouped, ok := item.(map[string]any); ok {
					for id := range grouped {
						if pixivNumericID(id) && !seen[id] {
							seen[id] = true
							*ids = append(*ids, id)
						}
					}
				}
			}
			if key == "id" {
				id := jsonValueString(item)
				if pixivNumericID(id) && !seen[id] {
					seen[id] = true
					*ids = append(*ids, id)
				}
			}
			collectPixivIDs(item, ids, seen)
		}
	}
}

func parsePixivTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05-07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Now()
}

func pixivOriginalURLs(client *http.Client, credentials PixivCredentials, id string, detail map[string]any) []string {
	urls := make([]string, 0)
	pageCount := 0
	if body, ok := detail["body"].(map[string]any); ok {
		if urlsMap, ok := body["urls"].(map[string]any); ok {
			if original := normalizeRemoteImage(jsonValueString(urlsMap["original"])); original != "" {
				urls = append(urls, original)
			}
		}
		pageCount, _ = strconv.Atoi(jsonValueString(body["pageCount"]))
	}
	// 单图作品的详情已经包含原图地址，无需再请求 pages 接口。
	if pageCount == 1 && len(urls) > 0 {
		return urls
	}
	var pages map[string]any
	endpoint := "https://www.pixiv.net/ajax/illust/" + url.PathEscape(id) + "/pages?lang=zh"
	if pixivRequestJSON(client, endpoint, credentials, &pages) == nil {
		if body, ok := pages["body"].([]any); ok {
			for _, page := range body {
				if item, ok := page.(map[string]any); ok {
					if urlsMap, ok := item["urls"].(map[string]any); ok {
						if original := normalizeRemoteImage(jsonValueString(urlsMap["original"])); original != "" {
							urls = append(urls, original)
						}
					}
				}
			}
		}
	}
	return mergeUniqueStrings(urls)
}

func pixivRequestDelay(sequence int) time.Duration {
	return 360*time.Millisecond + time.Duration(sequence%3)*90*time.Millisecond
}

func pixivPayloadFailed(payload map[string]any) bool {
	failed, _ := payload["error"].(bool)
	return failed
}

func (b *BilibiliStore) fetchPixivPosts(feed SourceConfig, full bool) ([]Post, error) {
	b.RLock()
	credentials, proxyURL := b.config.Pixiv, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" || credentials.UserID == "" {
		return nil, errors.New("Pixiv 账号未连接")
	}
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	if strings.HasPrefix(feed.ID, "pixiv-bookmarks-") {
		seen := make(map[string]bool)
		pageLimit := 1
		if full {
			pageLimit = 40
		}
		for page := 0; page < pageLimit; page++ {
			offset := page * 48
			endpoint := "https://www.pixiv.net/ajax/user/" + url.PathEscape(credentials.UserID) + "/illusts/bookmarks?tag=&offset=" + strconv.Itoa(offset) + "&limit=48&rest=show&lang=zh"
			var payload map[string]any
			if err := pixivRequestJSON(client, endpoint, credentials, &payload); err != nil {
				return nil, fmt.Errorf("Pixiv 收藏请求失败: %w", err)
			}
			if pixivPayloadFailed(payload) {
				return nil, errors.New("Pixiv 收藏接口拒绝访问")
			}
			before := len(ids)
			collectPixivIDs(payload["body"], &ids, seen)
			if len(ids) == before || len(ids)-before < 48 {
				break
			}
			if page+1 < pageLimit {
				time.Sleep(250 * time.Millisecond)
			}
		}
	} else {
		userID := strings.TrimPrefix(feed.ID, "pixiv-")
		endpoint := "https://www.pixiv.net/ajax/user/" + url.PathEscape(userID) + "/profile/all?lang=zh"
		var payload map[string]any
		if err := pixivRequestJSON(client, endpoint, credentials, &payload); err != nil {
			return nil, fmt.Errorf("Pixiv 画师请求失败: %w", err)
		}
		if pixivPayloadFailed(payload) {
			return nil, errors.New("Pixiv 画师接口拒绝访问")
		}
		collectPixivIDs(payload["body"], &ids, make(map[string]bool))
	}
	sort.SliceStable(ids, func(i, j int) bool {
		left, _ := strconv.ParseUint(ids[i], 10, 64)
		right, _ := strconv.ParseUint(ids[j], 10, 64)
		return left > right
	})
	if !full && len(ids) > 24 {
		ids = ids[:24]
	}
	posts := make([]Post, 0, len(ids))
	for index, id := range ids {
		var detail map[string]any
		if err := pixivRequestJSON(client, "https://www.pixiv.net/ajax/illust/"+url.PathEscape(id)+"?lang=zh", credentials, &detail); err != nil {
			if index+1 < len(ids) {
				time.Sleep(pixivRequestDelay(index))
			}
			continue
		}
		body, ok := detail["body"].(map[string]any)
		if !ok {
			continue
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
			if index+1 < len(ids) {
				time.Sleep(pixivRequestDelay(index))
			}
			continue
		}
		published := parsePixivTime(jsonValueString(body["createDate"]))
		posts = append(posts, Post{ID: "pixiv-illust-" + id, Source: SourcePixiv, FeedIDs: []string{feed.ID}, Author: author, Avatar: avatar, Caption: caption, Tags: append([]string(nil), feed.Tags...), Media: media, OriginalURL: "https://www.pixiv.net/artworks/" + id, Published: published})
		if index+1 < len(ids) {
			time.Sleep(pixivRequestDelay(index))
		}
	}
	return posts, nil
}

const twitterAPIBase = "https://api.x.com/2"

type twitterUserResponse struct {
	Data struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Username        string `json:"username"`
		ProfileImageURL string `json:"profile_image_url"`
	} `json:"data"`
}

type twitterLikedTweetsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		Text        string `json:"text"`
		CreatedAt   string `json:"created_at"`
		AuthorID    string `json:"author_id"`
		Attachments struct {
			MediaKeys []string `json:"media_keys"`
		} `json:"attachments"`
	} `json:"data"`
	Includes struct {
		Media []struct {
			MediaKey        string `json:"media_key"`
			Type            string `json:"type"`
			URL             string `json:"url"`
			PreviewImageURL string `json:"preview_image_url"`
			Variants        []struct {
				URL         string `json:"url"`
				ContentType string `json:"content_type"`
				BitRate     int64  `json:"bit_rate"`
			} `json:"variants"`
		} `json:"media"`
		Users []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Username        string `json:"username"`
			ProfileImageURL string `json:"profile_image_url"`
		} `json:"users"`
	} `json:"includes"`
	Meta struct {
		NextToken string `json:"next_token"`
	} `json:"meta"`
}

func twitterRequest(client *http.Client, endpoint string, credentials TwitterCredentials, target any) error {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(credentials.AccessToken))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Lumic/1.0")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
			Errors []struct {
				Message string `json:"message"`
				Detail  string `json:"detail"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(body, &failure)
		message := firstNonEmptyRemoteText(failure.Detail, failure.Title)
		if len(failure.Errors) > 0 {
			message = firstNonEmptyRemoteText(failure.Errors[0].Detail, failure.Errors[0].Message, message)
		}
		if message == "" {
			message = responseSummary(body)
		}
		if message == "" {
			message = "X API 请求失败"
		}
		return fmt.Errorf("X API 返回 HTTP %d：%s", response.StatusCode, message)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("X API 返回了无效响应：%w", err)
	}
	return nil
}

func normalizeTwitterAvatar(value string) string {
	value = normalizeRemoteImage(value)
	return strings.Replace(value, "_normal.", "_400x400.", 1)
}

func validateTwitterCredentials(accessToken, proxyURL string) (TwitterCredentials, error) {
	credentials := TwitterCredentials{AccessToken: strings.TrimSpace(accessToken)}
	if credentials.AccessToken == "" {
		return TwitterCredentials{}, errors.New("请填写 X OAuth 2.0 用户访问令牌")
	}
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return TwitterCredentials{}, err
	}
	var response twitterUserResponse
	if err := twitterRequest(client, twitterAPIBase+"/users/me?user.fields=profile_image_url", credentials, &response); err != nil {
		return TwitterCredentials{}, fmt.Errorf("推特访问令牌验证失败：%w", err)
	}
	credentials.UserID = strings.TrimSpace(response.Data.ID)
	credentials.UserName = strings.TrimSpace(response.Data.Username)
	credentials.Avatar = normalizeTwitterAvatar(response.Data.ProfileImageURL)
	if credentials.UserID == "" || credentials.UserName == "" {
		return TwitterCredentials{}, errors.New("X API 未返回当前账号资料，请确认令牌使用 OAuth 2.0 用户授权并具有 users.read 权限")
	}
	return credentials, nil
}

func parseTwitterTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	return time.Now()
}

func twitterVideoURL(variants []struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	BitRate     int64  `json:"bit_rate"`
}) string {
	selected := ""
	var selectedBitRate int64 = -1
	for _, variant := range variants {
		if !strings.EqualFold(variant.ContentType, "video/mp4") || strings.TrimSpace(variant.URL) == "" {
			continue
		}
		if variant.BitRate >= selectedBitRate {
			selected, selectedBitRate = variant.URL, variant.BitRate
		}
	}
	return selected
}

func (b *BilibiliStore) fetchTwitterLikedPosts(feed SourceConfig, full bool) ([]Post, error) {
	b.RLock()
	credentials, proxyURL := b.config.Twitter, b.config.ProxyURL
	b.RUnlock()
	if credentials.AccessToken == "" || credentials.UserID == "" {
		return nil, errors.New("推特账号未连接")
	}
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	if feed.ID == "" {
		feed = SourceConfig{ID: "twitter-likes-" + credentials.UserID, Source: SourceTwitter, Name: twitterLikesName, Handle: "@" + credentials.UserName, Avatar: credentials.Avatar}
	}
	posts := make([]Post, 0)
	seen := make(map[string]bool)
	nextToken := ""
	pageLimit := 1
	if full {
		pageLimit = 30
	}
	for page := 0; page < pageLimit; page++ {
		query := url.Values{
			"max_results":  {"100"},
			"tweet.fields": {"created_at,attachments,author_id"},
			"expansions":   {"attachments.media_keys,author_id"},
			"media.fields": {"url,preview_image_url,type,variants"},
			"user.fields":  {"name,username,profile_image_url"},
		}
		if nextToken != "" {
			query.Set("pagination_token", nextToken)
		}
		var payload twitterLikedTweetsResponse
		endpoint := twitterAPIBase + "/users/" + url.PathEscape(credentials.UserID) + "/liked_tweets?" + query.Encode()
		if err := twitterRequest(client, endpoint, credentials, &payload); err != nil {
			return nil, fmt.Errorf("推特点赞拉取失败：%w", err)
		}
		users := make(map[string]struct {
			Name     string
			Username string
			Avatar   string
		})
		for _, user := range payload.Includes.Users {
			users[user.ID] = struct {
				Name     string
				Username string
				Avatar   string
			}{Name: cleanRemoteText(user.Name), Username: strings.TrimSpace(user.Username), Avatar: normalizeTwitterAvatar(user.ProfileImageURL)}
		}
		media := make(map[string]struct {
			Kind   string
			URL    string
			Poster string
		})
		for _, item := range payload.Includes.Media {
			kind := strings.ToLower(strings.TrimSpace(item.Type))
			if kind == "photo" && item.URL != "" {
				media[item.MediaKey] = struct{ Kind, URL, Poster string }{Kind: kind, URL: normalizeRemoteImage(item.URL)}
				continue
			}
			if kind == "video" || kind == "animated_gif" {
				videoURL := twitterVideoURL(item.Variants)
				if videoURL != "" {
					media[item.MediaKey] = struct{ Kind, URL, Poster string }{Kind: kind, URL: videoURL, Poster: normalizeRemoteImage(item.PreviewImageURL)}
				}
			}
		}
		reachedBoundary := false
		for _, tweet := range payload.Data {
			if tweet.ID == "" || seen[tweet.ID] {
				continue
			}
			seen[tweet.ID] = true
			user := users[tweet.AuthorID]
			author := firstNonEmptyRemoteText(user.Name, "@"+user.Username, feed.Name)
			if user.Username == "" {
				user.Username = strings.TrimPrefix(credentials.UserName, "@")
			}
			post := Post{ID: "twitter-tweet-" + tweet.ID, Source: SourceTwitter, FeedIDs: []string{feed.ID}, Author: author, Avatar: user.Avatar, Caption: cleanRemoteText(tweet.Text), Tags: append([]string(nil), feed.Tags...), OriginalURL: "https://x.com/" + url.PathEscape(user.Username) + "/status/" + url.PathEscape(tweet.ID), Published: parseTwitterTime(tweet.CreatedAt)}
			for _, key := range tweet.Attachments.MediaKeys {
				item, exists := media[key]
				if !exists {
					continue
				}
				if item.Kind == "photo" {
					post.Media = append(post.Media, item.URL)
				} else {
					post.Videos = append(post.Videos, PostVideo{URL: item.URL, Poster: item.Poster})
				}
			}
			posts = append(posts, post)
			if !full && !feed.LastSyncedAt.IsZero() && !post.Published.After(feed.LastSyncedAt) {
				reachedBoundary = true
			}
		}
		if payload.Meta.NextToken == "" || (!full && reachedBoundary) {
			break
		}
		nextToken = payload.Meta.NextToken
	}
	return posts, nil
}

func (b *BilibiliStore) twitterAccountHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	current, proxyURL := b.config.Twitter, b.config.ProxyURL
	b.RUnlock()
	if r.Method == http.MethodGet {
		if current.AccessToken != "" && (current.UserID == "" || current.UserName == "") {
			if refreshed, err := validateTwitterCredentials(current.AccessToken, proxyURL); err == nil {
				current = refreshed
				b.Lock()
				if b.config.Twitter.AccessToken == refreshed.AccessToken {
					b.config.Twitter = refreshed
				}
				b.Unlock()
				_ = b.save()
			}
		}
		if current.Avatar != "" {
			cachedAvatar := cacheAccountAvatar(SourceTwitter, twitterLikesName, current.Avatar, proxyURL, "https://x.com/", "")
			if cachedAvatar != current.Avatar {
				current.Avatar = cachedAvatar
				b.Lock()
				if b.config.Twitter.AccessToken == current.AccessToken {
					b.config.Twitter.Avatar = cachedAvatar
				}
				b.Unlock()
				_ = b.save()
			}
		}
		writeJSON(w, map[string]any{"configured": current.AccessToken != "", "userId": current.UserID, "userName": current.UserName, "avatar": current.Avatar})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "推特访问令牌格式无效")
		return
	}
	accessToken := firstNonEmptyRemoteText(input.AccessToken, input.Token)
	credentials, err := validateTwitterCredentials(accessToken, proxyURL)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if credentials.Avatar != "" {
		credentials.Avatar = cacheAccountAvatar(SourceTwitter, twitterLikesName, credentials.Avatar, proxyURL, "https://x.com/", "")
	}
	b.Lock()
	b.config.Twitter = credentials
	b.Unlock()
	if err := b.save(); err != nil {
		b.Lock()
		b.config.Twitter = current
		b.Unlock()
		writeAPIError(w, http.StatusInternalServerError, "无法保存推特访问令牌")
		return
	}
	writeJSON(w, map[string]any{"configured": true, "userId": credentials.UserID, "userName": credentials.UserName, "avatar": credentials.Avatar})
}

func (b *BilibiliStore) twitterSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig{}, b.config.TwitterSubscriptions...)
		applyCollectionAccountProfiles(result, b.config)
		b.RUnlock()
		writeJSON(w, result)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if r.Method == http.MethodDelete {
		b.Lock()
		for index, feed := range b.config.TwitterSubscriptions {
			if feed.ID != id {
				continue
			}
			previous := b.config.TwitterSubscriptions
			b.config.TwitterSubscriptions = append(append([]SourceConfig(nil), previous[:index]...), previous[index+1:]...)
			b.Unlock()
			if err := b.save(); err != nil {
				b.Lock()
				b.config.TwitterSubscriptions = previous
				b.Unlock()
				writeAPIError(w, http.StatusInternalServerError, "无法保存推特来源删除")
				return
			}
			writeJSON(w, map[string]string{"status": "deleted", "message": "推特点赞来源已删除，已采集文件予以保留"})
			return
		}
		b.Unlock()
		writeAPIError(w, http.StatusNotFound, "推特来源不存在")
		return
	}
	if r.Method == http.MethodPost && (r.URL.Query().Get("action") == "sync" || r.URL.Query().Get("action") == "resync") {
		b.RLock()
		var feed SourceConfig
		for _, candidate := range b.config.TwitterSubscriptions {
			if candidate.ID == id {
				feed = candidate
				break
			}
		}
		b.RUnlock()
		if feed.ID == "" {
			writeAPIError(w, http.StatusNotFound, "推特来源不存在")
			return
		}
		updated, err := b.syncSource(feed, r.URL.Query().Get("action") == "resync")
		if err != nil {
			writeJSON(w, map[string]any{"status": "failed", "message": err.Error(), "source": updated})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "message": updated.LastSyncMessage, "source": updated})
		return
	}
	if r.Method == http.MethodPut {
		var input SourceConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || !strings.HasPrefix(input.ID, "twitter-likes-") {
			writeAPIError(w, http.StatusBadRequest, "推特来源设置无效")
			return
		}
		input.Schedule = normalizeSchedule(input.Schedule)
		if !validCron(input.Schedule) {
			writeAPIError(w, http.StatusBadRequest, "Cron 表达式无效，请使用五段式格式")
			return
		}
		startDate, err := normalizeSourceStartDate(input.StartDate)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		input.StartDate = startDate
		b.Lock()
		found := false
		for index := range b.config.TwitterSubscriptions {
			if b.config.TwitterSubscriptions[index].ID != input.ID {
				continue
			}
			stored := &b.config.TwitterSubscriptions[index]
			stored.Enabled = input.Enabled
			stored.Schedule = input.Schedule
			stored.StartDate = input.StartDate
			stored.Tags = normalizeTags(input.Tags)
			stored.OnlyWithImages = input.OnlyWithImages
			stored.IncludeVideos = input.IncludeVideos
			stored.IncludeKeywords = normalizeKeywords(input.IncludeKeywords)
			stored.ExcludeKeywords = normalizeKeywords(input.ExcludeKeywords)
			input = *stored
			found = true
			break
		}
		b.Unlock()
		if !found {
			writeAPIError(w, http.StatusNotFound, "推特来源不存在")
			return
		}
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存推特来源设置")
			return
		}
		if b.content != nil {
			if err := b.content.setSourceTags(input, b.allSourceConfigs()); err != nil {
				writeAPIError(w, http.StatusInternalServerError, "无法将标签应用到来源动态")
				return
			}
		}
		writeJSON(w, input)
		return
	}
	if r.Method != http.MethodPost || r.URL.Query().Get("type") != "likes" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.RLock()
	credentials := b.config.Twitter
	b.RUnlock()
	if credentials.AccessToken == "" || credentials.UserID == "" {
		writeAPIError(w, http.StatusPreconditionFailed, "请先连接推特账号")
		return
	}
	feed := SourceConfig{ID: "twitter-likes-" + credentials.UserID, Source: SourceTwitter, Name: twitterLikesName, Handle: "@" + strings.TrimPrefix(credentials.UserName, "@"), Avatar: credentials.Avatar, Enabled: true, Schedule: "0 6 * * *", StoragePath: sourceStoragePath(SourceTwitter, twitterLikesName), OnlyWithImages: true}
	prepared, err := prepareSourceStorage(feed)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法创建推特点赞内容目录")
		return
	}
	feed = prepared
	b.Lock()
	for _, existing := range b.config.TwitterSubscriptions {
		if existing.ID == feed.ID {
			b.Unlock()
			writeAPIError(w, http.StatusConflict, "推特账号点赞来源已添加")
			return
		}
	}
	b.config.TwitterSubscriptions = append(b.config.TwitterSubscriptions, feed)
	b.Unlock()
	if err := b.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存推特点赞来源")
		return
	}
	writeJSON(w, feed)
}

func (b *BilibiliStore) pixivSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig{}, b.config.PixivSubscriptions...)
		applyCollectionAccountProfiles(result, b.config)
		b.RUnlock()
		writeJSON(w, result)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if r.Method == http.MethodDelete {
		b.Lock()
		for index, feed := range b.config.PixivSubscriptions {
			if feed.ID == id {
				previous := b.config.PixivSubscriptions
				b.config.PixivSubscriptions = append(append([]SourceConfig(nil), previous[:index]...), previous[index+1:]...)
				b.Unlock()
				if err := b.save(); err != nil {
					writeAPIError(w, http.StatusInternalServerError, "无法保存 Pixiv 来源")
					return
				}
				writeJSON(w, map[string]string{"status": "deleted"})
				return
			}
		}
		b.Unlock()
		writeAPIError(w, http.StatusNotFound, "Pixiv 来源不存在")
		return
	}
	if r.Method == http.MethodPost && (r.URL.Query().Get("action") == "sync" || r.URL.Query().Get("action") == "resync") {
		b.RLock()
		var feed SourceConfig
		for _, candidate := range b.config.PixivSubscriptions {
			if candidate.ID == id {
				feed = candidate
				break
			}
		}
		b.RUnlock()
		if feed.ID == "" {
			writeAPIError(w, http.StatusNotFound, "Pixiv 来源不存在")
			return
		}
		updated, err := b.syncSource(feed, r.URL.Query().Get("action") == "resync")
		if err != nil {
			writeJSON(w, map[string]any{"status": "failed", "message": err.Error(), "source": updated})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "message": updated.LastSyncMessage, "source": updated})
		return
	}
	if r.Method == http.MethodPut {
		var input SourceConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || !strings.HasPrefix(input.ID, "pixiv-") {
			writeAPIError(w, http.StatusBadRequest, "Pixiv 来源设置无效")
			return
		}
		input.Schedule = normalizeSchedule(input.Schedule)
		if !validCron(input.Schedule) {
			writeAPIError(w, http.StatusBadRequest, "Cron 表达式无效")
			return
		}
		startDate, startDateErr := normalizeSourceStartDate(input.StartDate)
		if startDateErr != nil {
			writeAPIError(w, http.StatusBadRequest, startDateErr.Error())
			return
		}
		input.StartDate = startDate
		b.Lock()
		found := false
		for index := range b.config.PixivSubscriptions {
			if b.config.PixivSubscriptions[index].ID == input.ID {
				b.config.PixivSubscriptions[index].Enabled = input.Enabled
				b.config.PixivSubscriptions[index].Schedule = input.Schedule
				b.config.PixivSubscriptions[index].StartDate = input.StartDate
				b.config.PixivSubscriptions[index].Tags = normalizeTags(input.Tags)
				b.config.PixivSubscriptions[index].OnlyWithImages = input.OnlyWithImages
				b.config.PixivSubscriptions[index].IncludeVideos = input.IncludeVideos
				b.config.PixivSubscriptions[index].IncludeKeywords = normalizeKeywords(input.IncludeKeywords)
				b.config.PixivSubscriptions[index].ExcludeKeywords = normalizeKeywords(input.ExcludeKeywords)
				input = b.config.PixivSubscriptions[index]
				found = true
				break
			}
		}
		b.Unlock()
		if !found {
			writeAPIError(w, http.StatusNotFound, "Pixiv 来源不存在")
			return
		}
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存 Pixiv 来源")
			return
		}
		if b.content != nil {
			_ = b.content.setSourceTags(input, b.allSourceConfigs())
		}
		writeJSON(w, input)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		UserID    string   `json:"userId"`
		Name      string   `json:"name"`
		Avatar    string   `json:"avatar"`
		Schedule  string   `json:"schedule"`
		Tags      []string `json:"tags"`
		Bookmarks bool     `json:"bookmarks"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "Pixiv 来源信息无效")
		return
	}
	b.RLock()
	credentials, proxyURL := b.config.Pixiv, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" || credentials.UserID == "" {
		writeAPIError(w, http.StatusPreconditionFailed, "请先连接 Pixiv 账号")
		return
	}
	userID := strings.TrimSpace(input.UserID)
	if input.Bookmarks {
		userID = credentials.UserID
		input.Name = "P站收藏"
		input.Avatar = credentials.Avatar
	}
	if !pixivNumericID(userID) {
		writeAPIError(w, http.StatusBadRequest, "Pixiv 用户 ID 无效")
		return
	}
	if !input.Bookmarks {
		if client, err := externalHTTPClient(proxyURL); err == nil {
			if profile, profileErr := fetchPixivUserProfile(client, credentials, userID); profileErr == nil {
				input.Name, input.Avatar = profile.Name, profile.Avatar
			}
		}
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = "Pixiv 画师 " + userID
	}
	if input.Schedule == "" {
		input.Schedule = "0 6 * * *"
	}
	idPrefix := "pixiv-"
	tags := normalizeTags(input.Tags)
	if input.Bookmarks {
		idPrefix = "pixiv-bookmarks-"
		tags = mergeUniqueStrings([]string{"P站收藏"}, tags)
	}
	feed := SourceConfig{ID: idPrefix + userID, Source: SourcePixiv, Name: strings.TrimSpace(input.Name), Handle: "UID " + userID, Avatar: normalizeRemoteImage(input.Avatar), Enabled: true, Schedule: normalizeSchedule(input.Schedule), Tags: tags, OnlyWithImages: true}
	if prepared, err := prepareSourceStorage(feed); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法创建 Pixiv 来源目录")
		return
	} else {
		feed = prepared
	}
	b.Lock()
	for _, existing := range b.config.PixivSubscriptions {
		if existing.ID == feed.ID {
			b.Unlock()
			writeAPIError(w, http.StatusConflict, "Pixiv 来源已经存在")
			return
		}
	}
	b.config.PixivSubscriptions = append(b.config.PixivSubscriptions, feed)
	b.Unlock()
	if err := b.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法保存 Pixiv 来源")
		return
	}
	if err := b.reconcileCollectionArchives(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法整理收藏归档")
		return
	}
	writeJSON(w, feed)
}

func browserClient(proxyURL string) (*http.Client, error) {
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client.Jar = jar
	return client, nil
}

func setBilibiliAPIHeaders(request *http.Request) {
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Referer", "https://www.bilibili.com/")
	request.Header.Set("Origin", "https://www.bilibili.com")
	request.Header.Set("Sec-CH-UA", `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`)
	request.Header.Set("Sec-CH-UA-Mobile", "?0")
	request.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
}

func randomSessionID() string {
	idBytes := make([]byte, 24)
	_, _ = rand.Read(idBytes)
	return base64.RawURLEncoding.EncodeToString(idBytes)
}

func cookiesFromResponse(response *http.Response) BilibiliCredentials {
	credentials := BilibiliCredentials{}
	for _, cookie := range response.Cookies() {
		switch cookie.Name {
		case "SESSDATA":
			credentials.SESSDATA = cookie.Value
		case "bili_jct":
			credentials.BiliJCT = cookie.Value
		case "DedeUserID":
			credentials.DedeUserID = cookie.Value
		case "DedeUserID__ckMd5":
			credentials.DedeUserIDCKMd5 = cookie.Value
		case "buvid3":
			credentials.Buvid3 = cookie.Value
		case "buvid4":
			credentials.Buvid4 = cookie.Value
		}
	}
	return credentials
}

func mergeBilibiliCookies(credentials BilibiliCredentials, client *http.Client) BilibiliCredentials {
	for _, rawURL := range []string{"https://www.bilibili.com/", "https://passport.bilibili.com/", "https://api.bilibili.com/"} {
		parsed, _ := url.Parse(rawURL)
		for _, cookie := range client.Jar.Cookies(parsed) {
			switch cookie.Name {
			case "SESSDATA":
				credentials.SESSDATA = cookie.Value
			case "bili_jct":
				credentials.BiliJCT = cookie.Value
			case "DedeUserID":
				credentials.DedeUserID = cookie.Value
			case "DedeUserID__ckMd5":
				credentials.DedeUserIDCKMd5 = cookie.Value
			case "buvid3":
				credentials.Buvid3 = cookie.Value
			case "buvid4":
				credentials.Buvid4 = cookie.Value
			}
		}
	}
	return credentials
}

func (b *BilibiliStore) bilibiliQRHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	if r.Method == http.MethodPost {
		client, err := browserClient(proxyURL)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, err.Error())
			return
		}
		homeRequest, _ := http.NewRequest(http.MethodGet, "https://www.bilibili.com/", nil)
		homeRequest.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
		if response, requestErr := client.Do(homeRequest); requestErr == nil {
			response.Body.Close()
		}
		request, _ := http.NewRequest(http.MethodGet, "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil)
		setBilibiliAPIHeaders(request)
		response, err := client.Do(request)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "无法连接 B 站二维码服务")
			return
		}
		defer response.Body.Close()
		var payload struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				URL string `json:"url"`
				Key string `json:"qrcode_key"`
			} `json:"data"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil || payload.Code != 0 || payload.Data.URL == "" || payload.Data.Key == "" {
			writeAPIError(w, http.StatusBadGateway, "B 站二维码接口响应异常："+payload.Message)
			return
		}
		now := time.Now()
		session := BilibiliQRSession{ID: randomSessionID(), Key: payload.Data.Key, URL: payload.Data.URL, CreatedAt: now, ExpiresAt: now.Add(3 * time.Minute)}
		b.Lock()
		b.bilibiliQR[session.ID] = session
		b.bilibiliClients[session.ID] = client
		b.Unlock()
		writeJSON(w, session)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	b.RLock()
	session, ok := b.bilibiliQR[id]
	client := b.bilibiliClients[id]
	b.RUnlock()
	if !ok || client == nil {
		writeAPIError(w, http.StatusNotFound, "B 站扫码会话不存在")
		return
	}
	if time.Now().After(session.ExpiresAt) {
		b.Lock()
		delete(b.bilibiliQR, id)
		delete(b.bilibiliClients, id)
		b.Unlock()
		writeAPIError(w, http.StatusGone, "B 站二维码已过期")
		return
	}
	endpoint := "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key=" + url.QueryEscape(session.Key)
	request, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	setBilibiliAPIHeaders(request)
	response, err := client.Do(request)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "无法查询 B 站扫码状态")
		return
	}
	defer response.Body.Close()
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Code         int    `json:"code"`
			Message      string `json:"message"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil || payload.Code != 0 {
		writeAPIError(w, http.StatusBadGateway, "B 站扫码状态响应异常："+payload.Message)
		return
	}
	switch payload.Data.Code {
	case 86101:
		writeJSON(w, map[string]string{"status": "waiting", "message": "等待扫码"})
		return
	case 86090:
		writeJSON(w, map[string]string{"status": "scanned", "message": "已扫码，请在手机上确认"})
		return
	case 86038:
		writeAPIError(w, http.StatusGone, "B 站二维码已过期")
		return
	case 0:
		credentials := mergeBilibiliCookies(cookiesFromResponse(response), client)
		credentials.AccessTimeValue = payload.Data.RefreshToken
		if credentials.SESSDATA == "" || credentials.BiliJCT == "" || credentials.DedeUserID == "" {
			writeAPIError(w, http.StatusBadGateway, "B 站登录成功但响应缺少必要 Cookie")
			return
		}
		if credentials.Buvid3 == "" {
			var spi struct {
				Code int `json:"code"`
				Data struct {
					B3 string `json:"b_3"`
					B4 string `json:"b_4"`
				} `json:"data"`
			}
			if err := bilibiliRequest("https://api.bilibili.com/x/frontend/finger/spi", credentials, proxyURL, &spi); err == nil {
				credentials.Buvid3, credentials.Buvid4 = spi.Data.B3, spi.Data.B4
			}
		}
		account := bilibiliAccountProfile(credentials, proxyURL)
		credentials.UserName, credentials.Avatar = account.UserName, account.Avatar
		b.Lock()
		b.config.Credentials = credentials
		delete(b.bilibiliQR, id)
		delete(b.bilibiliClients, id)
		b.Unlock()
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存 B 站登录状态")
			return
		}
		writeJSON(w, map[string]string{"status": "connected", "userId": credentials.DedeUserID, "userName": credentials.UserName, "avatar": credentials.Avatar})
		return
	default:
		writeAPIError(w, http.StatusBadGateway, "B 站扫码失败："+payload.Data.Message)
		return
	}
}

func weiboClient(proxyURL string) (*http.Client, error) {
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client.Jar = jar
	return client, nil
}

var weiboPreloginEndpoint = "https://login.sina.com.cn/sso/prelogin.php"
var weiboLoginEndpoint = "https://login.sina.com.cn/sso/login.php"
var validateWeiboLoginSession = validateWeiboCredentials
var fetchWeiboAccountProfile = fetchWeiboUser

type weiboPreloginResponse struct {
	ServerTime int64  `json:"servertime"`
	Nonce      string `json:"nonce"`
	PublicKey  string `json:"pubkey"`
	RSAKV      string `json:"rsakv"`
	ShowPin    int    `json:"showpin"`
}

func encodeWeiboUsername(username string) string {
	escaped := url.QueryEscape(username)
	return base64.StdEncoding.EncodeToString([]byte(escaped))
}

func encryptWeiboPassword(publicKeyHex string, serverTime int64, nonce, password string) (string, error) {
	modulus := new(big.Int)
	if _, ok := modulus.SetString(strings.TrimSpace(publicKeyHex), 16); !ok || modulus.Sign() <= 0 {
		return "", errors.New("微博预登录返回了无效的 RSA 公钥")
	}
	plain := []byte(strconv.FormatInt(serverTime, 10) + "\t" + nonce + "\n" + password)
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, &rsa.PublicKey{N: modulus, E: 65537}, plain)
	if err != nil {
		return "", fmt.Errorf("无法加密微博密码：%w", err)
	}
	return hex.EncodeToString(encrypted), nil
}

func requestWeiboPrelogin(client *http.Client, encodedUsername string) (weiboPreloginResponse, error) {
	query := url.Values{
		"entry":    {"weibo"},
		"callback": {"sinaSSOController.preloginCallBack"},
		"su":       {encodedUsername},
		"rsakt":    {"mod"},
		"checkpin": {"1"},
		"client":   {"ssologin.js(v1.4.19)"},
		"_":        {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	request, err := http.NewRequest(http.MethodGet, weiboPreloginEndpoint+"?"+query.Encode(), nil)
	if err != nil {
		return weiboPreloginResponse{}, err
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Referer", "https://weibo.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	response, err := client.Do(request)
	if err != nil {
		return weiboPreloginResponse{}, fmt.Errorf("微博预登录请求失败：%w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return weiboPreloginResponse{}, errors.New("无法读取微博预登录响应")
	}
	var payload weiboPreloginResponse
	if response.StatusCode != http.StatusOK || json.Unmarshal(unwrapJSONP(body), &payload) != nil || payload.ServerTime == 0 || payload.Nonce == "" || payload.PublicKey == "" || payload.RSAKV == "" {
		return weiboPreloginResponse{}, fmt.Errorf("微博预登录响应异常（HTTP %d）：%s", response.StatusCode, responseSummary(body))
	}
	if payload.ShowPin != 0 {
		return weiboPreloginResponse{}, errors.New("微博要求输入验证码，请改用扫码登录或浏览器 Cookie")
	}
	return payload, nil
}

func collectWeiboClientCookies(client *http.Client, extraURLs ...*url.URL) string {
	parts, seen := []string{}, map[string]bool{}
	cookieURLs := make([]*url.URL, 0, len(extraURLs)+6)
	cookieURLs = append(cookieURLs, extraURLs...)
	for _, rawURL := range []string{"https://weibo.com/", "https://www.weibo.com/", "https://m.weibo.cn/", "https://passport.weibo.cn/", "https://login.sina.com.cn/", "https://passport.weibo.com/"} {
		cookieURL, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		cookieURLs = append(cookieURLs, cookieURL)
	}
	for _, cookieURL := range cookieURLs {
		if cookieURL == nil {
			continue
		}
		for _, cookie := range client.Jar.Cookies(cookieURL) {
			key := strings.ToLower(cookie.Name)
			if cookie.Name == "" || seen[key] {
				continue
			}
			seen[key] = true
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	return normalizeWeiboCookie(strings.Join(parts, "; "))
}

func loginWeiboWithPassword(username, password, proxyURL string) (WeiboCredentials, error) {
	username, password = strings.TrimSpace(username), strings.TrimSpace(password)
	if username == "" || password == "" {
		return WeiboCredentials{}, errors.New("微博账号和密码均不能为空")
	}
	client, err := weiboClient(proxyURL)
	if err != nil {
		return WeiboCredentials{}, err
	}
	encodedUsername := encodeWeiboUsername(username)
	prelogin, err := requestWeiboPrelogin(client, encodedUsername)
	if err != nil {
		return WeiboCredentials{}, err
	}
	encryptedPassword, err := encryptWeiboPassword(prelogin.PublicKey, prelogin.ServerTime, prelogin.Nonce, password)
	if err != nil {
		return WeiboCredentials{}, err
	}
	values := url.Values{
		"entry":       {"weibo"},
		"gateway":     {"1"},
		"from":        {""},
		"savestate":   {"7"},
		"qrcode_flag": {"false"},
		"useticket":   {"1"},
		"pagerefer":   {"https://weibo.com/"},
		"vsnf":        {"1"},
		"su":          {encodedUsername},
		"service":     {"miniblog"},
		"servertime":  {strconv.FormatInt(prelogin.ServerTime, 10)},
		"nonce":       {prelogin.Nonce},
		"pwencode":    {"rsa2"},
		"rsakv":       {prelogin.RSAKV},
		"sp":          {encryptedPassword},
		"sr":          {"1920*1080"},
		"encoding":    {"UTF-8"},
		"prelt":       {"35"},
		"url":         {"https://weibo.com/ajaxlogin.php?framelogin=1&callback=parent.sinaSSOController.feedBackUrlCallBack"},
		"returntype":  {"TEXT"},
	}
	request, err := http.NewRequest(http.MethodPost, weiboLoginEndpoint+"?client=ssologin.js(v1.4.19)", strings.NewReader(values.Encode()))
	if err != nil {
		return WeiboCredentials{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Referer", "https://weibo.com/")
	request.Header.Set("Origin", "https://weibo.com")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	response, err := client.Do(request)
	if err != nil {
		return WeiboCredentials{}, fmt.Errorf("微博账号登录请求失败：%w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return WeiboCredentials{}, errors.New("无法读取微博账号登录响应")
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		return WeiboCredentials{}, fmt.Errorf("微博账号登录响应异常（HTTP %d）：%s", response.StatusCode, responseSummary(body))
	}
	retCode := jsonValueString(payload["retcode"])
	if response.StatusCode != http.StatusOK || (retCode != "0" && retCode != "20000000") {
		message := firstNonEmptyRemoteText(jsonValueString(payload["reason"]), jsonValueString(payload["msg"]), jsonValueString(payload["message"]))
		if message == "" {
			message = "微博拒绝了账号登录"
		}
		lower := strings.ToLower(message)
		if strings.Contains(lower, "verify") || strings.Contains(message, "验证码") || strings.Contains(message, "安全验证") || strings.Contains(message, "异常") || strings.Contains(message, "系统错误") {
			return WeiboCredentials{}, errors.New(message + "；请完成微博安全验证后使用扫码或 Cookie 登录")
		}
		return WeiboCredentials{}, errors.New(message)
	}
	userID := firstNonEmptyRemoteText(jsonValueString(payload["uid"]), jsonValueString(payload["user_id"]))
	if crossDomain, ok := payload["crossDomainUrlList"].([]any); ok {
		for _, raw := range crossDomain {
			crossURL := jsonValueString(raw)
			if crossURL == "" {
				continue
			}
			crossRequest, requestErr := http.NewRequest(http.MethodGet, crossURL, nil)
			if requestErr == nil {
				crossRequest.Header.Set("User-Agent", request.Header.Get("User-Agent"))
				if crossResponse, requestErr := client.Do(crossRequest); requestErr == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(crossResponse.Body, 1<<20))
					crossResponse.Body.Close()
				}
			}
		}
	}
	cookie := collectWeiboClientCookies(client, response.Request.URL)
	if cookie == "" || userID == "" {
		return WeiboCredentials{}, errors.New("微博账号登录成功但未返回完整会话，请改用扫码登录")
	}
	return validateWeiboLoginSession(cookie, userID, proxyURL)
}

func normalizeWeiboCookie(raw string) string {
	raw = strings.TrimSpace(raw)
	if index := strings.Index(raw, "\n"); index >= 0 {
		raw = raw[:index]
	}
	if strings.HasPrefix(strings.ToLower(raw), "cookie:") {
		raw = strings.TrimSpace(raw[len("cookie:"):])
	}
	parts, seen := make([]string, 0), make(map[string]bool)
	for _, item := range strings.Split(raw, ";") {
		pair := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" {
			continue
		}
		name := strings.TrimSpace(pair[0])
		if !seen[strings.ToLower(name)] {
			seen[strings.ToLower(name)] = true
			parts = append(parts, name+"="+strings.TrimSpace(pair[1]))
		}
	}
	return strings.Join(parts, "; ")
}

func weiboCookieValue(cookie, name string) string {
	for _, item := range strings.Split(cookie, ";") {
		pair := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(pair) == 2 && strings.EqualFold(pair[0], name) {
			return pair[1]
		}
	}
	return ""
}

func weiboRequestHeaders(request *http.Request, cookie string) {
	origin := "https://weibo.com"
	if strings.EqualFold(request.URL.Hostname(), "m.weibo.cn") {
		origin = "https://m.weibo.cn"
	}
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Referer", origin+"/")
	request.Header.Set("Origin", origin)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	if token := weiboCookieValue(cookie, "XSRF-TOKEN"); token != "" {
		request.Header.Set("X-XSRF-TOKEN", token)
	}
}

func weiboRawRequest(endpoint string, credentials WeiboCredentials, proxyURL string) (map[string]any, int, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	weiboRequestHeaders(request, credentials.Cookie)
	client, err := externalHTTPClient(proxyURL)
	if err != nil {
		return nil, 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, response.StatusCode, fmt.Errorf("微博登录 Cookie 已失效或当前出口被限制（HTTP %d）", response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, fmt.Errorf("微博接口返回 HTTP %d，请检查代理出口", response.StatusCode)
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		lowerBody := bytes.ToLower(body)
		if bytes.Contains(lowerBody, []byte("passport.weibo")) || bytes.Contains(lowerBody, []byte("login.sina")) {
			return nil, response.StatusCode, errors.New("微博登录 Cookie 已失效，请重新扫码或导入浏览器 Cookie")
		}
		if bytes.Contains(lowerBody, []byte("<html")) || bytes.Contains(lowerBody, []byte("<!doctype")) {
			return nil, response.StatusCode, errors.New("微博当前出口触发网页风控，请导入同一出口的浏览器 Cookie 或更换代理出口")
		}
		return nil, response.StatusCode, errors.New("微博接口返回了非 JSON 内容，请检查 Cookie 与代理出口")
	}
	if okValue, exists := payload["ok"]; exists && jsonValueString(okValue) == "0" {
		message := firstNonEmptyRemoteText(jsonValueString(payload["msg"]), jsonValueString(payload["message"]), jsonValueString(payload["errmsg"]))
		if message == "" {
			message = "微博接口拒绝了当前请求"
		}
		return nil, response.StatusCode, errors.New(message + "，请重新扫码登录或更换代理出口")
	}
	return payload, response.StatusCode, nil
}

func weiboRequest(endpoint string, credentials WeiboCredentials, proxyURL string, target any) error {
	payload, _, err := weiboRawRequest(endpoint, credentials, proxyURL)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func collectWeiboUsers(value any, users *[]WeiboUser, seen map[string]bool) {
	if object, ok := value.(map[string]any); ok {
		if nested, exists := object["user"].(map[string]any); exists {
			collectWeiboUsers(nested, users, seen)
		}
		id := firstNonEmptyRemoteText(jsonValueString(object["idstr"]), jsonValueString(object["id"]))
		name := strings.TrimSpace(fmt.Sprint(object["screen_name"]))
		if name == "<nil>" || name == "" {
			name = strings.TrimSpace(jsonValueString(object["name"]))
		}
		if id != "" && id != "<nil>" && name != "" && name != "<nil>" && !seen[id] {
			avatar := normalizeRemoteImage(jsonValueString(object["avatar_hd"]))
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(object["avatar_large"]))
			}
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(object["profile_image_url"]))
			}
			if avatar == "" {
				avatar = normalizeRemoteImage(jsonValueString(object["avatar"]))
			}
			fans := int64(0)
			if raw, ok := object["followers_count"]; ok {
				fans, _ = strconv.ParseInt(jsonValueString(raw), 10, 64)
			}
			description := strings.TrimSpace(fmt.Sprint(object["description"]))
			if description == "<nil>" {
				description = ""
			}
			seen[id] = true
			*users = append(*users, WeiboUser{UserID: id, Name: name, Avatar: avatar, Fans: fans, Description: description})
		}
		for _, child := range object {
			collectWeiboUsers(child, users, seen)
		}
		return
	}
	if list, ok := value.([]any); ok {
		for _, child := range list {
			collectWeiboUsers(child, users, seen)
		}
	}
}

func fetchWeiboUser(userID string, credentials WeiboCredentials, proxyURL string) (WeiboUser, error) {
	endpoints := []string{
		"https://weibo.com/ajax/profile/info?uid=" + url.QueryEscape(userID),
		"https://m.weibo.cn/api/container/getIndex?type=uid&value=" + url.QueryEscape(userID) + "&containerid=" + url.QueryEscape("100505"+userID),
	}
	var lastErr error
	for _, endpoint := range endpoints {
		payload, _, err := weiboRawRequest(endpoint, credentials, proxyURL)
		if err != nil {
			lastErr = err
			continue
		}
		users := make([]WeiboUser, 0)
		collectWeiboUsers(payload, &users, make(map[string]bool))
		for _, user := range users {
			if user.UserID == userID {
				return user, nil
			}
		}
		lastErr = errors.New("微博接口未返回指定 UID 的账号资料")
	}
	return WeiboUser{}, lastErr
}

func (b *BilibiliStore) weiboSearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if len([]rune(keyword)) < 1 || len([]rune(keyword)) > 40 {
		writeAPIError(w, http.StatusBadRequest, "搜索关键词长度应为 1 到 40 个字符")
		return
	}
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" {
		writeAPIError(w, http.StatusPreconditionFailed, "请先连接微博账号")
		return
	}
	endpoints := []string{
		"https://weibo.com/ajax/side/search?q=" + url.QueryEscape(keyword),
		"https://m.weibo.cn/api/container/getIndex?containerid=" + url.QueryEscape("100103type=3&q="+keyword) + "&page_type=searchall",
	}
	users := make([]WeiboUser, 0)
	seen := make(map[string]bool)
	statuses := make([]string, 0, len(endpoints))
	lastMessage := ""
	for _, endpoint := range endpoints {
		payload, status, err := weiboRawRequest(endpoint, credentials, proxyURL)
		if err != nil {
			statuses = append(statuses, strconv.Itoa(status))
			lastMessage = err.Error()
			continue
		}
		collectWeiboUsers(payload, &users, seen)
		if len(users) > 0 {
			break
		}
	}
	if len(users) == 0 {
		message := "微博搜索暂时不可用（接口状态：" + strings.Join(statuses, "/") + "）。请重新扫码登录或检查代理出口"
		if lastMessage != "" {
			message += "；" + lastMessage
		}
		writeAPIError(w, http.StatusBadGateway, message)
		return
	}
	client, clientErr := externalHTTPClient(proxyURL)
	if clientErr == nil {
		for index := range users {
			if users[index].Avatar == "" {
				continue
			}
			storage := sourceStoragePath(SourceWeibo, users[index].Name)
			if err := os.MkdirAll(storage, 0755); err != nil {
				continue
			}
			avatarPath, err := downloadRemoteImage(client, users[index].Avatar, filepath.Join(storage, "avatar"), "https://weibo.com/", credentials.Cookie)
			if err == nil {
				users[index].Avatar = flowPublicPath(SourceWeibo, users[index].Name, filepath.Base(avatarPath))
			}
		}
	}
	writeJSON(w, users)
}

func (b *BilibiliStore) weiboSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b.RLock()
		result := append([]SourceConfig{}, b.config.WeiboSubscriptions...)
		applyCollectionAccountProfiles(result, b.config)
		b.RUnlock()
		writeJSON(w, result)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if r.Method == http.MethodDelete {
		b.Lock()
		for index, feed := range b.config.WeiboSubscriptions {
			if feed.ID == id {
				previous := b.config.WeiboSubscriptions
				b.config.WeiboSubscriptions = append(append([]SourceConfig(nil), previous[:index]...), previous[index+1:]...)
				b.Unlock()
				if err := b.save(); err != nil {
					b.Lock()
					b.config.WeiboSubscriptions = previous
					b.Unlock()
					writeAPIError(w, http.StatusInternalServerError, "无法持久化微博订阅删除")
					return
				}
				writeJSON(w, map[string]string{"status": "deleted", "message": "微博博主订阅已永久删除，已采集文件予以保留"})
				return
			}
		}
		b.Unlock()
		writeAPIError(w, http.StatusNotFound, "微博订阅不存在")
		return
	}
	if r.Method == http.MethodPost && (r.URL.Query().Get("action") == "sync" || r.URL.Query().Get("action") == "resync") {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.RLock()
		var feed SourceConfig
		for _, candidate := range b.config.WeiboSubscriptions {
			if candidate.ID == id {
				feed = candidate
				break
			}
		}
		b.RUnlock()
		if feed.ID == "" {
			writeAPIError(w, http.StatusNotFound, "微博订阅不存在")
			return
		}
		updated, err := b.syncSource(feed, r.URL.Query().Get("action") == "resync")
		if err != nil {
			writeJSON(w, map[string]any{"status": "failed", "message": err.Error(), "source": updated})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "message": updated.LastSyncMessage, "source": updated})
		return
	}
	if r.Method == http.MethodPut {
		var input SourceConfig
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || !strings.HasPrefix(input.ID, "weibo-") {
			writeAPIError(w, http.StatusBadRequest, "来源设置无效")
			return
		}
		input.Schedule = normalizeSchedule(input.Schedule)
		if !validCron(input.Schedule) {
			writeAPIError(w, http.StatusBadRequest, "Cron 表达式无效，请使用五段式格式")
			return
		}
		startDate, startDateErr := normalizeSourceStartDate(input.StartDate)
		if startDateErr != nil {
			writeAPIError(w, http.StatusBadRequest, startDateErr.Error())
			return
		}
		input.StartDate = startDate
		b.Lock()
		found := false
		for index := range b.config.WeiboSubscriptions {
			if b.config.WeiboSubscriptions[index].ID == input.ID {
				b.config.WeiboSubscriptions[index].Enabled = input.Enabled
				b.config.WeiboSubscriptions[index].Schedule = input.Schedule
				b.config.WeiboSubscriptions[index].StartDate = input.StartDate
				b.config.WeiboSubscriptions[index].Tags = normalizeTags(input.Tags)
				b.config.WeiboSubscriptions[index].OnlyWithImages = input.OnlyWithImages
				b.config.WeiboSubscriptions[index].IncludeVideos = input.IncludeVideos
				b.config.WeiboSubscriptions[index].IncludeKeywords = normalizeKeywords(input.IncludeKeywords)
				b.config.WeiboSubscriptions[index].ExcludeKeywords = normalizeKeywords(input.ExcludeKeywords)
				input = b.config.WeiboSubscriptions[index]
				found = true
				break
			}
		}
		b.Unlock()
		if !found {
			writeAPIError(w, http.StatusNotFound, "微博来源不存在")
			return
		}
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存微博来源设置")
			return
		}
		if b.content != nil {
			if err := b.content.setSourceTags(input, b.allSourceConfigs()); err != nil {
				writeAPIError(w, http.StatusInternalServerError, "无法将标签应用到来源动态")
				return
			}
		}
		writeJSON(w, input)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("type") == "likes" {
		b.RLock()
		credentials := b.config.Weibo
		b.RUnlock()
		if credentials.Cookie == "" || credentials.UserID == "" {
			writeAPIError(w, http.StatusPreconditionFailed, "请先连接微博账号")
			return
		}
		feed := SourceConfig{ID: "weibo-likes-" + credentials.UserID, Source: SourceWeibo, Name: "我的点赞", Handle: credentials.UserName, Avatar: credentials.Avatar, Enabled: true, Schedule: "0 6 * * *", StoragePath: sourceStoragePath(SourceWeibo, "我的点赞"), OnlyWithImages: true}
		if prepared, err := prepareSourceStorage(feed); err == nil {
			feed = prepared
		} else {
			writeAPIError(w, http.StatusInternalServerError, "无法创建微博点赞内容目录")
			return
		}
		b.Lock()
		for _, existing := range b.config.WeiboSubscriptions {
			if existing.ID == feed.ID {
				b.Unlock()
				writeAPIError(w, http.StatusConflict, "微博我的点赞来源已添加")
				return
			}
		}
		b.config.WeiboSubscriptions = append(b.config.WeiboSubscriptions, feed)
		b.Unlock()
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存微博点赞来源")
			return
		}
		writeJSON(w, feed)
		return
	}
	var input struct {
		UserID   string   `json:"userId"`
		Name     string   `json:"name"`
		Avatar   string   `json:"avatar"`
		Schedule string   `json:"schedule"`
		Tags     []string `json:"tags"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.Name) == "" {
		writeAPIError(w, http.StatusBadRequest, "微博订阅信息无效")
		return
	}
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if credentials.Cookie == "" {
		writeAPIError(w, http.StatusPreconditionFailed, "请先连接微博账号")
		return
	}
	if input.Schedule == "" {
		input.Schedule = "0 6 * * *"
	}
	userID := strings.TrimSpace(input.UserID)
	if strings.TrimSpace(input.Avatar) == "" {
		if user, err := fetchWeiboUser(userID, credentials, proxyURL); err == nil {
			input.Avatar = user.Avatar
			if strings.TrimSpace(input.Name) == "" {
				input.Name = user.Name
			}
		}
	}
	feed := SourceConfig{ID: "weibo-" + userID, Source: SourceWeibo, Name: strings.TrimSpace(input.Name), Handle: "UID " + userID, Avatar: normalizeRemoteImage(input.Avatar), Enabled: true, Schedule: input.Schedule, Tags: normalizeTags(input.Tags), OnlyWithImages: true}
	var err error
	if feed, err = prepareSourceStorage(feed); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法创建微博博主内容目录")
		return
	}
	if feed.Avatar != "" && !strings.HasPrefix(feed.Avatar, "/flow/") {
		if client, clientErr := externalHTTPClient(proxyURL); clientErr == nil {
			avatarPath, downloadErr := downloadRemoteImage(client, feed.Avatar, filepath.Join(feed.StoragePath, "avatar"), "https://weibo.com/", credentials.Cookie)
			if downloadErr == nil {
				feed.Avatar = flowPublicPath(SourceWeibo, feed.Name, filepath.Base(avatarPath))
			}
		}
	}
	b.Lock()
	for _, existing := range b.config.WeiboSubscriptions {
		if existing.ID == feed.ID {
			b.Unlock()
			writeAPIError(w, http.StatusConflict, "已经订阅该微博博主")
			return
		}
	}
	b.config.WeiboSubscriptions = append(b.config.WeiboSubscriptions, feed)
	b.Unlock()
	if err := b.save(); err != nil {
		b.Lock()
		b.config.WeiboSubscriptions = b.config.WeiboSubscriptions[:len(b.config.WeiboSubscriptions)-1]
		b.Unlock()
		writeAPIError(w, http.StatusInternalServerError, "无法保存微博订阅")
		return
	}
	if err := b.reconcileCollectionArchives(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "无法整理收藏归档")
		return
	}
	writeJSON(w, feed)
}

func jsonValueString(value any) string {
	if value == nil {
		return ""
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	if strings.ContainsAny(text, ".eE") {
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			return strconv.FormatFloat(number, 'f', -1, 64)
		}
	}
	return text
}

func jsonScalarString(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return strings.Trim(value, "\"")
}

func unwrapJSONP(body []byte) []byte {
	value := strings.TrimSpace(string(body))
	start, end := strings.IndexByte(value, '{'), strings.LastIndexByte(value, '}')
	if start >= 0 && end >= start {
		return []byte(value[start : end+1])
	}
	return body
}

func weiboCrossDomainURL(rawURL string, index int) string {
	if index != 0 {
		return rawURL
	}
	separator := "&"
	if !strings.Contains(rawURL, "?") {
		separator = "?"
	}
	return rawURL + separator + "action=login"
}

func responseSummary(body []byte) string {
	value := strings.TrimSpace(string(body))
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func validateWeiboCredentials(cookie, userID, proxyURL string) (WeiboCredentials, error) {
	cookie, userID = normalizeWeiboCookie(cookie), strings.TrimSpace(userID)
	if cookie == "" || userID == "" {
		return WeiboCredentials{}, errors.New("完整 Cookie 和微博 UID 均不能为空")
	}
	if weiboCookieValue(cookie, "SUB") == "" && weiboCookieValue(cookie, "SUBP") == "" {
		return WeiboCredentials{}, errors.New("Cookie 缺少 SUB/SUBP 登录凭证，请从已登录微博的浏览器请求头中复制完整 Cookie")
	}
	credentials := WeiboCredentials{Cookie: cookie, UserID: userID}
	user, err := fetchWeiboAccountProfile(userID, credentials, proxyURL)
	if err != nil {
		return WeiboCredentials{}, fmt.Errorf("微博 Cookie 验证失败：%w", err)
	}
	credentials.UserName = user.Name
	credentials.Avatar = user.Avatar
	return credentials, nil
}

func (b *BilibiliStore) weiboAccountHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	current, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	if r.Method == http.MethodGet {
		current.Avatar = normalizeRemoteImage(current.Avatar)
		if current.Cookie != "" && current.UserID != "" && (strings.TrimSpace(current.UserName) == "" || strings.TrimSpace(current.Avatar) == "") {
			if user, err := fetchWeiboAccountProfile(current.UserID, current, proxyURL); err == nil {
				if strings.TrimSpace(user.Name) != "" {
					current.UserName = user.Name
				}
				if strings.TrimSpace(user.Avatar) != "" {
					current.Avatar = user.Avatar
				}
				b.Lock()
				if b.config.Weibo.Cookie == current.Cookie && b.config.Weibo.UserID == current.UserID {
					b.config.Weibo = current
				}
				b.Unlock()
				_ = b.save()
			}
		}
		if current.Avatar != "" {
			cachedAvatar := cacheAccountAvatar(SourceWeibo, weiboLikesName, current.Avatar, proxyURL, "https://weibo.com/", current.Cookie)
			if cachedAvatar != current.Avatar {
				current.Avatar = cachedAvatar
				b.Lock()
				if b.config.Weibo.Cookie == current.Cookie && b.config.Weibo.UserID == current.UserID {
					b.config.Weibo.Avatar = cachedAvatar
				}
				b.Unlock()
				_ = b.save()
			}
		}
		writeJSON(w, map[string]any{"configured": current.Cookie != "", "userId": current.UserID, "userName": current.UserName, "avatar": current.Avatar})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Cookie   string `json:"cookie"`
		UserID   string `json:"userId"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "微博凭证格式无效")
		return
	}
	var credentials WeiboCredentials
	var err error
	if strings.TrimSpace(input.Username) != "" || strings.TrimSpace(input.Password) != "" {
		credentials, err = loginWeiboWithPassword(input.Username, input.Password, proxyURL)
	} else {
		credentials, err = validateWeiboLoginSession(input.Cookie, input.UserID, proxyURL)
	}
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.Lock()
	b.config.Weibo = credentials
	b.Unlock()
	if err := b.save(); err != nil {
		b.Lock()
		b.config.Weibo = current
		b.Unlock()
		writeAPIError(w, http.StatusInternalServerError, "无法保存微博凭证")
		return
	}
	writeJSON(w, map[string]any{"configured": true, "userId": credentials.UserID, "userName": credentials.UserName, "avatar": credentials.Avatar})
}

func (b *BilibiliStore) weiboQRHandler(w http.ResponseWriter, r *http.Request) {
	b.RLock()
	proxyURL := b.config.ProxyURL
	b.RUnlock()
	if r.Method == http.MethodGet {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		b.RLock()
		if id == "" {
			account := b.config.Weibo
			b.RUnlock()
			writeJSON(w, map[string]any{"configured": account.UserID != "", "userId": account.UserID, "userName": account.UserName, "avatar": account.Avatar})
			return
		}
		session, ok := b.weiboQR[id]
		client := b.weiboClients[id]
		b.RUnlock()
		if client == nil {
			writeAPIError(w, http.StatusNotFound, "扫码会话已失效，请重新获取二维码")
			return
		}
		if !ok {
			writeAPIError(w, http.StatusNotFound, "扫码会话不存在，请重新获取二维码")
			return
		}
		if time.Now().After(session.ExpiresAt) {
			b.Lock()
			delete(b.weiboQR, id)
			delete(b.weiboClients, id)
			b.Unlock()
			writeAPIError(w, http.StatusGone, "二维码已过期")
			return
		}
		checkQuery := url.Values{
			"entry":    {"weibo"},
			"qrid":     {session.QRID},
			"callback": {"STK_" + strconv.FormatInt(time.Now().UnixNano()/100000, 10)},
		}
		checkURL := "https://login.sina.com.cn/sso/qrcode/check?" + checkQuery.Encode()
		request, _ := http.NewRequest(http.MethodGet, checkURL, nil)
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
		request.Header.Set("Accept", "*/*")
		request.Header.Set("Referer", "https://mail.sina.com.cn/")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法查询微博扫码状态")
			return
		}
		defer response.Body.Close()
		checkBody, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if readErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法读取微博扫码状态")
			return
		}
		var payload struct {
			RetCode int    `json:"retcode"`
			Msg     string `json:"msg"`
			Data    struct {
				Alt string `json:"alt"`
			} `json:"data"`
		}
		if json.Unmarshal(unwrapJSONP(checkBody), &payload) != nil {
			writeAPIError(w, http.StatusBadGateway, "微博扫码状态响应异常："+responseSummary(checkBody))
			return
		}
		if payload.RetCode != 20000000 {
			writeJSON(w, map[string]any{"status": "waiting", "message": payload.Msg})
			return
		}
		b.Lock()
		current, exists := b.weiboQR[id]
		if !exists || current.Exchanging {
			b.Unlock()
			writeJSON(w, map[string]any{"status": "confirming", "message": "正在确认微博登录"})
			return
		}
		current.Exchanging = true
		b.weiboQR[id] = current
		b.Unlock()
		defer func() {
			b.Lock()
			if active, exists := b.weiboQR[id]; exists {
				active.Exchanging = false
				b.weiboQR[id] = active
			}
			b.Unlock()
		}()
		loginQuery := url.Values{
			"entry":       {"weibo"},
			"returntype":  {"TEXT"},
			"crossdomain": {"1"},
			"cdult":       {"3"},
			"domain":      {"weibo.com"},
			"alt":         {payload.Data.Alt},
			"savestate":   {"30"},
			"callback":    {"STK_" + strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		loginURL := "http://login.sina.com.cn/sso/login.php?" + loginQuery.Encode()
		loginRequest, _ := http.NewRequest(http.MethodGet, loginURL, nil)
		loginRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
		loginRequest.Header.Set("Accept", "*/*")
		loginRequest.Header.Set("Referer", "https://mail.sina.com.cn/")
		loginResponse, loginErr := client.Do(loginRequest)
		if loginErr != nil {
			writeAPIError(w, http.StatusBadGateway, "微博登录票据交换失败："+loginErr.Error())
			return
		}
		defer loginResponse.Body.Close()
		loginBody, readErr := io.ReadAll(io.LimitReader(loginResponse.Body, 2<<20))
		if readErr != nil {
			writeAPIError(w, http.StatusBadGateway, "无法读取微博登录响应")
			return
		}
		var loginPayload struct {
			RetCode     json.RawMessage `json:"retcode"`
			UID         json.RawMessage `json:"uid"`
			Nick        string          `json:"nick"`
			Reason      string          `json:"reason"`
			CrossDomain []string        `json:"crossDomainUrlList"`
		}
		if err := json.Unmarshal(unwrapJSONP(loginBody), &loginPayload); err != nil {
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("微博登录响应无法解析（HTTP %d）：%s", loginResponse.StatusCode, responseSummary(loginBody)))
			return
		}
		loginUID, loginRetCode := jsonScalarString(loginPayload.UID), jsonScalarString(loginPayload.RetCode)
		if loginResponse.StatusCode != http.StatusOK || (loginRetCode != "" && loginRetCode != "0") || loginUID == "" {
			detail := loginPayload.Reason
			if detail == "" {
				detail = responseSummary(loginBody)
			}
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("微博登录响应异常（HTTP %d，retcode %s）：%s", loginResponse.StatusCode, loginRetCode, detail))
			return
		}
		for index, crossURL := range loginPayload.CrossDomain {
			crossURL = weiboCrossDomainURL(crossURL, index)
			crossRequest, e := http.NewRequest(http.MethodGet, crossURL, nil)
			if e == nil {
				crossRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
				crossRequest.Header.Set("Referer", "https://mail.sina.com.cn/")
				if crossResponse, e := client.Do(crossRequest); e == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(crossResponse.Body, 1<<20))
					crossResponse.Body.Close()
				}
			}
		}
		homeRequest, _ := http.NewRequest(http.MethodGet, "https://weibo.com/", nil)
		homeRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
		if homeResponse, homeErr := client.Do(homeRequest); homeErr == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(homeResponse.Body, 1<<20))
			homeResponse.Body.Close()
		}
		cookieParts, cookieNames := []string{}, map[string]bool{}
		for _, rawURL := range []string{"https://weibo.com/", "https://www.weibo.com/", "https://m.weibo.cn/", "https://login.sina.com.cn/", "https://passport.weibo.com/"} {
			cookieURL, _ := url.Parse(rawURL)
			for _, cookie := range client.Jar.Cookies(cookieURL) {
				if !cookieNames[cookie.Name] {
					cookieNames[cookie.Name] = true
					cookieParts = append(cookieParts, cookie.Name+"="+cookie.Value)
				}
			}
		}
		if len(cookieParts) == 0 {
			writeAPIError(w, http.StatusBadGateway, "微博未返回登录 Cookie")
			return
		}
		credentials := WeiboCredentials{Cookie: strings.Join(cookieParts, "; "), UserID: loginUID, UserName: loginPayload.Nick}
		validated, validateErr := validateWeiboCredentials(credentials.Cookie, credentials.UserID, proxyURL)
		if validateErr != nil {
			writeAPIError(w, http.StatusBadGateway, "微博扫码登录已确认，但当前出口无法使用该会话："+validateErr.Error())
			return
		}
		if validated.UserName == "" {
			validated.UserName = credentials.UserName
		}
		credentials = validated
		b.Lock()
		b.config.Weibo = credentials
		delete(b.weiboQR, id)
		delete(b.weiboClients, id)
		b.Unlock()
		if err := b.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "无法保存微博登录状态")
			return
		}
		writeJSON(w, map[string]any{"status": "connected", "userId": credentials.UserID, "userName": credentials.UserName, "avatar": credentials.Avatar})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client, err := weiboClient(proxyURL)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	imageQuery := url.Values{
		"entry":    {"weibo"},
		"size":     {"180"},
		"callback": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	request, _ := http.NewRequest(http.MethodGet, "https://login.sina.com.cn/sso/qrcode/image?"+imageQuery.Encode(), nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Referer", "https://mail.sina.com.cn/")
	response, err := client.Do(request)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "无法连接微博扫码服务")
		return
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if readErr != nil {
		writeAPIError(w, http.StatusBadGateway, "无法读取微博扫码响应")
		return
	}
	var payload struct {
		RetCode int `json:"retcode"`
		Data    struct {
			QRID  string `json:"qrid"`
			Image string `json:"image"`
		} `json:"data"`
	}
	if err := json.Unmarshal(unwrapJSONP(body), &payload); err != nil || response.StatusCode != http.StatusOK || payload.RetCode != 20000000 || payload.Data.QRID == "" || payload.Data.Image == "" {
		summary := strings.TrimSpace(string(body))
		if len(summary) > 180 {
			summary = summary[:180]
		}
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("微博扫码接口响应异常（HTTP %d）：%s", response.StatusCode, summary))
		return
	}
	now := time.Now()
	idBytes := make([]byte, 24)
	_, _ = rand.Read(idBytes)
	session := WeiboQRSession{ID: base64.RawURLEncoding.EncodeToString(idBytes), QRID: payload.Data.QRID, Image: payload.Data.Image, CreatedAt: now, ExpiresAt: now.Add(3 * time.Minute)}
	b.Lock()
	b.weiboQR[session.ID] = session
	b.weiboClients[session.ID] = client
	b.Unlock()
	writeJSON(w, session)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(value)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	requested := filepath.Clean(filepath.Join("public", filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/"))))
	publicRoot, rootErr := filepath.Abs("public")
	requestedPath, pathErr := filepath.Abs(requested)
	if rootErr == nil && pathErr == nil && (requestedPath == publicRoot || strings.HasPrefix(requestedPath, publicRoot+string(os.PathSeparator))) {
		if info, err := os.Stat(requestedPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, requestedPath)
			return
		}
	}
	if path.Ext(r.URL.Path) != "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("public", "index.html"))
}

func main() {
	if err := initializeFlowStorage(); err != nil {
		log.Fatal("unable to initialize flow storage: ", err)
	}
	startMediaPreviewCleanup()
	store, err := loadStore()
	if err != nil {
		log.Fatal("unable to load content data: ", err)
	}
	auth := loadAuthConfig()
	bilibili, err := loadBilibiliStore()
	if err != nil {
		log.Fatal("unable to load Bilibili configuration: ", err)
	}
	bilibili.content = store
	if err := bilibili.reconcileFlowStorage(); err != nil {
		log.Fatal("unable to prepare source flow storage: ", err)
	}
	if err := bilibili.save(); err != nil {
		log.Fatal("unable to persist source storage paths: ", err)
	}
	bilibili.startScheduler()
	sessions := &SessionStore{tokens: make(map[string]time.Time)}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", loginHandler(sessions, auth))
	mux.HandleFunc("/api/session", sessionHandler(sessions))
	mux.HandleFunc("/api/logout", logoutHandler(sessions))
	mux.HandleFunc("/api/settings", settingsHandler(auth))
	mux.HandleFunc("/api/posts", store.postsHandler)
	mux.HandleFunc("/api/feeds", store.feedsHandler)
	mux.HandleFunc("/api/sync", bilibili.syncHandler)
	mux.HandleFunc("/api/bilibili/account", bilibili.accountHandler)
	mux.HandleFunc("/api/bilibili/qr", bilibili.bilibiliQRHandler)
	mux.HandleFunc("/api/pixiv/account", bilibili.pixivHandler)
	mux.HandleFunc("/api/pixiv/subscriptions", bilibili.pixivSubscriptionsHandler)
	mux.HandleFunc("/api/twitter/account", bilibili.twitterAccountHandler)
	mux.HandleFunc("/api/twitter/subscriptions", bilibili.twitterSubscriptionsHandler)
	mux.HandleFunc("/api/weibo/account", bilibili.weiboAccountHandler)
	mux.HandleFunc("/api/weibo/qr", bilibili.weiboQRHandler)
	mux.HandleFunc("/api/weibo/search", bilibili.weiboSearchHandler)
	mux.HandleFunc("/api/weibo/likes", bilibili.weiboLikesHandler)
	mux.HandleFunc("/api/weibo/subscriptions", bilibili.weiboSubscriptionsHandler)
	mux.HandleFunc("/api/bilibili/search", bilibili.searchHandler)
	mux.HandleFunc("/api/bilibili/subscriptions", bilibili.subscriptionsHandler)
	mux.HandleFunc("/api/project/settings", bilibili.projectSettingsHandler)
	mux.HandleFunc("/api/configuration/backup", configurationBackupHandler(auth, bilibili))
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, map[string]string{"status": "ok"}) })
	registerAPIV1Routes(mux, sessions, auth, store, bilibili)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; media-src 'self' https: blob:; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; script-src 'self'; connect-src 'self'")
		if r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			w.Header().Set("X-Lumic-API-Version", "1")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			authMiddleware(sessions, mux).ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/flow/") {
			http.StripPrefix("/flow/", http.FileServer(http.Dir(flowRoot))).ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/preview/") {
			mediaPreviewHandler(w, r)
			return
		}
		serveFrontend(w, r)
	})
	log.Println("Lumic API listening on :5500")
	log.Fatal(http.ListenAndServe(":5500", handler))
}
