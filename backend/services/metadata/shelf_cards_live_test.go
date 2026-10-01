package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in provider contract/performance probe. Normal tests never need a key or
// contact a live service; configured secondary providers are deliberately absent.
func TestShelfCardsLiveTMDBOnly(t *testing.T) {
	key := os.Getenv("STRMR_TMDB_BENCH_API_KEY")
	if key == "" {
		t.Skip("set STRMR_TMDB_BENCH_API_KEY to run live TMDB probe")
	}
	var calls atomic.Int64
	client := &http.Client{Timeout: 15 * time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.themoviedb.org" {
			return nil, fmt.Errorf("unexpected provider host")
		}
		calls.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	cache := newFileCache(t.TempDir(), 24)
	svc := &Service{client: newTVDBClient("", "eng", client, 24), tmdb: newTMDBClient(key, "eng", client, cache), cache: cache, idCache: newFileCache(t.TempDir(), 168), inflightRequests: make(map[string]*inflightRequest)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, mt := range []string{"movie", "tv"} {
		var source struct {
			Results []struct {
				ID int64 `json:"id"`
			} `json:"results"`
		}
		if err := svc.tmdb.doGET(ctx, tmdbBaseURL+"/trending/"+mt+"/day?api_key="+url.QueryEscape(key), &source); err != nil {
			t.Fatal("trending provider request failed")
		}
		input := make([]CuratedItem, 0, 20)
		for i, item := range source.Results {
			if i == 20 {
				break
			}
			typ := "movie"
			if mt == "tv" {
				typ = "series"
			}
			input = append(input, CuratedItem{TMDBID: item.ID, MediaType: typ})
		}
		for _, phase := range []string{"cold", "warm"} {
			start, before := time.Now(), calls.Load()
			cards, err := svc.GetShelfCards(ctx, input)
			if err != nil {
				t.Fatal("card request failed")
			}
			visible := svc.FilterShelfVisibility(ctx, cards, true, true)
			for _, card := range cards {
				if card.Title.Poster == nil {
					t.Fatal("missing card poster")
				}
			}
			row, _ := json.Marshal(map[string]any{"provider": "tmdb-only", "type": mt, "phase": phase, "cards": len(cards), "released": len(visible), "requests": calls.Load() - before, "ms": time.Since(start).Milliseconds()})
			t.Log(string(row))
		}
	}
}
