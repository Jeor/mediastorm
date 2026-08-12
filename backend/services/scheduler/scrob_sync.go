package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"novastream/config"
	"novastream/internal/mediaidentity"
	"novastream/models"
	"novastream/services/scrob"
)

// executeScrobHistorySync synchronizes completed watch state with a self-hosted
// Scrob user. Scrob exposes API-key reads, while writes currently require a JWT
// obtained from its password login endpoint.
func (s *Service) executeScrobHistorySync(task config.ScheduledTask) (SyncResult, error) {
	s.mu.RLock()
	historySvc := s.historyService
	client := s.scrobClient
	s.mu.RUnlock()
	if historySvc == nil {
		return SyncResult{}, errors.New("history service not configured")
	}
	if client == nil {
		return SyncResult{}, errors.New("Scrob client not configured")
	}

	profileID, err := s.resolveTaskProfileID(task)
	if err != nil {
		return SyncResult{}, err
	}
	accountID := strings.TrimSpace(task.Config["scrobAccountId"])
	if accountID == "" || profileID == "" {
		return SyncResult{}, errors.New("missing scrobAccountId or profileId in task config")
	}

	settings, err := s.configManager.Load()
	if err != nil {
		return SyncResult{}, fmt.Errorf("load settings: %w", err)
	}
	account := settings.Scrob.GetAccountByID(accountID)
	if account == nil {
		return SyncResult{}, errors.New("Scrob account not found")
	}
	if strings.TrimSpace(account.BaseURL) == "" || strings.TrimSpace(account.APIKey) == "" {
		return SyncResult{}, errors.New("Scrob account requires a base URL and API key")
	}

	direction := task.Config["syncDirection"]
	if direction == "" {
		direction = "scrob_to_local"
	}
	dryRun := task.Config["dryRun"] == "true"
	switch direction {
	case "scrob_to_local":
		return s.syncScrobHistoryToLocal(account, profileID, dryRun)
	case "local_to_scrob":
		return s.syncLocalHistoryToScrob(task, account, profileID, dryRun)
	case "bidirectional":
		in, err := s.syncScrobHistoryToLocal(account, profileID, dryRun)
		if err != nil {
			return in, err
		}
		out, err := s.syncLocalHistoryToScrob(task, account, profileID, dryRun)
		if err != nil {
			return out, err
		}
		return SyncResult{Count: in.Count + out.Count, DryRun: dryRun, ToAdd: append(in.ToAdd, out.ToAdd...), ToRemove: append(in.ToRemove, out.ToRemove...)}, nil
	default:
		return SyncResult{}, fmt.Errorf("unknown sync direction: %s", direction)
	}
}

