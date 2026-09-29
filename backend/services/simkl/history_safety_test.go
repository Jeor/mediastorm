package simkl

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestEpisodeExportSuppressesAutoWatchingAndPreservesExistingShow(t *testing.T) {
	for _, accidentalComplete := range []bool{false, true} {
		t.Run(strconv.FormatBool(accidentalComplete), func(t *testing.T) {
			calls := 0
			client := NewClient()
			client.SetHTTPClientForTest(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				status, body := 200, `{"added":{"shows":0},"not_found":{"episodes":[{"ids":{"tmdb":1},"seasons":[{"number":1,"episodes":[{"number":2}]}]}]}}`
				if r.URL.Path == "/sync/history/remove" {
					status, body = 500, `{"error":"undo failed"}`
				} else {
					if r.URL.Query().Get("skip_auto_watching") != "yes" {
						t.Fatal("episode export may mark the whole series completed")
					}
					if accidentalComplete {
						body = strings.Replace(body, `"shows":0`, `"shows":1`, 1)
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})})
			request := SyncHistoryRequest{Shows: []SyncHistoryShow{{IDs: IDs{TMDB: 1}, Seasons: []SyncHistorySeason{{Number: 1, Episodes: []SyncHistoryEpisode{{Number: 2}}}}}}}
			_, err := client.SyncHistorySafe("client", "token", request)
			if accidentalComplete {
				if err == nil || calls != 2 {
					t.Fatalf("failed undo claimed success: calls=%d err=%v", calls, err)
				}
			} else if err != nil || calls != 1 {
				t.Fatalf("unmatched episode removed preexisting show: calls=%d err=%v", calls, err)
			}
		})
	}
}
