package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
)

func TestNHLRecordedPlayerStats(t *testing.T) {
	raw, e := os.ReadFile("testdata/nhl-player-captured.json")
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		Game    models.SportsGame `json:"game"`
		Summary teamSportSummary  `json:"summary"`
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	rows := normalizePlayerGameStats(p.Game, p.Summary.Boxscore.Players)
	if len(rows) != 38 {
		t.Fatalf("want 36 skaters and two goalies, got %d", len(rows))
	}
	goalies := 0
	for _, r := range rows {
		if r.Category == "Goalie" {
			goalies++
			if len(r.Stats) != 5 {
				t.Fatalf("goalie columns: %+v", r)
			}
		}
		for _, s := range r.Stats {
			if s.Label == "SOG" {
				t.Fatal("shootout goals presented as shots on goal")
			}
			if s.Label == "YTDG" {
				t.Fatal("season total leaked")
			}
		}
	}
	if goalies != 2 {
		t.Fatal("missing goalie")
	}
	p.Game.Status = models.SportsGameScheduled
	if len(normalizePlayerGameStats(p.Game, p.Summary.Boxscore.Players)) != 0 {
		t.Fatal("pregame leaked stats")
	}
}
