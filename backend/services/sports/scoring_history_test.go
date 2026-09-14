package sports

import (
	"encoding/json"
	"novastream/models"
	"os"
	"testing"
	"time"
)

func historySource(t *testing.T, name, league string) (models.SportsGame, teamSportSummary) {
	t.Helper()
	raw, err := os.ReadFile("testdata/score-history-" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var source teamSportSummary
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	var game models.SportsGame
	game.ID, game.League, game.Status = source.Header.ID, league, models.SportsGameFinal
	for _, team := range source.Header.Competitions[0].Competitors {
		if team.HomeAway == "away" {
			game.AwayTeam.ID = team.ID
		} else {
			game.HomeTeam.ID = team.ID
		}
	}
	return game, source
}
func TestScoreHistoryCompleteSources(t *testing.T) {
	for _, fixture := range [][2]string{{"nba", "nba"}, {"ncaam", "mens-college-basketball"}, {"ncaaw", "womens-college-basketball"}} {
		t.Run(fixture[0], func(t *testing.T) {
			game, source := historySource(t, fixture[0], fixture[1])
			normalized, err := normalizeTeamDetail(game, source, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			history := normalized.Detail.ScoreHistory
			if history == nil || len(history.Points) != len(source.Plays) || len(normalized.Detail.Plays) != 200 {
				t.Fatal("full history must survive the recent-play cap")
			}
			if history.Points[0].PeriodLabel != source.Plays[0].Period.DisplayValue {
				t.Fatal("source period label lost")
			}
		})
	}
}
func TestScoreHistoryRejectsIncompleteSnapshots(t *testing.T) {
	cases := map[string]func(*models.SportsGame, *teamSportSummary){
		"truncated":      func(g *models.SportsGame, p *teamSportSummary) { p.Plays = p.Plays[len(p.Plays)-200:] },
		"missing end":    func(g *models.SportsGame, p *teamSportSummary) { p.Plays = p.Plays[:len(p.Plays)-1] },
		"duplicate":      func(g *models.SportsGame, p *teamSportSummary) { p.Plays[1].ID = p.Plays[0].ID },
		"missing score":  func(g *models.SportsGame, p *teamSportSummary) { p.Plays[1].AwayScore = nil },
		"wrong final":    func(g *models.SportsGame, p *teamSportSummary) { v := 999.; p.Plays[len(p.Plays)-1].HomeScore = &v },
		"period order":   func(g *models.SportsGame, p *teamSportSummary) { p.Plays[1].Period.Number = 3 },
		"clock order":    func(g *models.SportsGame, p *teamSportSummary) { p.Plays[2].Clock.DisplayValue = "59:59" },
		"live":           func(g *models.SportsGame, p *teamSportSummary) { g.Status = models.SportsGameLive },
		"wrong identity": func(g *models.SportsGame, p *teamSportSummary) { p.Header.ID = "other" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			g, p := historySource(t, "nba", "nba")
			mutate(&g, &p)
			if normalizeScoreHistory(g, p) != nil {
				t.Fatal("invalid full-game claim")
			}
		})
	}
}
func TestScoreHistoryRejectsTruncatedScorelessOpening(t *testing.T) {
	game, source := historySource(t, "nba", "nba")
	source.Plays = source.Plays[1:]
	first := source.Plays[0]
	if first.Period.Number != 1 || *first.AwayScore != 0 || *first.HomeScore != 0 || first.Clock.DisplayValue != "11:33" {
		t.Fatal("fixture must retain a scoreless first-period play after removing the opening")
	}
	if normalizeScoreHistory(game, source) != nil {
		t.Fatal("a scoreless play after tip-off must not establish complete start coverage")
	}
}

func TestScoreHistoryCorrectedSnapshotReplaces(t *testing.T) {
	game, source := historySource(t, "nba", "nba")
	first, err := normalizeTeamDetail(game, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	index := len(source.Plays) / 2
	original := first.Detail.ScoreHistory.Points[index].Away
	corrected := original - 1
	source.Plays[index].AwayScore = &corrected
	second, err := normalizeTeamDetail(first, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if second.Detail.ScoreHistory == nil || len(second.Detail.ScoreHistory.Points) != len(source.Plays) || second.Detail.ScoreHistory.Points[index].Away != corrected || first.Detail.ScoreHistory.Points[index].Away != original {
		t.Fatal("correction must replace, not append or mutate the prior snapshot")
	}
	source.Plays = source.Plays[200:]
	third, err := normalizeTeamDetail(second, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if third.Detail.ScoreHistory != nil {
		t.Fatal("incomplete refresh must remove prior full history")
	}
}
