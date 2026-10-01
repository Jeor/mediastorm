package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"novastream/models"
)

func shelfTestService(t *testing.T) (*Service, func() map[string]int) {
	t.Helper()
	var mu sync.Mutex
	counts := map[string]int{}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.themoviedb.org" {
			t.Errorf("optional provider on critical path: %s", r.URL.Host)
		}
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		var id int
		fmt.Sscanf(r.URL.Path, "/3/movie/%d", &id)
		body := fmt.Sprintf(`{"id":%d,"title":"Movie %d","poster_path":"/poster.jpg","genres":[{"id":18,"name":"Drama"}],"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"type":4,"certification":"PG","release_date":"2020-01-01T00:00:00Z"}]}]}}`, id, id)
		if strings.Contains(r.URL.Path, "/tv/") {
			fmt.Sscanf(r.URL.Path, "/3/tv/%d", &id)
			body = fmt.Sprintf(`{"id":%d,"name":"Series","poster_path":"/poster.jpg","first_air_date":"2020-01-01"}`, id)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	cache := newFileCache(t.TempDir(), 24)
	svc := &Service{client: newTVDBClient("", "eng", client, 24), tmdb: newTMDBClient("test", "eng", client, cache), cache: cache, idCache: newFileCache(t.TempDir(), 168), inflightRequests: make(map[string]*inflightRequest)}
	svc.tmdb.minInterval = 0
	return svc, func() map[string]int {
		mu.Lock()
		defer mu.Unlock()
		c := map[string]int{}
		for k, v := range counts {
			c[k] = v
		}
		return c
	}
}

func TestShelfCardsTMDBOnlyPageCacheAndReleaseReuse(t *testing.T) {
	svc, counts := shelfTestService(t)
	input := make([]CuratedItem, 20)
	for i := range input {
		input[i] = CuratedItem{TMDBID: int64(i + 1), MediaType: "movie"}
	}
	cards, err := svc.GetShelfCards(context.Background(), input)
	if err != nil || len(cards) != 20 {
		t.Fatalf("cards=%d err=%v", len(cards), err)
	}
	visible := svc.FilterShelfVisibility(context.Background(), cards, true, true)
	if len(visible) != 20 || visible[0].Title.Certification != "PG" {
		t.Fatalf("release metadata not reused: %+v", visible)
	}
	for path, n := range counts() {
		if strings.HasSuffix(path, "release_dates") || strings.HasSuffix(path, "images") || n != 1 {
			t.Fatalf("redundant requests %v", counts())
		}
	}
	if len(counts()) != 20 {
		t.Fatalf("expected one TMDB request per card: %v", counts())
	}
	cards[0].Title.Name = "mutated"
	again, err := svc.GetShelfCards(context.Background(), input)
	if err != nil || again[0].Title.Name == "mutated" {
		t.Fatal("cache result was shared/mutated", err)
	}
	for _, n := range counts() {
		if n != 1 {
			t.Fatalf("repeat contacted provider: %v", counts())
		}
	}
	t.Logf("TMDB-only: 20 cards + release filtering = 20 requests; repeated page = 0 additional")
}

func TestShelfCardsOverlappingRequestsShareWork(t *testing.T) {
	svc, counts := shelfTestService(t)
	input := []CuratedItem{{TMDBID: 1, MediaType: "movie"}, {TMDBID: 2, MediaType: "series"}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.GetShelfCards(context.Background(), input)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, n := range counts() {
		if n != 1 {
			t.Fatalf("duplicate work: %v", counts())
		}
	}
}

func TestShelfCachedWaiterCancellationAndPrivateCopies(t *testing.T) {
	svc, _ := shelfTestService(t)
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	fetch := func(context.Context) (any, error) {
		close(started)
		<-release
		return []models.Title{{Name: "original"}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		var out []models.Title
		finished <- svc.fetchShelfCached(ctx, "cancel-test", time.Hour, &out, fetch)
	}()
	<-started
	cancel()
	if err := <-finished; err != context.Canceled {
		t.Fatalf("waiter not cancelled: %v", err)
	}
	close(release)
	var out []models.Title
	if err := svc.fetchShelfCached(context.Background(), "cancel-test", time.Hour, &out, fetch); err != nil {
		t.Fatal(err)
	}
	if out[0].Name != "original" {
		t.Fatal(out)
	}
}

func TestShelfSourceCachesMembershipWithoutMetadata(t *testing.T) {
	svc, _ := shelfTestService(t)
	calls := 0
	svc.client.httpc = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := json.Marshal([]mdblistItem{{IMDBID: "tt123", Title: "Source", MediaType: "movie"}})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	for i := 0; i < 2; i++ {
		items, err := svc.GetCustomListSource(context.Background(), "https://mdblist.com/lists/test/source/json")
		if err != nil || len(items) != 1 {
			t.Fatalf("%v %v", items, err)
		}
	}
	if calls != 1 {
		t.Fatalf("source refetched %d times", calls)
	}
}

func TestCanceledCuratedLoadDoesNotPoisonCaches(t *testing.T) {
	svc, _ := shelfTestService(t)
	started := make(chan struct{})
	cancelMode := true
	svc.tmdb.httpc = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if cancelMode {
			close(started)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{"id":7,"title":"Movie","release_date":"2020-01-01"}]}`))}, nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	input := []CuratedItem{{Title: "Movie", Year: 2020, MediaType: "movie"}}
	go func() { _, err := svc.GetCuratedList(ctx, input, "cancelled"); done <- err }()
	<-started
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("canceled hydration returned %v", err)
	}
	var cached []models.TrendingItem
	if ok, _ := svc.cache.get(svc.curatedListCacheID(input), &cached); ok {
		t.Fatal("cached incomplete list after cancellation")
	}
	cancelMode = false
	if id := svc.resolveTMDBMovieByTitleYear(context.Background(), "Movie", 2020); id != 7 {
		t.Fatalf("canceled lookup poisoned a retry: %d", id)
	}
}
