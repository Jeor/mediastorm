package mdblist

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"novastream/config"
	"novastream/models"
)

func newBulkTestScrobbler(t *testing.T, transport func(*http.Request) (*http.Response, error)) *Scrobbler {
	t.Helper()
	client := NewScrobbleClient("")
	client.httpClient = &http.Client{Transport: roundTripFunc(transport)}
	mgr := newTestConfigManager(t, config.Settings{MDBList: config.MDBListSettings{Accounts: []config.MDBListAccount{{ID: "account", APIKey: "key"}}}})
	s := NewScrobbler(client, mgr)
	s.SetUserService(&mockMDBListUserService{users: map[string]models.User{"user": {ID: "user", MdblistAccountID: "account"}}})
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
				path := "/sync/watched"
				if !watched {
					path += "/remove"
				}
				if r.URL.Path != path {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				requests++
				var req SyncWatchedRequest
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
				if count == 0 || count > 500 {
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
			if requests != 3 || total != 1201 {
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

func TestBulkUnwatchScansAndRemovesPlaysOnce(t *testing.T) {
	reads, removes, syncs := 0, 0, 0
	s := newBulkTestScrobbler(t, func(r *http.Request) (*http.Response, error) {
		body := `{}`
		switch {
		case r.Method == http.MethodGet:
			reads++
			body = `{"episodes":[{"play_id":10,"episode":{"season":1,"number":1,"show":{"ids":{"tmdb":1667}}}},{"play_id":11,"episode":{"season":1,"number":2,"show":{"ids":{"tmdb":1667}}}},{"play_id":12,"episode":{"season":1,"number":3,"show":{"ids":{"tmdb":999}}}}]}`
		case r.URL.Path == "/sync/watched/remove":
			syncs++
		case r.URL.Path == "/sync/watched/plays/remove":
			removes++
			var req struct {
				PlayIDs []int `json:"play_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if len(req.PlayIDs) != 2 || req.PlayIDs[0] != 10 || req.PlayIDs[1] != 11 {
				t.Fatalf("plays=%v", req.PlayIDs)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	items := []models.WatchHistoryItem{
		{MediaType: "episode", SeasonNumber: 1, EpisodeNumber: 1, ExternalIDs: map[string]string{"tmdb": "1667"}},
		{MediaType: "episode", SeasonNumber: 1, EpisodeNumber: 2, ExternalIDs: map[string]string{"tmdb": "1667"}},
	}
	if err := s.SyncWatchHistory("user", items); err != nil {
		t.Fatal(err)
	}
	if syncs != 1 || reads != 1 || removes != 1 {
		t.Fatalf("syncs=%d reads=%d removes=%d", syncs, reads, removes)
	}
}

func TestBulkRetriesOnlyRejectedEpisodes(t *testing.T) {
	calls := 0
	s := newBulkTestScrobbler(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var req SyncWatchedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		eps := req.Shows[0].Seasons[0].Episodes
		body := `{"not_found":{"episodes":[{"season":2,"number":2}]}}`
		if calls == 1 {
			if len(eps) != 2 || eps[0].Number != 1 || eps[1].Number != 2 {
				t.Fatalf("episodes=%v", eps)
			}
		} else {
			if calls != 2 || len(eps) != 1 || eps[0].Number != 27 {
				t.Fatalf("retry episodes=%v", eps)
			}
			body = `{"added":{"episodes":1}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	items := []models.WatchHistoryItem{
		{MediaType: "episode", Watched: true, SeasonNumber: 2, EpisodeNumber: 1, ExternalIDs: map[string]string{"tmdb": "1667", "absoluteEpisode": "26"}},
		{MediaType: "episode", Watched: true, SeasonNumber: 2, EpisodeNumber: 2, ExternalIDs: map[string]string{"tmdb": "1667", "absoluteEpisode": "27"}},
	}
	if err := s.SyncWatchHistory("user", items); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
