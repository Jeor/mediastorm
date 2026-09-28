package playback

import (
	"testing"
	"time"
)

func TestReadyUpdatesPreserveRenewalDeadline(t *testing.T) {
	for _, worker := range []bool{false, true} {
		name := "Update"
		if worker {
			name = "UpdateWorker"
		}
		t.Run(name, func(t *testing.T) {
			store := NewPrequeueStore(time.Hour)
			entry, _ := store.Create("title", "Title", "user", "movie", 2026, nil, "prewarm")
			update := store.Update
			if worker {
				update = store.UpdateWorker
			}
			initial := time.Now().Add(time.Minute)
			store.ForceExpiry(entry.ID, initial)
			update(entry.ID, func(e *PrequeueEntry) { e.Status = PrequeueStatusReady; e.StreamPath = "/stream" })
			ready, _ := store.Get(entry.ID)
			if !ready.ExpiresAt.After(initial) {
				t.Fatal("becoming ready must extend TTL")
			}
			deadline := time.Now().Add(5 * time.Minute)
			update(entry.ID, func(e *PrequeueEntry) { e.ExpiresAt = deadline })
			for i := 0; i < 3; i++ {
				update(entry.ID, func(e *PrequeueEntry) { e.TitleName = "Updated title" })
			}
			ready, _ = store.Get(entry.ID)
			if !ready.ExpiresAt.Equal(deadline) {
				t.Fatalf("renewal postponed: got %v, want %v", ready.ExpiresAt, deadline)
			}
			if len(store.ListExpiringBefore(deadline.Add(time.Second))) != 1 {
				t.Fatal("ready entry disappeared from renewal candidates")
			}
			store.MakePersistent(entry.ID)
			update(entry.ID, func(e *PrequeueEntry) { e.ExpiresAt = deadline })
			ready, _ = store.Get(entry.ID)
			if !ready.ExpiresAt.Equal(manualPrequeueExpiry) {
				t.Fatal("manual entry lost persistent expiry")
			}
		})
	}
}
