package prewarm

import (
	"context"
	"testing"
	"time"

	"novastream/models"
	"novastream/services/playback"
)

type renewalClients struct{}

func (renewalClients) ListByUser(string) []models.Client { return []models.Client{{ID: "tv"}} }

func TestRunOnceRenewsReusedRecentReleaseInDeviceScope(t *testing.T) {
	now := time.Now()
	episode := &models.EpisodeReference{SeasonNumber: 23, EpisodeNumber: 25, AirDateTimeUTC: now.Add(-4 * time.Hour).UTC().Format(time.RFC3339)}
	store := playback.NewPrequeueStore(time.Hour)
	entry, _ := store.CreateScoped("title", "One Piece", "user", "series", 1999, episode, "prewarm", "tv-scope")
	store.Update(entry.ID, func(e *playback.PrequeueEntry) {
		e.Status = playback.PrequeueStatusReady
		e.StreamPath = "/old"
		e.AudioTracks = []playback.AudioTrackInfo{{Index: 0, Language: "jpn", Codec: "aac"}}
	})
	svc := NewService(nil, "")
	svc.SetPrequeueStore(store)
	svc.SetUsersService(&mockUsersProvider{users: []models.User{{ID: "user"}}})
	svc.SetHistoryService(&mockHistoryProvider{continueWatching: map[string][]models.SeriesWatchState{"user": {{SeriesID: "title", SeriesTitle: "One Piece", Year: 1999, UpdatedAt: now, NextEpisode: episode, ExternalIDs: map[string]string{"imdbId": "tt0388629"}}}}})
	svc.SetClientsService(renewalClients{})
	svc.SetScopeKeyFunc(func(_, clientID, _ string) string {
		if clientID == "tv" {
			return "tv-scope"
		}
		return "profile-scope"
	})
	svc.jitterFn = func() time.Duration { return 0 }
	svc.entries[entryKey("title", "user", "tv-scope")] = &WarmEntry{TitleID: "title", UserID: "user", SettingsScopeKey: "tv-scope", ImdbID: "tt0388629", PrequeueID: entry.ID, LastResolve: now.Add(-55 * time.Minute), ExpiresAt: now.Add(5 * time.Minute)}
	renewed := 0
	svc.SetScopedWorkerFunc(func(ctx context.Context, title, name, imdb, media string, year int, user, client, scope string, ep *models.EpisodeReference) (string, error) {
		if scope == "tv-scope" {
			renewed++
			if client != "tv" || imdb != "tt0388629" || !playback.EpisodeReferencesMatch(ep, episode) || ep.AirDateTimeUTC != episode.AirDateTimeUTC {
				t.Errorf("renewal lost original context: client=%s imdb=%s episode=%+v", client, imdb, ep)
			}
		}
		next, _ := store.CreateScoped(title, name, user, media, year, ep, "prewarm", scope)
		store.Update(next.ID, func(e *playback.PrequeueEntry) {
			e.Status = playback.PrequeueStatusReady
			e.StreamPath = "/new"
			e.AudioTracks = []playback.AudioTrackInfo{{Index: 0, Language: "jpn", Codec: "aac"}}
		})
		return next.ID, nil
	})
	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if renewed != 1 {
		t.Fatalf("expected fresh device-scoped resolve after reuse, got %d", renewed)
	}
	warm := svc.entries[entryKey("title", "user", "tv-scope")]
	if warm.PrequeueID == entry.ID || warm.StreamPath != "/new" {
		t.Fatalf("warm entry not replaced: %+v", warm)
	}
	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if renewed != 1 {
		t.Fatal("freshly renewed entry searched again before deadline")
	}
}
