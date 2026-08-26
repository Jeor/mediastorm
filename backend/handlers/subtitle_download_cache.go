package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultSubtitleDownloadCacheTTL        = 15 * time.Minute
	defaultSubtitleDownloadCacheMaxEntries = 128
)

type subtitleDownloadCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

type subtitleDownloadCache struct {
	mu         sync.Mutex
	entries    map[string]subtitleDownloadCacheEntry
	flights    singleflight.Group
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func newSubtitleDownloadCache(ttl time.Duration, maxEntries int) *subtitleDownloadCache {
	return &subtitleDownloadCache{
		entries:    make(map[string]subtitleDownloadCacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (c *subtitleDownloadCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return append([]byte(nil), entry.data...), true
}

func (c *subtitleDownloadCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for entryKey, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, entryKey)
		}
	}

	if c.maxEntries <= 0 {
		return
	}
	if _, replacing := c.entries[key]; !replacing && len(c.entries) >= c.maxEntries {
		var oldestKey string
		var oldestExpiry time.Time
		for entryKey, entry := range c.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = entryKey
				oldestExpiry = entry.expiresAt
			}
		}
		delete(c.entries, oldestKey)
	}

	c.entries[key] = subtitleDownloadCacheEntry{
		data:      append([]byte(nil), data...),
		expiresAt: now.Add(c.ttl),
	}
}

func subtitleDownloadCacheKey(params SubtitleDownloadParams) string {
	value := func(number *int) string {
		if number == nil {
			return ""
		}
		return fmt.Sprintf("%d", *number)
	}
	canonical := []byte(params.Provider + "\x00" + params.SubtitleID + "\x00" + params.ImdbID + "\x00" +
		params.Title + "\x00" + value(params.Year) + "\x00" + value(params.Season) + "\x00" +
		value(params.Episode) + "\x00" + params.Language)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func isCacheableSubtitleVTT(data []byte) bool {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	return bytes.HasPrefix(trimmed, []byte("WEBVTT"))
}
