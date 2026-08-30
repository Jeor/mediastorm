package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"novastream/models"
	"novastream/services/prewarm"

	"github.com/gorilla/mux"
)

// ListPrewarmShelfItems resolves playable titles from one of a profile's
// effective home shelves. It deliberately uses DisplayListHandler so prewarm
// observes the same source accounts, hidden-title filters, kids restrictions,
// release filters, and per-profile settings as the home UI.
func (h *StartupHandler) ListPrewarmShelfItems(ctx context.Context, userID, shelfID, itemScope string) ([]prewarm.ShelfItem, error) {
	if h.displayList == nil {
		return nil, fmt.Errorf("display list handler is unavailable")
	}
	defaults := h.getDefaultsFromGlobal()
	settings, err := h.userSettings.GetWithDefaults(userID, defaults)
	if err != nil {
		return nil, fmt.Errorf("load profile home shelves: %w", err)
	}

	var shelf *models.ShelfConfig
	for i := range settings.HomeShelves.Shelves {
		if settings.HomeShelves.Shelves[i].ID == shelfID {
			shelf = &settings.HomeShelves.Shelves[i]
			break
		}
	}
	if shelf == nil {
		return nil, fmt.Errorf("home shelf no longer exists")
	}
	query, ok := prewarmDisplayListQuery(*shelf)
	if !ok {
		return nil, fmt.Errorf("home shelf is not playable by the prewarm worker")
	}

	if itemScope == prewarm.PrewarmItemScopeDisplayed {
		limit := settings.HomeShelves.ItemCap
		if limit <= 0 {
			limit = defaultStartupShelfLimit
		}
		if shelf.Limit > 0 && shelf.Limit < limit {
			limit = shelf.Limit
		}
		query.Set("limit", strconv.Itoa(limit))
	} else {
		query.Del("limit")
	}
	if shelf.HideUnreleased {
		query.Set("hideUnreleased", "true")
	}
	if strings.TrimSpace(shelf.Name) != "" {
		query.Set("name", shelf.Name)
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/users/"+url.PathEscape(userID)+"/display-list?"+query.Encode(), nil)
	req = mux.SetURLVars(req, map[string]string{"userID": userID})
	rec := httptest.NewRecorder()
	h.displayList.Get(rec, req)
	if rec.Code >= http.StatusBadRequest {
		message := strings.TrimSpace(rec.Body.String())
		if message == "" {
			message = http.StatusText(rec.Code)
		}
		return nil, fmt.Errorf("display list returned %d: %s", rec.Code, message)
	}

	var payload struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode display list: %w", err)
	}
	items := make([]prewarm.ShelfItem, 0, len(payload.Items))
	for _, raw := range payload.Items {
		if item, ok := decodePrewarmShelfItem(raw); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func prewarmDisplayListQuery(shelf models.ShelfConfig) (url.Values, bool) {
	switch shelf.ID {
	case "continue-watching", "popular-on-server", "recently-watched", "permanent-prequeue":
		return url.Values{}, false
	}

	limit := startupCustomShelfFetchLimit(shelf, defaultStartupShelfLimit)
	if shelf.Type == "" {
		// Prewarm historically left built-in query limits and display names to
		// the worker's displayed/all-items policy. Keep that boundary intact.
		limit = 0
	}
	query, ok := displayListQueryForShelf(shelf, limit, defaultStartupShelfLimit, false, "")
	if shelf.Type == "" {
		query.Del("name")
	}
	return query, ok
}

func decodePrewarmShelfItem(raw json.RawMessage) (prewarm.ShelfItem, bool) {
	var wrapper struct {
		Title       *models.Title     `json:"title"`
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		MediaType   string            `json:"mediaType"`
		Year        int               `json:"year"`
		IMDBID      string            `json:"imdbId"`
		ExternalIDs map[string]string `json:"externalIds"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return prewarm.ShelfItem{}, false
	}
	if wrapper.Title != nil {
		title := wrapper.Title
		externalIDs := map[string]string{}
		if title.IMDBID != "" {
			externalIDs["imdbId"] = title.IMDBID
		}
		if title.TMDBID > 0 {
			externalIDs["tmdbId"] = strconv.FormatInt(title.TMDBID, 10)
		}
		if title.TVDBID > 0 {
			externalIDs["tvdbId"] = strconv.FormatInt(title.TVDBID, 10)
		}
		return prewarm.ShelfItem{
			TitleID:     title.ID,
			TitleName:   title.Name,
			MediaType:   title.MediaType,
			Year:        title.Year,
			ImdbID:      title.IMDBID,
			ExternalIDs: externalIDs,
		}, strings.TrimSpace(title.ID) != "" && strings.TrimSpace(title.Name) != ""
	}
	return prewarm.ShelfItem{
		TitleID:     wrapper.ID,
		TitleName:   wrapper.Name,
		MediaType:   wrapper.MediaType,
		Year:        wrapper.Year,
		ImdbID:      firstNonEmptyPrewarmValue(wrapper.IMDBID, wrapper.ExternalIDs["imdbId"], wrapper.ExternalIDs["imdb"]),
		ExternalIDs: wrapper.ExternalIDs,
	}, strings.TrimSpace(wrapper.ID) != "" && strings.TrimSpace(wrapper.Name) != ""
}

func firstNonEmptyPrewarmValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
