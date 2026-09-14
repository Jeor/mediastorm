package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
)

func TestExpandedCapturedPlayerColumns(t *testing.T) {
	for _, tc := range []struct{ file, league, category, label string }{
		{"college-football", "college-football", "Defense", "TOT"},
		{"college-football", "college-football", "Kicking", "FG"},
		{"mens-college-basketball", "mens-college-basketball", "Game stats", "FG"},
		{"womens-college-basketball", "womens-college-basketball", "Game stats", "STL"},
		{"nhl-player", "nhl", "Skater", "BS"},
	} {
		t.Run(tc.league+tc.label, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + tc.file + "-captured.json")
			if err != nil {
				t.Fatal(err)
			}
			var p struct {
				Summary teamSportSummary `json:"summary"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				t.Fatal(err)
			}
			if len(p.Summary.Boxscore.Players) < 2 {
				t.Fatal("missing teams")
			}
			game := models.SportsGame{League: tc.league, Status: models.SportsGameFinal}
			game.AwayTeam.ID = p.Summary.Boxscore.Players[0].Team.ID
			game.HomeTeam.ID = p.Summary.Boxscore.Players[1].Team.ID
			found := false
			for _, row := range normalizePlayerGameStats(game, p.Summary.Boxscore.Players) {
				for _, stat := range row.Stats {
					if row.Category == tc.category && stat.Label == tc.label {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("missing verified %s %s", tc.category, tc.label)
			}
		})
	}
}

func TestSoccerCapturedMatchStats(t *testing.T) {
	raw, err := os.ReadFile("testdata/eng.1-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Summary teamSportSummary `json:"summary"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	game := models.SportsGame{League: "soccer-eng.1", Status: models.SportsGameFinal}
	game.HomeTeam.ID = p.Summary.Rosters[0].Team.ID
	game.AwayTeam.ID = p.Summary.Rosters[1].Team.ID
	rows := normalizeSoccerPlayerStats(game, p.Summary.Rosters)
	if len(rows) < 22 {
		t.Fatalf("expected starters and used substitutes, got %d", len(rows))
	}
	for _, roster := range p.Summary.Rosters {
		for _, player := range roster.Roster {
			for _, stat := range player.Stats {
				if stat.Name == "appearances" && stat.DisplayValue == "0" {
					for _, row := range rows {
						if row.ID == player.Athlete.ID {
							t.Fatal("unused substitute gained match stats")
						}
					}
				}
			}
		}
	}
	game.Status = models.SportsGameScheduled
	if len(normalizeSoccerPlayerStats(game, p.Summary.Rosters)) != 0 {
		t.Fatal("pregame player stats leaked")
	}
	game.Status = models.SportsGameFinal
	game.AwayTeam.ID = "unknown"
	game.HomeTeam.ID = "unknown"
	if len(normalizeSoccerPlayerStats(game, p.Summary.Rosters)) != 0 {
		t.Fatal("wrong team matched")
	}
}

func TestRaceLapsLedScope(t *testing.T) {
	var stats raceStatistics
	if err := json.Unmarshal([]byte(`{"splits":{"categories":[{"stats":[{"name":"lapsLead","value":0,"displayValue":"0"},{"name":"behindLaps","value":-128,"displayValue":"-128"}]}]}}`), &stats); err != nil {
		t.Fatal(err)
	}
	var driver models.SportsParticipant
	applyRaceStatistics(&driver, stats, "Race")
	if len(driver.Statistics) != 1 || driver.Statistics[0].Name != "lapsLead" {
		t.Fatalf("zero laps led or invalid gap: %+v", driver.Statistics)
	}
	driver.Statistics = nil
	applyRaceStatistics(&driver, stats, "Sprint Qualifying")
	if len(driver.Statistics) != 0 {
		t.Fatal("race counts leaked into qualifying")
	}
}

func TestSprintQualifyingTimeLabel(t *testing.T) {
	var stats raceStatistics
	if err := json.Unmarshal([]byte(`{"splits":{"categories":[{"stats":[{"name":"totalTime","value":82.5,"displayValue":"1:22.500"}]}]}}`), &stats); err != nil {
		t.Fatal(err)
	}
	var driver models.SportsParticipant
	applyRaceStatistics(&driver, stats, "Sprint Qualifying")
	if len(driver.Statistics) != 1 || driver.Statistics[0].Label != "Session time" {
		t.Fatalf("qualifying mislabeled: %+v", driver.Statistics)
	}
}
