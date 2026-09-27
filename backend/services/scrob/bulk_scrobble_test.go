package scrob

import (
	"encoding/json"
	"net/http"
	"novastream/config"
	"novastream/models"
	"path/filepath"
	"testing"
	"time"
)

func newBulkTestScrobbler(t *testing.T, transport func(*http.Request) (*http.Response, error)) *Scrobbler {
	t.Helper()
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(transport)})
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{Scrob: config.ScrobSettings{Accounts: []config.ScrobAccount{{ID: "account", BaseURL: "https://scrob.example", APIKey: "key", Username: "user", Password: "pass"}}}}); err != nil {
		t.Fatal(err)
	}
	s := NewScrobbler(client, mgr)
	s.SetUserService(&mockScrobUserService{users: map[string]models.User{"user": {ID: "user", ScrobAccountID: "account"}}})
	return s
}
func TestScopedBulkUsesNativeScrobEndpoints(t *testing.T) {
	for _, scope := range []string{"show", "season"} {
		for _, watched := range []bool{true, false} {
			t.Run(scope+map[bool]string{true: "Watched", false: "Unwatched"}[watched], func(t *testing.T) {
				logins, changes := 0, 0
				s := newBulkTestScrobbler(t, func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/api/proxy/auth/login" {
						logins++
						return jsonResponse(200, `{"access_token":"jwt"}`), nil
					}
					changes++
					path := "/api/proxy/history/show-all"
					if scope == "season" {
						path = "/api/proxy/history/season"
					}
					if r.URL.Path != path {
						t.Fatalf("path=%s", r.URL.Path)
					}
					if watched {
						if r.Method != http.MethodPost {
							t.Fatalf("method=%s", r.Method)
						}
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						if body["series_tmdb_id"] != float64(1667) {
							t.Fatalf("body=%v", body)
						}
						if scope == "season" && (body["season_number"] != float64(1) || body["episode_order"] != "tvdb:official") {
							t.Fatalf("body=%v", body)
						}
					} else {
						if r.Method != http.MethodDelete || r.URL.Query().Get("series_tmdb_id") != "1667" {
							t.Fatalf("request=%s %s", r.Method, r.URL)
						}
						if scope == "season" && r.URL.Query().Get("season_number") != "1" {
							t.Fatal("missing season")
						}
					}
					return jsonResponse(200, `{}`), nil
				})
				var items []models.WatchHistoryItem
				for i := 1; i <= 1201; i++ {
					items = append(items, models.WatchHistoryItem{MediaType: "episode", Watched: watched, WatchedAt: time.Now(), SeasonNumber: 1, EpisodeNumber: i, ExternalIDs: map[string]string{"tmdb": "1667", "tvdb": "76177"}})
				}
				if err := s.SyncScopedWatchHistory("user", items, scope); err != nil {
					t.Fatal(err)
				}
				if logins != 1 || changes != 1 {
					t.Fatalf("logins=%d changes=%d", logins, changes)
				}
			})
		}
	}
}
func TestPartialScrobBulkReusesLoginAndHistory(t *testing.T) {
	logins, reads, deletes := 0, 0, 0
	s := newBulkTestScrobbler(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/proxy/auth/login":
			logins++
			return jsonResponse(200, `{"access_token":"jwt"}`), nil
		case "/api/proxy/history":
			reads++
			return jsonResponse(200, `{"page":1,"total_pages":1,"results":[{"media":{"id":1,"type":"episode","show_tmdb_id":1667,"season_number":1,"episode_number":1}},{"media":{"id":2,"type":"episode","show_tmdb_id":1667,"season_number":1,"episode_number":2}}]}`), nil
		case "/api/proxy/history/item":
			deletes++
			if r.Method != http.MethodDelete {
				t.Fatal(r.Method)
			}
			return jsonResponse(200, `{}`), nil
		default:
			t.Fatalf("unexpected endpoint %s", r.URL.Path)
			return nil, nil
		}
	})
	items := []models.WatchHistoryItem{
		{MediaType: "episode", SeasonNumber: 1, EpisodeNumber: 1, ExternalIDs: map[string]string{"tmdb": "1667"}},
		{MediaType: "episode", SeasonNumber: 1, EpisodeNumber: 2, ExternalIDs: map[string]string{"tmdb": "1667"}},
	}
	if err := s.SyncWatchHistory("user", items); err != nil {
		t.Fatal(err)
	}
	if logins != 1 || reads != 1 || deletes != 2 {
		t.Fatalf("login=%d reads=%d deletes=%d", logins, reads, deletes)
	}
}
