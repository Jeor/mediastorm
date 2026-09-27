package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestCricketSummaryScorecards(t *testing.T) {
	raw, err := os.ReadFile("testdata/cricket-summary.json")
	if err != nil {
		t.Fatal(err)
	}
	game := models.SportsGame{ID: "1535465", League: "cricket-8048", HomeTeam: models.SportsTeam{ID: "335970", Name: "Royal Challengers Bengaluru", Abbreviation: "RCB"}, AwayTeam: models.SportsTeam{ID: "gujarat", Name: "Gujarat Titans", Abbreviation: "GT"}, Detail: &models.SportsGameDetail{Innings: []models.SportsCricketInnings{{TeamID: "335970", Number: 2}}}}
	got, err := normalizeCricketDetail(game, json.RawMessage(raw), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Detail.CricketScorecards) < 2 || len(got.Detail.Innings) != 1 || !got.Detail.Capabilities.Stats {
		t.Fatal("missing scorecards or dropped innings")
	}
	found := false
	for _, card := range got.Detail.CricketScorecards {
		for _, p := range card.Players {
			if len(p.Stats) == 0 {
				t.Fatal("unplayed player rendered as performance")
			}
			if p.Name == "V Kohli" {
				found = true
				if p.Dismissal != "not out" {
					t.Fatal("dismissal lost")
				}
			}
		}
	}
	if !found {
		t.Fatal("captured batter missing")
	}
	if _, err := normalizeCricketDetail(models.SportsGame{ID: "wrong"}, raw, time.Now()); err == nil {
		t.Fatal("accepted wrong match")
	}
	if _, err := normalizeCricketDetail(game, json.RawMessage(`{"header":{"id":"1535465"},"matchcards":[]}`), time.Now()); err == nil {
		t.Fatal("accepted unavailable cards")
	}
	if game.Detail.CricketScorecards != nil {
		t.Fatal("mutated cached scoreboard detail")
	}
}
