package history

import (
	"fmt"
	"testing"
	"time"

	"novastream/models"
)

func TestContinueWatchingImportedSpecials(t *testing.T) {
	for _, tc := range []struct {
		name           string
		regularWatched int
		resumeSpecial  bool
		wantSeason     int
		wantEpisode    int
	}{
		{name: "only imported specials"},
		{name: "all regular episodes watched", regularWatched: 2},
		{name: "continue regular series", regularWatched: 1, wantSeason: 1, wantEpisode: 2},
		{name: "explicitly started special remains resumable", regularWatched: 2, resumeSpecial: true, wantEpisode: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			svc.SetMetadataService(&mockMetadataService{seriesDetails: &models.SeriesDetails{
				Title: models.Title{ID: "tmdb:tv:123", Name: "Example", TMDBID: 123},
				Seasons: []models.SeriesSeason{
					{Number: 0, Episodes: []models.SeriesEpisode{
						{SeasonNumber: 0, EpisodeNumber: 1, AiredDate: "2020-01-01"},
						{SeasonNumber: 0, EpisodeNumber: 2, AiredDate: "2020-01-02"},
					}},
					{Number: 1, Episodes: []models.SeriesEpisode{
						{SeasonNumber: 1, EpisodeNumber: 1, AbsoluteEpisodeNumber: 1, AiredDate: "2020-01-03"},
						{SeasonNumber: 1, EpisodeNumber: 2, AbsoluteEpisodeNumber: 2, AiredDate: "2020-01-04"},
					}},
				},
			}})
			watched := true
			updates := []models.WatchHistoryUpdate{{
				MediaType: "episode", ItemID: "tmdb:tv:123:s00e01", SeriesID: "tmdb:tv:123",
				SeriesName: "Example", EpisodeNumber: 1, Watched: &watched, WatchedAt: time.Now().Add(-time.Hour),
				ExternalIDs: map[string]string{"tmdb": "123"},
			}}
			for episode := 1; episode <= tc.regularWatched; episode++ {
				updates = append(updates, models.WatchHistoryUpdate{
					MediaType: "episode", ItemID: fmt.Sprintf("tmdb:tv:123:s01e%02d", episode), SeriesID: "tmdb:tv:123",
					SeriesName: "Example", SeasonNumber: 1, EpisodeNumber: episode, Watched: &watched,
					WatchedAt: time.Now().Add(-time.Duration(4-episode) * time.Hour), ExternalIDs: map[string]string{"tmdb": "123"},
				})
			}
			if _, err := svc.ImportWatchHistory("user", updates); err != nil {
				t.Fatal(err)
			}
			if tc.resumeSpecial {
				_, err := svc.UpdatePlaybackProgress("user", models.PlaybackProgressUpdate{
					MediaType: "episode", ItemID: "tmdb:tv:123:s00e02", SeriesID: "tmdb:tv:123", SeriesName: "Example",
					EpisodeNumber: 2, Position: 300, Duration: 1200, ExternalIDs: map[string]string{"tmdb": "123"},
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			states, err := svc.ListContinueWatching("user")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantEpisode == 0 {
				if len(states) != 0 {
					t.Fatalf("specials resurfaced completed/unstarted series: %+v", states)
				}
			} else {
				if len(states) != 1 || states[0].NextEpisode == nil {
					t.Fatalf("missing next episode: %+v", states)
				}
				next := states[0].NextEpisode
				if next.SeasonNumber != tc.wantSeason || next.EpisodeNumber != tc.wantEpisode {
					t.Fatalf("next = S%02dE%02d, want S%02dE%02d", next.SeasonNumber, next.EpisodeNumber, tc.wantSeason, tc.wantEpisode)
				}
			}
			history, err := svc.ListWatchHistory("user")
			if err != nil || len(history) != len(updates) {
				t.Fatalf("imported history changed: %+v, %v", history, err)
			}
		})
	}
}

func TestSpecialDoesNotAliasRegularAbsoluteEpisode(t *testing.T) {
	details := &models.SeriesDetails{Seasons: []models.SeriesSeason{{Number: 1, Episodes: []models.SeriesEpisode{
		{SeasonNumber: 1, EpisodeNumber: 1, AbsoluteEpisodeNumber: 1},
	}}}}
	season, episode := newEpisodeNumberingIndex(details).canonical(0, 1)
	if season != 0 || episode != 1 {
		t.Fatalf("special aliased regular episode: S%02dE%02d", season, episode)
	}
}
