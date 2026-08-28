package realtimesessions

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"novastream/models"
)

const DefaultCleanupInterval = time.Hour

type Store interface {
	Upsert(ctx context.Context, session *models.RealtimeScrobbleSession) error
	List(ctx context.Context) ([]models.RealtimeScrobbleSession, error)
	Delete(ctx context.Context, provider, userID, mediaType, itemID string) error
}

type Cleaner interface {
	CleanupRealtimeSession(ctx context.Context, session models.RealtimeScrobbleSession) error
}

type ActivePlaybackProvider interface {
	IsPlaybackActive(userID string, update models.PlaybackProgressUpdate) bool
}

// Registry records successful provider-side starts and owns the single cleanup
// worker shared by all realtime scrobblers.
type Registry struct {
	store    Store
	mu       sync.RWMutex
	cleaners map[string]Cleaner
	active   ActivePlaybackProvider
	interval time.Duration
}

func New(store Store, interval time.Duration) *Registry {
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	return &Registry{store: store, cleaners: make(map[string]Cleaner), interval: interval}
}

func (r *Registry) RegisterCleaner(provider string, cleaner Cleaner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleaners[strings.ToLower(strings.TrimSpace(provider))] = cleaner
}

func (r *Registry) SetActivePlaybackProvider(provider ActivePlaybackProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = provider
}

func (r *Registry) Record(provider, userID, state, remoteKey string, update models.PlaybackProgressUpdate, percentWatched float64) {
	if r == nil || r.store == nil {
		return
	}
	now := time.Now().UTC()
	session := models.RealtimeScrobbleSession{
		Provider: strings.ToLower(strings.TrimSpace(provider)), UserID: userID,
		MediaType: strings.ToLower(update.MediaType), ItemID: strings.ToLower(update.ItemID),
		RemoteKey: remoteKey, State: state, PercentWatched: percentWatched,
		Update: update, StartedAt: now, UpdatedAt: now,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.store.Upsert(ctx, &session); err != nil {
		log.Printf("[realtime-sessions] record %s session failed user=%s item=%s: %v", session.Provider, userID, session.ItemID, err)
	}
}

func (r *Registry) Remove(provider, userID string, update models.PlaybackProgressUpdate) {
	if r == nil || r.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.store.Delete(ctx, strings.ToLower(provider), userID, strings.ToLower(update.MediaType), strings.ToLower(update.ItemID)); err != nil {
		log.Printf("[realtime-sessions] remove %s session failed user=%s item=%s: %v", provider, userID, update.ItemID, err)
	}
}

func (r *Registry) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Sweep(ctx)
		}
	}
}

func (r *Registry) Sweep(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	sessions, err := r.store.List(ctx)
	if err != nil {
		log.Printf("[realtime-sessions] list failed: %v", err)
		return
	}
	for _, session := range sessions {
		r.mu.RLock()
		active := r.active
		cleaner := r.cleaners[session.Provider]
		r.mu.RUnlock()
		if active == nil || active.IsPlaybackActive(session.UserID, session.Update) {
			continue
		}
		if cleaner == nil {
			log.Printf("[realtime-sessions] no cleaner registered for provider %s", session.Provider)
			continue
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := cleaner.CleanupRealtimeSession(cleanupCtx, session)
		cancel()
		if err != nil {
			log.Printf("[realtime-sessions] cleanup failed provider=%s user=%s item=%s: %v", session.Provider, session.UserID, session.ItemID, err)
			continue
		}
		if err := r.store.Delete(ctx, session.Provider, session.UserID, session.MediaType, session.ItemID); err != nil {
			log.Printf("[realtime-sessions] delete cleaned record failed provider=%s user=%s item=%s: %v", session.Provider, session.UserID, session.ItemID, err)
			continue
		}
		log.Printf("[realtime-sessions] removed lingering %s session user=%s item=%s", session.Provider, session.UserID, session.ItemID)
	}
}

// MemoryStore keeps legacy/non-PostgreSQL startup paths working.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]models.RealtimeScrobbleSession
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]models.RealtimeScrobbleSession)}
}

func recordKey(provider, userID, mediaType, itemID string) string {
	return strings.Join([]string{provider, userID, mediaType, itemID}, "\x00")
}

func (s *MemoryStore) Upsert(_ context.Context, session *models.RealtimeScrobbleSession) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := recordKey(session.Provider, session.UserID, session.MediaType, session.ItemID)
	if existing, ok := s.sessions[key]; ok {
		session.StartedAt = existing.StartedAt
	}
	session.UpdatedAt = time.Now().UTC()
	s.sessions[key] = *session
	return nil
}

func (s *MemoryStore) List(_ context.Context) ([]models.RealtimeScrobbleSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]models.RealtimeScrobbleSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		result = append(result, session)
	}
	return result, nil
}

func (s *MemoryStore) Delete(_ context.Context, provider, userID, mediaType, itemID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, recordKey(provider, userID, mediaType, itemID))
	return nil
}
