package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"novastream/config"
	"novastream/internal/mediaidentity"
	"novastream/models"
	"novastream/services/mdblist"
	"strconv"
	"time"
)

// executeMDBListHistorySync syncs watch history between MDBList and local.
func (s *Service) executeMDBListHistorySync(task config.ScheduledTask) (SyncResult, error) {
	s.mu.RLock()
	historySvc := s.historyService
	s.mu.RUnlock()

	if historySvc == nil {
		return SyncResult{}, errors.New("history service not configured")
	}

	mdblistAccountID := task.Config["mdblistAccountId"]
	profileID, err := s.resolveTaskProfileID(task)

	if mdblistAccountID == "" || profileID == "" {
		return SyncResult{}, errors.New("missing mdblistAccountId or profileId in task config")
	}
	if err != nil {
		return SyncResult{}, err
	}

	syncDirection := task.Config["syncDirection"]
	if syncDirection == "" {
		syncDirection = "mdblist_to_local"
	}
	dryRun := task.Config["dryRun"] == "true"

	// Load settings to get MDBList account
	settings, err := s.configManager.Load()
	if err != nil {
		return SyncResult{}, fmt.Errorf("load settings: %w", err)
	}

	mdblistAccount := settings.MDBList.GetAccountByID(mdblistAccountID)
	if mdblistAccount == nil {
		return SyncResult{}, errors.New("MDBList account not found")
	}

	if mdblistAccount.APIKey == "" {
		return SyncResult{}, errors.New("MDBList account has no API key")
	}
	log.Printf("[scheduler] MDBList history configuration task=%q account=%q accountName=%q profile=%q configuredProfile=%q direction=%q dryRun=%v fullSync=%v fullExport=%v",
		task.ID, mdblistAccount.ID, mdblistAccount.Name, profileID, task.Config["profileId"], syncDirection, dryRun,
		task.Config["fullSync"] == "true", task.Config["fullExport"] == "true")

	switch syncDirection {
	case "mdblist_to_local":
		return s.syncMDBListHistoryToLocal(task, mdblistAccount, profileID, dryRun)
	case "local_to_mdblist":
		return s.syncLocalHistoryToMDBList(task, mdblistAccount, profileID, dryRun)
	case "bidirectional":
		// Import first, then export
		importResult, err := s.syncMDBListHistoryToLocal(task, mdblistAccount, profileID, dryRun)
		if err != nil {
			return importResult, err
		}
		exportResult, err := s.syncLocalHistoryToMDBList(task, mdblistAccount, profileID, dryRun)
		if err != nil {
			return combineHistorySyncResults(importResult, exportResult), err
		}
		return combineHistorySyncResults(importResult, exportResult), nil
	default:
		return SyncResult{}, fmt.Errorf("unknown sync direction: %s", syncDirection)
	}
}

