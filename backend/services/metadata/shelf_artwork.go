package metadata

import (
	"context"

	"novastream/models"
)

type deferredShelfArtworkKey struct{}

// withDeferredShelfArtwork carries the list's artwork policy through shared
// metadata hydration. Required metadata and visibility checks still run.
func withDeferredShelfArtwork(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferredShelfArtworkKey{}, true)
}

func shelfArtworkDeferred(ctx context.Context) bool {
	deferred, _ := ctx.Value(deferredShelfArtworkKey{}).(bool)
	return deferred
}

// GetTopTenCandidatesWithOptions also honors deferral on a cold chart cache.
// A later artwork request enriches a cached base chart without recomputing it.
func (s *Service) GetTopTenCandidatesWithOptions(ctx context.Context, mediaType string, customListURLs []string, opts ShelfLoadOptions) ([]models.TrendingItem, error) {
	if opts.DeferArtwork {
		ctx = withDeferredShelfArtwork(ctx)
	}
	items, err := s.GetTopTenCandidates(ctx, mediaType, customListURLs)
	if err != nil {
		return nil, err
	}
	items = append([]models.TrendingItem(nil), items...)
	s.enrichShelfArtworkForLoad(ctx, items, shelfLoadArtworkLimit(opts), opts.DeferArtwork)
	return items, nil
}

// Base posters/backdrops and cached artwork are enough to render a first page.
// The frontend requests the optional artwork separately once cards are visible.
func (s *Service) enrichShelfArtworkForLoad(ctx context.Context, items []models.TrendingItem, limit int, deferArtwork bool) bool {
	updated := s.enrichShelfArtworkFromCache(items)
	if !deferArtwork {
		s.enrichShelfArtwork(ctx, items, limit)
	}
	return updated
}
