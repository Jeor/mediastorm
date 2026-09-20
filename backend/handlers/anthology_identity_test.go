package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"novastream/models"
)

func TestIndexerSearchPropagatesHydratedTitleIdentity(t *testing.T) {
	for _, admin := range []bool{false, true} {
		fake := &fakeIndexerService{results: []models.NZBResult{}}
		handler := NewIndexerHandler(fake, false)
		handler.SetMetadataService(&fakeSeriesMetadataService{details: &models.SeriesDetails{
			Title:   models.Title{ID: "tmdb:tv:299939", Name: "Monster: The Lizzie Borden Story", Year: 2026},
			Seasons: []models.SeriesSeason{{Number: 1, EpisodeCount: 8}},
		}})
		req := httptest.NewRequest(http.MethodGet, "/api/indexers/search?q=Monster+S01E01&mediaType=series&year=2026", nil)
		rec := httptest.NewRecorder()
		if admin {
			handler.SearchTest(rec, req)
		} else {
			handler.Search(rec, req)
		}
		if rec.Code != http.StatusOK || fake.lastOpts.TitleID != "tmdb:tv:299939" || fake.lastOpts.IMDBID != "" || fake.lastOpts.Query != "Monster S01E01" {
			t.Fatalf("admin=%v status=%d options=%+v", admin, rec.Code, fake.lastOpts)
		}
	}
}
