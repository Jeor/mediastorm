package mediaidentity

import "testing"

func TestCrossMappingDataSupportsOtherSeriesAndEpisodeOverrides(t *testing.T) {
	mappings := []SeriesCrossMapping{{TitleID: "tmdb:tv:123", CatalogSeason: 2, FirstEpisode: 1, EpisodeCount: 3,
		Provider:         AnthologyEpisode{IMDBID: "tt1234", ReleaseTitle: "Another Anthology", Season: 5, Episode: 7, SeasonEpisodeCount: 12},
		EpisodeOverrides: map[int]EpisodeCoordinate{3: {Season: 6, Episode: 1}}}}
	for _, tc := range []struct{ episode, season, providerEpisode int }{{1, 5, 7}, {2, 5, 8}, {3, 6, 1}} {
		got, ok := lookupCrossMapping(mappings, "tmdb:tv:123", 2, tc.episode)
		if !ok || got.Season != tc.season || got.Episode != tc.providerEpisode || got.IMDBID != "tt1234" {
			t.Fatalf("mapping: %+v %v", got, ok)
		}
	}
	if _, ok := lookupCrossMapping(mappings, "tmdb:tv:999", 2, 1); ok {
		t.Fatal("unrelated series mapped")
	}
}
