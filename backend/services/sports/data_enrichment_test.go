package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestPlayerGameStatsKeepIdentityZeroAndColumnAlignment(t *testing.T) {
	var teams []playerBoxscoreTeam
	err := json.Unmarshal([]byte(`[{"team":{"id":"a"},"statistics":[{"names":["H","AB","RBI"],"athletes":[{"athlete":{"id":"b","displayName":"Batter"},"stats":["0","3","0"]},{"athlete":{"id":"bad","displayName":"Misaligned"},"stats":["7"]}]},{"names":["K","IP","H","ER"],"athletes":[{"athlete":{"id":"p","displayName":"Pitcher"},"stats":["5","6.1","4","0"]}]}]},{"team":{"id":"foreign"},"statistics":[{"names":["AB"],"athletes":[{"athlete":{"id":"x","displayName":"Other game"},"stats":["9"]}]}]}]`), &teams)
	if err != nil {
		t.Fatal(err)
	}
	game := models.SportsGame{League: "mlb", Status: models.SportsGameLive, AwayTeam: models.SportsTeam{ID: "a"}, HomeTeam: models.SportsTeam{ID: "h"}}
	rows := normalizePlayerGameStats(game, teams)
	if len(rows) != 2 || rows[0].ID != "b" || rows[0].Stats[0].Label != "H" || rows[0].Stats[0].Value != "0" || rows[1].Category != "Pitching" || rows[1].Stats[1].Value != "6.1" {
		t.Fatalf("lost identity/zero/alignment: %+v", rows)
	}
	game.Status = models.SportsGameScheduled
	if len(normalizePlayerGameStats(game, teams)) != 0 {
		t.Fatal("season stats leaked into pregame")
	}
}

func TestSoccerAndCollegeCapturedEnrichment(t *testing.T) {
	for _, slug := range []string{"eng.1", "mens-college-basketball", "womens-college-basketball", "college-football"} {
		t.Run(slug, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + slug + "-captured.json")
			if err != nil {
				t.Fatal(err)
			}
			var p struct {
				Event   espnEvent        `json:"event"`
				Summary teamSportSummary `json:"summary"`
			}
			if err = json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			id := slug
			if slug == "eng.1" {
				id = "soccer-eng.1"
			}
			var league League
			for _, l := range LeagueCatalog {
				if l.ID == id {
					league = l
				}
			}
			game, ok := espnEventToGame(p.Event, league)
			if !ok {
				t.Fatal("invalid capture")
			}
			got, err := normalizeTeamDetail(game, p.Summary, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if slug != "eng.1" {
				if len(got.Detail.PlayerStats) == 0 {
					t.Fatal("lost player rows")
				}
				return
			}
			matched := 0
			for _, raw := range p.Summary.KeyEvents {
				if raw.Team.ID == "" || len(raw.Participants) == 0 || raw.Text == "" {
					continue
				}
				for _, row := range got.Detail.Plays {
					if row.ID == raw.ID {
						if row.TeamID != raw.Team.ID || len(row.Participants) == 0 || row.Participants[0].ID != raw.Participants[0].Athlete.ID {
							t.Fatal("event attribution lost")
						}
						matched++
					}
				}
			}
			if matched == 0 {
				t.Fatal("no attributed events checked")
			}
		})
	}
}

func TestPitCountsPreserveZeroAndSkipQualifying(t *testing.T) {
	var stats raceStatistics
	if err := json.Unmarshal([]byte(`{"splits":{"categories":[{"stats":[{"name":"pitsTaken","value":0,"displayValue":"0"},{"name":"fastestLapNum","value":42,"displayValue":"42"}]}]}}`), &stats); err != nil {
		t.Fatal(err)
	}
	var driver models.SportsParticipant
	applyRaceStatistics(&driver, stats, "Race")
	if len(driver.Statistics) != 2 || driver.Statistics[0].Value != "0" {
		t.Fatalf("missing pit count: %+v", driver.Statistics)
	}
	for _, session := range []string{"Qualifying", "Sprint Shootout", "Sprint Qualifying"} {
		driver.Statistics = nil
		applyRaceStatistics(&driver, stats, session)
		for _, stat := range driver.Statistics {
			if stat.Name == "pitsTaken" {
				t.Fatalf("race pit count on %s", session)
			}
		}
	}
}

func TestCyclingRiderMetadataCaptured(t *testing.T) {
	for _, c := range cyclingCompetitions[:3] {
		var raw []asoRanking
		cyclingFixture(t, "aso-"+c.id+"-results.json", &raw)
		number := 21
		if c.id == "vuelta" {
			number = 16
		}
		if c.id == "tour-femmes" {
			number = 9
		}
		stage := CyclingStage{ID: "test"}
		normalizeCyclingResults(raw, &stage, 2026, number, time.Now())
		count := 0
		for _, row := range stage.Results.Data {
			if row.Bib > 0 && len(row.Nationality) == 3 {
				count++
			}
		}
		if count == 0 {
			t.Fatalf("no metadata retained for %s", c.id)
		}
	}
}
