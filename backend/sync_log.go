package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sync/atomic"
	"time"
)

var syncLogSequence atomic.Uint64
var syncHTTPStatusPattern = regexp.MustCompile(`(?:HTTP |HTTP状态码[:： ]*|接口返回 )(\d{3})\b`)
var syncPostIDPattern = regexp.MustCompile(`^weibo-status-[0-9]{1,20}$`)

func logArchiveProgress(feed SourceConfig, processed, total int, start time.Time) {
	log.Printf("event=archive_progress platform=%s source=%s processed=%d total=%d duration_ms=%d", syncLogPlatform(feed.Source), sessionDigest(feed.ID)[:12], processed, total, time.Since(start).Milliseconds())
}

func logLivePhotoFailure(feed SourceConfig, post Post, index int, err error) {
	id := post.ID
	if !syncPostIDPattern.MatchString(id) {
		id = sessionDigest(id)[:12]
	}
	log.Printf("event=media_download_failed platform=%s source=%s post=%s media=live_photo index=%d %s", syncLogPlatform(feed.Source), sessionDigest(feed.ID)[:12], id, index+1, syncLogError(err))
}

func syncLogPlatform(source Source) string {
	switch source {
	case SourceWeibo, SourceBilibili, SourcePixiv, SourceTwitter:
		return string(source)
	}
	return "unknown"
}

// Do not log err.Error(): transport errors include signed URLs and proxy secrets.
func syncLogError(err error) string {
	if err == nil {
		return "error_class=none"
	}
	var media *livePhotoDownloadError
	if errors.As(err, &media) {
		return fmt.Sprintf("error_class=%s http=%d media_host=%q", media.kind, media.status, media.host)
	}
	var platform *weiboRequestError
	if errors.As(err, &platform) {
		status := "0"
		if match := syncHTTPStatusPattern.FindStringSubmatch(platform.message); len(match) == 2 {
			status = match[1]
		}
		return fmt.Sprintf("error_class=%s http=%s", platform.kind, status)
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return "error_class=timeout"
		}
		return "error_class=network"
	}
	var file *os.PathError
	if errors.As(err, &file) {
		return "error_class=local_io"
	}
	status := "0"
	if match := syncHTTPStatusPattern.FindStringSubmatch(err.Error()); len(match) == 2 {
		status = match[1]
	}
	return fmt.Sprintf("error_class=upstream_or_storage http=%s error_type=%T", status, err)
}

type syncTrace struct {
	id               uint64
	platform, source string
	start, mark      time.Time
}

func startSyncTrace(feed SourceConfig, full bool) *syncTrace {
	now := time.Now()
	trace := &syncTrace{id: syncLogSequence.Add(1), platform: syncLogPlatform(feed.Source), source: sessionDigest(feed.ID)[:12], start: now, mark: now}
	mode := "incremental"
	if full {
		mode = "history"
	}
	log.Printf("event=sync_start sync=%d platform=%s source=%s mode=%s", trace.id, trace.platform, trace.source, mode)
	return trace
}

func (trace *syncTrace) stage(stage string, count int, err error) {
	status := "success"
	if err != nil {
		status = "failed"
	}
	log.Printf("event=sync_%s sync=%d platform=%s source=%s status=%s count=%d duration_ms=%d %s", stage, trace.id, trace.platform, trace.source, status, count, time.Since(trace.mark).Milliseconds(), syncLogError(err))
	trace.mark = time.Now()
}

func (trace *syncTrace) finish(added int, warning bool, err error) {
	status := "success"
	if warning {
		status = "partial"
	}
	if err != nil {
		status = "failed"
	}
	log.Printf("event=sync_end sync=%d platform=%s source=%s status=%s added=%d warning=%t duration_ms=%d %s", trace.id, trace.platform, trace.source, status, added, warning, time.Since(trace.start).Milliseconds(), syncLogError(err))
}
