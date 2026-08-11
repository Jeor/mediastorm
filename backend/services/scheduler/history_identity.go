package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"novastream/internal/mediaidentity"
	"novastream/models"
)

// enrichAndCollapseHistoryItems resolves every episode to the application's
// canonical identity before a provider export. Historical aliases can use
// seasonal, absolute, or hybrid coordinates; the strongest resolved episode
// ID becomes the deduplication key and the newest local state wins.
func (s *Service) enrichAndCollapseHistoryItems(items []models.WatchHistoryItem) []models.WatchHistoryItem {
	s.mu.RLock()
	metadataSvc := s.metadataService
	s.mu.RUnlock()

	cache := make(map[string]*models.SeriesDetails)
	collapsed := make([]models.WatchHistoryItem, 0, len(items))
	byIdentity := make(map[string]int, len(items))
	for _, item := range items {
		item = enrichHistoryEpisode(context.Background(), metadataSvc, cache, item)
		key := historyItemIdentityKey(item)
		if index, exists := byIdentity[key]; exists {
			if scrobHistoryItemChangedAt(item).After(scrobHistoryItemChangedAt(collapsed[index])) {
				collapsed[index] = item
			}
			continue
		}
		byIdentity[key] = len(collapsed)
		collapsed = append(collapsed, item)
	}
	return collapsed
}

func enrichHistoryEpisode(ctx context.Context, metadataSvc schedulerMetadataService, cache map[string]*models.SeriesDetails, item models.WatchHistoryItem) models.WatchHistoryItem {
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
			log.Printf("[scheduler] history export: unable to enrich %s S%02dE%02d: %v", cacheKey, item.SeasonNumber, item.EpisodeNumber, err)
		}
		cache[cacheKey] = details
	}
	match := matchHistoryEpisode(details, item)
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
	if strings.TrimSpace(item.Name) == "" && strings.TrimSpace(match.Name) != "" {
		item.Name = match.Name
	}
	return item
}

func matchHistoryEpisode(details *models.SeriesDetails, item models.WatchHistoryItem) *models.SeriesEpisode {
	if details == nil {
		return nil
	}
	episodeTMDBID := scrobPositiveID(item.ExternalIDs["episodeTmdb"])
	episodeTVDBID := scrobPositiveID(item.ExternalIDs["episodeTvdb"])
	absoluteEpisode := scrobPositiveID(item.ExternalIDs["absoluteEpisode"])
	var exact, explicitAbsolute, inferredAbsolute, titleMatch *models.SeriesEpisode
	normalizedTitle := normalizeHistoryEpisodeTitle(item.Name)
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
			if normalizedTitle != "" && normalizeHistoryEpisodeTitle(episode.Name) == normalizedTitle {
				if titleMatch != nil {
					titleMatch = nil // Ambiguous titles are not identities.
					normalizedTitle = ""
				} else {
					titleMatch = episode
				}
			}
		}
	}
	if explicitAbsolute != nil {
		return explicitAbsolute
	}
	if inferredAbsolute != nil && exact == nil {
		return inferredAbsolute
	}
	if exact != nil {
		return exact
	}
	return titleMatch
}

func normalizeHistoryEpisodeTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(title)), " "))
}

func historyItemIdentityKey(item models.WatchHistoryItem) string {
	mediaType := strings.ToLower(strings.TrimSpace(item.MediaType))
	if mediaType != "episode" {
		for _, key := range []string{"tmdb", "imdb", "tvdb"} {
			if value := strings.ToLower(strings.TrimSpace(item.ExternalIDs[key])); value != "" {
				return fmt.Sprintf("%s:%s:%s", mediaType, key, value)
			}
		}
		return fmt.Sprintf("%s:%s:%s:%d", mediaType, strings.ToLower(strings.TrimSpace(item.ItemID)), normalizeHistoryEpisodeTitle(item.Name), item.Year)
	}
	for _, key := range []string{"episodeTmdb", "episodeTvdb", "episodeTrakt"} {
		if value := strings.TrimSpace(item.ExternalIDs[key]); value != "" {
			return "episode:" + strings.ToLower(key) + ":" + strings.ToLower(value)
		}
	}
	ext := mediaidentity.EnrichShowExternalIDs(item.SeriesID, item.ItemID, item.ExternalIDs)
	showKey := strings.ToLower(strings.TrimSpace(item.SeriesID))
	for _, key := range []string{"tmdb", "tvdb", "imdb", "simkl"} {
		if value := strings.TrimSpace(ext[key]); value != "" {
			showKey = key + ":" + strings.ToLower(value)
			break
		}
	}
	if showKey == "" {
		showKey = strings.ToLower(strings.TrimSpace(item.ItemID))
	}
	if absolute := strings.TrimSpace(item.ExternalIDs["absoluteEpisode"]); absolute != "" {
		return fmt.Sprintf("episode:%s:absolute:%s", showKey, absolute)
	}
	return fmt.Sprintf("episode:%s:%d:%d", showKey, item.SeasonNumber, item.EpisodeNumber)
}

func buildMDBListRemovalPayload(items []models.WatchHistoryItem) ([]map[string]interface{}, []map[string]interface{}, int) {
	var movies []map[string]interface{}
	type showKey struct {
		imdb string
		tmdb int
	}
	showsByKey := make(map[showKey]map[int][]map[string]interface{})
	var order []showKey
	skipped := 0
	for _, item := range items {
		ids := extractMDBListIDs(mediaidentity.EnrichShowExternalIDs(item.SeriesID, item.ItemID, item.ExternalIDs))
		if ids.imdb == "" && ids.tmdb == 0 {
			skipped++
			continue
		}
		if item.MediaType == "movie" {
			movies = append(movies, map[string]interface{}{"ids": formatMDBListIDsMap(ids)})
			continue
		}
		if item.MediaType != "episode" || item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
			skipped++
			continue
		}
		key := showKey{imdb: ids.imdb, tmdb: ids.tmdb}
		if _, exists := showsByKey[key]; !exists {
			showsByKey[key] = make(map[int][]map[string]interface{})
			order = append(order, key)
		}
		showsByKey[key][item.SeasonNumber] = append(showsByKey[key][item.SeasonNumber], map[string]interface{}{"number": item.EpisodeNumber})
	}

	shows := make([]map[string]interface{}, 0, len(order))
	for _, key := range order {
		var seasons []map[string]interface{}
		for season, episodes := range showsByKey[key] {
			seasons = append(seasons, map[string]interface{}{"number": season, "episodes": episodes})
		}
		shows = append(shows, map[string]interface{}{
			"ids": formatMDBListIDsMap(mdblistIDs{imdb: key.imdb, tmdb: key.tmdb}), "seasons": seasons,
		})
	}
	return movies, shows, skipped
}

func simklExportEpisodeCoordinates(item models.WatchHistoryItem) (int, int) {
	absolute := scrobPositiveID(item.ExternalIDs["absoluteEpisode"])
	// Simkl's long-running anime catalog exposes cumulative episodes under
	// season 1 (for example One Piece S01E1173), rather than TVDB season order.
	if absolute >= 1000 {
		return 1, absolute
	}
	return item.SeasonNumber, item.EpisodeNumber
}
