package scrob

import (
	"encoding/json"
	"net/http"
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestBuildManualSessionStartMovie(t *testing.T) {
	request, ok := buildManualSessionStart(models.PlaybackProgressUpdate{
		MediaType: "movie", ItemID: "tmdb:movie:550", MovieName: "Fight Club", Duration: 8340,
	})
	if !ok || request.TMDBID != 550 || request.Runtime != 139 || request.Title != "Fight Club" {
		t.Fatalf("request=%+v ok=%v", request, ok)
	}
}

func TestBuildManualSessionStartEpisodePreservesSpecialSeason(t *testing.T) {
	request, ok := buildManualSessionStart(models.PlaybackProgressUpdate{
		MediaType: "episode", ItemID: "episode", SeriesID: "tmdb:tv:42",
		SeasonNumber: 0, EpisodeNumber: 3, EpisodeName: "Special", Duration: 1439,
		ExternalIDs: map[string]string{"episodeTmdb": "99"},
	})
	if !ok || request.TMDBID != 99 || request.ShowTMDBID != 42 || request.Runtime != 24 {
		t.Fatalf("request=%+v ok=%v", request, ok)
	}
	if request.SeasonNumber == nil || *request.SeasonNumber != 0 || request.EpisodeNumber == nil || *request.EpisodeNumber != 3 {
		t.Fatalf("episode coordinates were not preserved: %+v", request)
	}
}

func TestStopSessionPreservesEligiblePartialProgress(t *testing.T) {
	requests := 0
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Method != http.MethodPatch || r.URL.Path != "/api/proxy/history/session/manual-1-2" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var update ManualSessionUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			t.Fatal(err)
		}
		if update.ProgressSeconds != 600 || update.State != "paused" {
			t.Fatalf("update=%+v", update)
		}
		return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
	})})
	tracker := NewScrobbleStateTracker(client, &Scrobbler{}, 0)
	key := "user:movie:tmdb:550"
	tracker.sessions[key] = &realtimeSession{
		remoteKey: "manual-1-2", token: "jwt",
		account: config.ScrobAccount{BaseURL: "https://scrob.example", APIKey: "key"},
	}
	tracker.StopSession("user", models.PlaybackProgressUpdate{MediaType: "movie", ItemID: "tmdb:550", Position: 600}, 25)
	if requests != 1 || tracker.sessions[key] != nil {
		t.Fatalf("requests=%d session=%+v", requests, tracker.sessions[key])
	}
}
