package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"novastream/models"
)

// The service has already checked release dates before returning this page.
// Lite rows can still carry year-only statuses, so filtering them again loses
// released current-year movies and shows and truncates pagination totals.
func TestCustomListVisibilityPreservesVerifiedLitePage(t *testing.T) {
	for _, mediaType := range []string{"movie", "series"} {
		t.Run(mediaType, func(t *testing.T) {
			status := models.MovieReleaseStatusUpcoming
			if mediaType == "series" {
				status = models.SeriesReleaseStatusUnreleased
			}
			fake := &fakeMetadataService{
				customListResp:  []models.TrendingItem{{Title: models.Title{Name: "Released streaming title", MediaType: mediaType, Year: currentYear(), Status: status}}},
				customListTotal: 40, customListUnfiltered: 45,
			}
			cfg := testConfigManager(t)
			settings, err := cfg.Load()
			if err != nil {
				t.Fatal(err)
			}
			settings.Display.IncludeUnreleasedMoviesInLists = mediaType != "movie"
			settings.Display.IncludeUnreleasedShowsInLists = mediaType != "series"
			if err := cfg.Save(settings); err != nil {
				t.Fatal(err)
			}
			handler := NewMetadataHandler(fake, cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/lists/custom?url=https://mdblist.com/lists/test/streaming/json&limit=1&offset=2&lite=true", nil)
			rec := httptest.NewRecorder()
			handler.CustomList(rec, req)
			opts := fake.lastCustomListOptions
			if opts.HideUnreleasedMovies != (mediaType == "movie") || opts.HideUnreleasedShows != (mediaType == "series") || opts.Limit != 1 || opts.Offset != 2 {
				t.Fatalf("wrong filtering/pagination options: %+v", opts)
			}
			var response CustomListResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Items) != 1 || response.Total != 40 || response.UnfilteredTotal != 45 {
				t.Fatalf("released lite page lost or totals truncated: %+v", response)
			}
		})
	}
}
