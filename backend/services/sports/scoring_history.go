package sports

import (
	"math"
	"novastream/models"
	"strconv"
	"strings"
)

// Validate countdown ordering without converting the horizontal axis to elapsed time.
func scoreClock(value string) (float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= 60 {
		return 0, false
	}
	if len(parts) == 2 {
		minutes, err := strconv.Atoi(parts[0])
		if err != nil || minutes < 0 {
			return 0, false
		}
		seconds += float64(minutes) * 60
	}
	return seconds, true
}
func validScore(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && math.Trunc(*value) == *value
}

func normalizeScoreHistory(game models.SportsGame, p teamSportSummary) *models.SportsScoreHistory {
	if game.Status != models.SportsGameFinal || (game.League != "nba" && game.League != "mens-college-basketball" && game.League != "womens-college-basketball") || p.Header.ID != game.ID || len(p.Header.Competitions) != 1 || len(p.Plays) < 2 {
		return nil
	}
	c := p.Header.Competitions[0]
	if !c.Status.Type.Completed || len(c.Competitors) != 2 || game.AwayTeam.ID == "" || game.HomeTeam.ID == "" || game.AwayTeam.ID == game.HomeTeam.ID {
		return nil
	}
	finals := map[string]float64{}
	for _, team := range c.Competitors {
		expected := game.HomeTeam.ID
		if team.HomeAway == "away" {
			expected = game.AwayTeam.ID
		} else if team.HomeAway != "home" {
			return nil
		}
		score, err := strconv.ParseFloat(espnScore(team.Score), 64)
		if team.ID != expected || err != nil || !validScore(&score) {
			return nil
		}
		if _, duplicate := finals[team.HomeAway]; duplicate {
			return nil
		}
		finals[team.HomeAway] = score
	}
	first, last := p.Plays[0], p.Plays[len(p.Plays)-1]
	// Require the opening countdown observed in the supported complete sources;
	// an early scoreless play alone does not establish start coverage.
	openingSeconds := map[string]float64{"nba": 12 * 60, "mens-college-basketball": 20 * 60, "womens-college-basketball": 10 * 60}[game.League]
	openingClock, openingOK := scoreClock(first.Clock.DisplayValue)
	if !openingOK || openingClock != openingSeconds {
		return nil
	}
	if first.Period.Number != 1 || !validScore(first.AwayScore) || !validScore(first.HomeScore) || *first.AwayScore != 0 || *first.HomeScore != 0 || !strings.EqualFold(last.Type.Text, "End Game") || (c.Status.Period > 0 && last.Period.Number != c.Status.Period) {
		return nil
	}
	if !validScore(last.AwayScore) || !validScore(last.HomeScore) || *last.AwayScore != finals["away"] || *last.HomeScore != finals["home"] {
		return nil
	}
	endClock, ok := scoreClock(last.Clock.DisplayValue)
	if !ok || endClock != 0 {
		return nil
	}
	history := &models.SportsScoreHistory{EventID: game.ID, AwayTeamID: game.AwayTeam.ID, HomeTeamID: game.HomeTeam.ID, Complete: true}
	seen := map[string]bool{}
	previousPeriod, previousClock := 1, math.Inf(1)
	periodLabel := ""
	for _, play := range p.Plays {
		sequence, err := strconv.ParseInt(play.Sequence, 10, 64)
		clock, ok := scoreClock(play.Clock.DisplayValue)
		if strings.TrimSpace(play.ID) == "" || seen[play.ID] || err != nil || sequence < 0 || !ok || !validScore(play.AwayScore) || !validScore(play.HomeScore) || strings.TrimSpace(play.Period.DisplayValue) == "" {
			return nil
		}
		if play.Period.Number < previousPeriod || play.Period.Number > previousPeriod+1 {
			return nil
		}
		if play.Period.Number == previousPeriod && (clock > previousClock || (periodLabel != "" && periodLabel != play.Period.DisplayValue)) {
			return nil
		}
		seen[play.ID] = true
		history.Points = append(history.Points, models.SportsScorePoint{ID: play.ID, Sequence: play.Sequence, Period: play.Period.Number, PeriodLabel: play.Period.DisplayValue, Clock: play.Clock.DisplayValue, Away: *play.AwayScore, Home: *play.HomeScore})
		previousPeriod, previousClock, periodLabel = play.Period.Number, clock, play.Period.DisplayValue
	}
	return history
}
