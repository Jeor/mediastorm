package indexer

import (
	"path/filepath"
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestValidatedEpisodeYearsPreserveQualityRanking(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opts   SearchOptions
		titles []string // expected quality order
	}{
		{"mapped anthology", SearchOptions{TitleID: "tmdb:tv:299939", Query: "Monster: The Lizzie Borden Story S01E05", Year: 2026}, []string{
			"Monster.2022.S04E05.41.2160p.NF.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX",
			"Monster.The.Lizzie.Borden.Story.S01E05.1080p.HEVC.x265-MeGusta",
			"Monster.The.Lizzie.Borden.Story.[2026].S01E05.720p.NF.Web-DL.[TR-EN].DDP5.1.Atmos.H.264-TURG",
		}},
		{"season premiere", SearchOptions{Query: "Example Show S03E08", Year: 2000, EpisodeAirYear: 2012, SeasonPremiereYear: 2008}, []string{
			"Example.Show.2008.S03E08.2160p.WEB",
			"Example.Show.S03E08.1080p.WEB",
			"Example.Show.2000.S03E08.720p.WEB",
		}},
	} {
		for _, mode := range []string{"manual", "prequeue"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
				settings := config.DefaultSettings()
				settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
				settings.Display.BypassFilteringForAIOStreamsOnly = false
				settings.Filtering.HDRDVPolicy = config.HDRDVPolicy(models.HDRDVPolicyIncludeHDRDV)
				settings.Ranking.Criteria = []config.RankingCriterion{{ID: config.RankingResolution, Enabled: true}}
				if err := cfg.Save(settings); err != nil {
					t.Fatal(err)
				}
				var raw []models.NZBResult
				for i := len(tc.titles) - 1; i >= 0; i-- {
					raw = append(raw, models.NZBResult{Title: tc.titles[i], ServiceType: models.ServiceTypeDebrid})
				}
				svc := NewService(cfg, nil, stubDebridSearchService{results: raw})
				opts := tc.opts
				opts.MediaType = "series"
				var results []models.ScoredNZBResult
				if mode == "manual" {
					var err error
					results, err = svc.SearchWithScoring(t.Context(), opts)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					u, d := svc.SearchWithScoringSplit(t.Context(), opts)
					debridResult := <-d
					for range u {
					}
					if debridResult.Err != nil {
						t.Fatal(debridResult.Err)
					}
					results = debridResult.Scored
				}
				if len(results) != len(tc.titles) {
					t.Fatalf("got %d results, want %d", len(results), len(tc.titles))
				}
				for i, r := range results {
					if r.Title != tc.titles[i] || r.Attributes["episodeYearPriority"] == "true" {
						t.Errorf("rank %d: %s, priority=%s; want %s with normal quality ranking", i+1, r.Title, r.Attributes["episodeYearPriority"], tc.titles[i])
					}
				}
			})
		}
	}
}

func TestValidatedYearDoesNotHideAnotherRebootConflict(t *testing.T) {
	results := []models.NZBResult{
		{Title: "Series.1974.S01E01.720p", Attributes: map[string]string{"episodeReleaseYear": "1974", "episodeYearMatch": "true"}},
		{Title: "Series.S01E01.2160p"},
		{Title: "Series.2026.S01E01.1080p", Attributes: map[string]string{"episodeReleaseYear": "2026"}},
		{Title: "Verified.Alias.2000.S04E01.2160p", Attributes: map[string]string{"episodeReleaseYear": "2000", "episodeMappedYearMatch": "true"}},
	}
	applyEpisodeYearPriority(results, 1974, 1974)
	(&Service{}).sortResultsByScore(results, ScoringContext{RankingCriteria: []config.RankingCriterion{{ID: config.RankingResolution, Enabled: true}}})
	if results[0].Title != "Series.1974.S01E01.720p" {
		t.Fatalf("reboot safeguard lost: %s", results[0].Title)
	}
}

func TestSeasonYearAffectsSearchCacheAndFiltering(t *testing.T) {
	cfg := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.DefaultSettings()
	settings.Streaming.ServiceMode = config.StreamingServiceModeDebrid
	if err := cfg.Save(settings); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, nil, stubDebridSearchService{results: []models.NZBResult{{Title: "Example.Show.2008.S03E08.1080p.WEB", ServiceType: models.ServiceTypeDebrid}}})
	opts := SearchOptions{Query: "Example Show S03E08", MediaType: "series", Year: 2000, EpisodeAirYear: 2012, IncludeFiltered: true}
	for _, year := range []int{0, 2008, 0} {
		opts.SeasonPremiereYear = year
		results, err := svc.SearchWithScoring(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || (results[0].FilterStatus == "passed") != (year == 2008) {
			t.Fatalf("season year %d: results=%+v", year, results)
		}
	}
}
