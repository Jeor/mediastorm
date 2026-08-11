package scheduler

import (
	"context"
	"testing"
	"time"

	"novastream/models"
	"novastream/services/scrob"
)

func TestScrobEventToUpdateEpisode(t *testing.T) {
	watched := true
	when := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	update := scrobEventToUpdate(scrob.HistoryEvent{Completed: true, WatchedAt: &when, Media: scrob.Media{
		TMDBID: 7124432, Type: "episode", Title: "Episode", SeasonNumber: 23, EpisodeNumber: 8,
		ShowTitle: "One Piece", ShowTMDBID: 37854, ShowTVDBID: 81797,
	}}, &watched)
	if update == nil {
		t.Fatal("expected update")
	}
	if update.ItemID != "tmdb:tv:37854:s23e08" || update.SeriesID != "tmdb:tv:37854" {
		t.Fatalf("update=%+v", update)
	}
	if update.ExternalIDs["tmdb"] != "37854" || update.ExternalIDs["episodeTmdb"] != "7124432" || update.ExternalIDs["tvdb"] != "81797" {
		t.Fatalf("ids=%v", update.ExternalIDs)
	}
}

func TestLocalItemToScrobEpisodeUsesShowAndEpisodeIDs(t *testing.T) {
	when := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	event, key, ok := localItemToScrob(models.WatchHistoryItem{
		MediaType: "episode", ItemID: "tmdb:tv:37854:s23e08", SeriesID: "tmdb:tv:37854", SeasonNumber: 23, EpisodeNumber: 8,
		Watched: true, WatchedAt: when, ExternalIDs: map[string]string{"episodeTmdb": "7124432", "tvdb": "81797"},
	})
	if !ok {
		t.Fatal("expected item to be exportable")
	}
	if key != "episode:37854:23:8" || event.SeriesTMDBID != 37854 || event.TMDBID != 7124432 || event.SeriesTVDBID != 81797 {
		t.Fatalf("key=%s event=%+v", key, event)
	}
	if event.WatchedAt == nil || !event.WatchedAt.Equal(when) {
		t.Fatalf("watchedAt=%v", event.WatchedAt)
	}
}

func TestScrobEventToUpdateSkipsIncomplete(t *testing.T) {
	watched := true
	if got := scrobEventToUpdate(scrob.HistoryEvent{Completed: false, Media: scrob.Media{Type: "movie", TMDBID: 550}}, &watched); got != nil {
		t.Fatalf("got=%+v", got)
	}
}

func TestEnrichScrobExportEpisodeCanonicalizesProviderNumbering(t *testing.T) {
	details := &models.SeriesDetails{
		Title: models.Title{TMDBID: 37854, TVDBID: 81797, IMDBID: "tt0388629"},
		Seasons: []models.SeriesSeason{{Number: 23, Episodes: []models.SeriesEpisode{{
			ID: "tvdb:episode:11898626", TMDBID: 7550159, TMDBSeasonNumber: 23, TMDBEpisodeNumber: 1172, TVDBID: 11898626, Name: "Elbaph",
			SeasonNumber: 23, EpisodeNumber: 17, AbsoluteEpisodeNumber: 1172,
		}}}},
	}
	metadataSvc := &fakeSchedulerMetadataService{details: details}
	tests := []struct {
		name     string
		season   int
		episode  int
		external map[string]string
	}{
		{name: "season order", season: 23, episode: 17},
		{name: "absolute order", season: 1, episode: 1172},
		{name: "hybrid order", season: 23, episode: 1172},
		{name: "explicit absolute", season: 1, episode: 1, external: map[string]string{"absoluteEpisode": "1172"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			external := map[string]string{"tmdb": "37854", "tvdb": "81797"}
			for key, value := range tc.external {
				external[key] = value
			}
			item := enrichScrobExportEpisode(context.Background(), metadataSvc, make(map[string]*models.SeriesDetails), models.WatchHistoryItem{
				MediaType: "episode", SeriesID: "tmdb:tv:37854", SeasonNumber: tc.season, EpisodeNumber: tc.episode,
				ExternalIDs: external,
			})
			if item.SeasonNumber != 23 || item.EpisodeNumber != 1172 {
				t.Fatalf("coordinates=%d:%d", item.SeasonNumber, item.EpisodeNumber)
			}
			if item.ExternalIDs["episodeTmdb"] != "7550159" || item.ExternalIDs["episodeTvdb"] != "11898626" || item.ExternalIDs["absoluteEpisode"] != "1172" {
				t.Fatalf("ids=%v", item.ExternalIDs)
			}
		})
	}
}

