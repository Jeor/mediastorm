package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"novastream/models"
	"novastream/services/homedesigner"
	"novastream/services/watchlist"
)

// TestPreviewHomeDesignerProtectsProfileOwnershipAndRequestBounds catches a
// regression where the preview endpoint invokes a provider for a foreign
// profile or turns an oversized draft into unbounded resolver work.
func TestPreviewHomeDesignerProtectsProfileOwnershipAndRequestBounds(t *testing.T) {
	handler, owned := newHomeDesignerHandler(t)
	provider := &recordingHomeDesignerPreviewProvider{response: homedesigner.PreviewResponse{Rows: []homedesigner.PreviewRow{{
		ID: "watchlist", Name: "Your Watchlist", Layout: "shelf", Status: "ready", Items: []homedesigner.PreviewItem{{ID: "movie:1", Title: "Safe Title", ArtworkURL: "https://images.example/poster.jpg"}}, Total: 1,
	}}}}
	handler.SetHomeDesignerPreviewProvider(provider)
	foreign := profileIDForAccount(t, handler, "account-b")

	for _, test := range []struct {
		name          string
		session       models.Session
		profileID     string
		rows          int
		limit         int
		want          int
		wantCalls     int
		forbiddenText string
	}{
		{name: "owned profile", session: models.Session{AccountID: "account-a"}, profileID: owned.ID, rows: 1, limit: 12, want: http.StatusOK, wantCalls: 1},
		{name: "unowned profile", session: models.Session{AccountID: "account-a"}, profileID: foreign, rows: 1, limit: 12, want: http.StatusNotFound},
		{name: "too many rows", session: models.Session{IsMaster: true}, profileID: owned.ID, rows: 21, limit: 12, want: http.StatusUnprocessableEntity},
		{name: "too many items", session: models.Session{IsMaster: true}, profileID: owned.ID, rows: 1, limit: 13, want: http.StatusUnprocessableEntity},
	} {
		t.Run(test.name, func(t *testing.T) {
			beforeCalls := provider.calls
			payload := previewRequestJSON(t, owned.ID, test.profileID, test.rows, test.limit)
			recorder := httptest.NewRecorder()
			handler.PreviewHomeDesigner(recorder, homeDesignerRequest(http.MethodPost, "/account/api/home-designer/preview", test.session, strings.NewReader(payload)))

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
			if provider.calls-beforeCalls != test.wantCalls {
				t.Fatalf("provider calls = %d, want %d", provider.calls-beforeCalls, test.wantCalls)
			}
			if test.want == http.StatusOK {
				for _, forbidden := range []string{"\\\"path\\\"", "\\\"url\\\"", "\\\"token\\\"", "\\\"apiKey\\\"", "\\\"clientIp\\\"", "credential"} {
					if strings.Contains(recorder.Body.String(), forbidden) {
						t.Fatalf("preview JSON leaked %q: %s", forbidden, recorder.Body.String())
					}
				}
			}
		})
	}
}

