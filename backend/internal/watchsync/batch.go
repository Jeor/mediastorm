// Package watchsync groups manual watch-state changes for provider bulk APIs.
package watchsync

import (
	"strconv"

	"novastream/models"
)

// BatchSize bounds request bodies without turning large series into individual
// episode requests. Providers send these chunks sequentially.
const BatchSize = 500

// Groups keeps each show's watched and unwatched episodes together. In
// particular, provider not-found responses must never be confused with another
// show's episodes having the same season and episode numbers.
func Groups(items []models.WatchHistoryItem) [][]models.WatchHistoryItem {
	type key struct {
		watched                                   bool
		mediaType, tvdb, tmdb, imdb, trakt, simkl string
	}
	indices := make(map[key]int)
	var groups [][]models.WatchHistoryItem
	for _, item := range items {
		k := key{watched: item.Watched, mediaType: item.MediaType}
		if item.MediaType == "episode" {
			k.tvdb, k.tmdb, k.imdb = item.ExternalIDs["tvdb"], item.ExternalIDs["tmdb"], item.ExternalIDs["imdb"]
			k.trakt, k.simkl = item.ExternalIDs["trakt"], item.ExternalIDs["simkl"]
		}
		index, exists := indices[k]
		if !exists {
			index = len(groups)
			indices[k] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], item)
	}
	return groups
}

func ID(ids map[string]string, name string) int {
	id, _ := strconv.Atoi(ids[name])
	if id < 0 {
		return 0
	}
	return id
}
