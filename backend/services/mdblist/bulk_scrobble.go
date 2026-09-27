package mdblist

import (
	"fmt"
	"time"

	"novastream/internal/watchsync"
	"novastream/models"
)

func (s *Scrobbler) SyncWatchHistory(userID string, items []models.WatchHistoryItem) error {
	account := s.getAccountForUser(userID)
	if len(items) == 0 || account == nil || account.APIKey == "" {
		return nil
	}
	client := NewScrobbleClient(account.APIKey)
	client.httpClient = s.client.httpClient
	for _, group := range watchsync.Groups(items) {
		for start := 0; start < len(group); start += watchsync.BatchSize {
			chunk := group[start:min(start+watchsync.BatchSize, len(group))]
			req := bulkWatchedRequest(chunk)
			if len(req.Movies) == 0 && len(req.Shows) == 0 {
				continue
			}
			var result SyncWatchedResult
			var err error
			if group[0].Watched {
				result, err = client.SyncWatchedDetailed(req)
			} else {
				result, err = client.SyncUnwatchedDetailed(req)
			}
			if err != nil {
				return err
			}
			missing := make(map[string]bool)
			for _, key := range result.NotFoundEpisodeKeys {
				missing[key] = true
			}
			var retry []models.WatchHistoryItem
			for _, item := range chunk {
				if !missing[fmt.Sprintf("%d:%d", item.SeasonNumber, item.EpisodeNumber)] {
					continue
				}
				absolute := absoluteEpisodeFromExternalIDs(item.ExternalIDs)
				if absolute <= 0 || absolute == item.EpisodeNumber {
					continue
				}
				item.EpisodeNumber = absolute
				retry = append(retry, item)
			}
			if len(retry) > 0 {
				if group[0].Watched {
					err = client.SyncWatched(bulkWatchedRequest(retry))
				} else {
					err = client.SyncUnwatched(bulkWatchedRequest(retry))
				}
				if err != nil {
					return err
				}
			}
		}
	}
	// Scan retained play history once per media type for the entire operation,
	// and remove matching play IDs in bulk (not one scan per episode).
	for _, mediaType := range []string{"movie", "episode"} {
		var targets []models.WatchHistoryItem
		for _, item := range items {
			if !item.Watched && item.MediaType == mediaType && resolveShowScrobbleIDs(item.ExternalIDs) != (ScrobbleIDs{}) {
				targets = append(targets, item)
			}
		}
		if len(targets) == 0 {
			continue
		}
		if err := client.removeMatchingPlays(mediaType, func(page watchedPlaysResponse, index int) bool {
			for _, item := range targets {
				ids := resolveShowScrobbleIDs(item.ExternalIDs)
				if mediaType == "movie" {
					if watchedPlayIDsMatch(page.Movies[index].Movie.IDs, ids) {
						return true
					}
					continue
				}
				candidate := page.Episodes[index].Episode
				if id := watchsync.ID(item.ExternalIDs, "episodeTmdb"); id > 0 && candidate.IDs.TMDB == id {
					return true
				}
				if id := watchsync.ID(item.ExternalIDs, "episodeTvdb"); id > 0 && candidate.IDs.TVDB == id {
					return true
				}
				if candidate.Season == item.SeasonNumber && candidate.Number == item.EpisodeNumber && watchedPlayIDsMatch(candidate.Show.IDs, ids) {
					return true
				}
			}
			return false
		}); err != nil {
			return err
		}
	}
	return nil
}

func bulkWatchedRequest(items []models.WatchHistoryItem) SyncWatchedRequest {
	var req SyncWatchedRequest
	seasons := make(map[int]int)
	for _, item := range items {
		ids := resolveShowScrobbleIDs(item.ExternalIDs)
		if ids == (ScrobbleIDs{}) {
			continue
		}
		stamp := ""
		if item.Watched {
			stamp = item.WatchedAt.UTC().Format(time.RFC3339)
		}
		if item.MediaType == "movie" {
			req.Movies = append(req.Movies, SyncWatchedMovieItem{IDs: ids, WatchedAt: stamp})
			continue
		}
		if item.MediaType != "episode" || item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
			continue
		}
		if len(req.Shows) == 0 {
			req.Shows = append(req.Shows, SyncWatchedShowItem{IDs: ids})
		}
		index, exists := seasons[item.SeasonNumber]
		if !exists {
			index = len(req.Shows[0].Seasons)
			seasons[item.SeasonNumber] = index
			req.Shows[0].Seasons = append(req.Shows[0].Seasons, SyncWatchedSeason{Number: item.SeasonNumber})
		}
		season := &req.Shows[0].Seasons[index]
		season.Episodes = append(season.Episodes, SyncWatchedEpisode{Number: item.EpisodeNumber, WatchedAt: stamp})
	}
	return req
}
