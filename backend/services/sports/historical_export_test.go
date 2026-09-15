package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// Opt-in offline fixture export through production normalizers. Never contacts a server.
// SPORTS_HISTORY_ROOT points to Frontend; capture files retain source provenance there.
func TestExportHistoricalBatch(t *testing.T) {
	root := os.Getenv("SPORTS_HISTORY_ROOT")
	if root == "" {
		t.Skip("opt-in historical fixture export")
	}
	now := time.Now()
	dest := filepath.Join(root, "features/sports/replay/fixtures")
	rawRoot := filepath.Join(root, "docs/research/historical-captures")
	read := func(path string, v any) {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, v); e != nil {
			t.Fatal(e)
		}
	}
	write := func(id string, v any) {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dest, id+".json"), b, 0644); e != nil {
			t.Fatal(e)
		}
	}
	for _, id := range []string{"nfl", "college-football", "nba", "mens-college-basketball", "womens-college-basketball", "nhl", "soccer", "mlb"} {
		var p struct {
			Game    models.SportsGame `json:"game"`
			Event   espnEvent         `json:"event"`
			Summary teamSportSummary  `json:"summary"`
		}
		leagueID := id
		path := filepath.Join(rawRoot, id+".json")
		if id == "soccer" {
			leagueID = "soccer-eng.1"
			path = "testdata/eng.1-captured.json"
		}
		if id == "mlb" {
			path = filepath.Join(rawRoot, "batch2/mlb-wrapped.json")
		}
		read(path, &p)
		var league League
		for _, l := range LeagueCatalog {
			if l.ID == leagueID {
				league = l
			}
		}
		game, ok := espnEventToGame(p.Event, league)
		if !ok {
			t.Fatalf("bad event %s", id)
		}
		if p.Game.ID == game.ID {
			if game.AwayTeam.LogoURL == "" && p.Game.AwayTeam.ID == game.AwayTeam.ID {
				game.AwayTeam.LogoURL = p.Game.AwayTeam.LogoURL
			}
			if game.HomeTeam.LogoURL == "" && p.Game.HomeTeam.ID == game.HomeTeam.ID {
				game.HomeTeam.LogoURL = p.Game.HomeTeam.LogoURL
			}
		}
		if id == "mlb" {
			var m struct {
				Summary mlbSummary `json:"summary"`
			}
			read(path, &m)
			g, e := normalizeMLBDetail(game, m.Summary, now)
			if e != nil {
				t.Fatal(e)
			}
			write(id, g)
		} else {
			g, e := normalizeTeamDetail(game, p.Summary, now)
			if e != nil {
				t.Fatal(e)
			}
			write(id, g)
		}
	}
	for _, id := range []string{"f1", "nascar", "indycar"} {
		path := filepath.Join(rawRoot, "batch2", id+".json")
		if id == "f1" {
			path = "testdata/f1-racing-captured.json"
		}
		var board raceScoreboard
		read(path, &board)
		events := normalizeRaceBoard(board, id, now)
		if len(events) != 1 {
			t.Fatal("wrong race count", id)
		}
		e := events[0]
		for j := range e.SubEvents {
			session := &e.SubEvents[j]
			if session.SessionType != "Race" {
				continue
			}
			for k := range session.Participants {
				p := &session.Participants[k]
				var stats raceStatistics
				read(filepath.Join(rawRoot, "batch2", id+"-stats-"+p.ID+".json"), &stats)
				applyRaceStatistics(p, stats, session.SessionType)
			}
			sort.SliceStable(session.Participants, func(a, b int) bool {
				x, y := session.Participants[a].Position, session.Participants[b].Position
				return x > 0 && (y == 0 || x < y)
			})
		}
		write(id, e)
	}
	var sessions []motoGPSession
	read("testdata/motogp-sessions.json", &sessions)
	var original []struct {
		Event motoGPEvent `json:"event"`
	}
	read("testdata/motogp-sessions.json", &original)
	event := normalizeMotoGPSessions(original[0].Event, sessions, now)
	var classification motoGPClassification
	read("testdata/motogp-classification.json", &classification)
	for i := range event.SubEvents {
		if event.SubEvents[i].SessionType == "Race" {
			applyMotoGPClassification(&event.SubEvents[i], classification)
		}
	}
	write("motogp", event)
	for _, id := range []string{"tour", "tour-femmes", "paris-roubaix"} {
		var c cyclingCompetition
		for _, candidate := range cyclingCompetitions {
			if candidate.id == id {
				c = candidate
			}
		}
		var stages []asoStage
		read("testdata/aso-"+id+"-stages.json", &stages)
		race := normalizeCyclingSchedule(stages, c, 2026, now)
		var rankings []asoRanking
		read("testdata/aso-"+id+"-results.json", &rankings)
		i := len(race.Stages) - 1
		if i < 0 {
			t.Fatal("no stages", id)
		}
		normalizeCyclingResults(rankings, &race.Stages[i], 2026, i+1, now)
		write("cycling-"+id, race)
	}
}
