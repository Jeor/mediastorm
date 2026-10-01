package handlers

import (
	"net/http"
	"strings"

	"novastream/services/metadata"
)

// Public Letterboxd lists only supply title/year identities. Read just enough
// source rows to fill the visible page before resolving the remainder for facets.
func (h *MetadataHandler) progressiveLetterboxdShelf(r *http.Request, service metadataService, listURL, userID string, hideUnreleased, hideWatched bool, limit, offset int) (*CustomListResponse, error) {
	filtered := h.progressiveShelfFiltered(r, userID, hideUnreleased, hideWatched)
	complete := r.URL.Query().Get("shelfPhase") == "complete"
	maxItems := max(20, offset+limit)
	if complete && (filtered || parseDisplayListQuery(r).IncludeFacets) {
		maxItems = 1000 // Public Letterboxd client's supported browse window.
	}
	label := strings.TrimSpace(r.URL.Query().Get("name"))
	if label == "" {
		label = "Letterboxd List"
	}
	for {
		result, err := h.LetterboxdClient.GetListResult(r.Context(), listURL, maxItems)
		if err != nil {
			return nil, err
		}
		source := make([]metadata.CuratedItem, len(result.Items))
		for i, item := range result.Items {
			source[i] = metadata.CuratedItem{Title: item.Title, Year: item.Year, MediaType: item.MediaType}
		}
		response, err := h.progressiveCuratedShelf(r, service, source, label, userID, hideUnreleased, hideWatched, limit, offset)
		if err != nil {
			return nil, err
		}
		total := min(result.Total, 1000)
		exhausted := len(source) >= total || len(source) < maxItems
		response.UnfilteredTotal = total
		if !filtered {
			response.Total = total
			response.TotalPending = false
		} else if !exhausted {
			response.TotalPending = true
			response.Total = max(response.Total, offset+limit+1)
		}
		if complete || len(response.Items) >= limit || exhausted {
			return response, nil
		}
		maxItems = min(maxItems+max(100, limit), 1000)
	}
}
