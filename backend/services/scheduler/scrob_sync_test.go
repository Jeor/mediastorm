package scheduler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"novastream/config"
	"novastream/models"
	"novastream/services/history"
	"novastream/services/scrob"
)

type schedulerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f schedulerRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func schedulerJSONResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

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

func TestContinueWatchingToScrobSessionMovie(t *testing.T) {
	start, key, progress, ok := continueWatchingToScrobSession(models.SeriesWatchState{
		SeriesID: "tmdb:movie:550", SeriesTitle: "Fight Club", PercentWatched: 25,
		ExternalIDs: map[string]string{"tmdb": "550"},
		LastWatched: models.EpisodeReference{RuntimeMinutes: 139},
	})
	if !ok || key != "movie:550" || progress != 25 || start.TMDBID != 550 || start.Runtime != 139 {
		t.Fatalf("start=%+v key=%q progress=%v ok=%v", start, key, progress, ok)
	}
}

func TestContinueWatchingToScrobSessionEpisode(t *testing.T) {
	start, key, progress, ok := continueWatchingToScrobSession(models.SeriesWatchState{
		SeriesID: "tmdb:tv:42", SeriesTitle: "Show", ResumePercent: 37.5,
		ExternalIDs: map[string]string{"tmdb": "42"},
		NextEpisode: &models.EpisodeReference{SeasonNumber: 0, EpisodeNumber: 3, Title: "Special", RuntimeMinutes: 24},
	})
	if !ok || key != "episode:42:0:3" || progress != 37.5 || start.ShowTMDBID != 42 || start.Runtime != 24 {
		t.Fatalf("start=%+v key=%q progress=%v ok=%v", start, key, progress, ok)
	}
	if start.SeasonNumber == nil || *start.SeasonNumber != 0 || start.EpisodeNumber == nil || *start.EpisodeNumber != 3 {
		t.Fatalf("coordinates=%+v", start)
	}
}

func TestContinueWatchingToScrobSessionProgressWindow(t *testing.T) {
	for _, progress := range []float64{0, 5, 90, 100} {
		_, _, _, ok := continueWatchingToScrobSession(models.SeriesWatchState{
			SeriesID: "tmdb:movie:550", PercentWatched: progress,
			ExternalIDs: map[string]string{"tmdb": "550"},
		})
		if ok {
			t.Fatalf("progress %v should be excluded", progress)
		}
	}
	for _, progress := range []float64{5.01, 89.99} {
		_, _, _, ok := continueWatchingToScrobSession(models.SeriesWatchState{
			SeriesID: "tmdb:movie:550", PercentWatched: progress,
			ExternalIDs: map[string]string{"tmdb": "550"}, LastWatched: models.EpisodeReference{RuntimeMinutes: 139},
		})
		if !ok {
			t.Fatalf("progress %v should be eligible", progress)
		}
	}
}

func TestScrobProgressToUpdateMovie(t *testing.T) {
	when := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	update, key, ok := scrobProgressToUpdate(scrob.PlaybackProgressEvent{
		ProgressSeconds: 1800, ProgressPercent: 0.25, UpdatedAt: when,
		Media: scrob.Media{Type: "movie", TMDBID: 550, Title: "Fight Club", Runtime: 120, ReleaseDate: "1999-10-15"},
	})
	if !ok || key != "movie:550" || update.ItemID != "tmdb:movie:550" || update.Position != 1800 || update.Duration != 7200 || update.PercentWatched != 25 {
		t.Fatalf("update=%+v key=%q ok=%v", update, key, ok)
	}
	if !update.Timestamp.Equal(when) || !update.IsPaused || update.Year != 1999 {
		t.Fatalf("update=%+v", update)
	}
}

func TestScrobProgressToUpdateEpisode(t *testing.T) {
	update, key, ok := scrobProgressToUpdate(scrob.PlaybackProgressEvent{
		ProgressSeconds: 600, ProgressPercent: 0.5,
		Media: scrob.Media{Type: "episode", TMDBID: 99, ShowTMDBID: 42, ShowTVDBID: 84, ShowTitle: "Show", Title: "Pilot", SeasonNumber: 0, EpisodeNumber: 3, Runtime: 20},
	})
	if !ok || key != "episode:42:0:3" || update.ItemID != "tmdb:tv:42:s00e03" || update.SeriesID != "tmdb:tv:42" {
		t.Fatalf("update=%+v key=%q ok=%v", update, key, ok)
	}
	if update.ExternalIDs["tmdb"] != "42" || update.ExternalIDs["tvdb"] != "84" || update.ExternalIDs["episodeTmdb"] != "99" {
		t.Fatalf("ids=%v", update.ExternalIDs)
	}
}

func TestScrobProgressToUpdateUsesStrictProgressWindow(t *testing.T) {
	for _, fraction := range []float64{0.05, 0.90} {
		_, _, ok := scrobProgressToUpdate(scrob.PlaybackProgressEvent{
			ProgressPercent: fraction, Media: scrob.Media{Type: "movie", TMDBID: 550},
		})
		if ok {
			t.Fatalf("fraction %v should be excluded", fraction)
		}
	}
}

