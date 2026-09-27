package history

import (
	"errors"
	"fmt"
	"log"
	"time"

	"novastream/internal/mediaidentity"
	"novastream/internal/watchsync"
	"novastream/models"
)

var ErrInvalidBulkScope = errors.New("invalid bulk watch scope")

// BulkScrobbler preserves a manual bulk action through the provider boundary.
type BulkScrobbler interface {
	SyncWatchHistory(userID string, items []models.WatchHistoryItem) error
}

type ScopedBulkScrobbler interface {
	SyncScopedWatchHistory(userID string, items []models.WatchHistoryItem, scope string) error
}

// Validate scope before mutating local history. Never infer whole-series intent
// from a list of episodes: it may be a partially loaded season or a selection.
func validateBulkScope(updates []models.WatchHistoryUpdate, scope string) error {
	if scope == "" {
		return nil
	}
	if scope != "show" && scope != "season" {
		return fmt.Errorf("%w: %q", ErrInvalidBulkScope, scope)
	}
	var items []models.WatchHistoryItem
	for _, update := range updates {
		update = normalizeWatchHistoryUpdate(update)
		if update.MediaType != "episode" || update.Watched == nil || update.SeasonNumber < 0 || update.EpisodeNumber <= 0 {
			return fmt.Errorf("%w: requires episodes with an explicit watched state", ErrInvalidBulkScope)
		}
		items = append(items, models.WatchHistoryItem{MediaType: "episode", Watched: *update.Watched, SeasonNumber: update.SeasonNumber,
			ExternalIDs: mediaidentity.EnrichShowExternalIDs(update.SeriesID, update.ItemID, update.ExternalIDs)})
	}
	if len(items) == 0 || len(watchsync.Groups(items)) != 1 {
		return fmt.Errorf("%w: requires one show and watched state", ErrInvalidBulkScope)
	}
	if scope == "season" {
		for _, item := range items {
			if item.SeasonNumber != items[0].SeasonNumber {
				return fmt.Errorf("%w: season scope requires one season", ErrInvalidBulkScope)
			}
		}
	}
	return nil
}

func (s *Service) doBulkScrobble(scrobbler TraktScrobbler, userID string, items []models.WatchHistoryItem, scope string) {
	if scrobbler == nil || len(items) == 0 || !scrobbler.IsEnabledForUser(userID) {
		return
	}
	eligible := make([]models.WatchHistoryItem, 0, len(items))
	alternate := make(map[int]bool)
	for _, item := range items {
		item.ExternalIDs = mediaidentity.EnrichShowExternalIDs(item.SeriesID, item.ItemID, item.ExternalIDs)
		tvdb := watchsync.ID(item.ExternalIDs, "tvdb")
		if item.MediaType == "episode" {
			if item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
				continue
			}
			skip, known := alternate[tvdb]
			if !known {
				skip = s.seriesOrderingIsAlternate(userID, tvdb)
				alternate[tvdb] = skip
			}
			if skip {
				continue
			}
		} else if item.MediaType != "movie" {
			continue
		}
		if tvdb == 0 && watchsync.ID(item.ExternalIDs, "tmdb") == 0 && item.ExternalIDs["imdb"] == "" {
			continue
		}
		if item.Watched && item.WatchedAt.IsZero() {
			item.WatchedAt = time.Now().UTC()
		}
		eligible = append(eligible, item)
	}
	if len(eligible) == 0 {
		return
	}
	if len(eligible) != len(items) {
		scope = ""
	}
	// One worker per action, never one goroutine per episode.
	// Called while holding s.mu: capture queue order before returning to the UI.
	if s.bulkSyncTail == nil {
		s.bulkSyncTail = make(map[string]<-chan struct{})
	}
	previous := s.bulkSyncTail[userID]
	done := make(chan struct{})
	s.bulkSyncTail[userID] = done
	go func() {
		defer close(done)
		if previous != nil {
			<-previous
		}
		if err := syncScopedWatchHistory(scrobbler, userID, eligible, scope); err != nil {
			log.Printf("[history] bulk provider sync failed for user %s (%d items): %v", userID, len(eligible), err)
		}
	}()
}

func syncScopedWatchHistory(scrobbler TraktScrobbler, userID string, items []models.WatchHistoryItem, scope string) error {
	if !scrobbler.IsEnabledForUser(userID) {
		return nil
	}
	if scoped, ok := scrobbler.(ScopedBulkScrobbler); ok && scope != "" {
		return scoped.SyncScopedWatchHistory(userID, items, scope)
	}
	return syncWatchHistory(scrobbler, userID, items)
}

func syncWatchHistory(scrobbler TraktScrobbler, userID string, items []models.WatchHistoryItem) error {
	if !scrobbler.IsEnabledForUser(userID) {
		return nil
	}
	if bulk, ok := scrobbler.(BulkScrobbler); ok {
		return bulk.SyncWatchHistory(userID, items)
	}
	// Compatibility for integrations without a bulk implementation. Never fan
	// these out, and stop on failure instead of hammering a rate-limited service.
	for _, item := range items {
		tmdb, tvdb := watchsync.ID(item.ExternalIDs, "tmdb"), watchsync.ID(item.ExternalIDs, "tvdb")
		var err error
		if item.MediaType == "movie" {
			if item.Watched {
				err = scrobbler.ScrobbleMovie(userID, tmdb, tvdb, item.ExternalIDs["imdb"], item.WatchedAt)
			} else {
				err = scrobbler.UnscrobbleMovie(userID, tmdb, tvdb, item.ExternalIDs["imdb"])
			}
		} else if item.Watched {
			err = scrobbler.ScrobbleEpisode(userID, tvdb, item.SeasonNumber, item.EpisodeNumber, item.WatchedAt, item.ExternalIDs)
		} else {
			err = scrobbler.UnscrobbleEpisode(userID, tvdb, item.SeasonNumber, item.EpisodeNumber, item.ExternalIDs)
		}
		if err != nil {
			return fmt.Errorf("%T: %w", scrobbler, err)
		}
	}
	return nil
}
