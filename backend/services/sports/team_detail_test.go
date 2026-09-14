package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func TestTeamDetailCapturedGames(t *testing.T) {
	for _, league := range []string{"nfl", "nba", "nhl"} {
		t.Run(league, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + league + "-summary-final.json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Game    models.SportsGame `json:"game"`
				Summary teamSportSummary  `json:"summary"`
			}
			if err = json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			got, err := normalizeTeamDetail(fixture.Game, fixture.Summary, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			expectedPeriods := 4
			if league == "nhl" {
				expectedPeriods = 3
			}
			if got.Status != models.SportsGameFinal || len(got.Detail.Periods) != expectedPeriods || len(got.Detail.Plays) != 40 || len(got.Detail.Leaders) == 0 || !got.Detail.Capabilities.Stats {
				t.Fatalf("missing captured detail: %+v", got.Detail)
			}
			if got.Detail.Plays[0].Clock == "" {
				t.Fatal("lost play clock")
			}

			fixture.Summary.Header.Competitions[0].Status.Type.State = "pre"
			pre, err := normalizeTeamDetail(fixture.Game, fixture.Summary, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(pre.Detail.Leaders) > 0 || len(pre.Detail.Comparisons) > 0 || len(pre.Detail.Plays) > 0 {
				t.Fatal("pregame leaked game statistics")
			}
			fixture.Game.HomeTeam.ID = "wrong"
			if _, err = normalizeTeamDetail(fixture.Game, fixture.Summary, time.Now()); err == nil {
				t.Fatal("accepted mismatched identity")
			}
		})
	}
}

func TestExpansionCapturedDetails(t *testing.T) {
	for _, slug := range []string{"eng.1", "fifa.world", "college-football", "mens-college-basketball", "womens-college-basketball"} {
		t.Run(slug, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + slug + "-captured.json")
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Event   espnEvent        `json:"event"`
				Summary teamSportSummary `json:"summary"`
			}
			if err = json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			id := slug
			if slug == "eng.1" || slug == "fifa.world" {
				id = "soccer-" + slug
			}
			var league League
			for _, l := range LeagueCatalog {
				if l.ID == id {
					league = l
				}
			}
			game, ok := espnEventToGame(payload.Event, league)
			if !ok {
				t.Fatal("scoreboard rejected")
			}
			clock := game.Clock
			got, err := normalizeTeamDetail(game, payload.Summary, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != models.SportsGameFinal || !got.Detail.Capabilities.Stats || !got.Detail.Capabilities.Plays {
				t.Fatalf("missing details: %+v", got.Detail)
			}
			if clock != "" && got.Clock != clock {
				t.Fatal("summary without clock discarded scoreboard clock")
			}
			if slug == "mens-college-basketball" && (len(got.Detail.Periods) != 2 || got.Detail.Periods[1].Label != "H2") {
				t.Fatal("men's basketball must use halves")
			}
			if slug == "womens-college-basketball" && (len(got.Detail.Periods) != 4 || got.Detail.Periods[3].Label != "Q4") {
				t.Fatal("women's basketball must use quarters")
			}
			if slug == "eng.1" && (got.HomeTeam.Winner || got.AwayTeam.Winner || got.HomeTeam.Score != got.AwayTeam.Score) {
				t.Fatal("draw became a winner")
			}
			if slug == "fifa.world" && (got.HomeTeam.ShootoutScore == nil || *got.HomeTeam.ShootoutScore != 4 || got.HomeTeam.Score != "3" || len(got.Detail.Periods) != 5 || got.Detail.Periods[4].Label != "PEN") {
				t.Fatal("penalties merged with match goals")
			}

		})
	}
}

func TestTeamContextIdentityAndPregame(t *testing.T) {
	data, err := os.ReadFile("testdata/eng.1-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Event   espnEvent        `json:"event"`
		Summary teamSportSummary `json:"summary"`
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	var league League
	for _, l := range LeagueCatalog {
		if l.ID == "soccer-eng.1" {
			league = l
		}
	}
	game, _ := espnEventToGame(payload.Event, league)
	payload.Summary.Header.Competitions[0].Status.Type.State = "pre"
	got, err := normalizeTeamDetail(game, payload.Summary, time.Now())
	if err != nil || len(got.Detail.Lineups) != 2 || len(got.Detail.Lineups[0].Players) != 20 || got.Detail.Lineups[0].Formation != "4-2-3-1" || len(got.Detail.Standings) != 1 {
		t.Fatalf("missing context: %+v %v", got.Detail, err)
	}
	if len(got.Detail.Comparisons) != 0 {
		t.Fatal("pregame leaked game stats")
	}
	payload.Summary.Rosters[0].Team.ID = "other-team"
	got, err = normalizeTeamDetail(game, payload.Summary, time.Now())
	if err != nil || len(got.Detail.Lineups) != 1 {
		t.Fatal("foreign roster accepted")
	}
	payload.Summary.Standings.Groups[0].Standings.Entries[0].Stats = nil
	got, _ = normalizeTeamDetail(game, payload.Summary, time.Now())
	if got.Detail.Standings[0].Rows[0].Rank != "" || got.Detail.Standings[0].Rows[0].Points != "" {
		t.Fatal("missing table values fabricated")
	}
}
