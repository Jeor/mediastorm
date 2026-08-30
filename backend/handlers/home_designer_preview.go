package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
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

// Resolver slots are intentionally process-wide. A provider that ignores
// cancellation can occupy a slot indefinitely, but it cannot create an
// unbounded goroutine buildup across repeated preview requests.
var homeDesignerPreviewResolverSlots = make(chan struct{}, homeDesignerPreviewWorkers)

// HomeDesignerPreviewProvider resolves an unsaved editor draft without
// persisting it. Its result is restricted to homedesigner's render-only
// preview contract.
type HomeDesignerPreviewProvider interface {
	Preview(context.Context, *http.Request, homedesigner.PreviewRequest) (homedesigner.PreviewResponse, error)
}

type homeDesignerPreviewProvider struct {
	displayList   *DisplayListHandler
	timeout       time.Duration
	resolverSlots chan struct{}
}

// NewHomeDesignerPreviewProvider reuses the display-list boundary that also
// powers startup and prewarm. The shared shelf mapper keeps provider selection
// and per-profile behavior aligned without exposing its internal request data.
func NewHomeDesignerPreviewProvider(displayList *DisplayListHandler) HomeDesignerPreviewProvider {
	return newHomeDesignerPreviewProvider(displayList, homeDesignerPreviewTimeout, homeDesignerPreviewResolverSlots)
}

func newHomeDesignerPreviewProvider(displayList *DisplayListHandler, timeout time.Duration, resolverSlots chan struct{}) *homeDesignerPreviewProvider {
	return &homeDesignerPreviewProvider{displayList: displayList, timeout: timeout, resolverSlots: resolverSlots}
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
	timeout := p.timeout
	if timeout <= 0 {
		timeout = homeDesignerPreviewTimeout
	}
	previewCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	group, groupCtx := errgroup.WithContext(previewCtx)
	type rowResult struct {
		index int
		row   homedesigner.PreviewRow
	}
	results := make(chan rowResult, len(rows))
	pending := make([]bool, len(rows))
	remaining := 0
	for index, shelf := range rows {
		index, shelf := index, shelf
		response.Rows[index] = initialPreviewRow(shelf)
		if !shelf.Enabled {
			continue
		}
		pending[index] = true
		remaining++
		group.Go(func() error {
			select {
			case p.resolverSlots <- struct{}{}:
			case <-groupCtx.Done():
				results <- rowResult{index: index, row: previewTimeoutRow(shelf)}
				return nil
			}
			defer func() { <-p.resolverSlots }()

			results <- rowResult{index: index, row: p.resolveRow(groupCtx, sourceReq, request.PreviewProfileID, shelf)}
			return nil
		})
	}
	for remaining > 0 {
		select {
		case result := <-results:
			if pending[result.index] {
				response.Rows[result.index] = result.row
				pending[result.index] = false
				remaining--
			}
		case <-previewCtx.Done():
			for index, shelf := range rows {
				if pending[index] {
					response.Rows[index] = previewTimeoutRow(shelf)
				}
			}
			// Do not wait for group.Wait: a legacy resolver can ignore request
			// cancellation. Its globally capped slot prevents further buildup.
			return response, nil
		}
	}
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
	if !p.displayListSourceAvailable(query.Get("source")) {
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

func previewTimeoutRow(shelf models.ShelfConfig) homedesigner.PreviewRow {
	row := previewErrorRow(shelf)
	row.Message = "Content timed out for this row."
	return row
}

func (p *homeDesignerPreviewProvider) displayListSourceAvailable(source string) bool {
	switch source {
	case "popular-on-server", "recently-watched":
		if p.displayList == nil || p.displayList.MetadataHandler == nil {
			return false
		}
		_, historyOK := p.displayList.MetadataHandler.HistoryService.(sharedShelfHistoryService)
		_, usersOK := p.displayList.MetadataHandler.UsersService.(sharedShelfUsersService)
		return historyOK && usersOK
	default:
		return true
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

// safePreviewArtworkURL permits only public-FQDN http(s) artwork locations
// without credentials, local/private routing, or transport-shaped path/query
// details. It does no DNS lookup: every literal address is rejected before the
// browser can make a request.
func safePreviewArtworkURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ""
	}
	if !previewArtworkHostIsPublic(parsed.Hostname()) || previewArtworkPathIsSensitive(parsed.EscapedPath()) {
		return ""
	}
	if !previewArtworkQueryIsSafe(parsed.RawQuery) {
		return ""
	}
	return parsed.String()
}

func previewArtworkHostIsPublic(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || strings.Contains(host, "%") || host == "localhost" || host == "local" || host == "internal" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".localdomain") || strings.HasSuffix(host, ".home") || strings.HasSuffix(host, ".corp") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	if _, ok := parseNonCanonicalIPv4(host); ok {
		return false
	}
	return previewArtworkFQDNIsPublic(host)
}

func previewArtworkFQDNIsPublic(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for index, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		hasLetter := false
		for _, character := range label {
			if character >= 'a' && character <= 'z' {
				hasLetter = true
				continue
			}
			if !(character >= '0' && character <= '9') && character != '-' {
				return false
			}
		}
		if index == len(labels)-1 && !hasLetter {
			return false
		}
	}
	return true
}

