package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestBasketballTrackingAllLeagues(t *testing.T) {
	for _, league := range []string{"nba", "wnba", "mens-college-basketball", "womens-college-basketball"} {
		t.Run(league, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/basketball-tracking-" + league + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var source teamSportSummary
			if err = json.Unmarshal(raw, &source); err != nil {
				t.Fatal(err)
			}
			game := models.SportsGame{ID: source.Header.ID, League: league, Status: models.SportsGameFinal}
			for _, team := range source.Header.Competitions[0].Competitors {
				if team.HomeAway == "home" {
					game.HomeTeam.ID = team.ID
				} else {
					game.AwayTeam.ID = team.ID
				}
			}
			result, err := normalizeTeamDetail(game, source, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			d := result.Detail
			if len(d.BasketballShots) < 30 || len(d.WinProbability) < 100 || !d.Capabilities.WinProbability || len(d.PlayerStats) == 0 || d.ScoreHistory == nil {
				t.Fatalf("missing tracking: shots %d probabilities %d players %d history %v", len(d.BasketballShots), len(d.WinProbability), len(d.PlayerStats), d.ScoreHistory != nil)
			}
			if league == "nba" && d.BasketballShots[0].Jersey == "" {
				t.Fatal("provider jersey not joined to shot")
			}
			if d.BasketballShots[0].PlayerName == "" {
				t.Fatal("shooter identity not joined")
			}
			for _, shot := range d.BasketballShots {
				if shot.X < 0 || shot.X > 50 || shot.Y < -5.25 {
					t.Fatal("sentinel leaked")
				}
			}
			// Live history must be consistent with the same header and retain opening coverage.
			live := source
			live.Header.Competitions[0].Status.Type.Completed = false
			live.Header.Competitions[0].Status.Type.State = "in"
			game.Status = models.SportsGameLive
			if normalizeScoreHistory(game, live) == nil {
				t.Fatal("live history unavailable")
			}
		})
	}
}
func TestBasketballRejectsUnknownProbabilityAndInvalidShots(t *testing.T) {
	raw, _ := os.ReadFile("testdata/basketball-tracking-nba.json")
	var p teamSportSummary
	_ = json.Unmarshal(raw, &p)
	g := models.SportsGame{ID: p.Header.ID, League: "nba"}
	g.AwayTeam.ID = "11"
	g.HomeTeam.ID = "25"
	for i := range p.Plays {
		if p.Plays[i].ShootingPlay {
			v := -99999.
			p.Plays[i].Coordinate = &struct {
				X *float64 `json:"x"`
				Y *float64 `json:"y"`
			}{&v, &v}
		}
	}
	for i := range p.WinProbability {
		p.WinProbability[i].PlayID = "unknown"
	}
	d := &models.SportsGameDetail{}
	normalizeBasketballTracking(g, p, d)
	if len(d.BasketballShots) != 0 || len(d.WinProbability) != 0 || d.Capabilities.WinProbability {
		t.Fatal("invalid provider data accepted")
	}
}
