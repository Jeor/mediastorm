package realtimesessions

import (
	"context"
	"errors"
	"testing"

	"novastream/models"
)

type activeStub struct{ activeItems map[string]bool }

func (s activeStub) IsPlaybackActive(_ string, update models.PlaybackProgressUpdate) bool {
	return s.activeItems[update.ItemID]
}

type cleanerStub struct {
	cleaned []string
	err     error
}

func (s *cleanerStub) CleanupRealtimeSession(_ context.Context, session models.RealtimeScrobbleSession) error {
	s.cleaned = append(s.cleaned, session.ItemID)
	return s.err
}

func TestSweepCleansOnlySessionsMissingFromDashboard(t *testing.T) {
	store := NewMemoryStore()
	registry := New(store, 0)
	cleaner := &cleanerStub{}
	registry.RegisterCleaner("trakt", cleaner)
	registry.SetActivePlaybackProvider(activeStub{activeItems: map[string]bool{"active": true}})
	registry.Record("trakt", "user", "playing", "", models.PlaybackProgressUpdate{MediaType: "movie", ItemID: "active"}, 25)
	registry.Record("trakt", "user", "paused", "", models.PlaybackProgressUpdate{MediaType: "movie", ItemID: "lingering"}, 40)

	registry.Sweep(context.Background())
	if len(cleaner.cleaned) != 1 || cleaner.cleaned[0] != "lingering" {
		t.Fatalf("cleaned = %v, want [lingering]", cleaner.cleaned)
	}
	sessions, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ItemID != "active" {
		t.Fatalf("remaining sessions = %+v, want active only", sessions)
	}
}

func TestSweepRetainsRecordWhenProviderCleanupFails(t *testing.T) {
	store := NewMemoryStore()
	registry := New(store, 0)
	registry.RegisterCleaner("scrob", &cleanerStub{err: errors.New("temporary failure")})
	registry.SetActivePlaybackProvider(activeStub{activeItems: map[string]bool{}})
	registry.Record("scrob", "user", "paused", "remote-1", models.PlaybackProgressUpdate{MediaType: "episode", ItemID: "episode"}, 14)

	registry.Sweep(context.Background())
	sessions, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].RemoteKey != "remote-1" {
		t.Fatalf("failed cleanup record was removed: %+v", sessions)
	}
}