// parseNonCanonicalIPv4 handles legacy integer, hexadecimal, octal, and
// shortened dotted forms without treating them as DNS names.
func parseNonCanonicalIPv4(host string) (netip.Addr, bool) {
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return netip.Addr{}, false
	}
	values := make([]uint64, len(parts))
	for i, part := range parts {
		if part == "" {
			return netip.Addr{}, false
		}
		base := 10
		input := part
		if len(input) > 2 && (strings.HasPrefix(input, "0x") || strings.HasPrefix(input, "0X")) {
			base, input = 16, input[2:]
		} else if len(input) > 1 && strings.HasPrefix(input, "0") {
			base, input = 8, input[1:]
		}
		value, err := strconv.ParseUint(input, base, 32)
		if err != nil {
			return netip.Addr{}, false
		}
		values[i] = value
	}
	var number uint64
	switch len(values) {
	case 1:
		if values[0] > 0xffffffff {
			return netip.Addr{}, false
		}
		number = values[0]
	case 2:
		if values[0] > 0xff || values[1] > 0xffffff {
			return netip.Addr{}, false
		}
		number = values[0]<<24 | values[1]
	case 3:
		if values[0] > 0xff || values[1] > 0xff || values[2] > 0xffff {
			return netip.Addr{}, false
		}
		number = values[0]<<24 | values[1]<<16 | values[2]
	case 4:
		for _, value := range values {
			if value > 0xff {
				return netip.Addr{}, false
			}
		}
		number = values[0]<<24 | values[1]<<16 | values[2]<<8 | values[3]
	}
	return netip.AddrFrom4([4]byte{byte(number >> 24), byte(number >> 16), byte(number >> 8), byte(number)}), true
}

func previewArtworkQueryIsSafe(rawQuery string) bool {
	if rawQuery == "" {
		return true
	}
	if strings.Contains(rawQuery, ";") {
		return false
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false
	}
	for key, entries := range values {
		if len(entries) != 1 || !previewArtworkTransformValueIsSafe(key, entries[0]) {
			return false
		}
	}
	return true
}

func previewArtworkTransformValueIsSafe(key, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch key {
	case "width", "height", "w", "h", "quality", "q", "blur":
		number, err := strconv.ParseUint(value, 10, 16)
		if err != nil || number == 0 {
			return false
		}
		if key == "quality" || key == "q" {
			return number <= 100
		}
		return number <= 10000
	case "dpr":
		return value == "1" || value == "2" || value == "3" || value == "4"
	case "fit":
		return value == "cover" || value == "contain" || value == "fill" || value == "inside" || value == "outside" || value == "clip" || value == "crop"
	case "format":
		return value == "jpg" || value == "jpeg" || value == "png" || value == "webp" || value == "avif"
	case "crop":
		return value == "center" || value == "faces" || value == "entropy" || value == "attention"
	default:
		return false
	}
}

func previewArtworkPathIsSensitive(rawPath string) bool {
	path, err := url.PathUnescape(rawPath)
	if err != nil || strings.Contains(path, "%") {
		return true
	}
	path = strings.ToLower(path)
	for _, marker := range []string{"/library/metadata/", "/playback", "/play/", "/stream", "/source", "/hls", "/manifest", "/download", "/transcode"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	for _, segment := range strings.Split(path, "/") {
		normalized := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, segment)
		for _, marker := range []string{"token", "auth", "credential", "secret", "password", "apikey", "signature", "session", "sig"} {
			if strings.Contains(normalized, marker) {
				return true
			}
		}
	}
	return false
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
