package history

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"maps"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"novastream/models"
)

type episodeSeriesIdentityRepair struct {
	before models.WatchHistoryItem
	after  models.WatchHistoryItem
}

type episodeSeriesRepairLookup struct {
	once    sync.Once
	details *models.SeriesDetails
}

// Rekeying history preserves timestamps and can preserve the row count. Include
// the identities in client revisions so those corrections still invalidate UI
// caches. XOR makes the aggregate independent of Go map iteration order.
func watchHistoryIdentityRevision(items map[string]models.WatchHistoryItem) uint64 {
	var revision uint64
	for key, item := range items {
		h := fnv.New64a()
		_, _ = h.Write([]byte(key))
		// Alias enrichment preserves the watch timestamp and storage key but
		// changes the watched-status keys sent to clients.
		idTypes := make([]string, 0, len(item.ExternalIDs))
		for idType := range item.ExternalIDs {
			idTypes = append(idTypes, idType)
		}
		sort.Strings(idTypes)
		for _, idType := range idTypes {
			_, _ = fmt.Fprintf(h, "\x00%s=%s", idType, item.ExternalIDs[idType])
		}
		revision ^= h.Sum64()
	}
	return revision
}

// Cache both hits and misses so unresolved legacy rows do not trigger another
// name lookup on every shelf refresh. Names are candidates, never identities.
func (s *Service) getEpisodeSeriesRepairMetadata(ctx context.Context, name string) *models.SeriesDetails {
	key := "episode-series-repair:" + strings.ToLower(name)
	s.mu.RLock()
	cached := s.metadataCache[key]
	metadata := s.metadataService
	s.mu.RUnlock()
	if cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.details
	}
	if metadata == nil {
		return nil
	}
	details, err := metadata.SeriesDetailsLite(ctx, models.SeriesDetailsQuery{Name: name})
	if err != nil {
		details = nil
	}
	s.mu.Lock()
	s.metadataCache[key] = &cachedSeriesMetadata{details: details, cachedAt: time.Now(), expiresAt: time.Now().Add(s.metadataCacheTTL)}
	s.mu.Unlock()
	return details
}

// Correct only a proven episode-as-series identity: the stored series provider
// ID must equal the catalog episode ID at the same season/episode coordinates.
// A name match, a failed lookup, or mismatched show metadata alone is not proof.
func repairEpisodeSeriesIdentity(item models.WatchHistoryItem, details *models.SeriesDetails) (models.WatchHistoryItem, bool) {
	if details == nil || item.MediaType != "episode" || item.EpisodeNumber <= 0 {
		return item, false
	}
	provider, value := seriesProviderAndID(item.SeriesID)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || (provider != "tmdb" && provider != "tvdb") {
		return item, false
	}
	if (provider == "tmdb" && id == details.Title.TMDBID) || (provider == "tvdb" && id == details.Title.TVDBID) {
		return item, false
	}
	for _, season := range details.Seasons {
		for _, episode := range season.Episodes {
			if episode.SeasonNumber != item.SeasonNumber || episode.EpisodeNumber != item.EpisodeNumber {
				continue
			}
			if (provider == "tmdb" && episode.TMDBID != id) || (provider == "tvdb" && episode.TVDBID != id) {
				continue
			}
			var seriesID string
			if details.Title.TVDBID > 0 {
				seriesID = fmt.Sprintf("tvdb:series:%d", details.Title.TVDBID)
			} else if details.Title.TMDBID > 0 {
				seriesID = fmt.Sprintf("tmdb:tv:%d", details.Title.TMDBID)
			} else {
				return item, false
			}
			repaired := item
			repaired.SeriesID = seriesID
			repaired.ItemID = fmt.Sprintf("%s:s%02de%02d", seriesID, item.SeasonNumber, item.EpisodeNumber)
			repaired.ExternalIDs = cloneStringMap(item.ExternalIDs)
			for _, key := range []string{"tmdb", "tvdb", "imdb"} {
				delete(repaired.ExternalIDs, key)
			}
			if details.Title.TMDBID > 0 {
				repaired.ExternalIDs["tmdb"] = strconv.FormatInt(details.Title.TMDBID, 10)
			}
			if details.Title.TVDBID > 0 {
				repaired.ExternalIDs["tvdb"] = strconv.FormatInt(details.Title.TVDBID, 10)
			}
			if details.Title.IMDBID != "" {
				repaired.ExternalIDs["imdb"] = details.Title.IMDBID
			}
			return normalizeWatchHistoryItem(repaired), true
		}
	}
	return item, false
}

func (s *Service) applyEpisodeSeriesIdentityRepairs(userID string, repairs []episodeSeriesIdentityRepair) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	original := s.watchHistory[userID]
	updated := maps.Clone(original)
	count := 0
	for _, repair := range repairs {
		current, exists := updated[repair.before.ID]
		// Metadata requests run outside the lock. Never overwrite a watch action
		// or removal that happened while the repair was being resolved.
		if !exists || !reflect.DeepEqual(current, repair.before) {
			continue
		}
		delete(updated, repair.before.ID)
		repaired := repair.after
		if existing, ok := updated[repaired.ID]; ok {
			repaired, _ = mergeEquivalentEpisodeWatchHistoryItem(repaired, existing)
		}
		updated[repaired.ID] = repaired
		count++
	}
	if count == 0 {
		return false, nil
	}
	s.watchHistory[userID] = updated
	if err := s.saveWatchHistoryLocked(); err != nil {
		s.watchHistory[userID] = original
		return false, err
	}
	s.invalidateContinueWatchingLocked(userID)
	log.Printf("[history] repaired %d episode-as-series history identities", count)
	return true, nil
}
