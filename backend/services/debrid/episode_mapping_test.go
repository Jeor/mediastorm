package debrid

import (
	"encoding/json"
	"fmt"
	"net/http"
	"novastream/config"
	"novastream/internal/mappingtest"
	"novastream/internal/mediaresolve"
	"novastream/models"
	"novastream/utils/filter"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAnimeSearchKeepsBothCoordinatesOnSameIMDb(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	req := SearchRequest{TitleID: "tmdb:tv:207468", IMDBID: "tt21975436", Query: "Kaiju No 8 S01E13", Parsed: ParsedQuery{MediaType: MediaTypeSeries, Title: "Kaiju No 8", Season: 1, Episode: 13}}
	for _, imdb := range []bool{true, false} {
		requests := mappedSearchRequests(req, imdb)
		if len(requests) != 2 || requests[0].Parsed.Episode != 13 || requests[1].Parsed.Season != 2 || requests[1].Parsed.Episode != 1 || requests[1].IMDBID != req.IMDBID {
			t.Fatalf("imdb=%v requests=%+v", imdb, requests)
		}
	}
}
func TestMappedAnimeTorrentPackSelection(t *testing.T) {
	mappingtest.Install(t)
	for _, tc := range []struct {
		id, name     string
		s, e, ms, me int
	}{{"207468", "Kaiju No 8", 1, 13, 2, 1}, {"30984", "Bleach", 2, 14, 17, 14}, {"209867", "Frieren", 1, 29, 2, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			id := "tmdb:tv:" + tc.id
			mappingtest.Warm(t.Context(), id, tc.s, tc.e)
			results := filter.Results([]models.NZBResult{{Title: fmt.Sprintf("%s S%02d COMPLETE 1080p WEB", tc.name, tc.ms)}}, filter.Options{TitleID: id, ExpectedTitle: tc.name, TargetSeason: tc.s, TargetEpisode: tc.e, IsAnime: true})
			if len(results) != 1 {
				t.Fatal("pack rejected")
			}
			h := buildSelectionHints(results[0], "")
			files := []mediaresolve.Candidate{{Label: fmt.Sprintf("%s S%02dE%02d.mkv", tc.name, tc.s, tc.e)}, {Label: fmt.Sprintf("%s S%02dE%02d.mkv", tc.name, tc.ms, tc.me+1)}, {Label: fmt.Sprintf("%s S%02dE%02d.mkv", tc.name, tc.ms, tc.me)}}
			if i, r := mediaresolve.SelectBestCandidate(files, h); i != 2 {
				t.Fatalf("selected %d %s", i, r)
			}
			if i, r := mediaresolve.SelectBestCandidate(files[:2], h); i != -1 {
				t.Fatalf("wrong episode fallback %d %s", i, r)
			}
		})
	}
}

func TestAnimeMultiSeasonPackAndSingleFileAliases(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	results := filter.Results([]models.NZBResult{{Title: "Kaiju No 8 S01-S02 COMPLETE 1080p WEB"}}, filter.Options{TitleID: "tmdb:tv:207468", ExpectedTitle: "Kaiju No 8", TargetSeason: 1, TargetEpisode: 13, TargetAbsoluteEpisode: 13, IsAnime: true})
	if len(results) != 1 {
		t.Fatal("multi-season pack rejected")
	}
	h := buildSelectionHints(results[0], "")
	if len(h.AlternateEpisodes) != 1 {
		t.Fatalf("aliases lost: %+v", h)
	}
	files := []File{{ID: 1, Path: "Kaiju.No.8.S01E01.mkv"}, {ID: 2, Path: "Kaiju.No.8.S02E01.mkv"}, {ID: 3, Path: "Kaiju.No.8.S02E02.mkv"}}
	for _, subset := range [][]File{files, files[1:2]} {
		selected := selectMediaFiles(subset, h)
		if selected == nil || selected.PreferredID != "2" {
			t.Fatalf("wrong selection %+v", selected)
		}
	}
	wrong := selectMediaFiles([]File{files[0], files[2]}, h)
	if wrong != nil && wrong.PreferredID != "" {
		t.Fatalf("wrong episode accepted %+v", wrong)
	}
}

func TestAnimeMappingSearchEndToEnd(t *testing.T) {
	mappingtest.Install(t)
	var mu sync.Mutex
	paths := map[string]int{}
	client := newStubClient(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()
		var streams []any
		if strings.Contains(r.URL.Path, "tt21975436:2:1") {
			for _, code := range []string{"S02E01", "S02E02"} {
				streams = append(streams, map[string]any{"url": "https://cdn.test/" + code + ".mkv", "behaviorHints": map[string]any{"filename": "Kaiju.No.8." + code + ".1080p.WEB.mkv"}})
			}
		}
		body, _ := json.Marshal(map[string]any{"streams": streams})
		return jsonResponse(200, string(body)), nil
	})
	cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.DefaultSettings()
	settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
	settings.Streaming.DebridProviders = []config.DebridProviderSettings{{Name: "RealDebrid", Enabled: true, APIKey: "test"}}
	settings.Display.BypassFilteringForAIOStreamsOnly = false
	if err := cfg.Save(settings); err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(cfg, NewAIOStreamsScraper("https://aio.test/manifest.json", "AIO", false, client))
	results, err := service.Search(t.Context(), SearchOptions{Query: "Kaiju No 8 S01E13", MediaType: "series", IMDBID: "tt21975436", TitleID: "tmdb:tv:207468", IsAnime: true, AbsoluteEpisodeNumber: 13})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths["/stream/series/tt21975436:1:13.json"] != 1 || paths["/stream/series/tt21975436:2:1.json"] != 1 {
		t.Fatalf("requests=%v", paths)
	}
	if len(results) != 1 || results[0].Attributes["targetSeason"] != "2" || results[0].Attributes["targetEpisode"] != "1" {
		t.Fatalf("results=%+v", results)
	}
	selected := selectMediaFiles([]File{{ID: 1, Path: "Kaiju.No.8.S02E01.mkv"}}, buildSelectionHints(results[0], ""))
	if selected == nil || selected.PreferredID != "1" {
		t.Fatalf("selection=%+v", selected)
	}
}

func TestTVDBEpisodesUnderTMDBTitleUseReverseStreams(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	req := SearchRequest{TitleID: "tmdb:tv:207468", IMDBID: "tt21975436", Query: "Kaiju No 8 S02E01", Parsed: ParseQuery("Kaiju No 8 S02E01"), Numbering: &models.EpisodeNumbering{SeriesID: "tvdb:series:423075", Ordering: "official"}}
	got := mappedSearchRequests(req, true)
	if len(got) != 2 || got[1].Parsed.Season != 1 || got[1].Parsed.Episode != 13 || got[1].IMDBID != req.IMDBID {
		t.Fatalf("stream aliases=%+v", got)
	}
}
