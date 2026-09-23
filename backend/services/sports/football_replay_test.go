package sports

import (
	"encoding/json"
	"novastream/models"
	"testing"
)

func TestFootballReplayMetadata(t *testing.T) {
	var summary teamSportSummary
	err := json.Unmarshal([]byte(`{"drives":{"previous":[{"id":"d","team":{"id":"a"},"offensivePlays":2,"yards":0,"timeElapsed":{"displayValue":"1:20"},"plays":[{"id":"p","type":{"id":"5","text":"Rush"},"statYardage":0,"wallclock":"2026-09-19T21:30:00Z","start":{"possessionText":"AWY 0","team":{"id":"a"},"down":1,"distance":10},"end":{"possessionText":"AWY 0"}},{"id":"unknown","type":{"id":"8","text":"Penalty"}}]}]}}`), &summary)
	if err != nil {
		t.Fatal(err)
	}
	game := models.SportsGame{League: "college-football", Status: models.SportsGameLive}
	game.AwayTeam.ID = "a"
	game.AwayTeam.Abbreviation = "AWY"
	game.HomeTeam.ID = "h"
	game.HomeTeam.Abbreviation = "HOM"
	rows := normalizeFootballDrives(game, summary)
	if len(rows) != 1 || len(rows[0].Plays) != 2 {
		t.Fatal(rows)
	}
	if rows[0].Plays[0].Start == nil || *rows[0].Plays[0].Start != 0 {
		t.Fatal("zero coordinate lost")
	}
	if rows[0].Plays[1].Start != nil {
		t.Fatal("invented unknown coordinate")
	}
	data, _ := json.Marshal(rows[0])
	var row map[string]any
	json.Unmarshal(data, &row)
	plays := row["plays"].([]any)
	play := plays[0].(map[string]any)
	for key, want := range map[string]any{"typeId": "5", "yards": float64(0), "down": float64(1), "distance": float64(10), "wallclock": "2026-09-19T21:30:00Z", "possessionTeamId": "a"} {
		if play[key] != want {
			t.Errorf("%s: got %v want %v", key, play[key], want)
		}
	}
	if row["playCount"] != float64(2) || row["yards"] != float64(0) || row["elapsed"] != "1:20" {
		t.Errorf("drive totals lost: %v", row)
	}
	if _, ok := plays[1].(map[string]any)["yards"]; ok {
		t.Fatal("unknown yardage became zero")
	}
}
