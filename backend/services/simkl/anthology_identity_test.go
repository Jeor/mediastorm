package simkl

import (
	"testing"

	"novastream/models"
)

func TestAnthologyScrobbleRoundTrip(t *testing.T) {
	for episode := 1; episode <= 8; episode++ {
		req := BuildScrobbleRequest(models.PlaybackProgressUpdate{
			MediaType: "episode", SeriesID: "tmdb:tv:299939", SeriesName: "Monster: The Lizzie Borden Story",
			SeasonNumber: 1, EpisodeNumber: episode, ExternalIDs: map[string]string{"tmdb": "299939", "episodeTmdb": "6506516"},
		}, 95)
		if req.Show.IDs != (IDs{IMDB: "tt13207736"}) || req.Show.Title != "" || req.Episode.Season != 4 || req.Episode.Number != episode {
			t.Fatalf("incorrect provider request: %+v %+v", req.Show, req.Episode)
		}
		id, ids, season, ok := CatalogEpisodeIdentity(map[string]string{"simkl": "1446614", "tmdb": "113988", "imdb": "tt13207736"}, req.Episode.Season, episode)
		if !ok || id != "tmdb:tv:299939" || ids["tmdb"] != "299939" || len(ids) != 2 || season != 1 {
			t.Fatalf("incorrect local identity: %s %v %d %v", id, ids, season, ok)
		}
	}
}

func TestAnthologyMappingLeavesOtherEpisodesAlone(t *testing.T) {
	for _, tc := range []struct {
		ids             IDs
		season, episode int
	}{
		{IDs{TMDB: 113988}, 1, 1}, {IDs{TMDB: 299939}, 2, 1}, {IDs{TMDB: 299939}, 1, 9}, {IDs{TMDB: 299939}, 1, 0},
	} {
		ids, season, episode := EpisodeIdentity(tc.ids, tc.season, tc.episode)
		if ids != tc.ids || season != tc.season || episode != tc.episode {
			t.Fatal("unrelated episode changed")
		}
	}
	for _, tc := range []struct {
		ids             map[string]string
		season, episode int
	}{
		{map[string]string{"imdb": "tt13207736"}, 1, 1}, {map[string]string{"imdb": "tt13207736"}, 3, 1},
		{map[string]string{"imdb": "tt13207736"}, 4, 9}, {map[string]string{"imdb": "tt0000000"}, 4, 1},
	} {
		if _, _, _, ok := CatalogEpisodeIdentity(tc.ids, tc.season, tc.episode); ok {
			t.Fatal("unrelated import changed")
		}
	}
}
