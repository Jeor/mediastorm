package scrob

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"novastream/config"
	"novastream/internal/watchsync"
	"novastream/models"
)

// SyncScopedWatchHistory uses Scrob's native bulk endpoints only when the
// caller explicitly requested the entire show or season.
func (s *Scrobbler) SyncScopedWatchHistory(userID string, items []models.WatchHistoryItem, scope string) error {
	if len(items) == 0 {
		return nil
	}
	if scope == "" {
		return s.SyncWatchHistory(userID, items)
	}
	if scope != "show" && scope != "season" {
		return fmt.Errorf("invalid Scrob bulk scope %q", scope)
	}
	account := s.getAccountForUser(userID)
	if !scrobAccountCanPush(account) {
		return nil
	}
	item := items[0]
	tmdb := watchsync.ID(item.ExternalIDs, "tmdb")
	if tmdb == 0 {
		return s.SyncWatchHistory(userID, items)
	}
	// Scrob may hydrate decades of seasons on this request. Give native bulk
	// operations a longer deadline without changing normal playback requests.
	bulkHTTP := *s.client.httpClient
	bulkHTTP.Timeout = 2 * time.Minute
	client := NewClientWithHTTPClient(&bulkHTTP)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	token, err := s.login(ctx, account)
	if err != nil {
		return err
	}
	path := "/history/show-all"
	if scope == "season" {
		path = "/history/season"
	}
	if item.Watched {
		payload := map[string]any{"series_tmdb_id": tmdb, "watched_at": scrobbleTime(item.WatchedAt)}
		if tvdb := watchsync.ID(item.ExternalIDs, "tvdb"); tvdb > 0 {
			payload["series_tvdb_id"] = tvdb
		}
		if scope == "season" {
			payload["season_number"] = item.SeasonNumber
			if watchsync.ID(item.ExternalIDs, "tvdb") > 0 {
				payload["episode_order"] = "tvdb:official"
			}
		}
		if err := client.doJSON(ctx, http.MethodPost, account.BaseURL, path, account.APIKey, token, payload); err != nil {
			return err
		}
		// Scrob show-all excludes season zero. Explicitly include specials
		// when they were part of the user's whole-show selection.
		if scope == "show" {
			for _, selected := range items {
				if selected.SeasonNumber == 0 {
					payload["season_number"] = 0
					return client.doJSON(ctx, http.MethodPost, account.BaseURL, "/history/season", account.APIKey, token, payload)
				}
			}
		}
		return nil
	}
	query := url.Values{"series_tmdb_id": {strconv.Itoa(tmdb)}}
	if scope == "season" {
		query.Set("season_number", strconv.Itoa(item.SeasonNumber))
		if watchsync.ID(item.ExternalIDs, "tvdb") > 0 {
			query.Set("episode_order", "tvdb:official")
		}
	}
	return client.doJSON(ctx, http.MethodDelete, account.BaseURL, path+"?"+query.Encode(), account.APIKey, token, nil)
}

// SyncWatchHistory serializes exact selections. Scrob only offers whole-show
// and whole-season bulk endpoints, which would change unselected episodes.
// Authenticate once and fetch history at most once for missing episode IDs.
func (s *Scrobbler) SyncWatchHistory(userID string, items []models.WatchHistoryItem) error {
	account := s.getAccountForUser(userID)
	if len(items) == 0 || !scrobAccountCanPush(account) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	token, err := s.login(ctx, account)
	cancel()
	if err != nil {
		return err
	}
	var history []HistoryEvent
	loadedHistory := false
	for _, item := range items {
		if !item.Watched && item.MediaType == "episode" && watchsync.ID(item.ExternalIDs, "episodeTmdb") == 0 && !loadedHistory {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			history, err = s.client.GetHistory(ctx, account.BaseURL, account.APIKey)
			cancel()
			if err != nil {
				return err
			}
			loadedHistory = true
		}
		if err := s.syncHistoryItem(account, token, item, history); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scrobbler) syncHistoryItem(account *config.ScrobAccount, token string, item models.WatchHistoryItem, history []HistoryEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tmdb, tvdb := watchsync.ID(item.ExternalIDs, "tmdb"), watchsync.ID(item.ExternalIDs, "tvdb")
	if item.Watched {
		var event WatchEvent
		if item.MediaType == "movie" {
			if tmdb == 0 {
				return nil
			}
			event = WatchEvent{TMDBID: tmdb, MediaType: "movie", WatchedAt: scrobbleTime(item.WatchedAt), Completed: true}
		} else {
			var ok bool
			event, ok = scrobEpisodeEvent(tvdb, item.SeasonNumber, item.EpisodeNumber, item.WatchedAt, item.ExternalIDs)
			if !ok {
				return nil
			}
		}
		return s.client.AddHistory(ctx, account.BaseURL, account.APIKey, token, event)
	}
	if item.MediaType == "movie" {
		if tmdb == 0 {
			return nil
		}
		return s.client.RemoveHistory(ctx, account.BaseURL, account.APIKey, token, tmdb, "movie")
	}
	if item.MediaType != "episode" || item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
		return nil
	}
	if episodeID := watchsync.ID(item.ExternalIDs, "episodeTmdb"); episodeID > 0 {
		return s.client.RemoveHistory(ctx, account.BaseURL, account.APIKey, token, episodeID, "episode")
	}
	if tmdb == 0 && tvdb == 0 {
		return nil
	}
	for _, event := range history {
		media := event.Media
		if media.Type != "episode" || media.SeasonNumber != item.SeasonNumber || media.EpisodeNumber != item.EpisodeNumber {
			continue
		}
		if tmdb > 0 && media.ShowTMDBID != tmdb {
			continue
		}
		if tmdb == 0 && media.ShowTVDBID != tvdb {
			continue
		}
		if media.ID > 0 {
			return s.client.RemoveHistoryByID(ctx, account.BaseURL, account.APIKey, token, media.ID, "episode")
		}
	}
	return nil
}
