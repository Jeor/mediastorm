package history

import (
	"testing"

	"novastream/models"
)

func TestWatchHistoryUpdatePreservesUnrelatedEpisodeWithSameAbsoluteNumber(t *testing.T) {
	for _, action := range []string{"update", "toggle", "bulk", "import"} {
		t.Run(action, func(t *testing.T) {
			svc, err := NewService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			const userID = "default"
			const primalID = "tmdb:tv:89456:s03e08"
			original, err := svc.UpdatePlaybackProgress(userID, models.PlaybackProgressUpdate{
				MediaType: "episode", ItemID: primalID,
				PercentWatched: 58.67, IsPaused: true,
				SeriesID: "tmdb:tv:89456", SeriesName: "Primal",
				SeasonNumber: 3, EpisodeNumber: 8,
				ExternalIDs: map[string]string{"tmdb": "89456", "tvdb": "364007", "absoluteEpisode": "28"},
			})
			if err != nil {
				t.Fatal(err)
			}
			watched := true
			update := models.WatchHistoryUpdate{
				MediaType: "episode", ItemID: "tmdb:tv:78191:s03e08", Watched: &watched,
				SeriesID: "tmdb:tv:78191", SeriesName: "You",
				SeasonNumber: 3, EpisodeNumber: 8,
				ExternalIDs: map[string]string{"tmdb": "78191", "tvdb": "336924", "absoluteEpisode": "28"},
			}
			switch action {
			case "update":
				_, err = svc.UpdateWatchHistory(userID, update)
			case "toggle":
				_, err = svc.ToggleWatched(userID, update)
			case "bulk":
				_, err = svc.BulkUpdateWatchHistory(userID, []models.WatchHistoryUpdate{update})
			case "import":
				_, err = svc.ImportWatchHistory(userID, []models.WatchHistoryUpdate{update})
			}
			if err != nil {
				t.Fatal(err)
			}
			progress, err := svc.GetPlaybackProgress(userID, "episode", primalID)
			if err != nil {
				t.Fatal(err)
			}
			if progress == nil || progress.PercentWatched != original.PercentWatched || !progress.UpdatedAt.Equal(original.UpdatedAt) {
				t.Fatalf("Primal resume changed after marking You watched: %#v", progress)
			}
		})
	}
}
