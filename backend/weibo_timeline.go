package main

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type weiboFetchReport struct {
	Anonymous     bool
	LoginExpired  bool
	Fetched       int
	ReportedTotal int
	PageLimited   bool
}

func (report weiboFetchReport) warning(full bool) string {
	if !report.Anonymous {
		if report.PageLimited {
			return fmt.Sprintf("本次达到历史拉取分页上限，仅获取 %d 条动态，不代表完整历史", report.Fetched)
		}
		return ""
	}
	prefix := "本次使用免登录访问"
	if report.LoginExpired {
		prefix = "微博登录已失效，已改用免登录访问"
	}
	message := fmt.Sprintf("%s，仅获取 %d 条公开动态", prefix, report.Fetched)
	if !full {
		return message + "；登录后可获取更完整的动态"
	}
	// Only a logged-in, unfiltered timeline total is comparable here. The public
	// card-list count can itself be restricted, so it is never a global total.
	if report.ReportedTotal > report.Fetched {
		message += fmt.Sprintf("；按微博返回总数 %d 条估算，本次尚有约 %d 条未返回", report.ReportedTotal, report.ReportedTotal-report.Fetched)
	}
	return message + "；需登录后重新拉取历史动态，免登录结果不代表完整历史"
}

func weiboTimelineTotal(payload map[string]any) int {
	data, _ := payload["data"].(map[string]any)
	total, err := strconv.Atoi(jsonValueString(data["total"]))
	if err != nil || total < 0 {
		return 0
	}
	return total
}

func (b *BilibiliStore) fetchWeiboPostsWithReport(feed SourceConfig, full bool) ([]Post, weiboFetchReport, error) {
	b.RLock()
	credentials, proxyURL := b.config.Weibo, b.config.ProxyURL
	b.RUnlock()
	var previousReport weiboFetchReport
	if hasWeiboAccount(credentials) {
		posts, report, err := b.fetchWeiboTimeline(feed, full, &credentials, proxyURL, false)
		if err == nil {
			return posts, report, nil
		}
		if !isWeiboSessionFailure(err) {
			return nil, report, fmt.Errorf("微博拉取失败：%w", err)
		}
		previousReport = report
		previousReport.LoginExpired = true
	}
	// Restart at public page one on expiry; never join pages from different
	// visibility scopes, which could skip posts or falsely inflate completeness.
	posts, report, err := b.fetchWeiboTimeline(feed, full, &credentials, proxyURL, true)
	report.LoginExpired = previousReport.LoginExpired
	report.ReportedTotal = previousReport.ReportedTotal
	if err != nil {
		err = fmt.Errorf("微博公开动态拉取失败：%w", err)
	}
	return posts, report, err
}

func (b *BilibiliStore) fetchWeiboTimeline(feed SourceConfig, full bool, credentials *WeiboCredentials, proxyURL string, anonymous bool) ([]Post, weiboFetchReport, error) {
	userID := strings.TrimSpace(strings.TrimPrefix(feed.ID, "weibo-"))
	mobileEndpoint := func(page int) string {
		return fmt.Sprintf("https://m.weibo.cn/api/container/getIndex?type=uid&value=%s&containerid=%s&page=%d", url.QueryEscape(userID), url.QueryEscape("107603"+userID), page)
	}
	endpoints := []func(int) string{mobileEndpoint}
	if !anonymous {
		endpoints = []func(int) string{
			func(page int) string {
				return fmt.Sprintf("https://weibo.com/ajax/statuses/mymblog?uid=%s&page=%d&feature=0", url.QueryEscape(userID), page)
			}, mobileEndpoint,
		}
	}
	report := weiboFetchReport{Anonymous: anonymous}
	var lastErr error
	for index, endpoint := range endpoints {
		posts := make([]Post, 0)
		seen := make(map[string]bool)
		for page := 1; page <= 100; page++ {
			var payload map[string]any
			var err error
			if anonymous {
				public := WeiboCredentials{}
				payload, _, err = weiboSessionRequest(endpoint(page), &public, proxyURL)
			} else {
				payload, _, err = b.weiboRawRequest(endpoint(page), credentials, proxyURL)
			}
			if err != nil {
				if anonymous && isWeiboSessionFailure(err) && len(posts) > 0 {
					return posts, report, nil
				}
				if weiboErrorKind(err) == "restricted" || weiboErrorKind(err) == "limited" || isWeiboSessionFailure(err) || len(posts) > 0 {
					return nil, report, err
				}
				lastErr = err
				break
			}
			if !anonymous && index == 0 && page == 1 {
				report.ReportedTotal = weiboTimelineTotal(payload)
			}
			pagePosts := make([]Post, 0)
			collectWeiboPosts(payload, feed, &pagePosts, seen)
			if len(pagePosts) == 0 {
				return posts, report, nil
			}
			posts = append(posts, pagePosts...)
			report.Fetched = len(posts)
			if !full && feed.LastSyncedAt.IsZero() {
				return posts, report, nil
			}
			if !full && !feed.LastSyncedAt.IsZero() {
				for _, post := range pagePosts {
					if !post.Published.After(feed.LastSyncedAt) {
						return posts, report, nil
					}
				}
			}
			if page == 100 {
				report.PageLimited = true
			}
		}
		if len(posts) > 0 {
			return posts, report, nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("接口未返回该博主的动态")
	}
	return nil, report, lastErr
}
