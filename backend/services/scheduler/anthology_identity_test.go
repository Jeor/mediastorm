package scheduler

import (
	"encoding/json"
	"testing"

	"novastream/models"
	"novastream/services/simkl"
)

func TestSimklAnthologyImportAndExport(t *testing.T) {
	raw := json.RawMessage(`{"show":{"title":"Monster","year":2022,"ids":{"simkl":1446614,"tmdb":113988,"tvdb":389492,"imdb":"tt13207736"}},"seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-09-19T02:00:00Z"}]},{"number":4,"episodes":[{"number":1,"watched_at":"2026-09-20T02:38:13Z"},{"number":2,"watched_at":"2026-09-20T03:00:00Z"}]}]}`)
	watched := true
	updates := simklShowToUpdates(raw, &watched)
	if len(updates) != 3 {
		t.Fatalf("updates=%+v", updates)
	}
	if updates[0].SeriesID == "tmdb:tv:299939" || updates[0].SeasonNumber != 1 {
		t.Fatal("original Monster season changed")
	}
	for _, update := range updates[1:] {
		if update.SeriesID != "tmdb:tv:299939" || update.SeasonNumber != 1 || update.SeriesName != "Monster: The Lizzie Borden Story" || update.Year != 2026 || update.ExternalIDs["imdb"] != "" || update.ExternalIDs["tvdb"] != "" {
			t.Fatalf("wrong import: %+v", update)
		}
		item := models.WatchHistoryItem{MediaType: update.MediaType, ItemID: update.ItemID, SeriesID: update.SeriesID, ExternalIDs: update.ExternalIDs, SeasonNumber: update.SeasonNumber, EpisodeNumber: update.EpisodeNumber}
		season, episode := simklExportEpisodeCoordinates(item)
		ids, season, episode := simkl.EpisodeIdentity(extractSimklIDs(item.MediaType, item.ItemID, item.SeriesID, item.ExternalIDs), season, episode)
		if ids != (simkl.IDs{IMDB: "tt13207736"}) || season != 4 || episode != update.EpisodeNumber {
			t.Fatalf("wrong export: %+v %d %d", ids, season, episode)
		}
	}
}
