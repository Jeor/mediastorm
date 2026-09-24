package handlers

import (
	"novastream/internal/mappingtest"
	"novastream/models"
	"novastream/utils/filter"
	"testing"
)

func TestPrequeueAnnotationPreservesVerifiedAnimeMapping(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	results := filter.Results([]models.NZBResult{{Title: "Kaiju No 8 S02 COMPLETE 1080p WEB"}}, filter.Options{TitleID: "tmdb:tv:207468", ExpectedTitle: "Kaiju No 8", TargetSeason: 1, TargetEpisode: 13, TargetAbsoluteEpisode: 13, IsAnime: true})
	if len(results) != 1 {
		t.Fatal("pack rejected")
	}
	r := results[0]
	annotateResultEpisode(&r, &models.EpisodeReference{SeasonNumber: 1, EpisodeNumber: 13, AbsoluteEpisodeNumber: 13})
	if r.Attributes["targetSeason"] != "2" || r.Attributes["targetEpisode"] != "1" || r.Attributes["absoluteEpisodeNumber"] != "" {
		t.Fatalf("annotation overwrote verified mapping: %+v", r.Attributes)
	}
	annotateResultEpisode(&r, &models.EpisodeReference{SeasonNumber: 1, EpisodeNumber: 14, AbsoluteEpisodeNumber: 14})
	if r.Attributes["mappedCatalogEpisode"] != "" || r.Attributes["targetEpisode"] != "14" {
		t.Fatal("stale alias reused for different episode")
	}
	if results[0].Attributes["targetEpisode"] != "1" {
		t.Fatal("annotation mutated shared result")
	}
}

func TestPrequeueUsesEpisodeProviderIndependentlyOfTitle(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	tvdb := &models.EpisodeNumbering{SeriesID: "tvdb:series:423075", Ordering: "official"}
	tmdb := &models.EpisodeNumbering{SeriesID: "tmdb:tv:207468", Ordering: "official"}
	for _, tc := range []struct {
		name              string
		request, metadata *models.EpisodeNumbering
		s, e              int
		hydrate           bool
	}{
		{"both keys TVDB episodes TMDB title", tvdb, tvdb, 2, 1, true},
		{"TMDB fallback keeps selected TVDB episode", tvdb, tmdb, 2, 1, false},
		{"TVDB recovery keeps selected TMDB episode", tmdb, tvdb, 1, 13, false},
		{"TMDB only", tmdb, tmdb, 1, 13, true},
		{"legacy client uses actual provider", nil, tvdb, 2, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			details := &models.SeriesDetails{Title: models.Title{ID: "tmdb:tv:207468", Name: "Kaiju No 8", TVDBID: 423075}, Seasons: []models.SeriesSeason{{Number: tc.s, EpisodeCount: 24, Episodes: []models.SeriesEpisode{{SeasonNumber: tc.s, EpisodeNumber: tc.e, Name: "provider episode"}}}}}
			models.StampEpisodeNumbering(details, tc.metadata.SeriesID)
			h := &PrequeueHandler{metadataSvc: &mockSeriesDetailsProvider{details: details}}
			target := &models.EpisodeReference{Numbering: tc.request, SeasonNumber: tc.s, EpisodeNumber: tc.e}
			got := h.createEpisodeResolverAndLookupAbsoluteEp(t.Context(), "tmdb:tv:207468", "Kaiju No 8", 2024, "", target)
			want := tc.request
			if want == nil {
				want = tc.metadata
			}
			if !models.SameEpisodeNumbering(got.TargetEpisode.Numbering, want) || got.TargetEpisode.SeasonNumber != tc.s || got.TargetEpisode.EpisodeNumber != tc.e {
				t.Fatalf("identity changed: %+v", got.TargetEpisode)
			}
			if (got.EpisodeResolver != nil) != tc.hydrate {
				t.Fatalf("wrong-provider counts used: %+v", got)
			}
			if !tc.hydrate && got.TargetEpisode.Title != "" {
				t.Fatal("wrong-provider episode hydrated")
			}
		})
	}
	if prequeueEpisodeMatches(&models.EpisodeReference{SeasonNumber: 1, EpisodeNumber: 13, Numbering: tmdb}, &models.EpisodeReference{SeasonNumber: 1, EpisodeNumber: 13, Numbering: tvdb}) {
		t.Fatal("prequeue reused across numberings")
	}
}

func TestMappedSelectionHintsAreBoundToNumbering(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	tvdb := &models.EpisodeNumbering{SeriesID: "tvdb:series:423075"}
	tmdb := &models.EpisodeNumbering{SeriesID: "tmdb:tv:207468"}
	results := filter.Results([]models.NZBResult{{Title: "Kaiju No 8 S01E13 1080p WEB"}}, filter.Options{TitleID: "tmdb:tv:207468", Numbering: tvdb, ExpectedTitle: "Kaiju No 8", TargetSeason: 2, TargetEpisode: 1, IsAnime: true})
	if len(results) != 1 {
		t.Fatal("reverse-mapped release rejected")
	}
	r := results[0]
	annotateResultEpisode(&r, &models.EpisodeReference{SeasonNumber: 2, EpisodeNumber: 1, Numbering: tvdb})
	if r.Attributes["targetSeason"] != "1" || r.Attributes["targetEpisode"] != "13" {
		t.Fatalf("lost reverse selection hints: %v", r.Attributes)
	}
	annotateResultEpisode(&r, &models.EpisodeReference{SeasonNumber: 2, EpisodeNumber: 1, Numbering: tmdb})
	if r.Attributes["mappedCatalogEpisode"] != "" || r.Attributes["targetEpisode"] != "1" {
		t.Fatalf("reused hints for different numbering: %v", r.Attributes)
	}
}
