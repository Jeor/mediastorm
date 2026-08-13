package scrob

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"novastream/config"
	"novastream/models"
)

func TestScrobEpisodeEventUsesShowCoordinates(t *testing.T) {
	when := time.Date(2026, 8, 8, 12, 0, 0, 0, time.FixedZone("test", -6*60*60))
	event, ok := scrobEpisodeEvent(81797, 23, 8, when, map[string]string{"tmdb": "37854", "episodeTmdb": "7124432"})
	if !ok {
		t.Fatal("expected event")
	}
	if event.SeriesTMDBID != 37854 || event.SeriesTVDBID != 81797 || event.TMDBID != 7124432 || event.SeasonNumber != 23 || event.EpisodeNumber != 8 {
		t.Fatalf("event=%+v", event)
	}
	if event.WatchedAt == nil || !event.WatchedAt.Equal(when.UTC()) {
		t.Fatalf("watchedAt=%v", event.WatchedAt)
	}
}

func TestScrobEpisodeEventRequiresShowTMDBID(t *testing.T) {
	if _, ok := scrobEpisodeEvent(81797, 1, 2, time.Now(), map[string]string{"tvdb": "81797"}); ok {
		t.Fatal("expected TVDB-only episode to be skipped because Scrob requires series_tmdb_id")
	}
}

type mockScrobUserService struct{ users map[string]models.User }

func (m *mockScrobUserService) Get(id string) (models.User, bool) {
	u, ok := m.users[id]
	return u, ok
}

func TestUnscrobbleEpisodeDeletesByEpisodeTMDBID(t *testing.T) {
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/proxy/auth/login":
			return jsonResponse(http.StatusOK, `{"access_token":"jwt"}`), nil
		case "/api/proxy/history/item":
			if req.Method != http.MethodDelete || req.URL.Query().Get("tmdb_id") != "7124432" || req.URL.Query().Get("media_type") != "episode" {
				t.Fatalf("request=%s %s query=%v", req.Method, req.URL.Path, req.URL.Query())
			}
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})})
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{Scrob: config.ScrobSettings{Accounts: []config.ScrobAccount{{ID: "scrob-1", BaseURL: "https://scrob.example", APIKey: "key", Username: "user", Password: "pass"}}}}); err != nil {
		t.Fatal(err)
	}
	scrobbler := NewScrobbler(client, mgr)
	scrobbler.SetUserService(&mockScrobUserService{users: map[string]models.User{"user-1": {ID: "user-1", ScrobAccountID: "scrob-1"}}})
	if err := scrobbler.UnscrobbleEpisode("user-1", 81797, 23, 8, map[string]string{"episodeTmdb": "7124432"}); err != nil {
		t.Fatal(err)
	}
}

func TestUnscrobbleEpisodeLooksUpScrobMediaIDWhenEpisodeTMDBIDMissing(t *testing.T) {
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/proxy/history":
			return jsonResponse(http.StatusOK, `{"page":1,"total_pages":1,"results":[{"id":3404,"media":{"id":10997,"type":"episode","show_tmdb_id":4629,"show_tvdb_id":72449,"season_number":4,"episode_number":13}}]}`), nil
		case "/api/proxy/auth/login":
			return jsonResponse(http.StatusOK, `{"access_token":"jwt"}`), nil
		case "/api/proxy/history/item":
			if req.Method != http.MethodDelete || req.URL.Query().Get("id") != "10997" || req.URL.Query().Get("media_type") != "episode" {
				t.Fatalf("request=%s %s query=%v", req.Method, req.URL.Path, req.URL.Query())
			}
			return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})})
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{Scrob: config.ScrobSettings{Accounts: []config.ScrobAccount{{ID: "scrob-1", BaseURL: "https://scrob.example", APIKey: "key", Username: "user", Password: "pass"}}}}); err != nil {
		t.Fatal(err)
	}
	scrobbler := NewScrobbler(client, mgr)
	scrobbler.SetUserService(&mockScrobUserService{users: map[string]models.User{"user-1": {ID: "user-1", ScrobAccountID: "scrob-1"}}})
	if err := scrobbler.UnscrobbleEpisode("user-1", 72449, 4, 13, map[string]string{"tmdb": "4629"}); err != nil {
		t.Fatal(err)
	}
}
