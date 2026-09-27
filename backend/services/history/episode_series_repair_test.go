package history

import (
	"context"
	"fmt"
	"testing"
	"time"

	"novastream/models"
)

type legacyPlexMetadata struct{ *mockMetadataService }

func (m *legacyPlexMetadata) SeriesDetails(ctx context.Context, query models.SeriesDetailsQuery) (*models.SeriesDetails, error) {
	if query.TMDBID == 7269518 || query.TMDBID == 7269519 {
		return nil, fmt.Errorf("episode ID is not a series")
	}
	return m.mockMetadataService.SeriesDetails(ctx, query)
}

func (m *legacyPlexMetadata) SeriesDetailsLite(ctx context.Context, query models.SeriesDetailsQuery) (*models.SeriesDetails, error) {
	return m.SeriesDetails(ctx, query)
}

func legacyPlexSeriesMetadata() *models.SeriesDetails {
	return &models.SeriesDetails{
		Title: models.Title{ID: "tvdb:series:465664", Name: "Stuart", TVDBID: 465664, TMDBID: 287620},
		Seasons: []models.SeriesSeason{{Number: 1, Episodes: []models.SeriesEpisode{
			{SeasonNumber: 1, EpisodeNumber: 1, TMDBID: 7269518, TVDBID: 11900001, AiredDate: "2020-01-01"},
			{SeasonNumber: 1, EpisodeNumber: 2, TMDBID: 7269519, TVDBID: 11900002, AiredDate: "2020-01-02"},
			{SeasonNumber: 1, EpisodeNumber: 3, TMDBID: 7269520, TVDBID: 11900003, AiredDate: "2020-01-03"},
		}}},
	}
}

func legacyPlexEpisode(episode int) models.WatchHistoryItem {
	seriesID := fmt.Sprintf("tmdb:tv:%d", 7269517+episode)
	return normalizeWatchHistoryItem(models.WatchHistoryItem{
		MediaType: "episode", ItemID: fmt.Sprintf("%s:s01e%02d", seriesID, episode), SeriesID: seriesID,
		SeriesName: "Stuart", SeasonNumber: 1, EpisodeNumber: episode, Watched: true,
		WatchedAt: time.Now().UTC().Add(-time.Hour), UpdatedAt: time.Now().UTC().Add(-time.Hour),
		ExternalIDs: map[string]string{"tmdb": fmt.Sprint(7269517 + episode), "tvdb": fmt.Sprint(11900000 + episode)},
	})
}

