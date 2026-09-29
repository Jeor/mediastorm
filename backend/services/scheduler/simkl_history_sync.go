package scheduler

import (
	"errors"
	"fmt"
	"log"
	"novastream/config"
	"novastream/models"
	"novastream/services/simkl"
	"strings"
	"time"
)

// executeSimklHistorySync syncs watch history between Simkl and local.
// Live scrobble already uses POST /sync/history; scheduled export heals misses.
func (s *Service) executeSimklHistorySync(task config.ScheduledTask) (SyncResult, error) {
	s.mu.RLock()
	historySvc := s.historyService
	simklClient := s.simklClient
	s.mu.RUnlock()

	if historySvc == nil {
		return SyncResult{}, errors.New("history service not configured")
	}
	if simklClient == nil {
		return SyncResult{}, errors.New("simkl client not configured")
	}

	simklAccountID := task.Config["simklAccountId"]
	profileID, err := s.resolveTaskProfileID(task)
	if simklAccountID == "" || profileID == "" {
		return SyncResult{}, errors.New("missing simklAccountId or profileId in task config")
	}
	if err != nil {
		return SyncResult{}, err
	}

	syncDirection := task.Config["syncDirection"]
	if syncDirection == "" {
		syncDirection = "simkl_to_local"
	}
	dryRun := task.Config["dryRun"] == "true"

	settings, err := s.configManager.Load()
	if err != nil {
		return SyncResult{}, fmt.Errorf("load settings: %w", err)
	}
	simklAccount := settings.Simkl.GetAccountByID(simklAccountID)
	if simklAccount == nil {
		return SyncResult{}, errors.New("simkl account not found")
	}
	if simklAccount.ClientID == "" || simklAccount.AccessToken == "" {
		return SyncResult{}, errors.New("simkl account not authenticated")
	}

	switch syncDirection {
	case "simkl_to_local":
		return s.syncSimklHistoryToLocal(task, simklAccount, profileID, dryRun)
	case "local_to_simkl":
		return s.syncLocalHistoryToSimkl(task, simklAccount, profileID, dryRun)
	case "bidirectional":
		importResult, err := s.syncSimklHistoryToLocal(task, simklAccount, profileID, dryRun)
		if err != nil {
			return importResult, err
		}
		exportResult, err := s.syncLocalHistoryToSimkl(task, simklAccount, profileID, dryRun)
		if err != nil {
			return combineHistorySyncResults(importResult, exportResult), err
		}
		return combineHistorySyncResults(importResult, exportResult), nil
	default:
		return SyncResult{}, fmt.Errorf("unknown sync direction: %s", syncDirection)
	}
}

// syncSimklHistoryToLocal imports watch history from Simkl into local history.
func (s *Service) syncSimklHistoryToLocal(task config.ScheduledTask, simklAccount *config.SimklAccount, profileID string, dryRun bool) (SyncResult, error) {
	s.mu.RLock()
	historySvc := s.historyService
	simklClient := s.simklClient
	s.mu.RUnlock()

	result := SyncResult{DryRun: dryRun}

	activities, err := simklClient.GetActivities(simklAccount.ClientID, simklAccount.AccessToken)
	if err != nil {
		return result, fmt.Errorf("fetch simkl activities: %w", err)
	}
	latestActivity := latestTimeInSimklActivity(activities)
	savedActivity := strings.TrimSpace(task.Config["lastSimklActivityAt"])
	if task.Config["fullSync"] == "true" || task.Config["simklHistoryImportVersion"] != "2" {
		savedActivity = ""
	}
	if savedActivity != "" && !latestActivity.IsZero() {
		savedAt, parseErr := time.Parse(time.RFC3339, savedActivity)
		if parseErr == nil && !latestActivity.After(savedAt) {
			log.Printf("[scheduler] Simkl history unchanged since %s; skipping listing calls", savedActivity)
			return result, nil
		}
	}

	var responses []*simkl.AllItemsResponse
	if savedActivity == "" {
		log.Printf("[scheduler] Performing initial Simkl history sync using sequential bucket fetches")
		for _, bucket := range []string{"movies", "shows", "anime"} {
			resp, err := simklClient.GetInitialSyncItems(simklAccount.ClientID, simklAccount.AccessToken, bucket)
			if err != nil {
				return result, fmt.Errorf("fetch simkl %s history: %w", bucket, err)
			}
			responses = append(responses, resp)
		}
	} else {
		log.Printf("[scheduler] Fetching Simkl history delta since %s", savedActivity)
		resp, err := simklClient.GetAllItemsSince(simklAccount.ClientID, simklAccount.AccessToken, savedActivity)
		if err != nil {
			return result, fmt.Errorf("fetch simkl history delta: %w", err)
		}
		responses = append(responses, resp)
	}

	watched := true
	seen := make(map[string]bool)
	var updates []models.WatchHistoryUpdate
	for _, resp := range responses {
		for _, update := range s.simklAllItemsToWatchHistory(resp, &watched) {
			key := strings.ToLower(update.MediaType) + ":" + strings.ToLower(update.ItemID)
			if seen[key] {
				continue
			}
			seen[key] = true
			if dryRun {
				result.ToAdd = append(result.ToAdd, config.DryRunItem{Name: update.Name, MediaType: update.MediaType, ID: update.ItemID})
				continue
			}
			updates = append(updates, update)
		}
	}

	if dryRun {
		result.Count = len(result.ToAdd)
		return result, nil
	}
	if len(updates) > 0 {
		imported, err := historySvc.ImportWatchHistory(profileID, updates)
		if err != nil {
			return result, fmt.Errorf("import simkl watch history: %w", err)
		}
		result.Count = imported
	}
	if !latestActivity.IsZero() {
		result.Config = map[string]string{"lastSimklActivityAt": latestActivity.UTC().Format(time.RFC3339), "simklHistoryImportVersion": "2"}
	}
	log.Printf("[scheduler] Imported %d/%d items from Simkl history", result.Count, len(updates))
	return result, nil
}

