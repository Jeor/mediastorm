package history

import (
	"testing"
	"time"

	"novastream/models"
)

func TestImportWatchHistoryEnrichesSkippedIdentityWithoutChangingLocalState(t *testing.T) {
	for _, watched := range []bool{true, false} {
		for _, incomingID := range []string{"tmdb:movie:42", "tt0000042"} {
			t.Run(incomingID+"/watched="+map[bool]string{true: "true", false: "false"}[watched], func(t *testing.T) {
				dir := t.TempDir()
				svc, err := NewService(dir)
				if err != nil {
					t.Fatal(err)
				}
				newer := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
				local := models.WatchHistoryUpdate{
					MediaType: "movie", ItemID: "tmdb:movie:42", Name: "Local title", Watched: &watched, WatchedAt: newer,
					ExternalIDs: map[string]string{"tmdb": "42", "imdb": "tt0000042"},
				}
				if _, err := svc.ImportWatchHistory("user", []models.WatchHistoryUpdate{local}); err != nil {
					t.Fatal(err)
				}
				before, _ := svc.GetWatchHistoryRevision("user")
				remoteWatched := true
				incoming := models.WatchHistoryUpdate{
					MediaType: "movie", ItemID: incomingID, Name: "Older remote title", Watched: &remoteWatched, WatchedAt: newer.Add(-time.Hour),
					ExternalIDs: map[string]string{"imdb": "tt0000042", "tvdb": "99"},
				}
				count, err := svc.ImportWatchHistory("user", []models.WatchHistoryUpdate{incoming})
				if err != nil || count != 0 {
					t.Fatalf("count=%d err=%v", count, err)
				}
				after, _ := svc.GetWatchHistoryRevision("user")
				if before == after {
					t.Fatal("identity enrichment did not invalidate watched-status revision")
				}
				// Persistence must retain aliases even though the watch event was skipped.
				svc, err = NewService(dir)
				if err != nil {
					t.Fatal(err)
				}
				item, err := svc.GetWatchHistoryItem("user", "movie", "tvdb:movie:99")
				if err != nil || item == nil {
					t.Fatalf("TVDB badge cannot find imported movie: item=%+v err=%v", item, err)
				}
				if item.Watched != watched || !item.UpdatedAt.Equal(newer) || item.Name != "Local title" || item.ItemID != "tmdb:movie:42" {
					t.Fatalf("older import changed local state: %+v", item)
				}
			})
		}
	}
}
