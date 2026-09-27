package history

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"novastream/models"
)

func TestAirtimeDistinctShowsFinishAfterCallersStopWaiting(t *testing.T) {
	date := time.Now().UTC().Format("2006-01-02")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/lookup/shows" {
			fmt.Fprintf(w, `{"id":%s}`, r.URL.Query().Get("imdb")[2:])
			return
		}
		fmt.Fprintf(w, `[{"type":"regular","airdate":%q,"airtime":"23:15","airstamp":%q}]`, date, date+"T14:15:00Z")
	}))
	defer server.Close()
	svc := &Service{metadataService: &airtimeMetadataService{}, airtimeClient: &tvmazeAirtimeClient{baseURL: server.URL, http: server.Client()}}
	before, _ := svc.GetContinueWatchingRevision("user")
	var wg sync.WaitGroup
	start := make(chan struct{})
	began := time.Now()
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			ep := models.EpisodeReference{AirDate: date, AirDateTimeUTC: date + "T23:59:59Z", AirTimeEstimated: true}
			svc.enrichContinueWatchingAirtime(ctx, models.Title{ID: fmt.Sprintf("tmdb:tv:%d", i), IMDBID: fmt.Sprintf("tt%d", i)}, nil, &ep)
			if !ep.AirTimeEstimated {
				t.Error("cold request unexpectedly completed before rate-limit wait")
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if time.Since(began) > time.Second {
		t.Fatal("callers waited for background work")
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		svc.mu.RLock()
		n := len(svc.airtimeState.entries)
		svc.mu.RUnlock()
		if n == 10 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/10 background resolutions finished", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for i := 1; i <= 10; i++ {
		ep := models.EpisodeReference{AirDate: date, AirTimeEstimated: true}
		svc.enrichContinueWatchingAirtime(context.Background(), models.Title{ID: fmt.Sprintf("tmdb:tv:%d", i), IMDBID: fmt.Sprintf("tt%d", i)}, nil, &ep)
		if ep.AirTimeEstimated || ep.AirDateTimeUTC != date+"T14:15:00Z" {
			t.Fatalf("show %d unresolved: %+v", i, ep)
		}
	}
	if calls.Load() != 20 {
		t.Fatalf("got %d requests, want 20", calls.Load())
	}
	after, _ := svc.GetContinueWatchingRevision("user")
	if before == after {
		t.Fatal("resolution did not change revision")
	}
}

func TestAirtimeCompletionInvalidatesCacheAndRejectsOlderBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lookup/shows" {
			fmt.Fprint(w, `{"id":1505}`)
			return
		}
		fmt.Fprint(w, `[{"airdate":"2026-09-27","airtime":"23:15","airstamp":"2026-09-27T14:15:00Z"}]`)
	}))
	defer server.Close()
	svc := &Service{continueWatchingTTL: 10 * time.Minute, continueWatchingCache: map[string]*cachedContinueWatching{"user": {}}}
	generation := svc.airtimeGeneration()
	client := &tvmazeAirtimeClient{baseURL: server.URL, http: server.Client()}
	if got := svc.resolveAirtime(context.Background(), client, "tt0388629", models.EpisodeReference{AirDate: "2026-09-27"}); got != "2026-09-27T14:15:00Z" {
		t.Fatal(got)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.continueWatchingCache) != 0 {
		t.Fatal("old response retained")
	}
	svc.cacheContinueWatchingLocked("user", nil, generation)
	if len(svc.continueWatchingCache) != 0 {
		t.Fatal("in-flight stale build restored cache")
	}
	svc.cacheContinueWatchingLocked("user", []models.SeriesWatchState{{NextEpisode: &models.EpisodeReference{AirTimeEstimated: true}}}, svc.airtimeState.generation)
	if time.Until(svc.continueWatchingCache["user"].expiresAt) > 30*time.Second {
		t.Fatal("estimated response prevents early retry")
	}
}

func TestAirtimeTransientFailureRetriesSoon(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"id":1505}`)
	}))
	defer server.Close()
	c := &tvmazeAirtimeClient{baseURL: server.URL, http: server.Client()}
	path := "/lookup/shows?imdb=tt0388629"
	if got := c.get(context.Background(), path, time.Hour); len(got) != 0 {
		t.Fatal("failure returned data")
	}
	c.mu.Lock()
	entry := c.cache[path]
	if time.Until(entry.expires) > 30*time.Second {
		t.Fatal("transient failure cached too long")
	}
	entry.expires = time.Now().Add(-time.Second)
	c.cache[path] = entry
	c.mu.Unlock()
	if got := c.get(context.Background(), path, time.Hour); string(got) != `{"id":1505}` {
		t.Fatalf("retry did not recover: %s", got)
	}
}
