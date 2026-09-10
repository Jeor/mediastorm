package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"novastream/models"
)

func TestCachedTrendingLiteReturnsBeforeRemoteEnrichment(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	httpc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"genres":[{"name":"Drama"}],"logos":[],"posters":[],"backdrops":[]}`)), Header: make(http.Header)}, nil
	})}
	cache := newFileCache(t.TempDir(), 24)
	svc := &Service{cache: cache, client: &tvdbClient{language: "eng"}, tmdb: newTMDBClient("test", "eng", httpc, cache)}
	key := cacheKey("mdblist", "trending", "series", "v8", "fast", "eng")
	if err := cache.set(key, []models.TrendingItem{{Rank: 1, Title: models.Title{Name: "Cached show", MediaType: "series", TMDBID: 123}}}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(release)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, running := svc.trendingEnrichInProgress.Load("lite:" + key); !running {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Error("background refresh did not finish")
	}()
	returned := make(chan []models.TrendingItem, 1)
	go func() {
		items, _ := svc.TrendingWithOptions(context.Background(), "series", ShelfLoadOptions{Lite: true, ArtworkLimit: 20})
		returned <- items
	}()
	select {
	case items := <-returned:
		if len(items) != 1 || items[0].Title.Name != "Cached show" {
			t.Fatalf("unexpected items: %+v", items)
		}
		// Mutating the response must not affect the background snapshot.
		items[0].Title.Name = "User response"
	case <-time.After(time.Second):
		t.Fatal("cached list blocked on remote enrichment")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background enrichment did not start")
	}
}