// TestHomeDesignerPreviewProviderProjectsSafeItemsAndIsolatesFailures catches
// accidental forwarding of a display-list payload as well as a failed row
// preventing safe neighboring content from rendering.
func TestHomeDesignerPreviewProviderProjectsSafeItemsAndIsolatesFailures(t *testing.T) {
	list := &previewWatchlistService{items: []models.WatchlistItem{{
		ID: "movie:1", Name: "Northwind", MediaType: "movie", Year: 2024,
		PosterURL: "https://artwork.example/northwind.jpg", ExternalIDs: map[string]string{"token": "private-token"}, SyncSource: "provider:credential",
	}}}
	provider := NewHomeDesignerPreviewProvider(&DisplayListHandler{WatchlistService: list})
	response, err := provider.Preview(context.Background(), httptest.NewRequest(http.MethodPost, "/admin/api/home-designer/preview", nil), homedesigner.PreviewRequest{
		Scope:            homedesigner.Scope{Kind: "profile", ProfileID: "profile-a"},
		PreviewProfileID: "profile-a",
		Rows: &homedesigner.SectionMutation[models.HomeShelvesSettings]{Mode: homedesigner.ModeCustom, Value: &models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
			{ID: "watchlist", Name: "Safe row", Enabled: true, Limit: 1},
			{ID: "top-ten", Name: "Unavailable integration", Enabled: true, Limit: 1},
			{ID: "stremio-disabled", Name: "Disabled integration", Type: "stremio", Enabled: true, Limit: 1},
		}}},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(response.Rows) != 3 || response.Rows[0].Status != "ready" || len(response.Rows[0].Items) != 1 {
		t.Fatalf("safe row = %#v", response.Rows)
	}
	if response.Rows[0].Items[0].Title != "Northwind" || response.Rows[0].Items[0].Sample {
		t.Fatalf("safe item = %#v", response.Rows[0].Items[0])
	}
	if response.Rows[1].Status != "error" || len(response.Rows[1].Items) != 0 {
		t.Fatalf("failed row = %#v, want local error without samples", response.Rows[1])
	}
	if response.Rows[2].Status != "error" || len(response.Rows[2].Items) != 0 {
		t.Fatalf("disabled integration = %#v, want error without samples", response.Rows[2])
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	for _, forbidden := range []string{"\\\"path\\\"", "\\\"url\\\"", "\\\"token\\\"", "\\\"apiKey\\\"", "\\\"clientIp\\\"", "credential", "private-token"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("preview JSON leaked %q: %s", forbidden, payload)
		}
	}
}

