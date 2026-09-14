package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestSoccerTournamentContextCaptured(t *testing.T) {
	raw, err := os.ReadFile("testdata/soccer-context-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		League  string           `json:"league"`
		Summary teamSportSummary `json:"summary"`
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.League, func(t *testing.T) {
			p := fixture.Summary
			game := models.SportsGame{ID: p.Header.ID, League: fixture.League}
			for _, team := range p.Header.Competitions[0].Competitors {
				if team.HomeAway == "home" {
					game.HomeTeam.ID = team.ID
				} else {
					game.AwayTeam.ID = team.ID
				}
			}
			got, err := normalizeTeamDetail(game, p, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if got.Detail.TournamentContext != p.Header.Season.Name {
				t.Fatalf("lost literal label: %q", got.Detail.TournamentContext)
			}
			p.Header.Competitions[0].Status.Type.State = "pre"
			got, err = normalizeTeamDetail(game, p, time.Now())
			if err != nil || got.Detail.TournamentContext != p.Header.Season.Name {
				t.Fatal("pregame lost context", err)
			}
			p.Header.Season.Name = "  "
			got, err = normalizeTeamDetail(game, p, time.Now())
			if err != nil || got.Detail.TournamentContext != "" {
				t.Fatal("inferred missing context", err)
			}
			p.Header.Season.Name = "Synthetic soccer context"
			game.League = "nba"
			got, err = normalizeTeamDetail(game, p, time.Now())
			if err != nil || got.Detail.TournamentContext != "" {
				t.Fatal("leaked soccer context into another sport", err)
			}
		})
	}
}
