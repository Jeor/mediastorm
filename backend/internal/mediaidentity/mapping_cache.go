package mediaidentity

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const mappingTTL = 24 * time.Hour
const maxMappingBytes = 16 << 20

func (s *EpisodeMappingService) load(ctx context.Context, key, url string, parse func([]byte) (any, error)) {
	for {
		s.mu.Lock()
		old, exists := s.entries[key]
		if exists && (time.Now().Before(old.retry) || time.Since(old.snapshot.CheckedAt) < mappingTTL) {
			s.mu.Unlock()
			return
		}
		if wait := s.pending[key]; wait != nil {
			s.mu.Unlock()
			if old.data != nil {
				return
			}
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return
			}
		}
		if len(s.pending) >= 8 {
			s.mu.Unlock()
			return
		}
		done := make(chan struct{})
		s.pending[key] = done
		s.mu.Unlock()
		refresh := func() {
			defer func() { s.mu.Lock(); delete(s.pending, key); close(done); s.mu.Unlock() }()
			refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			s.refresh(refreshCtx, key, url, old, parse)
		}
		if old.data != nil { // keep a usable snapshot without adding playback latency
			go func() {
				bg, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				defer func() { s.mu.Lock(); delete(s.pending, key); close(done); s.mu.Unlock() }()
				s.refresh(bg, key, url, old, parse)
			}()
		} else {
			refresh()
		}
		return
	}
}
func (s *EpisodeMappingService) refresh(ctx context.Context, key, url string, old mappingEntry, parse func([]byte) (any, error)) {
	if old.data == nil && s.repo != nil {
		saved, err := s.repo.Get(ctx, key)
		if err != nil {
			log.Printf("[episode-mapping] cache read failed source=%s", key)
		}
		if saved != nil {
			if data, err := parse(saved.Body); err == nil {
				old = mappingEntry{snapshot: *saved, data: data}
				s.putEntry(key, old)
				if time.Since(saved.CheckedAt) < mappingTTL {
					return
				}
			}
		}
	}
	updated, err := s.fetch(ctx, key, url, old.snapshot, parse)
	if err != nil {
		old.retry = time.Now().Add(5 * time.Minute)
		s.putEntry(key, old)
		log.Printf("[episode-mapping] refresh failed source=%s stale=%t: %v", key, old.data != nil, err)
		return
	}
	if s.repo != nil {
		if err := s.repo.Put(ctx, updated.snapshot); err != nil {
			log.Printf("[episode-mapping] cache write failed source=%s", key)
		}
	}
	s.putEntry(key, updated)
	log.Printf("[episode-mapping] refreshed source=%s bytes=%d", key, len(updated.snapshot.Body))
}
func (s *EpisodeMappingService) putEntry(key string, e mappingEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Bound process memory; persisted snapshots can be loaded again on demand.
	if _, exists := s.entries[key]; !exists && len(s.entries) >= 512 {
		for k := range s.entries {
			if k != "anime-lists" && s.pending[k] == nil {
				delete(s.entries, k)
				break
			}
		}
	}
	s.entries[key] = e
}
func (s *EpisodeMappingService) fetch(ctx context.Context, key, url string, old MappingSnapshot, parse func([]byte) (any, error)) (mappingEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return mappingEntry{}, err
	}
	req.Header.Set("User-Agent", "MediaStorm/1.0 (episode mapping cache)")
	if len(old.Body) > 0 {
		req.Header.Set("If-None-Match", old.ETag)
		req.Header.Set("If-Modified-Since", old.Modified)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return mappingEntry{}, fmt.Errorf("upstream request failed")
	}
	defer resp.Body.Close()
	snapshot := old
	if resp.StatusCode != http.StatusNotModified {
		if resp.StatusCode != http.StatusOK {
			return mappingEntry{}, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxMappingBytes+1))
		if err != nil {
			return mappingEntry{}, err
		}
		if len(body) > maxMappingBytes {
			return mappingEntry{}, fmt.Errorf("mapping response too large")
		}
		snapshot = MappingSnapshot{Key: key, Body: body, ETag: resp.Header.Get("ETag"), Modified: resp.Header.Get("Last-Modified")}
	}
	data, err := parse(snapshot.Body)
	if err != nil {
		return mappingEntry{}, fmt.Errorf("invalid mapping payload: %w", err)
	}
	snapshot.CheckedAt = time.Now()
	return mappingEntry{snapshot: snapshot, data: data}, nil
}
