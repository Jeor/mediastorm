package simkl

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"novastream/config"
	"novastream/models"
)

func testEpisodeUpdate() models.PlaybackProgressUpdate {
	return models.PlaybackProgressUpdate{
		MediaType:     "episode",
		ItemID:        "tvdb:series:153021:s01e03",
		SeriesID:      "tvdb:series:153021",
		SeriesName:    "The Walking Dead",
		SeasonNumber:  1,
		EpisodeNumber: 3,
		ExternalIDs: map[string]string{
			"tvdb": "153021",
			"imdb": "tt1520211",
		},
	}
}

func TestRecentStopSuppressesImmediateWatchedSync(t *testing.T) {
	scrobbler := NewScrobbler(NewClient(), nil)
	update := testEpisodeUpdate()
	scrobbler.noteRecentStop("user-1", update)

	if !scrobbler.wasRecentlyStopped("user-1", "episode", 0, 153021, "", 1, 3) {
		t.Fatal("expected recent stop to suppress matching watched sync")
	}
	if scrobbler.wasRecentlyStopped("user-1", "episode", 0, 153021, "", 1, 4) {
		t.Fatal("different episode should not be suppressed")
	}
}

func TestRecentStopExpires(t *testing.T) {
	scrobbler := NewScrobbler(NewClient(), nil)
	scrobbler.recentStops[recentStopKey("user-1", "movie", "tmdb", "27205", 0, 0)] = time.Now().Add(-10 * time.Minute)

	if scrobbler.wasRecentlyStopped("user-1", "movie", 27205, 0, "", 0, 0) {
		t.Fatal("expired recent stop should not suppress watched sync")
	}
}

func TestShowSyncIDs(t *testing.T) {
	tests := []struct {
		name       string
		tvdbID     int
		externalID map[string]string
		want       IDs
	}{
		{
			name:       "explicit tvdb wins and other IDs are preserved",
			tvdbID:     153021,
			externalID: map[string]string{"tvdb": "999999", "tmdb": "1402", "imdb": "tt1520211", "simkl": "41086"},
			want:       IDs{TVDB: 153021, TMDB: 1402, IMDB: "tt1520211", Simkl: 41086},
		},
		{
			name:       "falls back to external IDs without tvdb param",
			externalID: map[string]string{"tvdb": "401003", "tmdb": "124364", "imdb": "tt9813792", "simkl": "1481305"},
			want:       IDs{TVDB: 401003, TMDB: 124364, IMDB: "tt9813792", Simkl: 1481305},
		},
		{
			name:       "tmdb imdb and simkl work without tvdb",
			externalID: map[string]string{"tmdb": "124364", "imdb": "tt9813792", "simkl": "1481305"},
			want:       IDs{TMDB: 124364, IMDB: "tt9813792", Simkl: 1481305},
		},
		{
			name: "empty map returns zero value",
			want: IDs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := showSyncIDs(tt.tvdbID, tt.externalID); got != tt.want {
				t.Fatalf("showSyncIDs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

type mockSimklUserService struct{ users map[string]models.User }

func (m *mockSimklUserService) Get(id string) (models.User, bool) {
	u, ok := m.users[id]
	return u, ok
}

func TestUnscrobbleEpisodeRemovesSimklHistory(t *testing.T) {
	var body SyncHistoryRequest
	client := NewClient()
	client.SetHTTPClientForTest(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/sync/history/remove" {
			t.Fatalf("path = %s", req.URL.Path)
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})})
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{Simkl: config.SimklSettings{Accounts: []config.SimklAccount{{ID: "simkl-1", ClientID: "client-id", AccessToken: "token"}}}}); err != nil {
		t.Fatal(err)
	}
	scrobbler := NewScrobbler(client, mgr)
	scrobbler.SetUserService(&mockSimklUserService{users: map[string]models.User{"user-1": {ID: "user-1", SimklAccountID: "simkl-1"}}})

	if err := scrobbler.UnscrobbleEpisode("user-1", 153021, 1, 3, map[string]string{"tmdb": "1402"}); err != nil {
		t.Fatal(err)
	}
	if len(body.Shows) != 1 || body.Shows[0].IDs.TVDB != 153021 || len(body.Shows[0].Seasons) != 1 || len(body.Shows[0].Seasons[0].Episodes) != 1 || body.Shows[0].Seasons[0].Episodes[0].Number != 3 {
		t.Fatalf("body=%+v", body)
	}
}
