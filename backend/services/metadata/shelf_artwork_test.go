package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"novastream/models"
)

func TestDiscoveryFirstPageDefersArtworkAndContinuesWithoutGaps(t *testing.T) {
	for _, kind := range []string{"genre", "decade"} {
		t.Run(kind, func(t *testing.T) {
			cache := newFileCache(t.TempDir(), 24)
			var artworkRequests atomic.Int32
			var pages []int
			httpc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := `{"backdrops":[],"posters":[],"logos":[]}`
				if req.URL.Path == "/3/discover/movie" {
					page, _ := strconv.Atoi(req.URL.Query().Get("page"))
					pages = append(pages, page)
					var titles []string
					for i := 1; i <= 20; i++ {
						id := (page-1)*20 + i
						titles = append(titles, fmt.Sprintf(`{"id":%d,"title":"Movie %d","poster_path":"/base.jpg","release_date":"2020-01-01"}`, id, id))
					}
					body = fmt.Sprintf(`{"results":[%s],"total_results":100}`, strings.Join(titles, ","))
				} else {
					artworkRequests.Add(1)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			svc := &Service{client: &tvdbClient{language: "eng"}, cache: cache, tmdb: newTMDBClient("test", "eng", httpc, cache)}
			fetch := func(limit, offset int, deferArtwork bool) ([]models.TrendingItem, int, error) {
				opts := ShelfLoadOptions{Lite: true, ArtworkLimit: 20, DeferArtwork: deferArtwork}
				if kind == "genre" {
					return svc.DiscoverByGenreWithOptions(context.Background(), "movie", 28, limit, offset, opts)
				}
				return svc.DiscoverByDecadeWithOptions(context.Background(), "movie", 2020, limit, offset, opts)
			}
			first, total, err := fetch(20, 0, true)
			if err != nil || len(first) != 20 || total != 100 {
				t.Fatalf("first page: len=%d total=%d err=%v", len(first), total, err)
			}
			if len(pages) != 1 || artworkRequests.Load() != 0 || first[0].Title.Poster == nil {
				t.Fatalf("expected one source request, base poster and no artwork fetch: pages=%v artwork=%d", pages, artworkRequests.Load())
			}
			next, _, err := fetch(50, 20, true)
			if err != nil || len(next) != 50 || next[0].Title.TMDBID != 21 || next[49].Title.TMDBID != 70 {
				t.Fatalf("continuation skipped or repeated titles: len=%d err=%v", len(next), err)
			}
			if _, _, err := fetch(20, 0, false); err != nil || artworkRequests.Load() == 0 {
				t.Fatalf("explicit refresh should fetch artwork: requests=%d err=%v", artworkRequests.Load(), err)
			}
		})
	}
}

func TestDeferredCustomListKeepsCachedArtworkAndTotal(t *testing.T) {
	cache := newFileCache(t.TempDir(), 24)
	var requests atomic.Int32
	httpc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	})}
	svc := &Service{client: &tvdbClient{language: "eng"}, cache: cache, tmdb: newTMDBClient("test", "eng", httpc, cache)}
	const listURL = "https://mdblist.com/lists/test/discovery/json"
	items := []models.TrendingItem{
		{Rank: 1, Title: models.Title{TMDBID: 1, MediaType: "series", Name: "First", Genres: []string{"Drama"}}},
		{Rank: 2, Title: models.Title{TMDBID: 2, MediaType: "series", Name: "Second", Genres: []string{"Drama"}}},
	}
	if err := cache.set(cacheKey("mdblist", "custom", "v8", "fast", listURL, "eng"), items); err != nil {
		t.Fatal(err)
	}
	if err := cache.set(cacheKey("tmdb", "images", "v10", "eng", "series", "1"), tmdbImagesResult{
		Logo: &models.Image{URL: "https://example.com/logo.png", Type: "logo"},
	}); err != nil {
		t.Fatal(err)
	}
	page, total, unfiltered, err := svc.GetCustomList(context.Background(), listURL, CustomListOptions{
		Lite: true, Limit: 1, ArtworkLimit: 20, DeferArtwork: true,
	})
	if err != nil || len(page) != 1 || total != 2 || unfiltered != 2 {
		t.Fatalf("page=%v total=%d unfiltered=%d err=%v", page, total, unfiltered, err)
	}
	if requests.Load() != 0 || page[0].Title.Logo == nil {
		t.Fatalf("cached artwork should render without network: requests=%d logo=%v", requests.Load(), page[0].Title.Logo)
	}
}

func TestImportedAndTopTenListsDeferArtworkWithoutPoisoningCache(t *testing.T) {
	for _, kind := range []string{"curated", "top-ten"} {
		t.Run(kind, func(t *testing.T) {
			cache := newFileCache(t.TempDir(), 24)
			var requests atomic.Int32
			httpc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests.Add(1)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"backdrops":[],"posters":[],"logos":[]}`))}, nil
			})}
			svc := &Service{client: &tvdbClient{language: "eng"}, cache: cache, tmdb: newTMDBClient("test", "eng", httpc, cache)}
			raw := []CuratedItem{{Title: "First", TMDBID: 1, MediaType: "movie"}}
			items := []models.TrendingItem{{Rank: 1, Title: models.Title{ID: "tmdb:movie:1", TMDBID: 1, MediaType: "movie", Name: "First", Poster: &models.Image{URL: "base.jpg"}}}}
			key := svc.curatedListCacheID(raw)
			if kind == "top-ten" {
				key = topTenCacheKey("movie", nil, "eng")
			}
			if err := cache.set(key, items); err != nil {
				t.Fatal(err)
			}
			fetch := func(deferArtwork bool) ([]models.TrendingItem, error) {
				opts := ShelfLoadOptions{DeferArtwork: deferArtwork}
				if kind == "top-ten" {
					return svc.GetTopTenCandidatesWithOptions(context.Background(), "movie", nil, opts)
				}
				return svc.GetCuratedListWithOptions(context.Background(), raw, "Imported", opts)
			}
			got, err := fetch(true)
			if err != nil || len(got) != 1 || got[0].Title.Poster == nil || requests.Load() != 0 {
				t.Fatalf("base cards must not fetch optional images: items=%v requests=%d err=%v", got, requests.Load(), err)
			}
			// Even nested metadata hydration must not fetch uncached artwork.
			if _, err := svc.cachedFetchImages(withDeferredShelfArtwork(context.Background()), "movie", 99); err != nil || requests.Load() != 0 {
				t.Fatalf("nested deferred image fetch: requests=%d err=%v", requests.Load(), err)
			}
			if _, err := fetch(false); err != nil || requests.Load() == 0 {
				t.Fatalf("explicit artwork refresh did not fetch images: requests=%d err=%v", requests.Load(), err)
			}
		})
	}
}
