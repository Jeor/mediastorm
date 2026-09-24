package sports

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestActivatedOptionalProviderFixtures(t *testing.T) {
	for _, league := range LeagueCatalog {
		if league.Provider != "espn" || !league.active() {
			continue
		}
		for _, capability := range []string{"standings", "summary", "team-identities"} {
			if !league.hasCapability(capability) {
				continue
			}
			t.Run(league.ID+"/"+capability, func(t *testing.T) {
				kind := capability
				if kind == "team-identities" {
					kind = "teams"
				}
				data, err := os.ReadFile(filepath.Join("../../../docs/sports-coverage/evidence/optional", league.Sport+"--"+league.Slug+"--"+kind+".json"))
				if err != nil {
					t.Fatal(err)
				}
				service := NewService(t.TempDir())
				service.client = &http.Client{Transport: coverageTransport(func(r *http.Request) (*http.Response, error) { return coverageResponse(200, string(data)), nil })}
				switch capability {
				case "standings":
					var raw standingsResponse
					if err = json.Unmarshal(data, &raw); err != nil {
						t.Fatal(err)
					}
					if len(normalizeLeagueStandings(raw, league.ID)) == 0 {
						t.Fatal("no usable standings")
					}
				case "team-identities":
					teams, err := service.fetchLeagueTeams(context.Background(), league)
					if err != nil || len(teams) == 0 {
						t.Fatalf("identities=%d err=%v", len(teams), err)
					}
				case "summary":
					fixture, err := os.ReadFile(filepath.Join("testdata/coverage", league.Sport+"--"+league.Slug+".json"))
					if err != nil {
						t.Fatal(err)
					}
					var board espnScoreboardResponse
					json.Unmarshal(fixture, &board)
					if len(board.Events) == 0 {
						t.Fatal("missing score fixture")
					}
					games := scoreboardEventGames(board.Events[0], league)
					if len(games) == 0 {
						t.Fatal("missing match")
					}
					// Captured summaries may select a different match than the first board row.
					var header struct {
						Header struct {
							ID string `json:"id"`
						} `json:"header"`
					}
					json.Unmarshal(data, &header)
					for _, event := range board.Events {
						if event.ID == header.Header.ID {
							games = scoreboardEventGames(event, league)
							break
						}
					}
					if games[0].ProviderEventID != header.Header.ID {
						t.Skip("summary event not among compact scoreboard fixtures")
					}
					got := service.EnrichGame(context.Background(), games[0])
					if got.Detail == nil {
						t.Fatal("summary not normalized")
					}
				}
			})
		}
	}
}