// TestHomeDesignerPreviewProviderUsesSamplesOnlyForSuccessfulEmptyRows catches
// an empty-result path that would leave the mock layout uninspectable or hide
// an authentication/resolver failure behind fictional content.
func TestHomeDesignerPreviewProviderUsesSamplesOnlyForSuccessfulEmptyRows(t *testing.T) {
	provider := NewHomeDesignerPreviewProvider(&DisplayListHandler{WatchlistService: &previewWatchlistService{}})
	response, err := provider.Preview(context.Background(), httptest.NewRequest(http.MethodPost, "/admin/api/home-designer/preview", nil), homedesigner.PreviewRequest{
		PreviewProfileID: "profile-a",
		Rows: &homedesigner.SectionMutation[models.HomeShelvesSettings]{Mode: homedesigner.ModeCustom, Value: &models.HomeShelvesSettings{Shelves: []models.ShelfConfig{{
			ID: "watchlist", Name: "Empty but valid", Enabled: true, Limit: 2,
		}}}},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	row := response.Rows[0]
	if row.Status != "ready" || row.Total != 0 || len(row.Items) != 2 {
		t.Fatalf("empty row = %#v", row)
	}
	for _, item := range row.Items {
		if !item.Sample || !strings.HasPrefix(item.ArtworkURL, "linear-gradient(") {
			t.Fatalf("sample item = %#v, want deterministic local sample", item)
		}
	}
}

func TestHomeDesignerPreviewProjectionRejectsUnsafeArtworkLocations(t *testing.T) {
	item := previewItemFromTitle(models.Title{
		ID: "movie:unsafe", Name: "Unsafe", MediaType: "movie",
		Poster: &models.Image{URL: "file:///private/library/unsafe.jpg?token=secret"},
	})
	if item.ArtworkURL != "" {
		t.Fatalf("artwork URL = %q, want unsafe local/token location removed", item.ArtworkURL)
	}
}

func TestSafePreviewArtworkURLRejectsPrivateAndCredentialBearingBypasses(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "public artwork", url: "https://cdn.example/images/poster.jpg?width=300", want: true},
		{name: "public artwork without transform", url: "https://images.example-cdn.com/posters/title.jpg", want: true},
		{name: "ipv4 loopback", url: "http://127.0.0.1:32400/poster.jpg"},
		{name: "short ipv4 loopback", url: "http://127.1/poster.jpg"},
		{name: "integer ipv4 loopback", url: "http://2130706433/poster.jpg"},
		{name: "hex ipv4 loopback", url: "http://0x7f000001/poster.jpg"},
		{name: "octal ipv4 loopback", url: "http://0177.0.0.1/poster.jpg"},
		{name: "private ipv4", url: "https://10.0.0.8/poster.jpg"},
		{name: "unspecified ipv4", url: "https://0.0.0.0/poster.jpg"},
		{name: "carrier grade nat ipv4", url: "https://100.64.0.1/poster.jpg"},
		{name: "documentation ipv4", url: "https://192.0.2.1/poster.jpg"},
		{name: "benchmark ipv4", url: "https://198.18.0.1/poster.jpg"},
		{name: "multicast ipv4", url: "https://224.0.0.1/poster.jpg"},
		{name: "reserved ipv4", url: "https://240.0.0.1/poster.jpg"},
		{name: "ipv6 loopback", url: "http://[::1]/poster.jpg"},
		{name: "ipv4 mapped ipv6 loopback", url: "http://[::ffff:127.0.0.1]/poster.jpg"},
		{name: "ipv4 mapped ipv6 carrier grade nat", url: "http://[::ffff:100.64.0.1]/poster.jpg"},
		{name: "link local ipv6", url: "http://[fe80::1]/poster.jpg"},
		{name: "link local ipv6 zone", url: "http://[fe80::1%25en0]/poster.jpg"},
		{name: "unspecified ipv6", url: "http://[::]/poster.jpg"},
		{name: "documentation ipv6", url: "http://[2001:db8::1]/poster.jpg"},
		{name: "benchmark ipv6", url: "https://[2001:2::1]/poster.jpg"},
		{name: "orchidv2 ipv6", url: "https://[2001:20::1]/poster.jpg"},
		{name: "documentation ipv6 current", url: "https://[3fff::1]/poster.jpg"},
		{name: "localhost", url: "https://localhost/poster.jpg"},
		{name: "local hostname", url: "https://media.local/poster.jpg"},
		{name: "lan hostname", url: "https://media.lan/poster.jpg"},
		{name: "internal hostname", url: "https://image.internal/poster.jpg"},
		{name: "single label hostname", url: "https://nas/poster.jpg"},
		{name: "userinfo", url: "https://user:secret@cdn.example/poster.jpg"},
		{name: "access token", url: "https://cdn.example/poster.jpg?access_token=secret"},
		{name: "plex token", url: "https://cdn.example/poster.jpg?X-Plex-Token=secret"},
		{name: "aws credential", url: "https://cdn.example/poster.jpg?X-Amz-Credential=secret"},
		{name: "signature", url: "https://cdn.example/poster.jpg?sig=secret"},
		{name: "unapproved query key", url: "https://cdn.example/poster.jpg?cache=private"},
		{name: "alternate query separator", url: "https://cdn.example/poster.jpg?width=300;sig=secret"},
		{name: "fragment secret", url: "https://cdn.example/poster.jpg#token=secret"},
		{name: "plex playback path", url: "https://cdn.example/library/metadata/1/thumb"},
		{name: "playback path", url: "https://cdn.example/api/playback/1"},
		{name: "stream path", url: "https://cdn.example/stream/source.m3u8"},
		{name: "token path", url: "https://cdn.example/token/secret/poster.jpg"},
		{name: "encoded token path", url: "https://cdn.example/%74oken/secret/poster.jpg"},
		{name: "auth path", url: "https://cdn.example/auth/credential/poster.jpg"},
		{name: "credential path", url: "https://cdn.example/credential/secret/poster.jpg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := safePreviewArtworkURL(test.url) != ""; got != test.want {
				t.Fatalf("safePreviewArtworkURL(%q) allowed = %v, want %v", test.url, got, test.want)
			}
		})
	}
}

