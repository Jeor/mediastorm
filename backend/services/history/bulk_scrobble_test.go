package history

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"novastream/models"
)

type bulkSyncCall struct {
	items []models.WatchHistoryItem
	scope string
}
type bulkTestScrobbler struct {
	mockScrobbler
	calls chan bulkSyncCall
	block <-chan struct{}
}

func (s *bulkTestScrobbler) SyncWatchHistory(userID string, items []models.WatchHistoryItem) error {
	return s.SyncScopedWatchHistory(userID, items, "")
}
func (s *bulkTestScrobbler) SyncScopedWatchHistory(_ string, items []models.WatchHistoryItem, scope string) error {
	s.calls <- bulkSyncCall{items: items, scope: scope}
	if s.block != nil {
		<-s.block
	}
	return s.returnErr
}
func nextBulkCall(t *testing.T, s *bulkTestScrobbler) bulkSyncCall {
	t.Helper()
	select {
	case call := <-s.calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("bulk sync timed out")
		return bulkSyncCall{}
	}
}

func TestBulkHistoryPreservesBatchScopeAndOrder(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	provider := &bulkTestScrobbler{mockScrobbler: mockScrobbler{enabled: true}, calls: make(chan bulkSyncCall, 10), block: gate}
	svc.SetTraktScrobbler(NewMultiScrobbler(provider))
	watched, unwatched := true, false
	var updates []models.WatchHistoryUpdate
	for i := 1; i <= 1201; i++ {
		updates = append(updates, models.WatchHistoryUpdate{
			MediaType: "episode", ItemID: fmt.Sprintf("tmdb:tv:1667:s01e%04d", i), SeriesID: "tmdb:tv:1667",
			SeasonNumber: 1, EpisodeNumber: i, Watched: &watched,
		})
	}
	if _, err := svc.BulkUpdateScopedWatchHistory("user", updates, "show"); err != nil {
		t.Fatal(err)
	}
	call := nextBulkCall(t, provider)
	if len(call.items) != 1201 || call.scope != "show" || call.items[0].ExternalIDs["tmdb"] != "1667" {
		t.Fatalf("items=%d scope=%s", len(call.items), call.scope)
	}
	// Redundant updates do not generate another history add.
	if _, err := svc.BulkUpdateScopedWatchHistory("user", updates, "show"); err != nil {
		t.Fatal(err)
	}
	for i := range updates {
		updates[i].Watched = &unwatched
	}
	if _, err := svc.BulkUpdateScopedWatchHistory("user", updates, "show"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.calls:
		t.Fatal("unwatch overtook watch")
	default:
	}
	close(gate)
	call = nextBulkCall(t, provider)
	if len(call.items) != 1201 || call.items[0].Watched {
		t.Fatalf("unwatch batch = %+v", call)
	}
	if provider.episodeCalls != 0 || provider.episodeRemovals != 0 {
		t.Fatal("bulk sync used single-episode API")
	}
}

func TestBulkFanoutContinuesAfterProviderFailure(t *testing.T) {
	failure := errors.New("rate limited")
	first := &bulkTestScrobbler{mockScrobbler: mockScrobbler{enabled: true, returnErr: failure}, calls: make(chan bulkSyncCall, 1)}
	second := &bulkTestScrobbler{mockScrobbler: mockScrobbler{enabled: true}, calls: make(chan bulkSyncCall, 1)}
	disabled := &bulkTestScrobbler{calls: make(chan bulkSyncCall, 1)}
	if err := NewMultiScrobbler(first, disabled, second).SyncWatchHistory("user", []models.WatchHistoryItem{{MediaType: "movie", Watched: true}}); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	if len(first.calls) != 1 || len(second.calls) != 1 || len(disabled.calls) != 0 {
		t.Fatal("incorrect provider fanout")
	}
}

func TestInvalidBulkScopeDoesNotMutateHistory(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	watched := true
	updates := []models.WatchHistoryUpdate{
		{MediaType: "episode", ItemID: "tmdb:tv:1:s01e01", SeriesID: "tmdb:tv:1", SeasonNumber: 1, EpisodeNumber: 1, Watched: &watched},
		{MediaType: "episode", ItemID: "tmdb:tv:2:s01e01", SeriesID: "tmdb:tv:2", SeasonNumber: 1, EpisodeNumber: 1, Watched: &watched},
	}
	if _, err := svc.BulkUpdateScopedWatchHistory("user", updates, "show"); !errors.Is(err, ErrInvalidBulkScope) {
		t.Fatalf("error=%v", err)
	}
	if len(svc.watchHistory["user"]) != 0 {
		t.Fatal("invalid scoped request mutated history")
	}
}
