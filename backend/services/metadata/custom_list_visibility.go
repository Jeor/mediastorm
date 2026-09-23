package metadata

import (
	"context"
	"log"
	"sync"

	"novastream/models"
)

// preFilterUnreleasedTypes does a lightweight concurrent pass to remove unreleased items
// before full enrichment. For movies it checks TMDB release data; for series it checks
// whether any known episode has aired.
func (s *Service) preFilterUnreleasedTypes(ctx context.Context, items []mdblistItem, movies, shows bool) []mdblistItem {
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	keep := make([]bool, len(items))
	for i := range keep {
		keep[i] = true // default: keep
	}

	for i, item := range items {
		mediaType := mdblistItemMediaType(item)
		if (mediaType == "movie" && !movies) || (mediaType == "series" && !shows) {
			continue
		}

		wg.Add(1)
		go func(idx int, it mdblistItem, mt string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if mt == "movie" {
				tmdbID := int64(0)
				if it.TMDBID != nil && *it.TMDBID > 0 {
					tmdbID = *it.TMDBID
				} else if it.IMDBID != "" {
					tmdbID = s.getTMDBIDForIMDB(ctx, it.IMDBID)
				}
				if tmdbID <= 0 {
					return // can't determine, keep
				}
				title := models.Title{MediaType: "movie", TMDBID: tmdbID}
				if s.enrichMovieReleases(ctx, &title, tmdbID) {
					if title.Status != models.MovieReleaseStatusReleased {
						keep[idx] = false
					}
				}
			} else {
				// Series: check status via lightweight extended call (no artworks)
				if s.client.isConfigured() && it.TVDBID != nil && *it.TVDBID > 0 {
					ext, err := s.cachedSeriesExtended(*it.TVDBID, []string{"episodes"})
					if err == nil {
						status := seriesReleaseStatusFromTVDBExtended(ext, models.Title{
							MediaType: "series",
							Year:      it.ReleaseYear,
						})
						if status != models.SeriesReleaseStatusReleased {
							keep[idx] = false
						}
					}
				} else if s.tmdb != nil && s.tmdb.isConfigured() {
					tmdbID := int64(0)
					if it.TMDBID != nil {
						tmdbID = *it.TMDBID
					}
					if tmdbID <= 0 {
						tmdbID = s.resolveTMDBSeriesID(ctx, models.SeriesDetailsQuery{Name: it.Title, Year: it.ReleaseYear, IMDBID: it.IMDBID})
					}
					if tmdbID > 0 {
						if title, err := s.tmdb.seriesDetails(ctx, tmdbID); err == nil && title != nil && title.Status != models.SeriesReleaseStatusReleased {
							keep[idx] = false
						}
					}
				}
			}
		}(i, item, mediaType)
	}
	wg.Wait()

	result := make([]mdblistItem, 0, len(items))
	filteredCount := 0
	for i, item := range items {
		if keep[i] {
			result = append(result, item)
		} else {
			filteredCount++
			if filteredCount <= 3 {
				log.Printf("[hideUnreleased] pre-filtered: %s (type=%s)", item.Title, mdblistItemMediaType(item))
			}
		}
	}
	if filteredCount > 0 {
		log.Printf("[hideUnreleased] pre-filter result: %d/%d items kept (filtered %d)", len(result), len(items), filteredCount)
	}
	return result
}

// filterCachedCustomList applies request-specific filters to a cached full list.
// Recheck release dates: cached lite statuses can be inferred from year alone.
func (s *Service) filterCachedCustomList(ctx context.Context, items []models.TrendingItem, opts CustomListOptions) []models.TrendingItem {
	raw := make([]mdblistItem, len(items))
	for i, item := range items {
		title := item.Title
		raw[i] = mdblistItem{Rank: i, Title: title.Name, ReleaseYear: title.Year, MediaType: title.MediaType, IMDBID: title.IMDBID}
		if title.TMDBID > 0 {
			id := title.TMDBID
			raw[i].TMDBID = &id
		}
		if title.TVDBID > 0 {
			id := title.TVDBID
			raw[i].TVDBID = &id
		}
	}
	if opts.HideWatched && opts.UserID != "" && opts.HistorySvc != nil {
		raw = filterWatchedMDBListItems(raw, opts.UserID, opts.HistorySvc)
	}
	if opts.HideUnreleased || opts.HideUnreleasedMovies || opts.HideUnreleasedShows {
		raw = s.preFilterUnreleasedTypes(ctx, raw, opts.HideUnreleased || opts.HideUnreleasedMovies, opts.HideUnreleased || opts.HideUnreleasedShows)
	}
	result := make([]models.TrendingItem, 0, len(raw))
	for _, item := range raw {
		result = append(result, items[item.Rank])
	}
	return result
}