func TestHomeDesignerPreviewReturnsAtDeadlineWhenResolverBlocks(t *testing.T) {
	var calls atomic.Int32
	provider := newHomeDesignerPreviewProvider(&DisplayListHandler{WatchlistService: blockingPreviewWatchlist{calls: &calls}}, 40*time.Millisecond, make(chan struct{}, 1))
	request := homedesigner.PreviewRequest{PreviewProfileID: "profile-a", Rows: &homedesigner.SectionMutation[models.HomeShelvesSettings]{Mode: homedesigner.ModeCustom, Value: &models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "watchlist", Name: "First", Enabled: true},
		{ID: "watchlist", Name: "Second", Enabled: true},
		{ID: "watchlist", Name: "Third", Enabled: true},
	}}}}
	started := time.Now()
	response, err := provider.Preview(context.Background(), httptest.NewRequest(http.MethodPost, "/admin/api/home-designer/preview", nil), request)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("preview took %v, want hard deadline despite blocked resolver", elapsed)
	}
	if got := []string{response.Rows[0].Name, response.Rows[1].Name, response.Rows[2].Name}; strings.Join(got, ",") != "First,Second,Third" {
		t.Fatalf("row order = %v", got)
	}
	for _, row := range response.Rows {
		if row.Status != "error" || len(row.Items) != 0 {
			t.Fatalf("timed out row = %#v", row)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("blocked resolver calls = %d, want global slot cap of 1", calls.Load())
	}
	_, _ = provider.Preview(context.Background(), httptest.NewRequest(http.MethodPost, "/admin/api/home-designer/preview", nil), request)
	if calls.Load() != 1 {
		t.Fatalf("second preview bypassed occupied resolver cap: calls = %d", calls.Load())
	}
}

func TestHomeDesignerPreviewDoesNotSampleActivityRowsWithoutDependencies(t *testing.T) {
	provider := NewHomeDesignerPreviewProvider(&DisplayListHandler{MetadataHandler: &MetadataHandler{}})
	response, err := provider.Preview(context.Background(), httptest.NewRequest(http.MethodPost, "/admin/api/home-designer/preview", nil), homedesigner.PreviewRequest{
		PreviewProfileID: "profile-a",
		Rows: &homedesigner.SectionMutation[models.HomeShelvesSettings]{Mode: homedesigner.ModeCustom, Value: &models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
			{ID: "popular-on-server", Name: "Popular", Enabled: true},
			{ID: "recently-watched", Name: "Recent", Enabled: true},
		}}},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	for _, row := range response.Rows {
		if row.Status != "error" || len(row.Items) != 0 {
			t.Fatalf("activity dependency failure = %#v, want error without samples", row)
		}
	}
}

func TestDisplayListQueryForShelfCoversBuiltinsAndProviderRows(t *testing.T) {
	tests := []struct {
		name   string
		shelf  models.ShelfConfig
		source string
	}{
		{name: "watchlist", shelf: models.ShelfConfig{ID: "watchlist"}, source: "watchlist"},
		{name: "continue", shelf: models.ShelfConfig{ID: "continue-watching"}, source: "continue-watching"},
		{name: "top ten", shelf: models.ShelfConfig{ID: "top-ten"}, source: "top-ten"},
		{name: "trending", shelf: models.ShelfConfig{ID: "trending-movies"}, source: "trending"},
		{name: "personalized", shelf: models.ShelfConfig{ID: "my-recommended"}, source: "personalized"},
		{name: "genre", shelf: models.ShelfConfig{ID: "genre-16-movie", Type: "genre"}, source: "genre"},
		{name: "decade", shelf: models.ShelfConfig{ID: "decade-1990-tv", Type: "decade"}, source: "decade"},
		{name: "mdblist", shelf: models.ShelfConfig{Type: "mdblist", ListURL: "https://lists.example/value"}, source: "mdblist"},
		{name: "stremio", shelf: models.ShelfConfig{Type: "stremio", AddonManifestURL: "https://addon.example/manifest", AddonCatalogType: "movie", AddonCatalogID: "top"}, source: "stremio"},
		{name: "tmdb", shelf: models.ShelfConfig{Type: "tmdb", TMDBSourceType: "company", TMDBSourceID: "1"}, source: "tmdb-list"},
		{name: "trakt", shelf: models.ShelfConfig{Type: "trakt", TraktAccountID: "account", TraktListType: "watchlist"}, source: "trakt-list"},
		{name: "simkl", shelf: models.ShelfConfig{Type: "simkl", SimklAccountID: "account", SimklMediaType: "movies"}, source: "simkl-list"},
		{name: "letterboxd", shelf: models.ShelfConfig{Type: "letterboxd", LetterboxdListID: "list"}, source: "letterboxd-list"},
		{name: "popular", shelf: models.ShelfConfig{ID: "popular-on-server"}, source: "popular-on-server"},
		{name: "recent", shelf: models.ShelfConfig{ID: "recently-watched"}, source: "recently-watched"},
		{name: "prequeue", shelf: models.ShelfConfig{ID: "permanent-prequeue"}, source: "permanent-prequeue"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query, ok := displayListQueryForShelf(test.shelf, 12, 12, false, "")
			if !ok || query.Get("source") != test.source {
				t.Fatalf("query = %v, ok = %v, want source %q", query, ok, test.source)
			}
		})
	}
}