func (s *Service) syncScrobHistoryToLocal(account *config.ScrobAccount, profileID string, dryRun bool) (SyncResult, error) {
	result := SyncResult{DryRun: dryRun}
	s.mu.RLock()
	client, historySvc := s.scrobClient, s.historyService
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	events, err := client.GetHistory(ctx, account.BaseURL, account.APIKey)
	if err != nil {
		return result, fmt.Errorf("fetch Scrob history: %w", err)
	}
	remoteProgress, err := client.GetContinueWatching(ctx, account.BaseURL, account.APIKey)
	if err != nil {
		return result, fmt.Errorf("fetch Scrob continue watching: %w", err)
	}
	localProgress, err := historySvc.ListPlaybackProgress(profileID)
	if err != nil {
		return result, fmt.Errorf("list local playback progress: %w", err)
	}
	localHistory, err := historySvc.ListWatchHistory(profileID)
	if err != nil {
		return result, fmt.Errorf("list local watch history: %w", err)
	}
	localProgressByKey := make(map[string]models.PlaybackProgress)
	for _, progress := range localProgress {
		if key := localPlaybackProgressScrobKey(progress); key != "" {
			existing, ok := localProgressByKey[key]
			if !ok || progress.UpdatedAt.After(existing.UpdatedAt) {
				localProgressByKey[key] = progress
			}
		}
	}
	localHistoryByKey := make(map[string]time.Time)
	for _, item := range localHistory {
		_, key, ok := localItemToScrob(item)
		if !ok {
			continue
		}
		changedAt := scrobHistoryItemChangedAt(item)
		if changedAt.After(localHistoryByKey[key]) {
			localHistoryByKey[key] = changedAt
		}
	}

	watched := true
	seen := make(map[string]struct{})
	updates := make([]models.WatchHistoryUpdate, 0, len(events))
	for _, event := range events {
		update := scrobEventToUpdate(event, &watched)
		if update == nil {
			continue
		}
		key := strings.ToLower(update.MediaType + ":" + update.ItemID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if dryRun {
			result.ToAdd = append(result.ToAdd, config.DryRunItem{Name: update.Name, MediaType: update.MediaType, ID: update.ItemID})
		} else {
			updates = append(updates, *update)
		}
	}
	if !dryRun && len(updates) > 0 {
		result.Count, err = historySvc.ImportWatchHistory(profileID, updates)
		if err != nil {
			return result, fmt.Errorf("import Scrob watch history: %w", err)
		}
	}
	partialImported := 0
	for _, remote := range remoteProgress {
		update, key, ok := scrobProgressToUpdate(remote)
		if !ok {
			continue
		}
		if local, exists := localProgressByKey[key]; exists {
			// Local wins exact ties. A missing remote timestamp is never allowed
			// to overwrite known local progress.
			if !scrobRemoteProgressIsNewer(remote.UpdatedAt, local.UpdatedAt) {
				continue
			}
		}
		if localChangedAt, exists := localHistoryByKey[key]; exists &&
			!scrobRemoteProgressIsNewer(remote.UpdatedAt, localChangedAt) {
			continue
		}
		if dryRun {
			result.ToAdd = append(result.ToAdd, config.DryRunItem{Name: scrobProgressName(remote), MediaType: update.MediaType, ID: update.ItemID})
			partialImported++
			continue
		}
		if _, err := historySvc.ImportPlaybackProgress(profileID, update); err != nil {
			return result, fmt.Errorf("import Scrob playback progress for %s: %w", update.ItemID, err)
		}
		partialImported++
	}
	if dryRun {
		result.Count = len(result.ToAdd)
		return result, nil
	}
	result.Count += partialImported
	log.Printf("[scheduler] Imported %d completed history items and %d playback positions from Scrob", result.Count-partialImported, partialImported)
	return result, nil
}

func scrobProgressToUpdate(progress scrob.PlaybackProgressEvent) (models.PlaybackProgressUpdate, string, bool) {
	percent := progress.ProgressPercent * 100
	if percent <= 5 || percent >= 90 {
		return models.PlaybackProgressUpdate{}, "", false
	}
	m := progress.Media
	duration := float64(max(0, m.Runtime) * 60)
	position := float64(max(0, progress.ProgressSeconds))
	if duration > 0 && position <= 0 {
		position = duration * progress.ProgressPercent
	}
	updated := models.PlaybackProgressUpdate{
		MediaType: m.Type, Position: position, Duration: duration, PercentWatched: percent,
		Timestamp: progress.UpdatedAt, IsPaused: true,
	}
	switch m.Type {
	case "movie":
		if m.TMDBID <= 0 {
			return models.PlaybackProgressUpdate{}, "", false
		}
		id := strconv.Itoa(m.TMDBID)
		updated.ItemID = "tmdb:movie:" + id
		updated.MovieName = m.Title
		updated.ExternalIDs = map[string]string{"tmdb": id}
		if len(m.ReleaseDate) >= 4 {
			updated.Year, _ = strconv.Atoi(m.ReleaseDate[:4])
		}
		return updated, fmt.Sprintf("movie:%d", m.TMDBID), true
	case "episode":
		if m.ShowTMDBID <= 0 || m.SeasonNumber < 0 || m.EpisodeNumber <= 0 {
			return models.PlaybackProgressUpdate{}, "", false
		}
		showID := strconv.Itoa(m.ShowTMDBID)
		seriesID := "tmdb:tv:" + showID
		updated.ItemID = fmt.Sprintf("%s:s%02de%02d", seriesID, m.SeasonNumber, m.EpisodeNumber)
		updated.SeriesID = seriesID
		updated.SeriesName = m.ShowTitle
		updated.EpisodeName = m.Title
		updated.SeasonNumber = m.SeasonNumber
		updated.EpisodeNumber = m.EpisodeNumber
		updated.ExternalIDs = map[string]string{"tmdb": showID}
		if m.TMDBID > 0 {
			updated.ExternalIDs["episodeTmdb"] = strconv.Itoa(m.TMDBID)
		}
		if m.ShowTVDBID > 0 {
			updated.ExternalIDs["tvdb"] = strconv.Itoa(m.ShowTVDBID)
		}
		return updated, fmt.Sprintf("episode:%d:%d:%d", m.ShowTMDBID, m.SeasonNumber, m.EpisodeNumber), true
	default:
		return models.PlaybackProgressUpdate{}, "", false
	}
}

func scrobProgressName(progress scrob.PlaybackProgressEvent) string {
	if strings.TrimSpace(progress.Media.ShowTitle) != "" && progress.Media.Type == "episode" {
		return progress.Media.ShowTitle + " — " + progress.Media.Title
	}
	return progress.Media.Title
}

func localPlaybackProgressScrobKey(progress models.PlaybackProgress) string {
	switch progress.MediaType {
	case "movie":
		id := scrobPositiveID(progress.ExternalIDs["tmdb"])
		if id == 0 {
			id = scrobIDFromItem(progress.ItemID, "tmdb:movie:")
		}
		if id == 0 {
			id = scrobIDFromItem(progress.ItemID, "tmdb:")
		}
		if id > 0 {
			return fmt.Sprintf("movie:%d", id)
		}
	case "episode":
		ext := mediaidentity.EnrichShowExternalIDs(progress.SeriesID, progress.ItemID, progress.ExternalIDs)
		showID := scrobPositiveID(ext["tmdb"])
		if showID > 0 && progress.SeasonNumber >= 0 && progress.EpisodeNumber > 0 {
			return fmt.Sprintf("episode:%d:%d:%d", showID, progress.SeasonNumber, progress.EpisodeNumber)
		}
	}
	return ""
}

func scrobEventToUpdate(event scrob.HistoryEvent, watched *bool) *models.WatchHistoryUpdate {
	if !event.Completed {
		return nil
	}
	m := event.Media
	var watchedAt time.Time
	if event.WatchedAt != nil {
		watchedAt = event.WatchedAt.UTC()
	}
	switch m.Type {
	case "movie":
		if m.TMDBID <= 0 {
			return nil
		}
		year := 0
		if len(m.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(m.ReleaseDate[:4])
		}
		id := strconv.Itoa(m.TMDBID)
		return &models.WatchHistoryUpdate{MediaType: "movie", ItemID: "tmdb:" + id, Name: m.Title, Year: year, Watched: watched, WatchedAt: watchedAt, ExternalIDs: map[string]string{"tmdb": id}}
	case "episode":
		if m.ShowTMDBID <= 0 || m.SeasonNumber < 0 || m.EpisodeNumber <= 0 {
			return nil
		}
		showID := strconv.Itoa(m.ShowTMDBID)
		seriesID := "tmdb:tv:" + showID
		ext := map[string]string{"tmdb": showID}
		if m.TMDBID > 0 {
			ext["episodeTmdb"] = strconv.Itoa(m.TMDBID)
		}
		if m.ShowTVDBID > 0 {
			ext["tvdb"] = strconv.Itoa(m.ShowTVDBID)
		}
		return &models.WatchHistoryUpdate{
			MediaType: "episode", ItemID: fmt.Sprintf("%s:s%02de%02d", seriesID, m.SeasonNumber, m.EpisodeNumber), Name: m.Title,
			Watched: watched, WatchedAt: watchedAt, ExternalIDs: ext, SeasonNumber: m.SeasonNumber, EpisodeNumber: m.EpisodeNumber,
			SeriesID: seriesID, SeriesName: m.ShowTitle,
		}
	}
	return nil
}

func (s *Service) syncLocalHistoryToScrob(task config.ScheduledTask, account *config.ScrobAccount, profileID string, dryRun bool) (SyncResult, error) {
	result := SyncResult{DryRun: dryRun}
	s.mu.RLock()
	client, historySvc := s.scrobClient, s.historyService
	s.mu.RUnlock()
	items, err := historySvc.ListWatchHistory(profileID)
	if err != nil {
		return result, fmt.Errorf("list local history: %w", err)
	}
	continueWatching, err := historySvc.ListContinueWatching(profileID)
	if err != nil {
		return result, fmt.Errorf("list local continue watching: %w", err)
	}
	// Initial exports may contain thousands of plays and Scrob currently accepts
	// them one at a time. Keep enough headroom for a full backfill; later runs
	// deduplicate against remote history and are substantially shorter.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	remote, err := client.GetHistory(ctx, account.BaseURL, account.APIKey)
	if err != nil {
		return result, fmt.Errorf("fetch Scrob history for deduplication: %w", err)
	}
	remoteProgress, err := client.GetContinueWatching(ctx, account.BaseURL, account.APIKey)
	if err != nil {
		return result, fmt.Errorf("fetch Scrob continue watching for deduplication: %w", err)
	}
	remoteByKey := make(map[string][]scrob.Media)
	for _, event := range remote {
		if key := scrobRemoteKey(event.Media); key != "" {
			remoteByKey[key] = appendUniqueScrobMedia(remoteByKey[key], event.Media)
		}
	}
	remoteProgressByKey := make(map[string]scrob.PlaybackProgressEvent)
	for _, progress := range remoteProgress {
		if key := scrobRemoteKey(progress.Media); key != "" {
			remoteProgressByKey[key] = progress
		}
	}
	s.mu.RLock()
	metadataSvc := s.metadataService
	s.mu.RUnlock()
	metadataCache := make(map[string]*models.SeriesDetails)

	exportKey := task.ID + ":scrob_export"
	s.lastFullSyncTimesMu.Lock()
	lastFull, haveFull := s.lastFullSyncTimes[exportKey]
	s.lastFullSyncTimesMu.Unlock()
	isFull := task.Config["fullExport"] == "true" || !haveFull || time.Since(lastFull) >= 6*time.Hour
	var since time.Time
	if !isFull && task.LastRunAt != nil {
		since = task.LastRunAt.Add(-5 * time.Minute)
	}
	type outbound struct {
		item             models.WatchHistoryItem
		event            scrob.WatchEvent
		key              string
		aliasKeys        map[string]struct{}
		removeMediaID    int
		remove           bool
		requiresPresence string
	}
	var candidates []outbound
	candidateByKey := make(map[string]int)
	for _, item := range items {
		if !since.IsZero() && item.Watched && item.WatchedAt.Before(since) {
			continue
		}
		if !since.IsZero() && !item.Watched && item.UpdatedAt.Before(since) {
			continue
		}
		originalItem := item
		item = enrichScrobExportEpisode(ctx, metadataSvc, metadataCache, item)
		event, key, ok := localItemToScrob(item)
		if !ok {
			continue
		}
		candidate := outbound{item: item, event: event, key: key, aliasKeys: make(map[string]struct{})}
		if _, originalKey, originalOK := localItemToScrob(originalItem); originalOK && originalKey != key {
			candidate.aliasKeys[originalKey] = struct{}{}
		}
		if index, duplicate := candidateByKey[key]; duplicate {
			for aliasKey := range candidate.aliasKeys {
				candidates[index].aliasKeys[aliasKey] = struct{}{}
			}
			if scrobHistoryItemChangedAt(item).After(scrobHistoryItemChangedAt(candidates[index].item)) {
				candidate.aliasKeys = candidates[index].aliasKeys
				candidates[index] = candidate
			}
			continue
		}
		candidateByKey[key] = len(candidates)
		candidates = append(candidates, candidate)
	}
	changes := make([]outbound, 0, len(candidates))
	aliasCleanups := make([]outbound, 0)
	for _, candidate := range candidates {
		remoteItems := remoteByKey[candidate.key]
		exists := len(remoteItems) > 0
		if candidate.item.Watched && !exists {
			changes = append(changes, candidate)
		} else if !candidate.item.Watched && exists {
			for _, remoteItem := range remoteItems {
				removal := candidate
				removal.remove = true
				removal.removeMediaID = remoteItem.ID
				changes = append(changes, removal)
			}
		}
		if !candidate.item.Watched {
			continue
		}
		for _, alias := range scrobAliasCleanupMedia(candidate.item.Name, candidate.key, candidate.aliasKeys, candidateByKey, remoteByKey) {
			aliasCleanups = append(aliasCleanups, outbound{
				item:  models.WatchHistoryItem{MediaType: alias.Type, Name: alias.Title},
				event: scrob.WatchEvent{MediaType: alias.Type}, key: candidate.key,
				remove: true, removeMediaID: alias.ID, requiresPresence: candidate.key,
			})
		}
	}
	changes = append(changes, aliasCleanups...)

	type partialOutbound struct {
		state    models.SeriesWatchState
		start    scrob.ManualSessionStart
		key      string
		progress float64
	}
	partialChanges := make([]partialOutbound, 0, len(continueWatching))
	for _, state := range continueWatching {
		start, key, progress, ok := continueWatchingToScrobSession(state)
		if !ok {
			continue
		}
		updatedAt := state.ResumeUpdatedAt
		if updatedAt.IsZero() {
			updatedAt = state.UpdatedAt
		}
		if remoteItem, exists := remoteProgressByKey[key]; exists {
			if scrobRemoteProgressIsNewer(remoteItem.UpdatedAt, updatedAt) {
				continue
			}
			if math.Abs(remoteItem.ProgressPercent*100-progress) < 0.5 {
				continue
			}
		}
		partialChanges = append(partialChanges, partialOutbound{state: state, start: start, key: key, progress: progress})
	}

	if dryRun {
		for _, change := range changes {
			d := config.DryRunItem{Name: change.item.Name, MediaType: change.item.MediaType, ID: change.item.ItemID}
			if !change.remove {
				result.ToAdd = append(result.ToAdd, d)
			} else {
				result.ToRemove = append(result.ToRemove, d)
			}
		}
		for _, change := range partialChanges {
			result.ToAdd = append(result.ToAdd, config.DryRunItem{
				Name: change.state.SeriesTitle, MediaType: change.start.MediaType, ID: change.state.SeriesID,
			})
		}
		result.Count = len(result.ToAdd) + len(result.ToRemove)
		return result, nil
	}
	if len(changes) == 0 && len(partialChanges) == 0 {
		if isFull {
			s.lastFullSyncTimesMu.Lock()
			s.lastFullSyncTimes[exportKey] = time.Now().UTC()
			s.lastFullSyncTimesMu.Unlock()
		}
		return result, nil
	}
	twoFactorCode := ""
	if strings.TrimSpace(account.TOTPSecret) != "" {
		twoFactorCode, err = scrob.GenerateTOTPCode(account.TOTPSecret, time.Now().UTC())
		if err != nil {
			return result, err
		}
	}
	token, err := client.Login(ctx, account.BaseURL, account.APIKey, account.Username, account.Password, twoFactorCode)
	if err != nil {
		return result, err
	}
	failed := 0
	firstFailureSet := false
	firstFailureType, firstFailureName := "", ""
	presentKeys := make(map[string]struct{}, len(remoteByKey))
	for key, media := range remoteByKey {
		if len(media) > 0 {
			presentKeys[key] = struct{}{}
		}
	}
	for _, change := range changes {
		if change.requiresPresence != "" {
			if _, present := presentKeys[change.requiresPresence]; !present {
				continue
			}
		}
		if !change.remove {
			err = client.AddHistory(ctx, account.BaseURL, account.APIKey, token, change.event)
		} else if change.removeMediaID > 0 {
			err = client.RemoveHistoryByID(ctx, account.BaseURL, account.APIKey, token, change.removeMediaID, change.event.MediaType)
		} else {
			continue
		}
		if err != nil {
			failure := fmt.Errorf("sync %s %q to Scrob: %w", change.item.MediaType, change.item.Name, err)
			if !firstFailureSet {
				firstFailureSet = true
				firstFailureType, firstFailureName = change.item.MediaType, change.item.Name
			}
			failed++
			log.Printf("[scheduler] Scrob export skipped item after error: %v", failure)
			if ctx.Err() != nil {
				return result, fmt.Errorf("Scrob export stopped after syncing %d of %d changes: %w", result.Count, len(changes), ctx.Err())
			}
			continue
		}
		if !change.remove {
			presentKeys[change.key] = struct{}{}
		}
		result.Count++
	}
	for _, change := range partialChanges {
		started, startErr := client.StartSession(ctx, account.BaseURL, account.APIKey, token, change.start)
		if startErr == nil {
			runtime := change.start.Runtime
			if runtime <= 0 {
				runtime = started.Runtime
			}
			if runtime <= 0 {
				startErr = errors.New("Scrob returned no runtime for partial progress")
			} else {
				position := int(math.Round(float64(runtime*60) * change.progress / 100))
				startErr = client.UpdateSession(ctx, account.BaseURL, account.APIKey, token, started.SessionKey, scrob.ManualSessionUpdate{
					ProgressSeconds: position,
					State:           "paused",
				})
			}
		}
		if startErr != nil {
			if !firstFailureSet {
				firstFailureSet = true
				firstFailureType, firstFailureName = change.start.MediaType, change.state.SeriesTitle
			}
			failed++
			log.Printf("[scheduler] Scrob partial-progress export skipped %s %q: %v", change.start.MediaType, change.state.SeriesTitle, startErr)
			if ctx.Err() != nil {
				return result, fmt.Errorf("Scrob export stopped after syncing %d of %d changes: %w", result.Count, len(changes)+len(partialChanges), ctx.Err())
			}
			continue
		}
		result.Count++
	}
	if failed > 0 {
		total := len(changes) + len(partialChanges)
		log.Printf("[scheduler] Scrob export completed partially: synced=%d failed=%d total=%d", result.Count, failed, total)
		return result, scrobExportPartialError(result.Count, total, failed, firstFailureType, firstFailureName)
	}
	if isFull {
		s.lastFullSyncTimesMu.Lock()
		s.lastFullSyncTimes[exportKey] = time.Now().UTC()
		s.lastFullSyncTimesMu.Unlock()
	}
	return result, nil
}

func scrobRemoteProgressIsNewer(remoteUpdatedAt, localUpdatedAt time.Time) bool {
	return !remoteUpdatedAt.IsZero() && localUpdatedAt.Before(remoteUpdatedAt)
}

// continueWatchingToScrobSession converts the public Continue Watching payload
// into Scrob's manual-session API. Completed-series "next up" entries carry no
// resume percentage and are intentionally excluded.
func continueWatchingToScrobSession(state models.SeriesWatchState) (scrob.ManualSessionStart, string, float64, bool) {
	progress := state.ResumePercent
	if progress == 0 {
		progress = state.PercentWatched
	}
	if progress <= 5 || progress >= 90 {
		return scrob.ManualSessionStart{}, "", progress, false
	}

	if state.NextEpisode == nil {
		tmdbID := scrobPositiveID(state.ExternalIDs["tmdb"])
		if tmdbID == 0 {
			tmdbID = scrobIDFromItem(state.SeriesID, "tmdb:movie:")
		}
		if tmdbID == 0 {
			tmdbID = scrobIDFromItem(state.SeriesID, "tmdb:")
		}
		if tmdbID == 0 {
			return scrob.ManualSessionStart{}, "", progress, false
		}
		return scrob.ManualSessionStart{
			TMDBID: tmdbID, MediaType: "movie", Title: state.SeriesTitle, Runtime: state.LastWatched.RuntimeMinutes,
		}, fmt.Sprintf("movie:%d", tmdbID), progress, true
	}

	showTMDBID := scrobPositiveID(state.ExternalIDs["tmdb"])
	if showTMDBID == 0 {
		showTMDBID = scrobIDFromItem(state.SeriesID, "tmdb:tv:")
	}
	if showTMDBID == 0 {
		return scrob.ManualSessionStart{}, "", progress, false
	}
	episode := state.NextEpisode
	if episode.EpisodeNumber <= 0 || episode.SeasonNumber < 0 {
		return scrob.ManualSessionStart{}, "", progress, false
	}
	seasonNumber, episodeNumber := episode.SeasonNumber, episode.EpisodeNumber
	return scrob.ManualSessionStart{
		MediaType: "episode", Title: episode.Title, Runtime: episode.RuntimeMinutes,
		ShowTMDBID: showTMDBID, SeasonNumber: &seasonNumber, EpisodeNumber: &episodeNumber,
	}, fmt.Sprintf("episode:%d:%d:%d", showTMDBID, seasonNumber, episodeNumber), progress, true
}

func appendUniqueScrobMedia(items []scrob.Media, candidate scrob.Media) []scrob.Media {
	for _, item := range items {
		if item.ID == candidate.ID {
			return items
		}
	}
	return append(items, candidate)
}

func scrobAliasCleanupMedia(title, canonicalKey string, aliasKeys map[string]struct{}, canonicalCandidates map[string]int, remoteByKey map[string][]scrob.Media) []scrob.Media {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	var cleanups []scrob.Media
	for aliasKey := range aliasKeys {
		if aliasKey == canonicalKey {
			continue
		}
		if _, isCanonicalCandidate := canonicalCandidates[aliasKey]; isCanonicalCandidate {
			continue
		}
		for _, remote := range remoteByKey[aliasKey] {
			if remote.ID > 0 && strings.EqualFold(strings.TrimSpace(remote.Title), title) {
				cleanups = append(cleanups, remote)
			}
		}
	}
	return cleanups
}

func scrobExportPartialError(synced, total, failed int, mediaType, name string) error {
	item := strings.TrimSpace(mediaType)
	if strings.TrimSpace(name) != "" {
		item += fmt.Sprintf(" %q", strings.TrimSpace(name))
	}
	if item == "" {
		item = "item"
	}
	return fmt.Errorf("Scrob export synced %d of %d changes; %d failed. First failure: %s. See backend logs for details", synced, total, failed, item)
}

func scrobHistoryItemChangedAt(item models.WatchHistoryItem) time.Time {
	if !item.UpdatedAt.IsZero() {
		return item.UpdatedAt
	}
	return item.WatchedAt
}

func enrichScrobExportEpisode(ctx context.Context, metadataSvc schedulerMetadataService, cache map[string]*models.SeriesDetails, item models.WatchHistoryItem) models.WatchHistoryItem {
	if item.MediaType != "episode" || metadataSvc == nil {
		return item
	}
	ext := mediaidentity.EnrichShowExternalIDs(item.SeriesID, item.ItemID, item.ExternalIDs)
	query, cacheKey := scrobSeriesDetailsQuery(ext)
	if cacheKey == "" {
		return item
	}
	details, cached := cache[cacheKey]
	if !cached {
		var err error
		details, err = metadataSvc.SeriesDetailsLite(ctx, query)
		if err != nil {
			log.Printf("[scheduler] Scrob export: unable to enrich %s S%02dE%02d: %v", cacheKey, item.SeasonNumber, item.EpisodeNumber, err)
		}
		cache[cacheKey] = details
	}
	if details == nil {
		return item
	}
	match := matchScrobExportEpisode(details, item)
	if match == nil {
		return item
	}

	item.ExternalIDs = cloneStringMap(ext)
	if item.ExternalIDs == nil {
		item.ExternalIDs = make(map[string]string)
	}
	if details.Title.TMDBID > 0 {
		item.ExternalIDs["tmdb"] = strconv.FormatInt(details.Title.TMDBID, 10)
	}
	if details.Title.TVDBID > 0 {
		item.ExternalIDs["tvdb"] = strconv.FormatInt(details.Title.TVDBID, 10)
	}
	if details.Title.IMDBID != "" {
		item.ExternalIDs["imdb"] = details.Title.IMDBID
	}
	if match.TMDBID > 0 {
		item.ExternalIDs["episodeTmdb"] = strconv.FormatInt(match.TMDBID, 10)
	}
	if match.TVDBID > 0 {
		item.ExternalIDs["episodeTvdb"] = strconv.FormatInt(match.TVDBID, 10)
	}
	if match.AbsoluteEpisodeNumber > 0 {
		item.ExternalIDs["absoluteEpisode"] = strconv.Itoa(match.AbsoluteEpisodeNumber)
	}
	item.SeasonNumber = match.SeasonNumber
	item.EpisodeNumber = match.EpisodeNumber
	if match.TMDBSeasonNumber >= 0 && match.TMDBEpisodeNumber > 0 {
		item.SeasonNumber = match.TMDBSeasonNumber
		item.EpisodeNumber = match.TMDBEpisodeNumber
	}
	if strings.TrimSpace(item.Name) == "" && strings.TrimSpace(match.Name) != "" {
		item.Name = match.Name
	}
	return item
}

func scrobSeriesDetailsQuery(ext map[string]string) (models.SeriesDetailsQuery, string) {
	var query models.SeriesDetailsQuery
	if id := scrobPositiveID(ext["tmdb"]); id > 0 {
		query.TMDBID = int64(id)
	}
	if id := scrobPositiveID(ext["tvdb"]); id > 0 {
		query.TVDBID = int64(id)
	}
	query.IMDBID = strings.TrimSpace(ext["imdb"])
	switch {
	case query.TVDBID > 0:
		query.TitleID = fmt.Sprintf("tvdb:series:%d", query.TVDBID)
		return query, query.TitleID
	case query.TMDBID > 0:
		query.TitleID = fmt.Sprintf("tmdb:tv:%d", query.TMDBID)
		return query, query.TitleID
	case query.IMDBID != "":
		query.TitleID = "imdb:" + query.IMDBID
		return query, query.TitleID
	default:
		return query, ""
	}
}

func matchScrobExportEpisode(details *models.SeriesDetails, item models.WatchHistoryItem) *models.SeriesEpisode {
	if details == nil {
		return nil
	}
	episodeTMDBID := scrobPositiveID(item.ExternalIDs["episodeTmdb"])
	episodeTVDBID := scrobPositiveID(item.ExternalIDs["episodeTvdb"])
	absoluteEpisode := scrobPositiveID(item.ExternalIDs["absoluteEpisode"])
	var exact, explicitAbsolute, inferredAbsolute *models.SeriesEpisode
	for _, season := range details.Seasons {
		for i := range season.Episodes {
			episode := &season.Episodes[i]
			if episodeTMDBID > 0 && episode.TMDBID == int64(episodeTMDBID) {
				return episode
			}
			if episodeTVDBID > 0 && episode.TVDBID == int64(episodeTVDBID) {
				return episode
			}
			if exact == nil && episode.SeasonNumber == item.SeasonNumber && episode.EpisodeNumber == item.EpisodeNumber {
				exact = episode
			}
			if explicitAbsolute == nil && absoluteEpisode > 0 && episode.AbsoluteEpisodeNumber == absoluteEpisode {
				explicitAbsolute = episode
			}
			if inferredAbsolute == nil && episode.AbsoluteEpisodeNumber > 0 && episode.AbsoluteEpisodeNumber == item.EpisodeNumber {
				inferredAbsolute = episode
			}
		}
	}
	if exact != nil {
		return exact
	}
	if explicitAbsolute != nil {
		return explicitAbsolute
	}
	return inferredAbsolute
}

func scrobRemoteKey(media scrob.Media) string {
	switch media.Type {
	case "movie":
		if media.TMDBID > 0 {
			return fmt.Sprintf("movie:%d", media.TMDBID)
		}
	case "episode":
		if media.ShowTMDBID > 0 && media.EpisodeNumber > 0 {
			return fmt.Sprintf("episode:%d:%d:%d", media.ShowTMDBID, media.SeasonNumber, media.EpisodeNumber)
		}
	}
	return ""
}

func localItemToScrob(item models.WatchHistoryItem) (scrob.WatchEvent, string, bool) {
	event := scrob.WatchEvent{MediaType: item.MediaType, Completed: true}
	if !item.WatchedAt.IsZero() {
		watchedAt := item.WatchedAt.UTC()
		event.WatchedAt = &watchedAt
	}
	switch item.MediaType {
	case "movie":
		id := scrobPositiveID(item.ExternalIDs["tmdb"])
		if id == 0 {
			id = scrobIDFromItem(item.ItemID, "tmdb:")
		}
		if id == 0 {
			return event, "", false
		}
		event.TMDBID = id
		return event, fmt.Sprintf("movie:%d", id), true
	case "episode":
		ext := mediaidentity.EnrichShowExternalIDs(item.SeriesID, item.ItemID, item.ExternalIDs)
		showID := scrobPositiveID(ext["tmdb"])
		if showID == 0 || item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
			return event, "", false
		}
		event.TMDBID = scrobPositiveID(ext["episodeTmdb"])
		event.SeriesTMDBID = showID
		event.SeriesTVDBID = scrobPositiveID(ext["tvdb"])
		event.SeasonNumber, event.EpisodeNumber = item.SeasonNumber, item.EpisodeNumber
		return event, fmt.Sprintf("episode:%d:%d:%d", showID, item.SeasonNumber, item.EpisodeNumber), true
	}
	return event, "", false
}

func scrobPositiveID(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	if n > 0 {
		return n
	}
	return 0
}
func scrobIDFromItem(itemID, prefix string) int {
	if strings.HasPrefix(strings.ToLower(itemID), prefix) {
		return scrobPositiveID(itemID[len(prefix):])
	}
	return 0
}
