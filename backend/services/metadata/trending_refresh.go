package metadata

import (
	"context"
	"encoding/json"
	"time"

	"novastream/models"
)

// Refresh an independent snapshot: callers filter and enrich their response in place.
func (s *Service) refreshCachedTrendingLite(key string, items []models.TrendingItem, artworkLimit int) {
	refreshKey := "lite:" + key
	if _, running := s.trendingEnrichInProgress.LoadOrStore(refreshKey, struct{}{}); running {
		return
	}
	data, err := json.Marshal(items)
	if err != nil {
		s.trendingEnrichInProgress.Delete(refreshKey)
		return
	}
	var snapshot []models.TrendingItem
	if err := json.Unmarshal(data, &snapshot); err != nil {
		s.trendingEnrichInProgress.Delete(refreshKey)
		return
	}
	go func() {
		defer s.trendingEnrichInProgress.Delete(refreshKey)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.enrichLiteMissingGenres(ctx, snapshot)
		s.enrichShelfArtwork(ctx, snapshot, artworkLimit)
		if ctx.Err() == nil {
			_ = s.cache.set(key, snapshot)
		}
	}()
}
