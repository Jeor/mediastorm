package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"novastream/config"
	"novastream/handlers"
	"novastream/models"
)

// Personal watchlists must retain saved titles even when discovery lists hide
// upcoming titles or the saved record predates release-status metadata.
func TestPersonalWatchlistIgnoresReleaseVisibility(t *testing.T) {
	items := []models.WatchlistItem{
		{ID: "released-movie", MediaType: "movie", Name: "Released movie", Status: "released"},
		{ID: "upcoming-movie", MediaType: "movie", Name: "Upcoming movie", Status: "upcoming"},
		{ID: "unknown-movie", MediaType: "movie", Name: "Unknown movie"},
		{ID: "released-series", MediaType: "series", Name: "Released series", Status: "released"},
		{ID: "unreleased-series", MediaType: "series", Name: "Unreleased series", Status: "unreleased"},
		{ID: "unknown-series", MediaType: "series", Name: "Unknown series"},
	}
	cfg := config.NewManager(t.TempDir() + "/settings.json")
	settings := config.DefaultSettings()
	settings.Display.IncludeUnreleasedMoviesInLists = false
	settings.Display.IncludeUnreleasedShowsInLists = false
	if err := cfg.Save(settings); err != nil {
		t.Fatal(err)
	}
	profiles := &mockUserSettingsService{settings: &models.UserSettings{Display: models.DisplaySettings{
		IncludeUnreleasedMoviesInLists: models.BoolPtr(false),
		IncludeUnreleasedShowsInLists:  models.BoolPtr(false),
	}}}
	users := &mockUserServiceStartup{exists: true}
	wl := &mockWatchlistService{items: items}
	display := handlers.NewDisplayListHandler(wl, nil, users)
	display.MetadataHandler = &handlers.MetadataHandler{CfgManager: cfg, UserSettings: profiles}
	startup := handlers.NewStartupHandler(profiles, wl, &mockHistoryService{}, &mockMetadataServiceStartup{}, cfg, users)
	for _, tc := range []struct {
		name, path string
		handler    http.HandlerFunc
		startup    bool
	}{
		{"display", "/display-list?source=watchlist", display.Get, false},
		{"default-source", "/display-list", display.Get, false},
		{"startup", "/startup?includeTrendingMovies=false&includeTrendingSeries=false&hideUnreleased=true", startup.GetStartup, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/users/user1"+tc.path, nil)
			req = mux.SetURLVars(req, map[string]string{"userID": "user1"})
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var got []models.WatchlistItem
			var total int
			if tc.startup {
				var response handlers.StartupResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				got, total = response.Watchlist, response.WatchlistTotal
			} else {
				var response struct {
					Items []models.WatchlistItem `json:"items"`
					Total int                    `json:"total"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				got, total = response.Items, response.Total
			}
			if len(got) != len(items) || total != len(items) {
				t.Fatalf("got %d items / total %d; want %d regardless of release status", len(got), total, len(items))
			}
			ids := map[string]bool{}
			for _, item := range got {
				ids[item.ID] = true
			}
			for _, item := range items {
				if !ids[item.ID] {
					t.Errorf("saved title %s missing", item.ID)
				}
			}
		})
	}
}
