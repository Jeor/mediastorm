package handlers

import (
	"net/http"

	"novastream/models"
	"novastream/services/metadata"
)

// TMDB catalog responses already contain card artwork. Fetch provider pages
// incrementally rather than requesting the entire 500-title browse window when
// a home shelf has a visibility filter.
func (h *MetadataHandler) progressiveTMDBShelf(r *http.Request, service metadataService, provider tmdbListService, opts metadata.TMDBListOptions, userID string, hideUnreleased, hideWatched bool, limit, offset int) (*CustomListResponse, error) {
	policy := resolveUnreleasedVisibilityPolicy(h.CfgManager, h.UserSettings, h.ClientSettings, userID, requestClientID(r), unreleasedVisibilityLists)
	_, _, kids := h.kidsRatingLimits(userID)
	hidden, _ := r.Context().Value(progressiveShelfFiltersKey{}).(*progressiveShelfFilters)
	query := parseDisplayListQuery(r)
	filtered := hideUnreleased || hideWatched || kids || !policy.IncludeMovies || !policy.IncludeShows || (hidden != nil && hidden.hasHidden) || (query.MediaType != "" && query.MediaType != "all")
	complete := r.URL.Query().Get("shelfPhase") == "complete"
	cardsRequest := r.Clone(r.Context())
	u := *r.URL
	q := u.Query()
	q.Set("shelfPhase", "cards")
	u.RawQuery = q.Encode()
	cardsRequest.URL = &u
	source := make([]metadata.CuratedItem, 0, limit)
	total := maxDiscoveryListItems
	var response *CustomListResponse
	for len(source) < total {
		opts.Offset = len(source)
		opts.Limit = 20
		opts.DeferArtwork = true
		items, providerTotal, err := provider.GetTMDBList(r.Context(), opts)
		if err != nil {
			return nil, err
		}
		total = providerTotal
		if total > maxDiscoveryListItems {
			total = maxDiscoveryListItems
		}
		if len(items) == 0 {
			total = len(source)
			break
		}
		for _, item := range items {
			if len(source) >= maxDiscoveryListItems {
				break
			}
			title := item.Title
			card := metadata.CuratedItem{Title: title.Name, Year: title.Year, TMDBID: title.TMDBID, TVDBID: title.TVDBID, IMDBID: title.IMDBID, MediaType: title.MediaType, Overview: title.Overview, Genres: title.Genres}
			if title.Poster != nil {
				card.PosterURL = title.Poster.URL
			}
			if title.Backdrop != nil {
				card.BackdropURL = title.Backdrop.URL
			}
			source = append(source, card)
		}
		if complete && filtered {
			continue
		}
		response, err = h.progressiveCuratedShelf(cardsRequest, service, source, "TMDB shelf", userID, hideUnreleased, hideWatched, limit, offset)
		if err != nil {
			return nil, err
		}
		if len(response.Items) >= limit {
			break
		}
	}
	if complete || response == nil {
		var err error
		response, err = h.progressiveCuratedShelf(r, service, source, "TMDB shelf", userID, hideUnreleased, hideWatched, limit, offset)
		if err != nil {
			return nil, err
		}
	}
	response.UnfilteredTotal = total
	if !filtered {
		response.Total = total
		response.TotalPending = false
	} else if len(source) < total {
		response.TotalPending = true
		if response.Total <= offset+limit {
			response.Total = offset + limit + 1
		}
	}
	if response.Items == nil {
		response.Items = []models.TrendingItem{}
	}
	return response, nil
}
