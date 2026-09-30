package scheduler

import (
	"net/http"
	"novastream/config"
	"novastream/models"
	"novastream/services/plex"
	"testing"
	"time"
)

func TestPlexHistoryUpdateUsesCatalogIdentity(t *testing.T) {
	for _, tc := range []struct {
		name               string
		item               plex.WatchHistoryItem
		wantID, wantSeries string
		wantOK             bool
	}{
		{name: "movie", item: plex.WatchHistoryItem{Type: "movie", RatingKey: "999", ExternalIDs: map[string]string{"tmdb": "42"}}, wantID: "tmdb:movie:42", wantOK: true},
		{name: "episode", item: plex.WatchHistoryItem{Type: "episode", RatingKey: "999", ParentIndex: 1, Index: 3, ExternalIDs: map[string]string{"tmdb": "42"}}, wantID: "tmdb:tv:42:s01e03", wantSeries: "tmdb:tv:42", wantOK: true},
		{name: "special", item: plex.WatchHistoryItem{Type: "episode", RatingKey: "999", ParentIndex: 0, Index: 3, ExternalIDs: map[string]string{"tvdb": "42"}}, wantID: "tvdb:series:42:s00e03", wantSeries: "tvdb:series:42", wantOK: true},
		{name: "local movie ID", item: plex.WatchHistoryItem{Type: "movie", RatingKey: "42"}},
		{name: "local episode ID", item: plex.WatchHistoryItem{Type: "episode", RatingKey: "42", ParentIndex: 1, Index: 3}},
		{name: "missing coordinates", item: plex.WatchHistoryItem{Type: "episode", RatingKey: "999", ExternalIDs: map[string]string{"tmdb": "42"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			update, ok := plexHistoryUpdate(tc.item)
			if ok != tc.wantOK || update.ItemID != tc.wantID || update.SeriesID != tc.wantSeries {
				t.Fatalf("update=%+v ok=%v", update, ok)
			}
		})
	}
}

func TestPlexHistoryReimportRepairsLegacySourceIdentity(t *testing.T) {
	h := auditHistoryService(t, 0, true)
	watched := true
	when := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	// Older imports stored a Plex rating key when only the Plex GUID was available.
	_, err := h.ImportWatchHistory("profile", []models.WatchHistoryUpdate{{
		MediaType: "movie", ItemID: "123", Name: "Movie", Watched: &watched, WatchedAt: when,
		ExternalIDs: map[string]string{"plex": "abcdef"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	update, ok := plexHistoryUpdate(plex.WatchHistoryItem{
		Type: "movie", RatingKey: "123", Title: "Movie", ViewedAt: when.Unix(),
		ExternalIDs: map[string]string{"tmdb": "42", "plex": "abcdef"},
	})
	if !ok {
		t.Fatal("movie identity unresolved")
	}
	if _, err := h.ImportWatchHistory("profile", []models.WatchHistoryUpdate{update}); err != nil {
		t.Fatal(err)
	}
	items, _ := h.ListWatchHistory("profile")
	if len(items) != 1 || items[0].ItemID != "tmdb:movie:42" || !items[0].Watched {
		t.Fatalf("legacy Plex row was not repaired: %+v", items)
	}

	// An older MDBList watch must still attach the IMDb alias used by a catalog.
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"movies":[{"movie":{"title":"Movie","ids":{"tmdb":42,"imdb":"tt0000042"}},"last_watched_at":"2026-09-29T08:00:00Z"}],"pagination":{"has_more":false}}`), nil
	})
	result, err := (&Service{historyService: h}).syncMDBListHistoryToLocal(config.ScheduledTask{}, &config.MDBListAccount{APIKey: "test"}, "profile", false)
	if err != nil || result.Count != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	item, err := h.GetWatchHistoryItem("profile", "movie", "tt0000042")
	if err != nil || item == nil || !item.Watched || !item.WatchedAt.Equal(when) {
		t.Fatalf("MDBList did not repair watched identity: %+v err=%v", item, err)
	}
}
