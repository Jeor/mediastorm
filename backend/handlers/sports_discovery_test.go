package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"novastream/models"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSportsAddonTargetsSearchAndLeavesReplayBrowsingAvailable(t *testing.T) {
	var search, ordinary, replay atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/manifest.json":
			w.Write([]byte(`{"id":"test","catalogs":[{"type":"tv","id":"replays","name":"Replays"},{"type":"tv","id":"live","name":"Live","extra":[{"name":"search"}]}]}`))
		case strings.Contains(r.URL.Path, "replays"):
			replay.Add(1)
			w.Write([]byte(`{"metas":[{"id":"historic","name":"History"}]}`))
		case strings.Contains(r.URL.Path, "search="):
			search.Add(1)
			w.Write([]byte(`{"metas":[{"id":"game","name":"New York Yankees vs Boston Red Sox"}]}`))
		case strings.Contains(r.URL.Path, "catalog"):
			ordinary.Add(1)
			w.Write([]byte(`{"metas":[{"id":"game","name":"New York Yankees vs Boston Red Sox"}]}`))
		default:
			t.Error("unexpected request: ", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := newStremioTestHandler(t, server.URL)
	d := &sportsDiscoveryRequest{game: models.SportsGame{League: "mlb", Sport: "baseball", HomeTeam: models.SportsTeam{Name: "Boston Red Sox"}, AwayTeam: models.SportsTeam{Name: "New York Yankees"}}}
	channels, err := h.fetchSportsAddon(context.Background(), server.URL+"/manifest.json", "", d)
	if err != nil || len(channels) != 1 {
		t.Fatalf("channels=%v error=%v", channels, err)
	}
	if search.Load() != 1 || ordinary.Load() != 0 || replay.Load() != 0 {
		t.Fatalf("search=%d ordinary=%d replay=%d", search.Load(), ordinary.Load(), replay.Load())
	}
	if _, err = h.fetchStremioChannels(context.Background(), server.URL+"/manifest.json", ""); err != nil {
		t.Fatal(err)
	}
	if replay.Load() != 1 {
		t.Fatal("ordinary Live TV replay browsing changed")
	}
}

func TestSportsAddonFallsBackWhenAdvertisedSearchUnsupported(t *testing.T) {
	var catalogs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/manifest.json":
			w.Write([]byte(`{"catalogs":[{"type":"tv","id":"live","extra":[{"name":"search"}]}]}`))
		case strings.Contains(r.URL.Path, "search="):
			http.NotFound(w, r)
		default:
			catalogs.Add(1)
			w.Write([]byte(`{"metas":[{"id":"game","name":"A vs B"}]}`))
		}
	}))
	defer server.Close()
	h := newStremioTestHandler(t, server.URL)
	channels, err := h.fetchSportsAddon(context.Background(), server.URL+"/manifest.json", "", &sportsDiscoveryRequest{game: models.SportsGame{Title: "A vs B"}})
	if err != nil || len(channels) != 1 || catalogs.Load() != 1 {
		t.Fatalf("fallback channels=%d calls=%d err=%v", len(channels), catalogs.Load(), err)
	}
}

func TestXtreamDiscoveryFetchesOnlySelectedCategories(t *testing.T) {
	var streamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "get_live_categories" {
			w.Write([]byte(`[{"category_id":"1","category_name":"Sports"},{"category_id":"2","category_name":"Movies"}]`))
			return
		}
		streamCalls.Add(1)
		if r.URL.Query().Get("category_id") != "1" {
			t.Error("fetched unwanted catalog")
		}
		w.Write([]byte(`[{"stream_id":1,"name":"Sports channel","stream_type":"live","category_id":"1"}]`))
	}))
	defer server.Close()
	h := newStremioTestHandler(t, server.URL)
	for i := 0; i < 2; i++ {
		channels, err := h.fetchXtreamChannelsUncached(context.Background(), server.URL, "user", "password", "", "sports")
		if err != nil || len(channels) != 1 || channels[0].Group != "Sports" {
			t.Fatalf("channels=%v error=%v", channels, err)
		}
	}
	if streamCalls.Load() != 1 {
		t.Fatal("category results not reused")
	}
}

func TestSportsDiscoverySkipsExcludedSourceBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "must not request excluded source", 500)
	}))
	defer server.Close()
	h := newStremioTestHandler(t, server.URL)
	req := httptest.NewRequest("GET", "/sports/game/event/streams", nil)
	req = req.WithContext(context.WithValue(req.Context(), sportsDiscoveryKey{}, &sportsDiscoveryRequest{sources: []string{"different-source"}}))
	channels, err := h.FetchFilteredChannelsForRequest(req)
	if err != nil || len(channels) != 0 || calls.Load() != 0 {
		t.Fatalf("excluded source fetched: channels=%d requests=%d error=%v", len(channels), calls.Load(), err)
	}
}
