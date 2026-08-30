package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"novastream/models"
	"novastream/services/homedesigner"

	"github.com/gorilla/mux"
	"golang.org/x/sync/errgroup"
)

const (
	maxHomeDesignerPreviewRows  = 20
	maxHomeDesignerPreviewItems = 12
	homeDesignerPreviewTimeout  = 4 * time.Second
	homeDesignerPreviewWorkers  = 4
)

// HomeDesignerPreviewProvider resolves an unsaved editor draft without
// persisting it. Its result is restricted to homedesigner's render-only
// preview contract.
type HomeDesignerPreviewProvider interface {
	Preview(context.Context, *http.Request, homedesigner.PreviewRequest) (homedesigner.PreviewResponse, error)
}

type homeDesignerPreviewProvider struct {
	displayList *DisplayListHandler
}

// NewHomeDesignerPreviewProvider reuses the display-list boundary that also
// powers startup and prewarm. The shared shelf mapper keeps provider selection
// and per-profile behavior aligned without exposing its internal request data.
func NewHomeDesignerPreviewProvider(displayList *DisplayListHandler) HomeDesignerPreviewProvider {
	return &homeDesignerPreviewProvider{displayList: displayList}
}

// Preview resolves independent rows concurrently. Individual resolver
// failures are represented on their row so one unavailable integration never
// makes the editor unusable.
func (p *homeDesignerPreviewProvider) Preview(ctx context.Context, sourceReq *http.Request, request homedesigner.PreviewRequest) (homedesigner.PreviewResponse, error) {
	if err := validateHomeDesignerPreviewRequest(request); err != nil {
		return homedesigner.PreviewResponse{}, err
	}
	if p.displayList == nil {
		return homedesigner.PreviewResponse{}, errors.New("preview content is unavailable")
	}

	rows := request.Rows.Value.Shelves
	response := homedesigner.PreviewResponse{
		Scope:     request.Scope,
		ProfileID: request.PreviewProfileID,
		Platform:  request.Platform,
		Rows:      make([]homedesigner.PreviewRow, len(rows)),
		Theme:     previewTheme(request.Theme),
	}
	previewCtx, cancel := context.WithTimeout(ctx, homeDesignerPreviewTimeout)
	defer cancel()

	group, groupCtx := errgroup.WithContext(previewCtx)
	semaphore := make(chan struct{}, homeDesignerPreviewWorkers)
	for index, shelf := range rows {
		index, shelf := index, shelf
		response.Rows[index] = initialPreviewRow(shelf)
		group.Go(func() error {
			select {
			case semaphore <- struct{}{}:
			case <-groupCtx.Done():
				response.Rows[index] = previewErrorRow(shelf)
				return nil
			}
			defer func() { <-semaphore }()

			response.Rows[index] = p.resolveRow(groupCtx, sourceReq, request.PreviewProfileID, shelf)
			return nil
		})
	}
	_ = group.Wait() // resolution errors are deliberately contained in their row
	return response, nil
}

func validateHomeDesignerPreviewRequest(request homedesigner.PreviewRequest) error {
	if strings.TrimSpace(request.PreviewProfileID) == "" {
		return homedesigner.ValidationError{Fields: []homedesigner.FieldError{{Section: "preview", Path: "previewProfileId", Message: "a preview profile is required"}}}
	}
	if request.Rows == nil || request.Rows.Value == nil {
		return homedesigner.ValidationError{Fields: []homedesigner.FieldError{{Section: "rows", Path: "rows", Message: "preview rows are required"}}}
	}
	if len(request.Rows.Value.Shelves) > maxHomeDesignerPreviewRows {
		return homedesigner.ValidationError{Fields: []homedesigner.FieldError{{Section: "rows", Path: "rows.shelves", Message: "at most 20 preview rows are allowed"}}}
	}
	for _, shelf := range request.Rows.Value.Shelves {
		if shelf.Limit > maxHomeDesignerPreviewItems {
			return homedesigner.ValidationError{Fields: []homedesigner.FieldError{{Section: "rows", RowID: shelf.ID, Path: "limit", Message: "at most 12 preview items per row are allowed"}}}
		}
	}
	return nil
}

