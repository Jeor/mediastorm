package sports

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExtendedScoreboardData(t *testing.T) {
	for _, sport := range []string{"golf", "cricket"} {
		t.Run(sport, func(t *testing.T) {
			raw, _ := os.ReadFile("testdata/" + sport + "-scoreboard.json")
			var payload espnScoreboardResponse
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			games := scoreboardEventGames(payload.Events[0], League{ID: sport, Sport: sport, EventKind: map[string]string{"golf": "tournament", "cricket": "matchup"}[sport]})
			if len(games) != 1 || games[0].Detail == nil {
				t.Fatal("missing detailed event")
			}
			g := games[0]
			if sport == "golf" {
				if g.HomeTeam.Name != "" || g.AwayTeam.Name != "" {
					t.Fatal("golf must not invent two sides")
				}
				if len(g.Detail.Leaderboard) != 3 || g.Detail.Leaderboard[0].Score != "-20" || len(g.Detail.Leaderboard[0].Rounds[0].Holes) != 18 {
					t.Fatal("lost golf round/hole data")
				}
				if g.EndTime.IsZero() {
					t.Fatal("missing multi-day end")
				}
			} else {
				if g.HomeTeam.Score != "161/5 (18/20 ov, target 156)" || !g.HomeTeam.Winner {
					t.Fatal("lost cricket score/winner")
				}
				if len(g.Detail.Innings) != 2 {
					t.Fatalf("want only actual batting innings: %+v", g.Detail.Innings)
				}
				if g.StatusDetail != "RCB won by 5 wkts (12b rem)" {
					t.Fatal(g.StatusDetail)
				}
			}
		})
	}
}

func TestBoxingScheduleDoesNotInventLiveState(t *testing.T) {
	raw, _ := os.ReadFile("testdata/boxing-day.json")
	var p boxingSchedule
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	for _, e := range p.Events {
		g, ok := normalizeBoxingEvent(e)
		if !ok || g.Status != "scheduled" || g.HomeTeam.Name != "" || g.Detail.CoverageNote == "" {
			t.Fatal(g)
		}
	}
	if _, ok := normalizeBoxingEvent(boxingEvent{ID: "1", LeagueID: "4445", Title: "Bout"}); ok {
		t.Fatal("accepted missing start date")
	}
}

func TestTennisSetsPreserveZeroAndDoNotInventFutureScores(t *testing.T) {
	var event espnEvent
	raw := `{"id":"event","date":"2026-09-19T10:00Z","competitions":[{"id":"match","status":{"type":{"state":"post"}},"competitors":[{"id":"1","homeAway":"home","athlete":{"displayName":"A"},"linescores":[{"value":0,"winner":false}]},{"id":"2","homeAway":"away","athlete":{"displayName":"B"},"linescores":[{"value":6,"winner":true}]}]}]}`
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatal(err)
	}
	g := scoreboardEventGames(event, League{ID: "atp", Sport: "tennis"})[0]
	if len(g.Detail.Periods) != 1 || g.Detail.Periods[0].Home != "0" || g.Detail.Periods[0].Away != "6" {
		t.Fatal(g.Detail.Periods)
	}
	event.Competitions[0].Status.Type.State = "pre"
	if len(scoreboardEventGames(event, League{ID: "atp", Sport: "tennis"})[0].Detail.Periods) != 0 {
		t.Fatal("future scores exposed")
	}
}
