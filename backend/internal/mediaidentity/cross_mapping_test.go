package mediaidentity

import (
	"testing"

	"novastream/models"
)

func TestDevilInSilverCrossMappingScope(t *testing.T) {
	for episode := 1; episode <= 6; episode++ {
		got, ok := KnownAnthologyEpisode("tmdb:tv:323903", 1, episode)
		if !ok || got.IMDBID != "tt2708480" || got.TVDBID != 322191 || got.ReleaseTitle != "The Terror" || got.Season != 3 || got.Episode != episode || got.SeasonEpisodeCount != 6 {
			t.Fatalf("episode %d: %+v, mapped=%v", episode, got, ok)
		}
	}
	for _, tc := range []struct {
		id   string
		s, e int
	}{{"tmdb:tv:323903", 1, 7}, {"tmdb:tv:323903", 0, 1}, {"tmdb:tv:323903", 2, 1}, {"tmdb:tv:75191", 1, 1}} {
		if _, ok := KnownAnthologyEpisode(tc.id, tc.s, tc.e); ok {
			t.Fatalf("unexpected mapping for %+v", tc)
		}
	}
	if got := ReleaseEpisodeAliases("tmdb:tv:323903", 1, 1, &models.EpisodeNumbering{SeriesID: "tvdb:series:322191", Ordering: "official"}); len(got) != 0 {
		t.Fatalf("TVDB season 1 must keep its own numbering: %+v", got)
	}
}

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
