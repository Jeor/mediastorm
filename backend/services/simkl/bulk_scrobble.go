package simkl

import (
	"time"

	"novastream/internal/watchsync"
	"novastream/models"
)

func (s *Scrobbler) SyncWatchHistory(userID string, items []models.WatchHistoryItem) error {
	account := s.getAccountForUser(userID)
	if len(items) == 0 || account == nil || account.ClientID == "" || account.AccessToken == "" {
		return nil
	}
	for _, group := range watchsync.Groups(items) {
		var req SyncHistoryRequest
		type seasonKey struct {
			ids    IDs
			number int
		}
		shows := make(map[IDs]int)
		seasons := make(map[seasonKey]int)
		for _, item := range group {
			ids := showSyncIDs(watchsync.ID(item.ExternalIDs, "tvdb"), item.ExternalIDs)
			if ids == (IDs{}) {
				continue
			}
			if item.Watched && s.wasRecentlyStopped(userID, item.MediaType, ids.TMDB, ids.TVDB, ids.IMDB, item.SeasonNumber, item.EpisodeNumber) {
				continue
			}
			stamp := ""
			if item.Watched {
				stamp = item.WatchedAt.UTC().Format(time.RFC3339)
			}
			if item.MediaType == "movie" {
				req.Movies = append(req.Movies, SyncHistoryMovie{IDs: ids, WatchedAt: stamp})
				continue
			}
			if item.MediaType != "episode" || item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
				continue
			}
			ids, season, episode := EpisodeIdentity(ids, item.SeasonNumber, item.EpisodeNumber)
			showIndex, exists := shows[ids]
			if !exists {
				showIndex = len(req.Shows)
				shows[ids] = showIndex
				req.Shows = append(req.Shows, SyncHistoryShow{IDs: ids})
			}
			key := seasonKey{ids, season}
			seasonIndex, exists := seasons[key]
			if !exists {
				seasonIndex = len(req.Shows[showIndex].Seasons)
				seasons[key] = seasonIndex
				req.Shows[showIndex].Seasons = append(req.Shows[showIndex].Seasons, SyncHistorySeason{Number: season})
			}
			target := &req.Shows[showIndex].Seasons[seasonIndex]
			target.Episodes = append(target.Episodes, SyncHistoryEpisode{Number: episode, WatchedAt: stamp})
		}
		if len(req.Movies) == 0 && len(req.Shows) == 0 {
			continue
		}
		var err error
		if group[0].Watched {
			// Keep a show's episodes together so the all-not-found safety check
			// cannot undo an earlier successful chunk of the same show.
			_, err = s.client.SyncHistorySafe(account.ClientID, account.AccessToken, req)
		} else {
			err = s.client.RemoveFromHistory(account.ClientID, account.AccessToken, req)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