func previewTheme(section *homedesigner.SectionMutation[models.AppearanceSettings]) homedesigner.PreviewTheme {
	if section == nil || section.Value == nil {
		return homedesigner.PreviewTheme{}
	}
	return homedesigner.BuildPreviewResponse(homedesigner.PreviewRequest{}, models.HomeShelvesSettings{}, *section.Value).Theme
}

func initialPreviewRow(shelf models.ShelfConfig) homedesigner.PreviewRow {
	row := homedesigner.PreviewRow{ID: shelf.ID, Name: shelf.Name, Layout: previewLayout(shelf), Items: []homedesigner.PreviewItem{}}
	if !shelf.Enabled {
		row.Status = "disabled"
		return row
	}
	row.Status = "ready"
	return row
}

func previewLayout(shelf models.ShelfConfig) string {
	if strings.EqualFold(strings.TrimSpace(shelf.Type), "collection-hub") {
		return "collection"
	}
	return "shelf"
}

func (p *homeDesignerPreviewProvider) resolveRow(ctx context.Context, sourceReq *http.Request, profileID string, shelf models.ShelfConfig) homedesigner.PreviewRow {
	row := initialPreviewRow(shelf)
	if !shelf.Enabled {
		return row
	}
	limit := shelf.Limit
	if limit <= 0 {
		limit = maxHomeDesignerPreviewItems
	}
	query, ok := displayListQueryForShelf(shelf, limit, maxHomeDesignerPreviewItems, false, "")
	if !ok {
		return previewErrorRow(shelf)
	}

	req := previewDisplayListRequest(ctx, sourceReq, profileID, query)
	rec := httptest.NewRecorder()
	p.displayList.Get(rec, req)
	if rec.Code >= http.StatusBadRequest {
		return previewErrorRow(shelf)
	}

	var payload struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		return previewErrorRow(shelf)
	}
	items, valid := previewItems(payload.Items, limit)
	if !valid {
		return previewErrorRow(shelf)
	}
	row.Total = payload.Total
	if row.Total == 0 && len(items) > 0 {
		row.Total = len(items)
	}
	if len(items) == 0 {
		row.Items = previewSampleItems(shelf.ID, limit)
		return row
	}
	row.Items = items
	return row
}

func previewDisplayListRequest(ctx context.Context, sourceReq *http.Request, profileID string, query url.Values) *http.Request {
	if sourceReq == nil {
		sourceReq = httptest.NewRequest(http.MethodPost, "/admin/api/home-designer/preview", nil)
	}
	req := sourceReq.Clone(ctx)
	req.Method = http.MethodGet
	req.URL = &url.URL{Path: "/api/users/" + url.PathEscape(profileID) + "/display-list", RawQuery: query.Encode()}
	return mux.SetURLVars(req, map[string]string{"userID": profileID})
}

func previewErrorRow(shelf models.ShelfConfig) homedesigner.PreviewRow {
	return homedesigner.PreviewRow{
		ID: shelf.ID, Name: shelf.Name, Layout: previewLayout(shelf), Status: "error",
		Message: "Content is unavailable for this row.", Items: []homedesigner.PreviewItem{},
	}
}

func previewItems(rawItems []json.RawMessage, limit int) ([]homedesigner.PreviewItem, bool) {
	items := make([]homedesigner.PreviewItem, 0, minInt(len(rawItems), limit))
	for _, raw := range rawItems {
		item, ok := previewItemFromRaw(raw)
		if !ok {
			continue
		}
		items = append(items, item)
		if len(items) == limit {
			break
		}
	}
	return items, len(rawItems) == 0 || len(items) > 0
}

