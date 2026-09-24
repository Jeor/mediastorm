package indexer

import (
	"novastream/config"
	"novastream/internal/mappingtest"
	"novastream/models"
	"novastream/services/debrid"
	"testing"
)

func TestAnimeIndexerQueriesCarryMappedTVDBCoordinates(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	opts := SearchOptions{TitleID: "tmdb:tv:207468", Query: "Kaiju No 8 S01E13", IMDBID: "tt21975436", Year: 2024, IsAnime: true}
	queries := buildSearchQueries(opts, debrid.ParseQuery(opts.Query), nil)
	seen := map[string]bool{}
	for _, q := range queries {
		seen[q] = true
	}
	for _, q := range []string{"Kaiju No 8 S01E13", "Kaiju No 8 S02E01", "Kaiju No 8 S02"} {
		if !seen[q] {
			t.Fatalf("missing %s: %v", q, queries)
		}
	}
	mapped := mappedQueryOptions(opts, "Kaiju No 8 S02E01")
	if mapped.TVDBID != 423075 || mapped.IMDBID != opts.IMDBID || mapped.Year != 2024 {
		t.Fatalf("mapped IDs/year lost: %+v", mapped)
	}
	if crossMappingSourceLimit(opts, 10) != 0 {
		t.Fatal("alias discarded before final ranking")
	}
}

func TestSearchCacheChangesWhenMappingBecomesAvailable(t *testing.T) {
	mappingtest.Install(t)
	opts := SearchOptions{TitleID: "tmdb:tv:207468", Query: "Kaiju No 8 S01E13", IsAnime: true}
	key := func() string {
		return (&Service{}).searchCacheKey("raw", opts, config.DefaultSettings(), nil, models.FilterSettings{}, effectiveFilterBundle{}, models.AnimeFilteringSettings{}, effectiveOverrides{}, nil, effectiveRankingBundle{})
	}
	before := key()
	mappingtest.Warm(t.Context(), opts.TitleID, 1, 13)
	after := key()
	if before == after {
		t.Fatal("cached search without S02E01 reused after mapping discovery")
	}
	if key() != after {
		t.Fatal("stable mapping produced unstable cache key")
	}
}

func TestMixedProviderQueriesAndCaches(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	opts := SearchOptions{TitleID: "tmdb:tv:207468", Query: "Kaiju No 8 S02E01", TVDBID: 423075, IMDBID: "tt21975436", Numbering: &models.EpisodeNumbering{SeriesID: "tvdb:series:423075", Ordering: "official"}}
	queries := buildSearchQueries(opts, debrid.ParseQuery(opts.Query), nil)
	found := false
	for _, q := range queries {
		if q == "Kaiju No 8 S01E13" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing reverse query: %v", queries)
	}
	mapped := mappedQueryOptions(opts, "Kaiju No 8 S01E13")
	if mapped.TVDBID != 0 || mapped.IMDBID != "" {
		t.Fatalf("TMDB coordinates paired with TVDB IDs: %+v", mapped)
	}
	original := mappedQueryOptions(opts, opts.Query)
	if original.TVDBID != 423075 {
		t.Fatal("TVDB identity lost")
	}
	before := buildSearchCacheOptions(opts)
	opts.Numbering = &models.EpisodeNumbering{SeriesID: "tmdb:tv:207468"}
	after := buildSearchCacheOptions(opts)
	if models.SameEpisodeNumbering(before.Numbering, after.Numbering) {
		t.Fatal("cache lacks numbering")
	}
}

func TestSceneCoordinatesDoNotUseTVDBStructuredIDs(t *testing.T) {
	mappingtest.InstallWithXEM(t, []byte(`{"result":"success","data":[{"tvdb":{"season":2,"episode":1},"scene":{"season":3,"episode":1,"absolute":13}}]}`))
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	opts := SearchOptions{TitleID: "tmdb:tv:207468", Query: "Kaiju No 8 S01E13", TVDBID: 423075, IMDBID: "tt21975436"}
	scene := mappedQueryOptions(opts, "Kaiju No 8 S03E01")
	if scene.TVDBID != 0 || scene.IMDBID != "" {
		t.Fatalf("scene coordinates paired with provider IDs: %+v", scene)
	}
	tvdb := mappedQueryOptions(opts, "Kaiju No 8 S02E01")
	if tvdb.TVDBID != 423075 {
		t.Fatal("valid TVDB mapping lost its ID")
	}
}
