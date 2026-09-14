package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestPlayerHeadshotShapes(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`"https://a.espncdn.com/player.png"`, "https://a.espncdn.com/player.png"},
		{`{"href":"https://a.espncdn.com/player.png"}`, "https://a.espncdn.com/player.png"},
		{`null`, ""}, {`{"href":42}`, ""}, {`"javascript:bad"`, ""},
	} {
		var h playerHeadshot
		if err := json.Unmarshal([]byte(tc.raw), &h); err != nil || string(h) != tc.want {
			t.Fatalf("%s: %s %v", tc.raw, h, err)
		}
	}
}
func TestCapturedHeadshotsSurviveNormalization(t *testing.T) {
	raw, err := os.ReadFile("testdata/college-football-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Game    models.SportsGame `json:"game"`
		Summary teamSportSummary  `json:"summary"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Game = models.SportsGame{ID: fixture.Summary.Header.ID, League: "college-football", Status: models.SportsGameFinal}
	fixture.Game.AwayTeam = models.SportsTeam{ID: "2050", Abbreviation: "BALL"}
	fixture.Game.HomeTeam = models.SportsTeam{ID: "194", Abbreviation: "OSU"}
	game, err := normalizeTeamDetail(fixture.Game, fixture.Summary, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, leader := range game.Detail.Leaders {
		if leader.HeadshotURL != "" && leader.AthleteID != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost supplied leader image or identity")
	}
	raw, err = os.ReadFile("testdata/nhl-player-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, player := range normalizePlayerGameStats(fixture.Game, fixture.Summary.Boxscore.Players) {
		if player.HeadshotURL != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost supplied boxscore portraits")
	}
}
