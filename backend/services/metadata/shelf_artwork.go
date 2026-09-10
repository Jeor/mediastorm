package metadata

import (
	"context"

	"novastream/models"
)

// Base posters/backdrops and cached artwork are enough to render a first page.
// The frontend requests the optional artwork separately once cards are visible.
func (s *Service) enrichShelfArtworkForLoad(ctx context.Context, items []models.TrendingItem, limit int, deferArtwork bool) bool {
	updated := s.enrichShelfArtworkFromCache(items)
	if !deferArtwork {
		s.enrichShelfArtwork(ctx, items, limit)
	}
	return updated
}