func TestLocalPlaybackProgressScrobKey(t *testing.T) {
	if got := localPlaybackProgressScrobKey(models.PlaybackProgress{MediaType: "movie", ItemID: "tmdb:movie:550"}); got != "movie:550" {
		t.Fatalf("movie key=%q", got)
	}
	if got := localPlaybackProgressScrobKey(models.PlaybackProgress{
		MediaType: "episode", SeriesID: "tvdb:series:84", SeasonNumber: 2, EpisodeNumber: 4,
		ExternalIDs: map[string]string{"tmdb": "42", "tvdb": "84"},
	}); got != "episode:42:2:4" {
		t.Fatalf("episode key=%q", got)
	}
}

func TestScrobRemoteProgressIsNewer(t *testing.T) {
	local := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	if !scrobRemoteProgressIsNewer(local.Add(time.Second), local) {
		t.Fatal("newer remote progress should win")
	}
	if scrobRemoteProgressIsNewer(local, local) {
		t.Fatal("local progress should win exact timestamp ties")
	}
	if scrobRemoteProgressIsNewer(local.Add(-time.Second), local) {
		t.Fatal("older remote progress should not win")
	}
	if scrobRemoteProgressIsNewer(time.Time{}, local) {
		t.Fatal("remote progress without a timestamp should not overwrite local progress")
	}
}

func TestSyncScrobHistoryToLocalImportsNewerPartialProgress(t *testing.T) {
	historySvc, err := history.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := scrob.NewClientWithHTTPClient(&http.Client{Transport: schedulerRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/proxy/history":
			return schedulerJSONResponse(`{"page":1,"total_pages":1,"results":[]}`), nil
		case "/api/proxy/history/continue-watching":
			return schedulerJSONResponse(`{"continue_watching":[{"id":1,"progress_seconds":1800,"progress_percent":0.25,"updated_at":"2026-08-12T12:00:00","media":{"tmdb_id":550,"type":"movie","title":"Fight Club","runtime":120}}]}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})})
	svc := &Service{historyService: historySvc, scrobClient: client}
	result, err := svc.syncScrobHistoryToLocal(&config.ScrobAccount{BaseURL: "https://scrob.example", APIKey: "key"}, "user", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 {
		t.Fatalf("result=%+v", result)
	}
	items, err := historySvc.ListPlaybackProgress("user")
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].ItemID != "tmdb:movie:550" || items[0].PercentWatched != 25 || items[0].Position != 1800 {
		t.Fatalf("progress=%+v", items[0])
	}
}

func TestSyncScrobHistoryToLocalPreservesNewerLocalProgress(t *testing.T) {
	historySvc, err := history.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	localTime := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	if _, err := historySvc.ImportPlaybackProgress("user", models.PlaybackProgressUpdate{
		MediaType: "movie", ItemID: "tmdb:movie:550", MovieName: "Fight Club",
		Position: 3600, Duration: 7200, Timestamp: localTime, IsPaused: true,
		ExternalIDs: map[string]string{"tmdb": "550"},
	}); err != nil {
		t.Fatal(err)
	}
	client := scrob.NewClientWithHTTPClient(&http.Client{Transport: schedulerRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/proxy/history" {
			return schedulerJSONResponse(`{"page":1,"total_pages":1,"results":[]}`), nil
		}
		return schedulerJSONResponse(`{"continue_watching":[{"id":1,"progress_seconds":1800,"progress_percent":0.25,"updated_at":"2026-08-12T12:00:00","media":{"tmdb_id":550,"type":"movie","title":"Fight Club","runtime":120}}]}`), nil
	})})
	svc := &Service{historyService: historySvc, scrobClient: client}
	result, err := svc.syncScrobHistoryToLocal(&config.ScrobAccount{BaseURL: "https://scrob.example", APIKey: "key"}, "user", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 0 {
		t.Fatalf("result=%+v", result)
	}
	items, _ := historySvc.ListPlaybackProgress("user")
	if len(items) != 1 || items[0].PercentWatched != 50 || !items[0].UpdatedAt.Equal(localTime) {
		t.Fatalf("progress=%+v", items)
	}
}

func TestSyncScrobHistoryToLocalDoesNotResurrectOlderPartialAfterCompletion(t *testing.T) {
	historySvc, err := history.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	watched := true
	completedAt := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	if _, err := historySvc.UpdateWatchHistory("user", models.WatchHistoryUpdate{
		MediaType: "movie", ItemID: "tmdb:movie:550", Name: "Fight Club", Watched: &watched,
		WatchedAt: completedAt, ExternalIDs: map[string]string{"tmdb": "550"},
	}); err != nil {
		t.Fatal(err)
	}
	client := scrob.NewClientWithHTTPClient(&http.Client{Transport: schedulerRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/proxy/history" {
			return schedulerJSONResponse(`{"page":1,"total_pages":1,"results":[]}`), nil
		}
		return schedulerJSONResponse(`{"continue_watching":[{"id":1,"progress_seconds":1800,"progress_percent":0.25,"updated_at":"2026-08-12T12:00:00","media":{"tmdb_id":550,"type":"movie","title":"Fight Club","runtime":120}}]}`), nil
	})})
	svc := &Service{historyService: historySvc, scrobClient: client}
	result, err := svc.syncScrobHistoryToLocal(&config.ScrobAccount{BaseURL: "https://scrob.example", APIKey: "key"}, "user", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 0 {
		t.Fatalf("result=%+v", result)
	}
	items, _ := historySvc.ListPlaybackProgress("user")
	if len(items) != 0 {
		t.Fatalf("older remote partial progress was resurrected: %+v", items)
	}
}
