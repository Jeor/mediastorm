package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"novastream/models"
	"novastream/services/customlists"
	"novastream/services/users"

	"github.com/gorilla/mux"
)

type customListShelfHistory struct {
	historyService
	items []models.WatchHistoryItem
}

func (h customListShelfHistory) ListWatchHistory(string) ([]models.WatchHistoryItem, error) {
	return h.items, nil
}
func (h customListShelfHistory) ListSeriesStates(string) ([]models.SeriesWatchState, error) {
	return nil, nil
}
func (h customListShelfHistory) ListPlaybackProgress(string) ([]models.PlaybackProgress, error) {
	return nil, nil
}

func TestProfileCustomListsEnforcesAccountScope(t *testing.T) {
	dir := t.TempDir()
	profiles, err := users.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := profiles.CreateForAccount("account-a", "Alex")
	if err != nil {
		t.Fatal(err)
	}
	other, err := profiles.CreateForAccount("account-b", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	lists, err := customlists.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	list, err := lists.CreateList(owner.ID, "Favorites")
	if err != nil {
		t.Fatal(err)
	}
	h := &AdminUIHandler{usersService: profiles}
	handler := h.ProfileCustomLists(NewCustomListsHandler(lists, profiles))
	for _, tc := range []struct {
		name, profile string
		master        bool
		status        int
	}{
		{"own profile", owner.ID, false, http.StatusOK},
		{"other account", other.ID, false, http.StatusNotFound},
		{"admin", owner.ID, true, http.StatusOK},
		{"unknown profile", "missing", true, http.StatusNotFound},
		{"missing profile", "", true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sportsAdminRequest(http.MethodGet, "/account/api/profiles/"+tc.profile+"/custom-lists", "", tc.master, "account-a")
			r = mux.SetURLVars(r, map[string]string{"userID": tc.profile})
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == http.StatusOK {
				var got []models.CustomList
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if len(got) != 1 || got[0].ID != list.ID || got[0].Name != "Favorites" {
					t.Fatalf("unexpected lists: %+v", got)
				}
			}
		})
	}
}

func TestCustomListShelfTransportAndStartup(t *testing.T) {
	lists, err := customlists.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	list, err := lists.CreateList("owner", "Favorites")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []models.WatchlistUpsert{
		{ID: "movie-1", Name: "Released Movie", MediaType: "movie", Year: 1999, PosterURL: "https://example.com/poster.jpg", ExternalIDs: map[string]string{"tmdb": "42"}},
		{ID: "series-1", Name: "Released Show", MediaType: "series", Year: 2000, Status: models.SeriesReleaseStatusReleased},
		{ID: "movie-2", Name: "Future Movie", MediaType: "movie", Year: 2099},
	} {
		if _, err := lists.AddItem("owner", list.ID, item); err != nil {
			t.Fatal(err)
		}
	}
	h := NewDisplayListHandler(nil, lists, nil)
	h.SetHistoryService(customListShelfHistory{items: []models.WatchHistoryItem{
		{MediaType: "movie", ItemID: "tmdb:movie:42", Watched: true, ExternalIDs: map[string]string{"tmdb": "42"}},
	}})
	shelf := models.ShelfConfig{ID: "shelf", Name: "Favorites", Type: "mdblist", ListURL: customListShelfURLPrefix + list.ID, Enabled: true}
	if !isStartupFetchableCustomShelf(shelf) {
		t.Fatal("custom list shelf is not eligible for startup")
	}
	query, ok := startupDisplayListQueryForShelf(shelf, 10, false, "")
	if !ok {
		t.Fatal("startup query unavailable")
	}
	for _, tc := range []struct {
		name, user, extra    string
		status, total, count int
	}{
		{"startup", "owner", "", 200, 3, 3},
		{"pagination", "owner", "&limit=1&offset=1", 200, 3, 1},
		{"movies", "owner", "&homeView=movies&limit=1", 200, 2, 1},
		{"shows", "owner", "&homeView=shows", 200, 1, 1},
		{"hide unreleased", "owner", "&hideUnreleased=true", 200, 2, 2},
		{"hide watched", "owner", "&hideWatched=true", 200, 2, 2},
		{"another profile", "other", "", 404, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := query.Encode()
			// Replace rather than duplicate query parameters from startup.
			params, _ := url.ParseQuery(q)
			extra, _ := url.ParseQuery(tc.extra)
			for key, values := range extra {
				params[key] = values
			}
			r := httptest.NewRequest(http.MethodGet, "/display-list?"+params.Encode(), nil)
			r = mux.SetURLVars(r, map[string]string{"userID": tc.user})
			w := httptest.NewRecorder()
			h.Get(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status != 200 {
				return
			}
			var got struct {
				Source string                `json:"source"`
				Items  []models.TrendingItem `json:"items"`
				Total  int                   `json:"total"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Source != "custom-list" || got.Total != tc.total || len(got.Items) != tc.count {
				t.Fatalf("unexpected shelf response: %+v", got)
			}
			for _, item := range got.Items {
				if item.Title.Name == "" || item.Title.ID == "" {
					t.Fatal("shelf items must use the nested title format")
				}
				if item.Title.Name == "Released Movie" && (item.Title.TMDBID != 42 || item.Title.Poster == nil) {
					t.Fatal("shelf conversion lost artwork or provider identity")
				}
			}
		})
	}
	r := httptest.NewRequest(http.MethodGet, "/display-list?source=mdblist&url="+url.QueryEscape(customListShelfURLPrefix), nil)
	r = mux.SetURLVars(r, map[string]string{"userID": "owner"})
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty sentinel must fail locally: %d", w.Code)
	}
}
