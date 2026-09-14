package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestFootballDriveCoordinates(t *testing.T) {
	game := models.SportsGame{League: "nfl", Status: models.SportsGameFinal}
	game.AwayTeam.ID = "a"
	game.AwayTeam.Abbreviation = "AWY"
	game.HomeTeam.ID = "h"
	game.HomeTeam.Abbreviation = "HME"
	for _, c := range []struct {
		label string
		want  float64
		ok    bool
	}{{"AWY 20", 20, true}, {"HME 20", 80, true}, {"50", 50, true}, {"HME NaN", 0, false}, {"UNK 20", 0, false}, {"AWY 70", 0, false}, {"", 0, false}} {
		got, ok := footballFieldPoint(c.label, game)
		if ok != c.ok || got != c.want {
			t.Fatalf("%s = %v %v", c.label, got, ok)
		}
	}
	raw := []byte(`{"drives":{"previous":[{"id":"1","team":{"id":"a"},"start":{"text":"AWY 20"},"end":{"text":"HME 30"}},{"id":"2","team":{"id":"h"},"start":{"text":"HME 25"},"end":{"text":"AWY 10"}},{"id":"3","team":{"id":"x"},"start":{"text":"HME 25"},"end":{"text":"AWY 10"}}]}}`)
	var p teamSportSummary
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	rows := normalizeFootballDrives(game, p)
	if len(rows) != 2 || rows[0].Start >= rows[0].End || rows[1].Start <= rows[1].End {
		t.Fatalf("directions: %+v", rows)
	}
	game.Status = models.SportsGameScheduled
	if len(normalizeFootballDrives(game, p)) != 0 {
		t.Fatal("pregame drives shown")
	}
}
func TestFootballDriveCaptured(t *testing.T) {
	raw, err := os.ReadFile("testdata/college-football-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Summary teamSportSummary `json:"summary"`
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	game := models.SportsGame{ID: p.Summary.Header.ID, League: "college-football", Status: models.SportsGameFinal}
	game.AwayTeam.ID = "2050"
	game.AwayTeam.Abbreviation = "BALL"
	game.AwayTeam.Name = "Ball State Cardinals"
	game.HomeTeam.ID = "194"
	game.HomeTeam.Abbreviation = "OSU"
	game.HomeTeam.Name = "Ohio State Buckeyes"
	got, err := normalizeTeamDetail(game, p.Summary, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Detail.FootballDrives) < 10 {
		t.Fatalf("missing drives: %d", len(got.Detail.FootballDrives))
	}
}

func TestCurrentDrivePlaysWithoutDriveEnd(t *testing.T) {
	game := models.SportsGame{League: "nfl", Status: models.SportsGameLive}
	game.AwayTeam.ID = "a"
	game.AwayTeam.Abbreviation = "AWY"
	game.HomeTeam.ID = "h"
	game.HomeTeam.Abbreviation = "HME"
	var p teamSportSummary
	err := json.Unmarshal([]byte(`{"drives":{"current":{"id":"cur","team":{"id":"h"},"start":{"text":"HME 25"},"plays":[{"id":"p1","type":{"text":"Rush"},"start":{"yardsToEndzone":75,"team":{"id":"h"}},"end":{"yardsToEndzone":70,"team":{"id":"h"}}},{"id":"p2","type":{"text":"Penalty"}}]}}}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	rows := normalizeFootballDrives(game, p)
	if len(rows) != 1 || !rows[0].Current || *rows[0].EndKnown || !*rows[0].StartKnown {
		t.Fatalf("missing current drive: %+v", rows)
	}
	if len(rows[0].Plays) != 2 || *rows[0].Plays[0].Start != 75 || *rows[0].Plays[0].End != 70 || rows[0].Plays[1].Start != nil {
		t.Fatalf("incorrect plays: %+v", rows[0].Plays)
	}
}
