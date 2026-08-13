package trakt

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

type traktRoundTripFunc func(*http.Request) (*http.Response, error)

func (f traktRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestShowSyncIDs(t *testing.T) {
	tests := []struct {
		name       string
		tvdbID     int
		externalID map[string]string
		want       SyncIDs
	}{
		{
			name:       "explicit tvdb wins and other IDs are preserved",
			tvdbID:     121361,
			externalID: map[string]string{"tvdb": "999999", "tmdb": "1399", "imdb": "tt0944947", "trakt": "353"},
			want:       SyncIDs{TVDB: 121361, TMDB: 1399, IMDB: "tt0944947", Trakt: 353},
		},
		{
			name:       "falls back to external IDs without tvdb param",
			externalID: map[string]string{"tvdb": "401003", "tmdb": "124364", "imdb": "tt9813792", "trakt": "164767"},
			want:       SyncIDs{TVDB: 401003, TMDB: 124364, IMDB: "tt9813792", Trakt: 164767},
		},
		{
			name:       "tmdb imdb and trakt work without tvdb",
			externalID: map[string]string{"tmdb": "124364", "imdb": "tt9813792", "trakt": "164767"},
			want:       SyncIDs{TMDB: 124364, IMDB: "tt9813792", Trakt: 164767},
		},
		{
			name: "empty map returns zero value",
			want: SyncIDs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShowSyncIDs(tt.tvdbID, tt.externalID); got != tt.want {
				t.Fatalf("ShowSyncIDs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUnscrobbleEpisodeRemovesTraktHistory(t *testing.T) {
	var body SyncHistoryRequest
	client := NewClient("client-id", "secret")
	client.SetHTTPClientForTest(&http.Client{Transport: traktRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/sync/history/remove" {
			t.Fatalf("request = %s %s", req.Method, req.URL.Path)
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"deleted":{"episodes":1}}`)), Header: make(http.Header)}, nil
	})})
	mgr := newTestConfigManager(t, config.Settings{Trakt: config.TraktSettings{Accounts: []config.TraktAccount{{
		ID: "trakt-1", ClientID: "client-id", ClientSecret: "secret", AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour).Unix(), ScrobblingEnabled: true,
	}}}})
	scrobbler := NewScrobbler(client, mgr)
	scrobbler.SetUserService(&mockTraktUserService{users: map[string]models.User{"user-1": {ID: "user-1", TraktAccountID: "trakt-1"}}})

	if err := scrobbler.UnscrobbleEpisode("user-1", 121361, 1, 2, map[string]string{"tmdb": "1399", "episodeTmdb": "63056"}); err != nil {
		t.Fatal(err)
	}
	if len(body.Shows) != 1 || body.Shows[0].IDs.TVDB != 121361 || len(body.Shows[0].Seasons) != 1 || len(body.Shows[0].Seasons[0].Episodes) != 1 {
		t.Fatalf("body=%+v", body)
	}
	episode := body.Shows[0].Seasons[0].Episodes[0]
	if episode.Number != 2 || episode.IDs.TMDB != 63056 || episode.WatchedAt != "" {
		t.Fatalf("episode=%+v", episode)
	}
}
