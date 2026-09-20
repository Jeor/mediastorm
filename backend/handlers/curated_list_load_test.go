package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"novastream/models"
	"novastream/services/metadata"
)

type deferredCuratedService struct {
	metadataService
	opts metadata.ShelfLoadOptions
}

func (s *deferredCuratedService) GetCuratedListWithOptions(_ context.Context, _ []metadata.CuratedItem, _ string, opts metadata.ShelfLoadOptions) ([]models.TrendingItem, error) {
	s.opts = opts
	return []models.TrendingItem{{Title: models.Title{Name: "First"}}}, nil
}

func TestImportedListLoadPassesArtworkPolicy(t *testing.T) {
	service := &deferredCuratedService{}
	r := httptest.NewRequest("GET", "/?lite=true&deferArtwork=true&artworkLimit=20", nil)
	items, err := getCuratedListForRequest(r, service, nil, "List")
	if err != nil || len(items) != 1 || !service.opts.DeferArtwork || !service.opts.Lite || service.opts.ArtworkLimit != 20 {
		t.Fatalf("items=%v opts=%+v err=%v", items, service.opts, err)
	}
	_, err = getCuratedListForRequest(httptest.NewRequest("GET", "/?deferArtwork=false", nil), service, nil, "List")
	if err != nil || service.opts.DeferArtwork {
		t.Fatalf("explicit artwork request ignored: %+v %v", service.opts, err)
	}
}

type personalizedArtworkService struct {
	metadataService
	requests int
}

func (s *personalizedArtworkService) GetCachedArtworkURLs(string, int64, int64) (string, string, []string) {
	return "cached-poster.jpg", "", nil
}

func (s *personalizedArtworkService) ApplyLocalizedArtwork(_ context.Context, title *models.Title) bool {
	s.requests++
	title.Backdrops = []models.Image{{URL: "one.jpg"}, {URL: "two.jpg"}}
	return true
}

func TestPersonalizedCardsUseCachedArtworkBeforeRefresh(t *testing.T) {
	service := &personalizedArtworkService{}
	items := []models.TrendingItem{{Title: models.Title{TMDBID: 1, MediaType: "movie"}}}
	enrichPersonalizedArtwork(context.Background(), items, service, true)
	if service.requests != 0 || items[0].Title.TextPoster == nil {
		t.Fatal("deferred cards should retain cached art without fetching")
	}
	enrichPersonalizedArtwork(context.Background(), items, service, false)
	if service.requests != 1 || len(items[0].Title.Backdrops) != 2 {
		t.Fatal("background artwork should enrich cards")
	}
}
