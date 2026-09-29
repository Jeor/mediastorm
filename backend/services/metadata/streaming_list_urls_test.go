package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"novastream/models"
)

func TestFetchMDBListCustomResolvesRetiredHuluDefaults(t *testing.T) {
	for _, test := range []struct {
		oldURL string
		path   string
	}{
		{"https://mdblist.com/lists/snoak/top-hulu-movies/json", "/lists/azodath/top-hulu-movies/json"},
		{"https://mdblist.com/lists/snoak/top-hulu-movies/", "/lists/azodath/top-hulu-movies/json"},
		{"https://mdblist.com/lists/snoak/top-tv-shows-hulu/json/", "/lists/azodath/top-hulu-shows/json"},
		{"https://mdblist.com/lists/snoak/top-tv-shows-hulu", "/lists/azodath/top-hulu-shows/json"},
	} {
		t.Run(test.oldURL, func(t *testing.T) {
			client := &tvdbClient{httpc: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "mdblist.com" || req.URL.Path != test.path {
					t.Fatalf("requested %s, want mdblist.com%s", req.URL, test.path)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"title":"Hulu title","imdb_id":"tt123","mediatype":"show"}]`))}, nil
			})}}
			items, err := client.FetchMDBListCustom(test.oldURL)
			if err != nil || len(items) != 1 || items[0].Title != "Hulu title" {
				t.Fatalf("FetchMDBListCustom = %+v, %v", items, err)
			}
		})
	}
}

func TestResolveStreamingListURLPreservesCustomLists(t *testing.T) {
	for _, listURL := range []string{
		"https://mdblist.com/lists/custom/top-hulu-movies/json",
		"https://mdblist.com/lists/snoak/netflix-top-10-movies/json",
		"https://example.com/lists/snoak/top-hulu-movies/json",
		"https://mdblist.com/lists/azodath/top-hulu-shows/json",
	} {
		if got := resolveStreamingListURL(listURL); got != listURL {
			t.Errorf("resolveStreamingListURL(%q) = %q", listURL, got)
		}
	}
}

func TestGetCustomListRetiredHuluURLUsesReplacementCache(t *testing.T) {
	cache := newFileCache(t.TempDir(), 24)
	items := []models.TrendingItem{{Title: models.Title{Name: "Hulu show", MediaType: "series"}}}
	if err := cache.set(cacheKey("mdblist", "custom", "v8", "lite", "https://mdblist.com/lists/azodath/top-hulu-shows/json", "en"), items); err != nil {
		t.Fatal(err)
	}
	svc := &Service{cache: cache, client: &tvdbClient{language: "en"}}
	got, total, unfiltered, err := svc.GetCustomList(context.Background(), "https://mdblist.com/lists/snoak/top-tv-shows-hulu/json", CustomListOptions{})
	if err != nil || len(got) != 1 || got[0].Title.Name != "Hulu show" || total != 1 || unfiltered != 1 {
		t.Fatalf("GetCustomList = %+v, %d, %d, %v", got, total, unfiltered, err)
	}
}