func TestCanonicalizeProviderEpisodeBackfillsResolvedIDs(t *testing.T) {
	svc := &Service{metadataService: &fakeSchedulerMetadataService{details: &models.SeriesDetails{
		Seasons: []models.SeriesSeason{{Episodes: []models.SeriesEpisode{{
			TMDBID: 7124432, TVDBID: 11898626, SeasonNumber: 23, EpisodeNumber: 17, AbsoluteEpisodeNumber: 1172,
		}}}},
	}}}
	episodeIDs := map[string]string{}
	season, episode, absolute, _ := svc.canonicalizeProviderEpisode(
		"test", map[string]string{"tmdb": "37854"}, episodeIDs, 23, 17, 0, "Elbaph",
	)
	if season != 23 || episode != 17 || absolute != 1172 {
		t.Fatalf("coordinates=%d:%d absolute=%d", season, episode, absolute)
	}
	if episodeIDs["tmdb"] != "7124432" || episodeIDs["tvdb"] != "11898626" {
		t.Fatalf("episode IDs=%v", episodeIDs)
	}
}

func TestScrobAliasCleanupMediaRequiresCanonicalizedMatchingTitle(t *testing.T) {
	remote := map[string][]scrob.Media{
		"episode:37854:23:18": {
			{ID: 9433, Type: "episode", Title: "A Nightmarish Game - The Dark Plot of the Knights of God", ShowTMDBID: 37854, SeasonNumber: 23, EpisodeNumber: 18},
			{ID: 9999, Type: "episode", Title: "A Different Episode", ShowTMDBID: 37854, SeasonNumber: 23, EpisodeNumber: 18},
		},
	}
	aliases := map[string]struct{}{"episode:37854:23:18": {}}
	canonicalCandidates := map[string]int{"episode:37854:23:1173": 0}
	got := scrobAliasCleanupMedia(
		"A Nightmarish Game - The Dark Plot of the Knights of God",
		"episode:37854:23:1173",
		aliases,
		canonicalCandidates,
		remote,
	)
	if len(got) != 1 || got[0].ID != 9433 {
		t.Fatalf("cleanups=%+v", got)
	}
}

func TestScrobAliasCleanupMediaPreservesAnotherCanonicalCandidate(t *testing.T) {
	aliasKey := "episode:37854:23:18"
	got := scrobAliasCleanupMedia(
		"Episode",
		"episode:37854:23:1173",
		map[string]struct{}{aliasKey: {}},
		map[string]int{aliasKey: 1, "episode:37854:23:1173": 0},
		map[string][]scrob.Media{aliasKey: {{ID: 9433, Type: "episode", Title: "Episode"}}},
	)
	if len(got) != 0 {
		t.Fatalf("cleanups=%+v", got)
	}
}

func TestScrobExportPartialErrorOmitsUpstreamResponse(t *testing.T) {
	err := scrobExportPartialError(1269, 1276, 6, "episode", "Living the Life of 1%")
	want := `Scrob export synced 1269 of 1276 changes; 6 failed. First failure: episode "Living the Life of 1%". See backend logs for details`
	if err.Error() != want {
		t.Fatalf("error=%q", err)
	}
}
