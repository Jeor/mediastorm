package handlers

import (
	"fmt"
	"testing"

	"novastream/models"
)

func TestSelectTopTenResponseItemsSkipsIncompleteCandidates(t *testing.T) {
	for _, mediaType := range []string{"all", "movie", "tv"} {
		t.Run(mediaType, func(t *testing.T) {
			items := []models.TrendingItem{}
			for _, kind := range []string{"movie", "series"} {
				for i := 0; i < 14; i++ {
					title := models.Title{Name: fmt.Sprintf("%s %d", kind, i), MediaType: kind, TMDBID: int64(i + 1), Poster: &models.Image{URL: "https://example.com/poster.jpg"}}
					switch i {
					case 0:
						title.Poster = nil
					case 1:
						title.Poster.URL = "  "
					case 2:
						title.TMDBID = 0
					case 3:
						title.TMDBID = 0
						title.IMDBID = "  "
					case 4:
						title.TMDBID = 0
						title.IMDBID = "tt1234567"
					case 5:
						title.TMDBID = 0
						title.TVDBID = 42
					}
					if mediaType == "all" || mediaType == "movie" && kind == "movie" || mediaType == "tv" && kind == "series" {
						items = append(items, models.TrendingItem{Rank: i + 1, Title: title})
					}
				}
			}
			selected := selectTopTenResponseItems(items, mediaType)
			if len(selected) != 10 {
				t.Fatalf("got %d items, want 10 backfilled items", len(selected))
			}
			for i, item := range selected {
				kind, index := "movie", i+4
				if mediaType == "tv" {
					kind = "series"
				}
				if mediaType == "all" {
					index = i/2 + 4
					if i%2 == 1 {
						kind = "series"
					}
				}
				if item.Title.Name != fmt.Sprintf("%s %d", kind, index) || item.Rank != i+1 {
					t.Fatalf("unexpected item at %d: %+v", i, item)
				}
			}
			if items[0].Title.Poster != nil || items[0].Rank != 1 {
				t.Fatal("input mutated")
			}
		})
	}
}
