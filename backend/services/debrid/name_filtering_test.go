package debrid

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"novastream/config"
	"novastream/utils/filter"
)

func TestSourceNameFilteringToggle(t *testing.T) {
	for _, mediaType := range []string{"movie", "series", "daily"} {
		t.Run(mediaType, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"streams":[{"name":"same upstream label","title":"Correct metadata","url":"https://stream.example/%s/986606.mp4"}]}`, strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0])
			}))
			defer server.Close()
			mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			settings := config.DefaultSettings()
			settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
			settings.Streaming.DebridProviders = nil
			// Duplicate display names must not share their filtering preference.
			settings.TorrentScrapers = []config.TorrentScraperConfig{
				{Name: "Addon", Type: directStremioType, URL: server.URL + "/trusted/manifest.json", Enabled: true, SkipNameFiltering: true},
				{Name: "Addon", Type: directStremioType, URL: server.URL + "/strict/manifest.json", Enabled: true},
			}
			if err := mgr.Save(settings); err != nil {
				t.Fatal(err)
			}
			opts := SearchOptions{Query: "The Matrix", IMDBID: "tt0133093", MediaType: "movie", Year: 1999}
			if mediaType != "movie" {
				opts.Query = "Breaking Bad S01E02"
				opts.IMDBID = "tt0903747"
				opts.MediaType = "series"
				opts.Year = 2008
			}
			if mediaType == "daily" {
				opts.IsDaily = true
				opts.TargetAirDate = "2026-10-02"
			}
			svc := NewSearchService(mgr)
			results, err := svc.Search(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || !strings.Contains(results[0].Link, "/trusted/") || results[0].Attributes[filter.SkipNameFilteringAttribute] != "true" {
				t.Fatalf("results = %+v", results)
			}
			opts.SkipFilter = true
			raw, err := svc.Search(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if mediaType != "daily" && len(raw) != 2 {
				t.Fatalf("raw = %+v", raw)
			}
			for _, r := range raw {
				if strings.Contains(r.Link, "/strict/") && r.Attributes[filter.SkipNameFilteringAttribute] != "" {
					t.Fatal("bypass leaked to strict source")
				}
			}
			if mediaType == "daily" {
				mu.Lock()
				observedPaths := append([]string(nil), paths...)
				mu.Unlock()
				for _, path := range observedPaths {
					if strings.HasPrefix(path, "/trusted/") && path != "/trusted/stream/series/tt0903747:1:2.json" {
						t.Fatalf("trusted daily source probed adjacent episode: %s", path)
					}
				}
			}
			settings.TorrentScrapers[0].SkipNameFiltering = false
			if err := mgr.Save(settings); err != nil {
				t.Fatal(err)
			}
			opts.SkipFilter = false
			results, err = svc.Search(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 0 {
				t.Fatalf("turning toggle off kept numeric streams: %+v", results)
			}
		})
	}
}
