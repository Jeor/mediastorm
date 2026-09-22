package debrid

import (
	"encoding/json"
	"net/http"
	"novastream/config"
	"novastream/models"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMappedSearchRetainsBothIMDbIdentitiesAndDolbyVision(t *testing.T) {
	for _, mapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "unmapped", true: "mapped"}[mapped], func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			client := newStubClient(func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				catalog := map[string]any{"url": "https://cdn.test/catalog.mkv", "behaviorHints": map[string]any{"filename": "Monster.The.Lizzie.Borden.Story.S01E01.1080p.WEB.mkv"}}
				streams := []any{catalog}
				if strings.Contains(r.URL.Path, "tt13207736:4:1") {
					streams = append(streams,
						map[string]any{"url": "https://cdn.test/dv.mkv", "behaviorHints": map[string]any{"filename": "Monster.2022.S04E01.2160p.DV.H265.WEB.mkv"}},
						map[string]any{"url": "https://cdn.test/wrong.mkv", "behaviorHints": map[string]any{"filename": "Monster.2022.S03E01.2160p.DV.H265.WEB.mkv"}})
				}
				body, _ := json.Marshal(map[string]any{"streams": streams})
				return jsonResponse(200, string(body)), nil
			})
			cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			settings := config.DefaultSettings()
			settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
			settings.Streaming.DebridProviders = []config.DebridProviderSettings{{Name: "RealDebrid", Enabled: true, APIKey: "test"}}
			settings.Display.BypassFilteringForAIOStreamsOnly = false
			settings.Filtering.HDRDVPolicy = config.HDRDVPolicy(models.HDRDVPolicyIncludeHDRDV)
			if err := cfg.Save(settings); err != nil {
				t.Fatal(err)
			}
			svc := NewSearchService(cfg, NewAIOStreamsScraper("https://aio.test/manifest.json", "AIO", false, client))
			opts := SearchOptions{Query: "Monster: The Lizzie Borden Story S01E01", MediaType: "series", Year: 2026, IMDBID: "tt9990001", TitleID: "tmdb:tv:999"}
			if mapped {
				opts.TitleID = "tmdb:tv:299939"
			}
			results, err := svc.Search(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if mapped {
				want = 2
			}
			counts := map[string]int{}
			for _, path := range paths {
				counts[path]++
			}
			if counts["/stream/series/tt9990001:1:1.json"] != 1 || (mapped && counts["/stream/series/tt13207736:4:1.json"] != 1) {
				t.Fatalf("wrong IMDb requests: %v", paths)
			}
			if len(paths) != want || len(results) != want {
				t.Fatalf("paths=%v results=%+v", paths, results)
			}
			if mapped {
				found := false
				for _, r := range results {
					if strings.Contains(r.Title, "2160p") {
						found = true
						if r.Attributes["hasDV"] != "true" || r.Attributes["targetSeason"] != "4" {
							t.Fatalf("DV/mapping lost: %+v", r)
						}
					}
				}
				if !found {
					t.Fatal("mapped DV result missing")
				}
			}
		})
	}
}

func TestMappedIMDbAlreadyKnownDoesNotSearchWrongSeason(t *testing.T) {
	req := SearchRequest{TitleID: "tmdb:tv:299939", IMDBID: "tt13207736", Parsed: ParsedQuery{MediaType: MediaTypeSeries, Season: 1, Episode: 1}}
	requests := mappedSearchRequests(req, true)
	if len(requests) != 1 || requests[0].Parsed.Season != 4 {
		t.Fatalf("requests=%+v", requests)
	}
}
