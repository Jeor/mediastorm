package indexer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"novastream/config"
	"novastream/models"
	"novastream/services/debrid"
)

type identityRecordingDebrid struct{ ids []string }

func (s *identityRecordingDebrid) Search(_ context.Context, opts debrid.SearchOptions) ([]models.NZBResult, error) {
	s.ids = append(s.ids, opts.TitleID)
	return []models.NZBResult{{Title: "Monster.The.Lizzie.Borden.Story.S01E01.1080p.WEB.mkv", ServiceType: models.ServiceTypeDebrid}}, nil
}

func TestSearchPreservesTitleIdentityAndSeparatesCache(t *testing.T) {
	for _, scored := range []bool{false, true} {
		t.Run(map[bool]string{false: "ranked", true: "raw"}[scored], func(t *testing.T) {
			cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			settings := config.DefaultSettings()
			settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
			if err := cfg.Save(settings); err != nil {
				t.Fatal(err)
			}
			provider := &identityRecordingDebrid{}
			svc := NewService(cfg, nil, provider)
			opts := SearchOptions{Query: "Monster: The Lizzie Borden Story S01E01", MediaType: "series", IncludeFiltered: scored}
			for _, id := range []string{"", "tmdb:tv:299939", "tmdb:tv:299939"} {
				opts.TitleID = id
				var err error
				if scored {
					_, err = svc.SearchWithScoring(t.Context(), opts)
				} else {
					_, err = svc.Search(t.Context(), opts)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(provider.ids) != 2 || provider.ids[0] != "" || provider.ids[1] != "tmdb:tv:299939" {
				t.Fatalf("identity must reach provider and distinguish cached searches: %v", provider.ids)
			}
		})
	}
}

func TestAnthologySeasonFourResultsSurviveScoring(t *testing.T) {
	cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.DefaultSettings()
	settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
	settings.Display.BypassFilteringForAIOStreamsOnly = false
	if err := cfg.Save(settings); err != nil {
		t.Fatal(err)
	}
	provider := &countingDebridSearchService{results: []models.NZBResult{
		{Title: "Monster.The.Lizzie.Borden.Story.S04E01.1080p.WEB.mkv", ServiceType: models.ServiceTypeDebrid},
		{Title: "Monster.The.Lizzie.Borden.Story.S01E01.1080p.WEB.mkv", ServiceType: models.ServiceTypeDebrid},
		{Title: "Monster.The.Lizzie.Borden.Story.S04E02.1080p.WEB.mkv", ServiceType: models.ServiceTypeDebrid},
	}}
	svc := NewService(cfg, nil, provider)
	results, err := svc.SearchWithScoring(t.Context(), SearchOptions{TitleID: "tmdb:tv:299939", Query: "Monster: The Lizzie Borden Story S01E01", MediaType: "series", Year: 2026, IncludeFiltered: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d", len(results))
	}
	for _, r := range results {
		wantFiltered := strings.Contains(r.Title, "S04E02")
		if (r.FilterStatus == "filtered") != wantFiltered {
			t.Errorf("%s status=%s reason=%s", r.Title, r.FilterStatus, r.FilterReason)
		}
	}
}
