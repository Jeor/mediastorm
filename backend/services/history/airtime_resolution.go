package history

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/sync/singleflight"
	"novastream/models"
)

type airtimeResolution struct {
	stamp   string
	expires time.Time
}

type airtimeResolutionState struct {
	group singleflight.Group
	// Entries and generation are protected by Service.mu.
	entries    map[string]airtimeResolution
	generation int64
}

// Let a shared lookup finish even when the home response stops waiting. Rate
// limiting still applies to every HTTP request, but queue time no longer consumes
// the response's four-second budget and cancels the work for every caller.
func (s *Service) resolveAirtime(ctx context.Context, client *tvmazeAirtimeClient, imdb string, ep models.EpisodeReference) string {
	key := fmt.Sprintf("%s:%s:%d:%d", imdb, ep.AirDate, ep.SeasonNumber, ep.EpisodeNumber)
	s.mu.RLock()
	entry, ok := s.airtimeState.entries[key]
	s.mu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.stamp
	}
	result := s.airtimeState.group.DoChan(key, func() (any, error) {
		s.mu.RLock()
		entry, ok := s.airtimeState.entries[key]
		s.mu.RUnlock()
		if ok && time.Now().Before(entry.expires) {
			return entry.stamp, nil
		}
		workCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		stamp := client.airtime(workCtx, imdb, ep)
		ttl := 30 * time.Second
		if stamp != "" {
			ttl = 6 * time.Hour
		}
		s.mu.Lock()
		if s.airtimeState.entries == nil {
			s.airtimeState.entries = make(map[string]airtimeResolution)
		}
		if len(s.airtimeState.entries) >= 512 {
			for k, v := range s.airtimeState.entries {
				if time.Now().After(v.expires) {
					delete(s.airtimeState.entries, k)
				}
			}
			if len(s.airtimeState.entries) >= 512 {
				for k := range s.airtimeState.entries {
					delete(s.airtimeState.entries, k)
					break
				}
			}
		}
		s.airtimeState.entries[key] = airtimeResolution{stamp: stamp, expires: time.Now().Add(ttl)}
		if stamp != "" {
			s.airtimeState.generation = time.Now().UnixNano()
			clear(s.continueWatchingCache)
		}
		s.mu.Unlock()
		if stamp == "" {
			log.Printf("[history] TVmaze airtime unresolved imdb=%s date=%s; retaining estimated time", imdb, ep.AirDate)
		} else {
			log.Printf("[history] TVmaze airtime resolved imdb=%s date=%s utc=%s", imdb, ep.AirDate, stamp)
		}
		return stamp, nil
	})
	select {
	case <-ctx.Done():
		return ""
	case resolved := <-result:
		stamp, _ := resolved.Val.(string)
		return stamp
	}
}

func (s *Service) airtimeGeneration() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.airtimeState.generation
}

// A background resolution can complete while a response is being built. Never
// put that older response back into the cache after the completion invalidates it.
func (s *Service) cacheContinueWatchingLocked(userID string, items []models.SeriesWatchState, generation int64) {
	if generation != s.airtimeState.generation {
		return
	}
	ttl := s.continueWatchingTTL
	for _, item := range items {
		if item.NextEpisode != nil && item.NextEpisode.AirTimeEstimated && ttl > 30*time.Second {
			ttl = 30 * time.Second
		}
	}
	if s.continueWatchingCache == nil {
		s.continueWatchingCache = make(map[string]*cachedContinueWatching)
	}
	s.continueWatchingCache[userID] = &cachedContinueWatching{items: items, cachedAt: time.Now(), expiresAt: time.Now().Add(ttl)}
}