// syncMDBListHistoryToLocal imports watch history from MDBList into local.
func (s *Service) syncMDBListHistoryToLocal(task config.ScheduledTask, account *config.MDBListAccount, profileID string, dryRun bool) (SyncResult, error) {
	result := SyncResult{DryRun: dryRun}

	s.mu.RLock()
	historySvc := s.historyService
	s.mu.RUnlock()

	// Determine incremental cursor
	var since string
	if historySyncLastRun(task) != nil {
		since = historySyncLastRun(task).Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	}
	mode := "full"
	if since != "" {
		mode = "incremental"
	}
	lastRun := ""
	if task.LastRunAt != nil {
		lastRun = task.LastRunAt.UTC().Format(time.RFC3339)
	}
	log.Printf("[scheduler] MDBList history import start task=%q account=%q profile=%q mode=%s since=%q previousRun=%q previousStatus=%q dryRun=%v",
		task.ID, account.ID, profileID, mode, since, lastRun, task.LastStatus, dryRun)

	// Fetch watched history from MDBList API with pagination
	apiKey := account.APIKey
	var allMovies []json.RawMessage
	var allEpisodes []json.RawMessage
	offset := 0
	limit := 500
	pages := 0

	for {
		url := fmt.Sprintf("https://api.mdblist.com/sync/watched?apikey=%s&limit=%d&offset=%d", apiKey, limit, offset)
		if since != "" {
			url += "&since=" + since
		}

		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "mediastorm/1.0")
		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return result, fmt.Errorf("fetch MDBList history: %w", mdblistRequestError(err))
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return result, fmt.Errorf("MDBList history request failed (HTTP %d, offset %d)", resp.StatusCode, offset)
		}

		var page *struct {
			Movies     []json.RawMessage `json:"movies"`
			Episodes   []json.RawMessage `json:"episodes"`
			Shows      []json.RawMessage `json:"shows"`
			Seasons    []json.RawMessage `json:"seasons"`
			Pagination struct {
				HasMore       bool `json:"has_more"`
				TotalMovies   *int `json:"total_movies"`
				TotalEpisodes *int `json:"total_episodes"`
				Limit         int  `json:"limit"`
			} `json:"pagination"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return result, fmt.Errorf("decode MDBList history: %w", err)
		}
		resp.Body.Close()
		if page == nil {
			return result, errors.New("MDBList returned null instead of history")
		}
		pages++
		log.Printf("[scheduler] MDBList history page task=%q page=%d offset=%d requestedLimit=%d responseLimit=%d movies=%d episodes=%d shows=%d seasons=%d hasMore=%v apiTotalMovies=%s apiTotalEpisodes=%s",
			task.ID, pages, offset, limit, page.Pagination.Limit, len(page.Movies), len(page.Episodes), len(page.Shows), len(page.Seasons),
			page.Pagination.HasMore, mdblistReportedTotal(page.Pagination.TotalMovies), mdblistReportedTotal(page.Pagination.TotalEpisodes))

		allMovies = append(allMovies, page.Movies...)
		allEpisodes = append(allEpisodes, page.Episodes...)

		if !page.Pagination.HasMore {
			break
		}
		offset += limit
	}

	log.Printf("[scheduler] Fetched %d movies + %d episodes from MDBList watch history (task=%q profile=%q mode=%s pages=%d)",
		len(allMovies), len(allEpisodes), task.ID, profileID, mode, pages)
	if len(allMovies) == 0 && since != "" {
		log.Printf("[scheduler] MDBList history import task=%q: zero movies returned with since=%q; Full sync requests older history without this filter", task.ID, since)
	}

	// Convert to WatchHistoryUpdate items
	watched := true
	var updates []models.WatchHistoryUpdate
	parsedMovies, parsedEpisodes := 0, 0
	skippedMalformed, skippedUnwatched, skippedIdentity, skippedCoordinates := 0, 0, 0, 0
	invalidTimestamps := 0

	// Parse movies
	for _, raw := range allMovies {
		var m struct {
			LastWatchedAt string `json:"last_watched_at"`
			Movie         struct {
				Title string `json:"title"`
				Year  int    `json:"year"`
				IDs   struct {
					IMDB string `json:"imdb"`
					TMDB int    `json:"tmdb"`
					TVDB int    `json:"tvdb"`
				} `json:"ids"`
			} `json:"movie"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			skippedMalformed++
			continue
		}

		if m.LastWatchedAt == "" {
			skippedUnwatched++
			continue
		}
		watchedAt, parseErr := time.Parse(time.RFC3339, m.LastWatchedAt)
		if parseErr != nil {
			invalidTimestamps++
		}
		extIDs := make(map[string]string)
		var itemID string
		if m.Movie.IDs.IMDB != "" {
			extIDs["imdb"] = m.Movie.IDs.IMDB
			itemID = m.Movie.IDs.IMDB
		}
		if m.Movie.IDs.TMDB != 0 {
			extIDs["tmdb"] = strconv.Itoa(m.Movie.IDs.TMDB)
			if itemID == "" {
				itemID = strconv.Itoa(m.Movie.IDs.TMDB)
			}
		}
		if itemID == "" {
			skippedIdentity++
			continue
		}
		parsedMovies++

		if dryRun {
			result.ToAdd = append(result.ToAdd, config.DryRunItem{
				Name:      m.Movie.Title,
				MediaType: "movie",
				ID:        itemID,
			})
			continue
		}

		updates = append(updates, models.WatchHistoryUpdate{
			MediaType:   "movie",
			ItemID:      itemID,
			Name:        m.Movie.Title,
			Year:        m.Movie.Year,
			Watched:     &watched,
			WatchedAt:   watchedAt,
			ExternalIDs: extIDs,
		})
	}

	// Parse episodes
	for _, raw := range allEpisodes {
		var e struct {
			LastWatchedAt string `json:"last_watched_at"`
			Episode       struct {
				Season int    `json:"season"`
				Number int    `json:"number"`
				Name   string `json:"name"`
				Show   struct {
					Title string `json:"title"`
					Year  int    `json:"year"`
					IDs   struct {
						IMDB string `json:"imdb"`
						TMDB int    `json:"tmdb"`
						TVDB int    `json:"tvdb"`
					} `json:"ids"`
				} `json:"show"`
			} `json:"episode"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			skippedMalformed++
			continue
		}

		if e.LastWatchedAt == "" {
			skippedUnwatched++
			continue
		}
		if e.Episode.Season < 0 || e.Episode.Number <= 0 {
			skippedCoordinates++
			continue
		}
		watchedAt, parseErr := time.Parse(time.RFC3339, e.LastWatchedAt)
		if parseErr != nil {
			invalidTimestamps++
		}
		extIDs := make(map[string]string)
		var seriesID string

		show := e.Episode.Show
		if show.IDs.TVDB != 0 {
			extIDs["tvdb"] = strconv.Itoa(show.IDs.TVDB)
			seriesID = fmt.Sprintf("tvdb:series:%d", show.IDs.TVDB)
		}
		if show.IDs.TMDB != 0 {
			extIDs["tmdb"] = strconv.Itoa(show.IDs.TMDB)
			if seriesID == "" {
				seriesID = fmt.Sprintf("tmdb:tv:%d", show.IDs.TMDB)
			}
		}
		if show.IDs.IMDB != "" {
			extIDs["imdb"] = show.IDs.IMDB
		}
		if seriesID == "" {
			skippedIdentity++
			continue
		}
		parsedEpisodes++

		absoluteEpisode := 0
		if e.Episode.Number >= 1000 {
			absoluteEpisode = e.Episode.Number
		}
		localSeason, localEpisode, localAbsolute, episodeTitle := s.canonicalizeProviderEpisode(
			"mdblist", extIDs, nil, e.Episode.Season, e.Episode.Number, absoluteEpisode, e.Episode.Name,
		)
		if localAbsolute > 0 {
			extIDs["absoluteEpisode"] = strconv.Itoa(localAbsolute)
		}
		itemID := fmt.Sprintf("%s:s%02de%02d", seriesID, localSeason, localEpisode)

		if dryRun {
			result.ToAdd = append(result.ToAdd, config.DryRunItem{
				Name:      fmt.Sprintf("%s S%02dE%02d", show.Title, localSeason, localEpisode),
				MediaType: "episode",
				ID:        itemID,
			})
			continue
		}

		updates = append(updates, models.WatchHistoryUpdate{
			MediaType:     "episode",
			ItemID:        itemID,
			Name:          episodeTitle,
			Watched:       &watched,
			WatchedAt:     watchedAt,
			ExternalIDs:   extIDs,
			SeasonNumber:  localSeason,
			EpisodeNumber: localEpisode,
			SeriesID:      seriesID,
			SeriesName:    show.Title,
		})
	}

	log.Printf("[scheduler] MDBList history parsed task=%q profile=%q movies=%d episodes=%d skippedMalformed=%d skippedUnwatched=%d skippedIdentity=%d skippedCoordinates=%d invalidTimestamps=%d dryRun=%v",
		task.ID, profileID, parsedMovies, parsedEpisodes, skippedMalformed, skippedUnwatched, skippedIdentity, skippedCoordinates, invalidTimestamps, dryRun)
	if dryRun {
		result.Count = len(result.ToAdd)
		return result, nil
	}

	if len(updates) > 0 {
		imported, err := historySvc.ImportWatchHistory(profileID, updates)
		if err != nil {
			return result, fmt.Errorf("import watch history: %w", err)
		}
		result.Count = imported
	}
	log.Printf("[scheduler] Imported %d/%d items from MDBList history (task=%q profile=%q movieCandidates=%d episodeCandidates=%d)",
		result.Count, len(updates), task.ID, profileID, parsedMovies, parsedEpisodes)

	return result, nil
}

// syncLocalHistoryToMDBList exports local watch history to MDBList.
func (s *Service) syncLocalHistoryToMDBList(task config.ScheduledTask, account *config.MDBListAccount, profileID string, dryRun bool) (SyncResult, error) {
	result := SyncResult{DryRun: dryRun}

	s.mu.RLock()
	historySvc := s.historyService
	s.mu.RUnlock()

	// Get all local watch history for this profile
	items, err := historySvc.ListWatchHistory(profileID)
	if err != nil {
		return result, fmt.Errorf("list local history: %w", err)
	}
	items = s.enrichAndCollapseHistoryItems(items)

	// Incremental export uses LastRunAt - 5min. That misses older watches that
	// never landed on MDBList (sparse-ID scrobble failures). Periodically do a
	// full export; MDBList /sync/watched is idempotent so re-export is safe.
	// Force with task config fullExport=true for one-shot healing.
	const fullExportInterval = 6 * time.Hour
	exportKey := task.ID + ":mdblist_export"
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
		if item.Watched {
			if since.IsZero() || item.WatchedAt.After(since) {
				toSync = append(toSync, item)
			}
		} else if task.Config["fullSync"] == "true" || (!removalSince.IsZero() && item.UpdatedAt.After(removalSince)) {
			toRemove = append(toRemove, item)
		}
	}

	log.Printf("[scheduler] Found %d watched items to sync and %d recent unwatches to remove from MDBList (since %v fullExport=%v force=%v)",
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

	// Build MDBList sync requests batched by type.
	// MDBList format: movies=[{"ids":{...}, "watched_at":"..."}],
	//                 shows=[{"ids":{...}, "seasons":[{"number":N, "episodes":[{"number":N, "watched_at":"..."}]}]}]
	apiKey := account.APIKey

	// Collect movies
	skippedNoIDs := 0
	var moviePayloads []map[string]interface{}
	for _, item := range toSync {
		if item.MediaType != "movie" {
			continue
		}
		ids := extractMDBListIDs(item.ExternalIDs)
		if ids.imdb == "" && ids.tmdb == 0 {
			skippedNoIDs++
			continue
		}
		m := map[string]interface{}{
			"ids": formatMDBListIDsMap(ids),
		}
		if !item.WatchedAt.IsZero() {
			m["watched_at"] = item.WatchedAt.UTC().Format(time.RFC3339)
		}
		moviePayloads = append(moviePayloads, m)
	}

	// Collect episodes grouped by show
	type showKey struct {
		imdb string
		tmdb int
	}
	type epEntry struct {
		season    int
		episode   int
		watchedAt time.Time
		absolute  int // distinct absoluteEpisode for hybrid retry; 0 if N/A
	}
	showMap := make(map[showKey][]epEntry)
	showOrder := make([]showKey, 0)
	for _, item := range toSync {
		if item.MediaType != "episode" {
			continue
		}
		// Enrich sparse rows that only store titleId (e.g. tmdb:tv:82782)
		// so bidirectional history export can heal MDBList after missed scrobbles.
		ids := extractMDBListIDs(mediaidentity.EnrichShowExternalIDs(item.SeriesID, item.ItemID, item.ExternalIDs))
		if ids.imdb == "" && ids.tmdb == 0 {
			skippedNoIDs++
			continue
		}
		key := showKey{imdb: ids.imdb, tmdb: ids.tmdb}
		if _, exists := showMap[key]; !exists {
			showOrder = append(showOrder, key)
		}
		// Pure seasonal first. Hybrid absolute retry happens after not_found
		// responses (absoluteEpisode is common on non-anime shows too).
		showMap[key] = append(showMap[key], epEntry{
			season:    item.SeasonNumber,
			episode:   item.EpisodeNumber,
			watchedAt: item.WatchedAt,
			absolute:  mdblist.HybridEpisodeNumber(item.EpisodeNumber, item.ExternalIDs),
		})
	}

	var showPayloads []map[string]interface{}
	for _, key := range showOrder {
		eps := showMap[key]
		ids := mdblistIDs{imdb: key.imdb, tmdb: key.tmdb}

		// Group episodes by season
		seasonMap := make(map[int][]map[string]interface{})
		for _, ep := range eps {
			epObj := map[string]interface{}{
				"number": ep.episode,
			}
			if !ep.watchedAt.IsZero() {
				epObj["watched_at"] = ep.watchedAt.UTC().Format(time.RFC3339)
			}
			seasonMap[ep.season] = append(seasonMap[ep.season], epObj)
		}

		var seasons []map[string]interface{}
		for sNum, sEps := range seasonMap {
			seasons = append(seasons, map[string]interface{}{
				"number":   sNum,
				"episodes": sEps,
			})
		}

		showPayloads = append(showPayloads, map[string]interface{}{
			"ids":     formatMDBListIDsMap(ids),
			"seasons": seasons,
		})
	}

	// Send batched request
	syncCount := 0
	batchSize := 100

	// Sync movies in batches
	for i := 0; i < len(moviePayloads); i += batchSize {
		end := i + batchSize
		if end > len(moviePayloads) {
			end = len(moviePayloads)
		}
		batch := map[string]interface{}{
			"movies": moviePayloads[i:end],
		}
		body, _ := json.Marshal(batch)
		respBody, err := postToMDBListBody(apiKey, "/sync/watched", string(body))
		if err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("MDBList sync movies batch: %w", err)
		}
		var response struct {
			NotFound struct {
				Movies []json.RawMessage `json:"movies"`
			} `json:"not_found"`
		}
		if len(respBody) > 0 {
			if err := json.Unmarshal(respBody, &response); err != nil {
				result.Count = syncCount
				return result, fmt.Errorf("decode MDBList movie sync response: %w", err)
			}
		}
		syncCount += end - i - len(response.NotFound.Movies)
	}

	// Sync shows in batches (seasonal numbering). Collect not_found for hybrid retry.
	type hybridRetry struct {
		key       showKey
		season    int
		absolute  int
		watchedAt time.Time
	}
	var hybridRetries []hybridRetry

	for i := 0; i < len(showPayloads); i += batchSize {
		end := i + batchSize
		if end > len(showPayloads) {
			end = len(showPayloads)
		}
		batch := map[string]interface{}{
			"shows": showPayloads[i:end],
		}
		body, _ := json.Marshal(batch)
		respBody, err := postToMDBListBody(apiKey, "/sync/watched", string(body))
		if err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("MDBList sync shows batch: %w", err)
		}
		notFound := parseMDBListNotFoundEpisodes(respBody)
		accepted := 0
		for _, show := range showPayloads[i:end] {
			for _, season := range show["seasons"].([]map[string]interface{}) {
				accepted += len(season["episodes"].([]map[string]interface{}))
			}
		}
		accepted -= len(notFound)
		if accepted > 0 {
			syncCount += accepted
		}
		// Map not_found seasonal keys back to absolute candidates (scoped by show ids).
		for _, key := range showOrder {
			for _, ep := range showMap[key] {
				if ep.absolute <= 0 {
					continue
				}
				nfKey := mdblistNotFoundKey(key.imdb, key.tmdb, ep.season, ep.episode)
				if !notFound[nfKey] {
					continue
				}
				hybridRetries = append(hybridRetries, hybridRetry{
					key:       key,
					season:    ep.season,
					absolute:  ep.absolute,
					watchedAt: ep.watchedAt,
				})
			}
		}
	}

	if len(hybridRetries) > 0 {
		log.Printf("[scheduler] MDBList hybrid absolute retry for %d not_found episodes", len(hybridRetries))
		// Group hybrid retries by show
		hybridByShow := make(map[showKey][]hybridRetry)
		var hybridOrder []showKey
		for _, hr := range hybridRetries {
			if _, ok := hybridByShow[hr.key]; !ok {
				hybridOrder = append(hybridOrder, hr.key)
			}
			hybridByShow[hr.key] = append(hybridByShow[hr.key], hr)
		}
		var hybridPayloads []map[string]interface{}
		for _, key := range hybridOrder {
			seasonMap := make(map[int][]map[string]interface{})
			for _, hr := range hybridByShow[key] {
				epObj := map[string]interface{}{"number": hr.absolute}
				if !hr.watchedAt.IsZero() {
					epObj["watched_at"] = hr.watchedAt.UTC().Format(time.RFC3339)
				}
				seasonMap[hr.season] = append(seasonMap[hr.season], epObj)
			}
			var seasons []map[string]interface{}
			for sNum, sEps := range seasonMap {
				seasons = append(seasons, map[string]interface{}{"number": sNum, "episodes": sEps})
			}
			hybridPayloads = append(hybridPayloads, map[string]interface{}{
				"ids":     formatMDBListIDsMap(mdblistIDs{imdb: key.imdb, tmdb: key.tmdb}),
				"seasons": seasons,
			})
		}
		for i := 0; i < len(hybridPayloads); i += batchSize {
			end := i + batchSize
			if end > len(hybridPayloads) {
				end = len(hybridPayloads)
			}
			batch := map[string]interface{}{"shows": hybridPayloads[i:end]}
			body, _ := json.Marshal(batch)
			respBody, err := postToMDBListBody(apiKey, "/sync/watched", string(body))
			if err != nil {
				result.Count = syncCount
				return result, fmt.Errorf("MDBList hybrid sync batch: %w", err)
			}
			nf := parseMDBListNotFoundEpisodes(respBody)
			count := 0
			for _, show := range hybridPayloads[i:end] {
				for _, season := range show["seasons"].([]map[string]interface{}) {
					count += len(season["episodes"].([]map[string]interface{}))
				}
			}
			count -= len(nf)
			if count > 0 {
				syncCount += count
			}
		}
	}

	removeMovies, removeShows, removeSkipped := buildMDBListRemovalPayload(toRemove)
	skippedNoIDs += removeSkipped
	for i := 0; i < len(removeMovies); i += batchSize {
		end := i + batchSize
		if end > len(removeMovies) {
			end = len(removeMovies)
		}
		body, _ := json.Marshal(map[string]interface{}{"movies": removeMovies[i:end]})
		if err := postToMDBList(apiKey, "/sync/watched/remove", string(body)); err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("MDBList remove movies batch: %w", err)
		}
		syncCount += end - i
	}
	for i := 0; i < len(removeShows); i += batchSize {
		end := i + batchSize
		if end > len(removeShows) {
			end = len(removeShows)
		}
		body, _ := json.Marshal(map[string]interface{}{"shows": removeShows[i:end]})
		if err := postToMDBList(apiKey, "/sync/watched/remove", string(body)); err != nil {
			result.Count = syncCount
			return result, fmt.Errorf("MDBList remove shows batch: %w", err)
		}
		for _, show := range removeShows[i:end] {
			for _, season := range show["seasons"].([]map[string]interface{}) {
				syncCount += len(season["episodes"].([]map[string]interface{}))
			}
		}
	}

	result.Count = syncCount
	log.Printf("[scheduler] Synced %d/%d changes to MDBList (skippedNoIDs=%d fullExport=%v)",
		syncCount, len(toSync)+len(toRemove), skippedNoIDs, isFullExport)

	if isFullExport && !dryRun {
		s.lastFullSyncTimesMu.Lock()
		s.lastFullSyncTimes[exportKey] = time.Now().UTC()
		s.lastFullSyncTimesMu.Unlock()
		log.Printf("[scheduler] Full MDBList history export complete, next full export in %v", fullExportInterval)
	}

	return result, nil
}

// API keys live in MDBList query strings; do not expose the URL in task errors.
func mdblistRequestError(err error) error {
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		return requestErr.Err
	}
	return err
}

func mdblistReportedTotal(total *int) string {
	if total == nil {
		return "unknown"
	}
	return strconv.Itoa(*total)
}
