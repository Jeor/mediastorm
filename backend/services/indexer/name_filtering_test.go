package indexer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"novastream/config"
	"novastream/services/debrid"
)

func TestSourceNameFilteringAcrossSearchPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"streams":[{"url":"https://stream.example/%s/986606.mp4","name":"Source 720p"}]}`, source)
	}))
	defer server.Close()
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.DefaultSettings()
	settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
	settings.Streaming.DebridProviders = nil
	settings.Indexers = nil
	settings.TorrentScrapers = []config.TorrentScraperConfig{
		{Name: "Trusted", Type: "stremio-direct", URL: server.URL + "/trusted/manifest.json", Enabled: true, SkipNameFiltering: true},
		{Name: "Strict", Type: "stremio-direct", URL: server.URL + "/strict/manifest.json", Enabled: true},
	}
	if err := mgr.Save(settings); err != nil {
		t.Fatal(err)
	}
	svc := NewService(mgr, nil, debrid.NewSearchService(mgr))
	opts := SearchOptions{Query: "The Matrix", IMDBID: "tt0133093", MediaType: "movie", Year: 1999}
	results, err := svc.Search(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Indexer != "Trusted" {
		t.Fatalf("normal results = %+v", results)
	}
	opts.IncludeFiltered = true
	scored, err := svc.SearchWithScoring(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(scored) != 2 {
		t.Fatalf("scored results = %+v", scored)
	}
	for _, r := range scored {
		if r.Indexer == "Trusted" && r.FilterStatus != "passed" {
			t.Fatalf("trusted tester result = %+v", r)
		}
		if r.Indexer == "Strict" && r.FilterStatus != "filtered" {
			t.Fatalf("strict tester result = %+v", r)
		}
	}
	opts.IncludeFiltered = false
	debridChan, usenetChan := svc.SearchSplit(t.Context(), opts)
	got := <-debridChan
	for range usenetChan {
	}
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if len(got.Results) != 1 || got.Results[0].Indexer != "Trusted" {
		t.Fatalf("split results = %+v", got.Results)
	}
}