// syncLocalHistoryToSimkl exports local watch history to Simkl via POST /sync/history.
func (s *Service) syncLocalHistoryToSimkl(task config.ScheduledTask, simklAccount *config.SimklAccount, profileID string, dryRun bool) (SyncResult, error) {
	result := SyncResult{DryRun: dryRun}

	s.mu.RLock()
	historySvc := s.historyService
	simklClient := s.simklClient
	s.mu.RUnlock()

	items, err := historySvc.ListWatchHistory(profileID)
	if err != nil {
		return result, fmt.Errorf("list local history: %w", err)
	}
	items = s.enrichAndCollapseHistoryItems(items)

	// Incremental export with periodic full export (heals missed live scrobbles).
	const fullExportInterval = 6 * time.Hour
	exportKey := task.ID + ":simkl_export"
	forceFull := task.Config["fullExport"] == "true" || task.Config["fullSync"] == "true"

	s.lastFullSyncTimesMu.Lock()
	lastFull, ok := s.lastFullSyncTimes[exportKey]
	s.lastFullSyncTimesMu.Unlock()

	isFullExport := forceFull || !ok || time.Since(lastFull) >= fullExportInterval
	var since time.Time
	if !isFullExport && historySyncLastRun(task) != nil {
		since = historySyncLastRun(task).Add(-5 * time.Minute)
	}
	var removalSince time.Time
	if historySyncLastRun(task) != nil {
		removalSince = historySyncLastRun(task).Add(-5 * time.Minute)
	}

	var toSync, toRemove []models.WatchHistoryItem
	for _, item := range items {
		if !item.Watched {
			if task.Config["fullSync"] == "true" || (!removalSince.IsZero() && item.UpdatedAt.After(removalSince)) {
				toRemove = append(toRemove, item)
			}
			continue
		}
		if since.IsZero() || item.WatchedAt.After(since) {
			toSync = append(toSync, item)
		}
	}

	log.Printf("[scheduler] Found %d watched items to sync and %d recent unwatches to remove from Simkl (since %v fullExport=%v force=%v)",
		len(toSync), len(toRemove), since, isFullExport, forceFull)

	if len(toSync) == 0 && len(toRemove) == 0 {
		if isFullExport && !dryRun {
			s.lastFullSyncTimesMu.Lock()
			s.lastFullSyncTimes[exportKey] = time.Now().UTC()
			s.lastFullSyncTimesMu.Unlock()
		}
		return result, nil
	}

	if dryRun {
		for _, item := range toSync {
			result.ToAdd = append(result.ToAdd, config.DryRunItem{
				Name:      item.Name,
				MediaType: item.MediaType,
				ID:        item.ItemID,
			})
		}
		for _, item := range toRemove {
			result.ToRemove = append(result.ToRemove, config.DryRunItem{Name: item.Name, MediaType: item.MediaType, ID: item.ItemID})
		}
		result.Count = len(result.ToAdd) + len(result.ToRemove)
		return result, nil
	}

	var movies []simkl.SyncHistoryMovie
	var removeMovies []simkl.SyncHistoryMovie
	// Group episodes by show ID key so we batch seasons/episodes.
	type showKey struct {
		simkl int
		imdb  string
		tmdb  int
		tvdb  int
	}
	type epEntry struct {
		season    int
		episode   int
		watchedAt time.Time
	}
	showMap := make(map[showKey][]epEntry)
	var showOrder []showKey
	removeShowMap := make(map[showKey][]epEntry)
	var removeShowOrder []showKey
	skippedNoIDs := 0

	for _, item := range toSync {
		switch item.MediaType {
		case "movie":
			ids := extractSimklIDs(item.MediaType, item.ItemID, "", item.ExternalIDs)
			if ids.IMDB == "" && ids.TMDB == 0 && ids.TVDB == 0 && ids.Simkl == 0 {
				skippedNoIDs++
				continue
			}
			m := simkl.SyncHistoryMovie{IDs: ids}
			if !item.WatchedAt.IsZero() {
				m.WatchedAt = item.WatchedAt.UTC().Format(time.RFC3339)
			}
			if item.Name != "" {
				m.Title = item.Name
			}
			if item.Year > 0 {
				m.Year = item.Year
			}
			movies = append(movies, m)
		case "episode":
			seasonNumber, episodeNumber := simklExportEpisodeCoordinates(item)
			if seasonNumber <= 0 || episodeNumber <= 0 {
				skippedNoIDs++
				continue
			}
			ids := extractSimklIDs(item.MediaType, item.ItemID, item.SeriesID, item.ExternalIDs)
			ids, seasonNumber, episodeNumber = simkl.EpisodeIdentity(ids, seasonNumber, episodeNumber)
			if ids.IMDB == "" && ids.TMDB == 0 && ids.TVDB == 0 && ids.Simkl == 0 {
				skippedNoIDs++
				continue
			}
			key := showKey{simkl: ids.Simkl, imdb: ids.IMDB, tmdb: ids.TMDB, tvdb: ids.TVDB}
			if _, exists := showMap[key]; !exists {
				showOrder = append(showOrder, key)
			}
			showMap[key] = append(showMap[key], epEntry{
				season:    seasonNumber,
				episode:   episodeNumber,
				watchedAt: item.WatchedAt,
			})
		}
	}
	for _, item := range toRemove {
		switch item.MediaType {
		case "movie":
			ids := extractSimklIDs(item.MediaType, item.ItemID, "", item.ExternalIDs)
			if ids.IMDB == "" && ids.TMDB == 0 && ids.TVDB == 0 && ids.Simkl == 0 {
				skippedNoIDs++
				continue
			}
			removeMovies = append(removeMovies, simkl.SyncHistoryMovie{IDs: ids})
		case "episode":
			seasonNumber, episodeNumber := simklExportEpisodeCoordinates(item)
			if seasonNumber <= 0 || episodeNumber <= 0 {
				skippedNoIDs++
				continue
			}
			ids := extractSimklIDs(item.MediaType, item.ItemID, item.SeriesID, item.ExternalIDs)
			ids, seasonNumber, episodeNumber = simkl.EpisodeIdentity(ids, seasonNumber, episodeNumber)
			if ids.IMDB == "" && ids.TMDB == 0 && ids.TVDB == 0 && ids.Simkl == 0 {
				skippedNoIDs++
				continue
			}
			key := showKey{simkl: ids.Simkl, imdb: ids.IMDB, tmdb: ids.TMDB, tvdb: ids.TVDB}
			if _, exists := removeShowMap[key]; !exists {
				removeShowOrder = append(removeShowOrder, key)
			}
			removeShowMap[key] = append(removeShowMap[key], epEntry{season: seasonNumber, episode: episodeNumber})
		}
	}

	var shows []simkl.SyncHistoryShow
	for _, key := range showOrder {
		eps := showMap[key]
		seasonMap := make(map[int][]simkl.SyncHistoryEpisode)
		for _, ep := range eps {
			entry := simkl.SyncHistoryEpisode{Number: ep.episode}
			if !ep.watchedAt.IsZero() {
				entry.WatchedAt = ep.watchedAt.UTC().Format(time.RFC3339)
			}
			seasonMap[ep.season] = append(seasonMap[ep.season], entry)
		}
		var seasons []simkl.SyncHistorySeason
		for sNum, sEps := range seasonMap {
			seasons = append(seasons, simkl.SyncHistorySeason{Number: sNum, Episodes: sEps})
		}
		shows = append(shows, simkl.SyncHistoryShow{
			IDs:     simkl.IDs{Simkl: key.simkl, IMDB: key.imdb, TMDB: key.tmdb, TVDB: key.tvdb},
			Seasons: seasons,
		})
	}
	var removeShows []simkl.SyncHistoryShow
	for _, key := range removeShowOrder {
		seasonMap := make(map[int][]simkl.SyncHistoryEpisode)
		for _, ep := range removeShowMap[key] {
			seasonMap[ep.season] = append(seasonMap[ep.season], simkl.SyncHistoryEpisode{Number: ep.episode})
		}
		var seasons []simkl.SyncHistorySeason
		for season, episodes := range seasonMap {
			seasons = append(seasons, simkl.SyncHistorySeason{Number: season, Episodes: episodes})
		}
		removeShows = append(removeShows, simkl.SyncHistoryShow{
			IDs: simkl.IDs{Simkl: key.simkl, IMDB: key.imdb, TMDB: key.tmdb, TVDB: key.tvdb}, Seasons: seasons,
		})
	}

	syncCount := 0
	const batchSize = 50

	for i := 0; i < len(movies); i += batchSize {
		end := i + batchSize
		if end > len(movies) {
			end = len(movies)
		}
		req := simkl.SyncHistoryRequest{Movies: movies[i:end]}
		resp, err := simklClient.SyncHistorySafe(simklAccount.ClientID, simklAccount.AccessToken, req)
		if err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("Simkl sync movies batch: %w", err)
		}
		syncCount += end - i - len(resp.NotFound.Movies)
	}

	for i := 0; i < len(shows); i += batchSize {
		end := i + batchSize
		if end > len(shows) {
			end = len(shows)
		}
		batch := shows[i:end]
		req := simkl.SyncHistoryRequest{Shows: batch}
		// SyncHistorySafe undoes accidental full-series completes when every
		// requested episode for a show is not_found (Columbo/DBZ numbering).
		resp, err := simklClient.SyncHistorySafe(simklAccount.ClientID, simklAccount.AccessToken, req)
		if err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("Simkl sync shows batch: %w", err)
		}
		intended := 0
		for _, show := range batch {
			for _, season := range show.Seasons {
				intended += len(season.Episodes)
			}
		}
		notFoundEps := 0
		if resp != nil {
			for _, nf := range resp.NotFound.Episodes {
				for _, season := range nf.Seasons {
					notFoundEps += len(season.Episodes)
				}
			}
		}
		// Prefer Simkl's added count when present; fall back to intended−not_found.
		if resp != nil && resp.Added.Episodes > 0 && notFoundEps == 0 {
			syncCount += resp.Added.Episodes
		} else {
			matched := intended - notFoundEps
			if matched > 0 {
				syncCount += matched
			}
		}
	}
	for i := 0; i < len(removeMovies); i += batchSize {
		end := i + batchSize
		if end > len(removeMovies) {
			end = len(removeMovies)
		}
		if err := simklClient.RemoveFromHistory(simklAccount.ClientID, simklAccount.AccessToken, simkl.SyncHistoryRequest{Movies: removeMovies[i:end]}); err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("Simkl remove movies batch: %w", err)
		}
		syncCount += end - i
	}
	for i := 0; i < len(removeShows); i += batchSize {
		end := i + batchSize
		if end > len(removeShows) {
			end = len(removeShows)
		}
		if err := simklClient.RemoveFromHistory(simklAccount.ClientID, simklAccount.AccessToken, simkl.SyncHistoryRequest{Shows: removeShows[i:end]}); err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("Simkl remove shows batch: %w", err)
		}
		for _, show := range removeShows[i:end] {
			for _, season := range show.Seasons {
				syncCount += len(season.Episodes)
			}
		}
	}

	result.Count = syncCount
	log.Printf("[scheduler] Synced %d items to Simkl (skippedNoIDs=%d fullExport=%v)",
		syncCount, skippedNoIDs, isFullExport)

	if isFullExport {
		s.lastFullSyncTimesMu.Lock()
		s.lastFullSyncTimes[exportKey] = time.Now().UTC()
		s.lastFullSyncTimesMu.Unlock()
		log.Printf("[scheduler] Full Simkl history export complete, next full export in %v", fullExportInterval)
	}

	return result, nil
}
