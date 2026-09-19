package debrid

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"novastream/config"
)

func TestAnthologyStreamIdentityScope(t *testing.T) {
	base := SearchRequest{TitleID: "tmdb:tv:299939", Query: "Monstre S01E01", Parsed: ParsedQuery{Title: "Monstre", MediaType: MediaTypeSeries, Season: 1, Episode: 1}}
	for _, tc := range []struct {
		name   string
		change func(*SearchRequest)
	}{
		{"other title", func(r *SearchRequest) { r.TitleID = "tmdb:tv:113988" }},
		{"no identity", func(r *SearchRequest) { r.TitleID = "" }},
		{"explicit IMDb", func(r *SearchRequest) { r.IMDBID = "tt1234567" }},
		{"movie", func(r *SearchRequest) { r.Parsed.MediaType = MediaTypeMovie }},
		{"special", func(r *SearchRequest) { r.Parsed.Season = 0 }},
		{"another season", func(r *SearchRequest) { r.Parsed.Season = 2 }},
		{"season pack", func(r *SearchRequest) { r.Parsed.Episode = 0 }},
		{"unverified episode", func(r *SearchRequest) { r.Parsed.Episode = 9 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.change(&req)
			got := req.forIMDBStreamProvider()
			if got.IMDBID != req.IMDBID || got.Parsed.Season != req.Parsed.Season || got.Parsed.Episode != req.Parsed.Episode {
				t.Fatalf("unrelated request changed: %+v", got)
			}
		})
	}
	for episode := 1; episode <= 8; episode++ {
		req := base
		req.Parsed.Episode = episode
		got := req.forIMDBStreamProvider()
		if got.IMDBID != "tt13207736" || got.Parsed.Season != 4 || got.Parsed.Episode != episode || got.Query != req.Query || req.Parsed.Season != 1 || req.IMDBID != "" {
			t.Fatalf("incorrect mapping or mutated original: %+v -> %+v", req, got)
		}
	}
}

type anthologyResolverSpy struct{ called bool }

func (s *anthologyResolverSpy) ResolveIMDBID(context.Context, string, string, int) string {
	s.called = true
	return "tt9999999"
}

type anthologyTextScraper struct{ request SearchRequest }

func (s *anthologyTextScraper) Name() string { return "text-search" }
func (s *anthologyTextScraper) Search(_ context.Context, req SearchRequest) ([]ScrapeResult, error) {
	s.request = req
	return nil, nil
}

func TestAnthologySearchUsesProviderCoordinates(t *testing.T) {
	for _, provider := range []string{"aiostreams", "torrentio"} {
		t.Run(provider, func(t *testing.T) {
			var paths []string
			client := newStubClient(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				return jsonResponse(http.StatusOK, `{"streams":[{"url":"https://example.test/video.mkv","infoHash":"0123456789012345678901234567890123456789","title":"Monster.The.Lizzie.Borden.Story.S04E02.1080p.WEB.mkv","behaviorHints":{"filename":"Monster.The.Lizzie.Borden.Story.S04E02.1080p.WEB.mkv"}}]}`), nil
			})
			var scraper Scraper = NewAIOStreamsScraper("https://example.test/manifest.json", "AIO", false, client)
			if provider == "torrentio" {
				scraper = NewTorrentioScraper(client, "", "Torrentio", "https://example.test")
			}
			cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			settings := config.DefaultSettings()
			settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
			settings.Streaming.DebridProviders = []config.DebridProviderSettings{{Name: "RealDebrid", Enabled: true, APIKey: "test"}}
			if err := cfg.Save(settings); err != nil {
				t.Fatal(err)
			}
			textScraper := &anthologyTextScraper{}
			svc := NewSearchService(cfg, scraper, textScraper)
			spy := &anthologyResolverSpy{}
			svc.SetIMDBResolver(spy)
			results, err := svc.Search(t.Context(), SearchOptions{TitleID: "tmdb:tv:299939", Query: "Monster: The Lizzie Borden Story S01E02", MediaType: "series", Year: 2026})
			if err != nil {
				t.Fatal(err)
			}
			if spy.called {
				t.Fatal("known mapping must not enter general show-ID resolver")
			}
			if textScraper.request.IMDBID != "" || textScraper.request.Parsed.Season != 1 || textScraper.request.Parsed.Episode != 2 {
				t.Fatalf("text search identity changed: %+v", textScraper.request)
			}
			if len(paths) != 1 || !strings.HasSuffix(paths[0], "/stream/series/tt13207736:4:2.json") {
				t.Fatalf("provider requests = %v", paths)
			}
			if len(results) != 1 {
				t.Fatalf("expected mapped S04 release to survive original-identity filtering, got %d", len(results))
			}
		})
	}
}