// previewItemFromRaw accepts the concrete display-list item shapes and then
// copies only the explicit presentation fields approved for PreviewItem.
func previewItemFromRaw(raw json.RawMessage) (homedesigner.PreviewItem, bool) {
	var trending models.TrendingItem
	if err := json.Unmarshal(raw, &trending); err == nil && (strings.TrimSpace(trending.Title.ID) != "" || strings.TrimSpace(trending.Title.Name) != "") {
		return previewItemFromTitle(trending.Title), true
	}

	var watchlist models.WatchlistItem
	if err := json.Unmarshal(raw, &watchlist); err == nil && (strings.TrimSpace(watchlist.ID) != "" || strings.TrimSpace(watchlist.Name) != "") {
		return homedesigner.PreviewItem{
			ID: watchlist.ID, Title: watchlist.Name, MediaType: watchlist.MediaType,
			Subtitle: previewYearSubtitle(watchlist.Year), ArtworkURL: firstPreviewArtwork(watchlist.PosterURL, watchlist.BackdropURL),
			Badges: previewBadges(watchlist.WatchState, watchlist.Status),
		}, true
	}

	var progress models.SeriesWatchState
	if err := json.Unmarshal(raw, &progress); err == nil && (strings.TrimSpace(progress.SeriesID) != "" || strings.TrimSpace(progress.SeriesTitle) != "") {
		item := homedesigner.PreviewItem{
			ID: progress.SeriesID, Title: progress.SeriesTitle, MediaType: "series", Subtitle: strings.TrimSpace(progress.LastWatched.Title),
			ArtworkURL: firstPreviewArtwork(progress.PosterURL, progress.BackdropURL), Badges: previewBadges("", progress.Status),
		}
		if progress.ResumePercent > 0 {
			value := progress.ResumePercent
			item.Progress = &value
		}
		return item, true
	}
	return homedesigner.PreviewItem{}, false
}

func previewItemFromTitle(title models.Title) homedesigner.PreviewItem {
	return homedesigner.PreviewItem{
		ID: title.ID, Title: title.Name, MediaType: title.MediaType, Subtitle: strings.TrimSpace(title.CardSubtitle),
		ArtworkURL: firstPreviewImageURL(title.CardImage, title.Poster, title.Backdrop), Badges: previewBadges(title.WatchState, title.Status),
	}
}

func firstPreviewImageURL(images ...*models.Image) string {
	for _, image := range images {
		if image != nil && strings.TrimSpace(image.URL) != "" {
			if safe := safePreviewArtworkURL(image.URL); safe != "" {
				return safe
			}
		}
	}
	return ""
}

func firstPreviewArtwork(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			if safe := safePreviewArtworkURL(value); safe != "" {
				return safe
			}
		}
	}
	return ""
}

// safePreviewArtworkURL permits only ordinary public image locations. It
// avoids forwarding local paths, embedded credentials, and image-proxy query
// parameters that can encode an upstream/provider URL or access secret.
func safePreviewArtworkURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	for key := range parsed.Query() {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "url", "path", "token", "apikey", "api_key", "credential", "credentials", "password", "clientip", "client_ip", "signature", "sig":
			return ""
		}
	}
	return parsed.String()
}

func previewYearSubtitle(year int) string {
	if year <= 0 {
		return ""
	}
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")
}

func previewBadges(watchState, status string) []string {
	badges := make([]string, 0, 2)
	if watchState = strings.TrimSpace(watchState); watchState != "" && watchState != "none" {
		badges = append(badges, watchState)
	}
	if status = strings.TrimSpace(status); status != "" {
		badges = append(badges, status)
	}
	return badges
}

type previewSample struct {
	title, mediaType, artwork string
}

var homeDesignerPreviewSamples = []previewSample{
	{title: "Orbit Harbor", mediaType: "movie", artwork: "linear-gradient(135deg, #243b55, #141e30)"},
	{title: "Signal Valley", mediaType: "series", artwork: "linear-gradient(135deg, #42275a, #734b6d)"},
	{title: "The Paper Moon", mediaType: "movie", artwork: "linear-gradient(135deg, #134e5e, #71b280)"},
}

func previewSampleItems(rowID string, limit int) []homedesigner.PreviewItem {
	count := minInt(limit, len(homeDesignerPreviewSamples))
	items := make([]homedesigner.PreviewItem, 0, count)
	for i := 0; i < count; i++ {
		sample := homeDesignerPreviewSamples[i]
		items = append(items, homedesigner.PreviewItem{ID: "sample:" + rowID + ":" + string(rune('1'+i)), Title: sample.title, MediaType: sample.mediaType, ArtworkURL: sample.artwork, Sample: true})
	}
	return items
}
