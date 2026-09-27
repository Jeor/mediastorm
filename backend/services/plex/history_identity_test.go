package plex

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWatchHistoryUsesShowIdentity(t *testing.T) {
	for _, tc := range []struct {
		name          string
		historyParent string
		detailsParent string
		episodeFails  bool
		showFails     bool
		showType      string
		wantShow      bool
	}{
		{name: "parent only in episode metadata", detailsParent: "show", showType: "show", wantShow: true},
		{name: "parent in history", historyParent: "show", showType: "show", wantShow: true},
		{name: "episode unavailable but history has parent", historyParent: "show", episodeFails: true, showType: "show", wantShow: true},
		{name: "show unavailable", detailsParent: "show", showFails: true},
		{name: "no parent available"},
		{name: "parent is not a show", detailsParent: "show", showType: "episode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			http.DefaultTransport = plexRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				status := http.StatusOK
				var body string
				switch req.URL.Path {
				case "/library/metadata/episode":
					if tc.episodeFails {
						status = http.StatusNotFound
					}
					body = `{"MediaContainer":{"Metadata":[{"type":"episode","grandparentRatingKey":"` + tc.detailsParent + `","Guid":[{"id":"tmdb://7269518"},{"id":"tvdb://11900000"}]}]}}`
				case "/library/metadata/show":
					if tc.showFails {
						status = http.StatusNotFound
					}
					body = `{"MediaContainer":{"Metadata":[{"type":"` + tc.showType + `","Guid":[{"id":"tmdb://287620"},{"id":"tvdb://465664"}]}]}}`
				case "/library/metadata/movie":
					body = `{"MediaContainer":{"Metadata":[{"type":"movie","Guid":[{"id":"tmdb://42"}]}]}}`
				default:
					t.Errorf("unexpected metadata request: %s", req.URL.Path)
					status = http.StatusNotFound
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			history := []WatchHistoryItem{
				{Type: "episode", RatingKey: "episode", GrandparentRatingKey: tc.historyParent,
					Index: 3, ParentIndex: 1, ViewedAt: 12345, ExternalIDs: map[string]string{"tmdb": "7269518"}},
				{Type: "movie", RatingKey: "movie"},
			}
			server := PlexResource{Connections: []PlexConnection{{URI: "https://plex.example", Protocol: "https", Local: true}}}
			NewClient("test").fetchDetailsParallel(server, history, nil)
			if tc.wantShow {
				if history[0].ExternalIDs["tmdb"] != "287620" || history[0].ExternalIDs["tvdb"] != "465664" {
					t.Fatalf("expected show IDs, got %#v", history[0].ExternalIDs)
				}
			} else if len(history[0].ExternalIDs) != 0 {
				t.Fatalf("unresolved show retained episode IDs: %#v", history[0].ExternalIDs)
			}
			if history[0].Index != 3 || history[0].ParentIndex != 1 || history[0].ViewedAt != 12345 {
				t.Fatalf("episode coordinates or watched timestamp changed: %#v", history[0])
			}
			if history[1].ExternalIDs["tmdb"] != "42" {
				t.Fatalf("movie identity changed: %#v", history[1])
			}
		})
	}
}