func TestContinueWatchingRepairsPlexEpisodeSeriesIDsBeforeMarkWatched(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetMetadataService(&legacyPlexMetadata{&mockMetadataService{seriesDetails: legacyPlexSeriesMetadata()}})
	first, second := legacyPlexEpisode(1), legacyPlexEpisode(2)
	svc.watchHistory["user"] = map[string]models.WatchHistoryItem{first.ID: first, second.ID: second}
	historyRevision, _ := svc.GetWatchHistoryRevision("user")
	continueRevision, _ := svc.GetContinueWatchingRevision("user")

	states, err := svc.ListContinueWatching("user")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].WatchedEpisodeCount != 2 || states[0].NextEpisode == nil || states[0].NextEpisode.EpisodeNumber != 3 {
		t.Fatalf("expected one series with two watched episodes and E3 next: %+v", states)
	}
	if states[0].SeriesID != "tmdb:tv:287620" {
		t.Fatalf("wrong series ID: %s", states[0].SeriesID)
	}
	if revision, _ := svc.GetWatchHistoryRevision("user"); revision == historyRevision {
		t.Fatal("history identity repair did not invalidate the client revision")
	}
	if revision, _ := svc.GetContinueWatchingRevision("user"); revision == continueRevision {
		t.Fatal("history identity repair did not invalidate the shelf revision")
	}
	for _, old := range []models.WatchHistoryItem{first, second} {
		if _, ok := svc.watchHistory["user"][old.ID]; ok {
			t.Fatalf("legacy row remains: %s", old.ID)
		}
	}
	// The action uses the same identity that the repaired shelf now exposes.
	watched := true
	_, err = svc.UpdateWatchHistory("user", models.WatchHistoryUpdate{
		MediaType: "episode", ItemID: states[0].SeriesID + ":s01e03", SeriesID: states[0].SeriesID,
		SeriesName: "Stuart", SeasonNumber: 1, EpisodeNumber: 3, Watched: &watched, ExternalIDs: states[0].ExternalIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	states, err = svc.ListContinueWatching("user")
	if err != nil || len(states) != 0 {
		t.Fatalf("completed series remains: %+v, err=%v", states, err)
	}

	reloaded, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.SetMetadataService(&legacyPlexMetadata{&mockMetadataService{seriesDetails: legacyPlexSeriesMetadata()}})
	states, err = reloaded.ListContinueWatching("user")
	if err != nil || len(states) != 0 {
		t.Fatalf("repair did not survive reload: %+v, err=%v", states, err)
	}
	if len(reloaded.watchHistory["user"]) != 3 {
		t.Fatalf("history lost: %+v", reloaded.watchHistory["user"])
	}
}

func TestContinueWatchingRepairsLegacyGroupsAfterBulkMarkWatched(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.SetMetadataService(&mockMetadataService{seriesDetails: legacyPlexSeriesMetadata()})
	svc.watchHistory["user"] = make(map[string]models.WatchHistoryItem)
	for episode := 1; episode <= 2; episode++ {
		item := legacyPlexEpisode(episode)
		item.SeriesID = fmt.Sprintf("tvdb:series:%d", 11900000+episode)
		item.ItemID = fmt.Sprintf("%s:s01e%02d", item.SeriesID, episode)
		item.ExternalIDs = map[string]string{"tvdb": fmt.Sprint(11900000 + episode)}
		item = normalizeWatchHistoryItem(item)
		svc.watchHistory["user"][item.ID] = item
	}
	// This reproduces the user's action: the real series is now fully watched,
	// but the imported groups still each contain only one watched episode.
	watched := true
	var updates []models.WatchHistoryUpdate
	for episode := 1; episode <= 3; episode++ {
		updates = append(updates, models.WatchHistoryUpdate{
			MediaType: "episode", ItemID: fmt.Sprintf("tmdb:tv:287620:s01e%02d", episode),
			SeriesID: "tmdb:tv:287620", SeriesName: "Stuart", SeasonNumber: 1, EpisodeNumber: episode,
			Watched: &watched, ExternalIDs: map[string]string{"tmdb": "287620", "tvdb": "465664"},
		})
	}
	if _, err := svc.BulkUpdateWatchHistory("user", updates); err != nil {
		t.Fatal(err)
	}
	before, err := svc.buildSeriesStatesWithIdentityRepair(context.Background(), "user", true, false)
	if err != nil || len(before) == 0 {
		t.Fatalf("fixture does not reproduce stuck Continue Watching: %+v, %v", before, err)
	}
	after, err := svc.ListContinueWatching("user")
	if err != nil || len(after) != 0 {
		t.Fatalf("fully watched show remains after repair: %+v, %v", after, err)
	}
	if len(svc.watchHistory["user"]) != 3 {
		t.Fatalf("expected canonical episode history only: %+v", svc.watchHistory["user"])
	}
}

func TestEpisodeSeriesRepairRequiresProviderAndCoordinateProof(t *testing.T) {
	item := legacyPlexEpisode(1)
	for _, tc := range []struct {
		name   string
		change func(*models.WatchHistoryItem)
	}{
		{"different episode coordinates", func(i *models.WatchHistoryItem) { i.EpisodeNumber = 2 }},
		{"name alone", func(i *models.WatchHistoryItem) { i.SeriesID = "tmdb:tv:999999" }},
		{"already correct", func(i *models.WatchHistoryItem) { i.SeriesID = "tmdb:tv:287620" }},
		{"different provider", func(i *models.WatchHistoryItem) { i.SeriesID = "tvdb:series:7269518" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := item
			tc.change(&candidate)
			if _, ok := repairEpisodeSeriesIdentity(candidate, legacyPlexSeriesMetadata()); ok {
				t.Fatal("unproven identity was repaired")
			}
		})
	}
}

func TestEpisodeSeriesRepairPreservesNewerUnwatchAndConcurrentChanges(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := legacyPlexEpisode(1)
	repaired, ok := repairEpisodeSeriesIdentity(old, legacyPlexSeriesMetadata())
	if !ok {
		t.Fatal("expected verified repair")
	}
	unwatched := repaired
	unwatched.Watched = false
	unwatched.UpdatedAt = time.Now().UTC()
	svc.watchHistory["user"] = map[string]models.WatchHistoryItem{old.ID: old, unwatched.ID: unwatched}
	changed, err := svc.applyEpisodeSeriesIdentityRepairs("user", []episodeSeriesIdentityRepair{{before: old, after: repaired}})
	if err != nil || !changed {
		t.Fatalf("repair failed: changed=%v err=%v", changed, err)
	}
	if len(svc.watchHistory["user"]) != 1 || svc.watchHistory["user"][repaired.ID].Watched {
		t.Fatal("repair overwrote newer unwatch")
	}

	concurrent := old
	concurrent.Name = "New metadata"
	svc.watchHistory["user"][old.ID] = concurrent
	changed, err = svc.applyEpisodeSeriesIdentityRepairs("user", []episodeSeriesIdentityRepair{{before: old, after: repaired}})
	if err != nil || changed {
		t.Fatalf("stale repair applied: changed=%v err=%v", changed, err)
	}
}
