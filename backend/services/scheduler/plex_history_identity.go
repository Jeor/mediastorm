package scheduler

import (
	"novastream/internal/mediaidentity"
	"novastream/models"
	"novastream/services/plex"
	"time"
)

// Plex rating keys are local library IDs, not TMDB IDs. Only import history
// addressable through catalog providers, using show IDs for episode identity.
func plexHistoryUpdate(item plex.WatchHistoryItem) (models.WatchHistoryUpdate, bool) {
	mediaType := plex.NormalizeMediaType(item.Type)
	ext := mediaidentity.NormalizeExternalIDs(item.ExternalIDs)
	if mediaType != "movie" && mediaType != "episode" {
		return models.WatchHistoryUpdate{}, false
	}
	if ext["tmdb"] == "" && ext["tvdb"] == "" && ext["imdb"] == "" {
		return models.WatchHistoryUpdate{}, false
	}
	if mediaType == "episode" && (item.ParentIndex < 0 || item.Index <= 0) {
		return models.WatchHistoryUpdate{}, false
	}
	identity := mediaidentity.Resolve(mediaidentity.Input{
		MediaType: mediaType, ExternalIDs: ext,
		SeasonNumber: item.ParentIndex, EpisodeNumber: item.Index,
	})
	watched := true
	update := models.WatchHistoryUpdate{
		MediaType: mediaType, ItemID: identity.ID, Name: item.Title, Year: item.Year,
		Watched: &watched, WatchedAt: time.Unix(item.ViewedAt, 0).UTC(), ExternalIDs: ext,
	}
	if mediaType == "episode" {
		update.SeriesID = identity.SeriesID
		update.SeriesName = item.GrandparentTitle
		update.SeasonNumber = identity.SeasonNumber
		update.EpisodeNumber = identity.EpisodeNumber
	}
	return update, identity.ID != ""
}
