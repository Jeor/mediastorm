package metadata

import (
	"fmt"
	"io"
	"net/http"
	"novastream/models"
	"strings"
	"testing"
)

func TestEpisodeNumberingWithProviderKeysAndCachedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, tvdbKey, tmdbKey string
		mismatch               bool
		want                   string
	}{
		{"both keys", "tvdb-key", "tmdb-key", false, "tvdb:series:423075"},
		{"TVDB only", "tvdb-key", "", false, "tvdb:series:423075"},
		{"TMDB only", "", "tmdb-key", false, "tmdb:tv:207468"},
		{"both keys provider fallback", "tvdb-key", "tmdb-key", true, "tmdb:tv:207468"},
	} {
		for _, lite := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/lite=%v", tc.name, lite), func(t *testing.T) {
				cache := newFileCache(t.TempDir(), 24)
				httpc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				})}
				svc := &Service{client: newTVDBClient(tc.tvdbKey, "eng", httpc, 24), tmdb: newTMDBClient(tc.tmdbKey, "eng", httpc, cache), cache: cache, idCache: newFileCache(t.TempDir(), 168), inflightRequests: make(map[string]*inflightRequest)}
				details := models.SeriesDetails{Title: models.Title{ID: "tvdb:series:423075", TVDBID: 423075, TMDBID: 207468, IMDBID: "tt21975436", Name: "Kaiju No 8", Backdrop: &models.Image{URL: "test"}, Logo: &models.Image{URL: "test", Width: 10, Height: 10}, Credits: &models.Credits{}, Genres: []string{"Anime"}, Certification: "TV-14"}, EpisodeTMDBEnriched: true, Seasons: []models.SeriesSeason{{Number: 2, EpisodeCount: 1, Episodes: []models.SeriesEpisode{{SeasonNumber: 2, EpisodeNumber: 1, Name: "Episode"}}}}}
				if tc.mismatch {
					details.Title.TMDBID = 999
				}
				if tc.tvdbKey != "" {
					if err := cache.set(seriesDetailsCacheKey("eng", 423075, ""), details); err != nil {
						t.Fatal(err)
					}
				}
				details.Title.ID = "tmdb:tv:207468"
				details.Title.TMDBID = 207468
				details.Seasons[0].Number = 1
				details.Seasons[0].Episodes[0].SeasonNumber = 1
				details.Seasons[0].Episodes[0].EpisodeNumber = 13
				if err := cache.set(cacheKey("tmdb", "series", "details-fallback", "v4", "eng", "207468"), details); err != nil {
					t.Fatal(err)
				}
				req := models.SeriesDetailsQuery{TitleID: "tmdb:tv:207468", TMDBID: 207468, TVDBID: 423075, IMDBID: "tt21975436"}
				fetch := svc.SeriesDetails
				if lite {
					fetch = svc.SeriesDetailsLite
				}
				got, err := fetch(t.Context(), req)
				if err != nil {
					t.Fatal(err)
				}
				if got.Numbering == nil || got.Numbering.SeriesID != tc.want {
					t.Fatalf("numbering=%+v want %s", got.Numbering, tc.want)
				}
				if !models.SameEpisodeNumbering(got.Numbering, got.Seasons[0].Episodes[0].Numbering) {
					t.Fatal("episode lost numbering")
				}
			})
		}
	}
}
