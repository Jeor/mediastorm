package simkl

import (
	"encoding/json"
	"io"
	"net/http"
	"novastream/config"
	"novastream/models"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newBulkTestScrobbler(t *testing.T, transport func(*http.Request) (*http.Response, error)) *Scrobbler {
	t.Helper()
	client := NewClient()
	client.SetHTTPClientForTest(&http.Client{Transport: roundTripFunc(transport)})
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{Simkl: config.SimklSettings{Accounts: []config.SimklAccount{{ID: "account", ClientID: "client", AccessToken: "token"}}}}); err != nil {
		t.Fatal(err)
	}
	s := NewScrobbler(client, mgr)
	s.SetUserService(&mockSimklUserService{users: map[string]models.User{"user": {ID: "user", SimklAccountID: "account"}}})
	return s
}
func TestBulkWatchHistoryLargeSeries(t *testing.T) {
	for _, watched := range []bool{true, false} {
		t.Run(map[bool]string{true: "watched", false: "unwatched"}[watched], func(t *testing.T) {
			requests, total := 0, 0
			seen := map[[2]int]bool{}
			stamp := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
			s := newBulkTestScrobbler(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodGet {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
				}
				path := "/sync/history"
				if !watched {
					path += "/remove"
				}
				if r.URL.Path != path {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				requests++
				var req SyncHistoryRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if len(req.Shows) != 1 || req.Shows[0].IDs.TMDB != 1667 {
					t.Fatalf("show identity: %+v", req)
				}
				count := 0
				for _, season := range req.Shows[0].Seasons {
					for _, ep := range season.Episodes {
						key := [2]int{season.Number, ep.Number}
						if seen[key] {
							t.Fatalf("duplicate episode %v", key)
						}
						seen[key] = true
						if watched && ep.WatchedAt != stamp.Format(time.RFC3339) {
							t.Fatalf("timestamp=%q", ep.WatchedAt)
						}
						if !watched && ep.WatchedAt != "" {
							t.Fatalf("unwatch timestamp=%q", ep.WatchedAt)
						}
						count++
					}
				}
				if count == 0 {
					t.Fatalf("batch size %d", count)
				}
				total += count
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"added":{"episodes":500}}`)), Header: make(http.Header)}, nil
			})
			var items []models.WatchHistoryItem
			for i := 0; i < 1201; i++ {
				items = append(items, models.WatchHistoryItem{MediaType: "episode", Watched: watched, WatchedAt: stamp, SeasonNumber: i/25 + 1, EpisodeNumber: i%25 + 1, ExternalIDs: map[string]string{"tmdb": "1667", "tvdb": "76177"}})
			}
			if err := s.SyncWatchHistory("user", items); err != nil {
				t.Fatal(err)
			}
			if requests != 1 || total != 1201 {
				t.Fatalf("requests=%d total=%d", requests, total)
			}
		})
	}
}
func TestBulkWatchHistoryStopsOnProviderError(t *testing.T) {
	calls := 0
	s := newBulkTestScrobbler(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	var items []models.WatchHistoryItem
	for i := 0; i < 1001; i++ {
		items = append(items, models.WatchHistoryItem{MediaType: "episode", Watched: true, SeasonNumber: 1, EpisodeNumber: i + 1, ExternalIDs: map[string]string{"tmdb": "1667"}})
	}
	if err := s.SyncWatchHistory("user", items); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
