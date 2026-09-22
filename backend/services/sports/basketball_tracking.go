package sports

import (
	"math"
	"novastream/models"
)

func basketballPeriodSeconds(league string, period int) float64 {
	count, seconds := 4, 600.
	if league == "nba" {
		seconds = 720
	}
	if league == "mens-college-basketball" {
		count, seconds = 2, 1200
	}
	if period > count {
		return 300
	}
	return seconds
}

func normalizeBasketballTracking(game models.SportsGame, p teamSportSummary, d *models.SportsGameDetail) {
	if p.Header.ID != game.ID {
		return
	}
	players := map[string]models.SportsPlayerGameStats{}
	for _, player := range d.PlayerStats {
		players[player.ID] = player
	}
	plays := map[string]teamDetailPlay{}
	for _, play := range p.Plays {
		remaining, valid := scoreClock(play.Clock.DisplayValue)
		if play.ID == "" || play.Period.Number < 1 || !valid || remaining > basketballPeriodSeconds(game.League, play.Period.Number) {
			continue
		}
		if _, exists := plays[play.ID]; exists {
			continue
		}
		plays[play.ID] = play
		if !play.ShootingPlay || play.Coordinate == nil || play.Coordinate.X == nil || play.Coordinate.Y == nil || (play.PointsAttempted != 2 && play.PointsAttempted != 3) {
			continue
		}
		if play.Team.ID != game.AwayTeam.ID && play.Team.ID != game.HomeTeam.ID {
			continue
		}
		x, y := *play.Coordinate.X, *play.Coordinate.Y
		// Reject provider sentinels; retain valid zero and shots slightly behind the rim.
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || x < 0 || x > 50 || y < -5.25 || y > 88.75 {
			continue
		}
		shot := models.SportsBasketballShot{ID: play.ID, TeamID: play.Team.ID, X: x, Y: y, Made: play.Scoring, Points: play.PointsAttempted, Period: play.Period.Number, PeriodLabel: teamPeriodLabel(game.League, play.Period.Number), Clock: play.Clock.DisplayValue, Description: play.Text}
		if len(play.Participants) > 0 {
			id := play.Participants[0].Athlete.ID
			if player, ok := players[id]; ok && player.TeamID == play.Team.ID {
				shot.AthleteID = id
				shot.PlayerName = player.Name
				shot.HeadshotURL = player.HeadshotURL
			}
		}
		d.BasketballShots = append(d.BasketballShots, shot)
	}
	// Join by play ID, then emit in play order rather than trusting provider array order.
	probabilities := map[string]float64{}
	for _, point := range p.WinProbability {
		if point.Home != nil && !math.IsNaN(*point.Home) && !math.IsInf(*point.Home, 0) && *point.Home >= 0 && *point.Home <= 1 {
			probabilities[point.PlayID] = *point.Home
		}
	}
	seen := map[string]bool{}
	for _, raw := range p.Plays {
		play, valid := plays[raw.ID]
		home, ok := probabilities[raw.ID]
		if !valid || !ok || seen[raw.ID] {
			continue
		}
		seen[raw.ID] = true
		d.WinProbability = append(d.WinProbability, models.SportsWinProbabilityPoint{PlayID: play.ID, Period: play.Period.Number, PeriodLabel: teamPeriodLabel(game.League, play.Period.Number), Clock: play.Clock.DisplayValue, Home: home})
	}
	d.Capabilities.WinProbability = len(d.WinProbability) > 1
}
