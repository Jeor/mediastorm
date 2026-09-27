package handlers

import (
	"fmt"
	"novastream/models"
	"testing"
)

func TestHomeViewMediaTypes(t *testing.T) {
	for _, mediaType := range []string{"series", "tv", "show", "episode"} {
		if !matchesHomeView(mediaType, "shows") || matchesHomeView(mediaType, "movies") {
			t.Fatal(mediaType)
		}
	}
	if matchesHomeView("channel", "shows") || matchesHomeView("", "movies") {
		t.Fatal("non-title content matched")
	}
}
func TestTopTenHomeViewsFillAfterFiltering(t *testing.T) {
	var candidates []models.TrendingItem
	// First ten are mixed; each scoped view must continue past those candidates.
	for i := 0; i < 24; i++ {
		mediaType := "movie"
		if i%2 == 1 {
			mediaType = "series"
		}
		candidates = append(candidates, models.TrendingItem{Title: models.Title{ID: fmt.Sprint(i), MediaType: mediaType, TMDBID: int64(i + 1), Poster: &models.Image{URL: "poster"}}})
	}
	for _, mediaType := range []string{"movie", "tv"} {
		got := selectTopTenResponseItems(candidates, mediaType)
		if len(got) != 10 {
			t.Fatalf("%s got %d", mediaType, len(got))
		}
		view := "movies"
		if mediaType == "tv" {
			view = "shows"
		}
		for i, item := range got {
			if !matchesHomeView(item.Title.MediaType, view) || item.Rank != i+1 {
				t.Fatalf("bad item: %+v", item)
			}
		}
	}
}