func previewRequestJSON(t *testing.T, scopeProfileID, previewProfileID string, rows, limit int) string {
	t.Helper()
	shelves := make([]models.ShelfConfig, rows)
	for i := range shelves {
		shelves[i] = models.ShelfConfig{ID: "watchlist", Name: "Watchlist", Enabled: true, Order: i, Limit: limit}
	}
	payload, err := json.Marshal(homedesigner.PreviewRequest{
		Scope:            homedesigner.Scope{Kind: "profile", ProfileID: scopeProfileID},
		PreviewProfileID: previewProfileID,
		Platform:         "tv",
		Rows:             &homedesigner.SectionMutation[models.HomeShelvesSettings]{Mode: homedesigner.ModeCustom, Value: &models.HomeShelvesSettings{Shelves: shelves}},
	})
	if err != nil {
		t.Fatalf("marshal preview request: %v", err)
	}
	return string(payload)
}

type recordingHomeDesignerPreviewProvider struct {
	calls    int
	response homedesigner.PreviewResponse
}

type previewWatchlistService struct {
	items []models.WatchlistItem
	err   error
}

type blockingPreviewWatchlist struct {
	calls *atomic.Int32
}

func (s blockingPreviewWatchlist) List(string) ([]models.WatchlistItem, error) {
	s.calls.Add(1)
	select {}
}

func (s blockingPreviewWatchlist) AddOrUpdate(string, models.WatchlistUpsert) (models.WatchlistItem, error) {
	return models.WatchlistItem{}, nil
}

func (s blockingPreviewWatchlist) UpdateState(string, string, string, *bool, interface{}) (models.WatchlistItem, error) {
	return models.WatchlistItem{}, nil
}

func (s blockingPreviewWatchlist) Remove(string, string, string) (bool, error) { return false, nil }

func (s blockingPreviewWatchlist) EnrichMissingArtwork([]string, watchlist.ArtworkMetadataProvider) int {
	return 0
}

func (s *previewWatchlistService) List(string) ([]models.WatchlistItem, error) {
	return s.items, s.err
}

func (s *previewWatchlistService) AddOrUpdate(string, models.WatchlistUpsert) (models.WatchlistItem, error) {
	return models.WatchlistItem{}, nil
}

func (s *previewWatchlistService) UpdateState(string, string, string, *bool, interface{}) (models.WatchlistItem, error) {
	return models.WatchlistItem{}, nil
}

func (s *previewWatchlistService) Remove(string, string, string) (bool, error) { return false, nil }

func (s *previewWatchlistService) EnrichMissingArtwork([]string, watchlist.ArtworkMetadataProvider) int {
	return 0
}

func (p *recordingHomeDesignerPreviewProvider) Preview(_ context.Context, _ *http.Request, _ homedesigner.PreviewRequest) (homedesigner.PreviewResponse, error) {
	p.calls++
	return p.response, nil
}
