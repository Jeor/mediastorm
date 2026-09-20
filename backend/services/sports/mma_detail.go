package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"novastream/models"
	"regexp"
	"strings"
	"time"
)

var espnNumericID = regexp.MustCompile(`^[0-9]+$`)

type mmaAthlete struct {
	ID     string `json:"id"`
	Name   string `json:"displayName"`
	Height string `json:"displayHeight"`
	Reach  string `json:"displayReach"`
	Weight string `json:"displayWeight"`
	Stance struct {
		Text string `json:"text"`
	} `json:"stance"`
	WeightClass struct {
		Text string `json:"text"`
	} `json:"weightClass"`
}
type mmaStatistics struct {
	Splits struct {
		Categories []struct {
			Stats []struct {
				Name         string `json:"name"`
				DisplayValue string `json:"displayValue"`
			} `json:"stats"`
		} `json:"categories"`
	} `json:"splits"`
}

func (s *Service) readSportsJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("sports detail HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}
func (s *Service) enrichMMA(ctx context.Context, g models.SportsGame) models.SportsGame {
	ids := strings.Split(g.ID, ":")
	if len(ids) != 3 || ids[0] != "ufc" || !espnNumericID.MatchString(ids[1]) || !espnNumericID.MatchString(ids[2]) {
		return g
	}
	for _, team := range []models.SportsTeam{g.AwayTeam, g.HomeTeam} {
		if !espnNumericID.MatchString(team.ID) {
			return g
		}
	}
	s.detailMu.Lock()
	defer s.detailMu.Unlock()
	if s.details == nil {
		s.details = map[string]detailCacheEntry{}
	}
	key := "mma:" + g.ID
	cached, ok := s.details[key]
	if ok && time.Now().Before(cached.expires) {
		return cached.game
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	d := &models.SportsGameDetail{Source: "espn", UpdatedAt: time.Now(), Periods: []models.SportsPeriodScore{}, Plays: []models.SportsDetailPlay{}, Comparisons: []models.SportsComparison{}}
	allStats := map[string]map[string]string{}
	failed := false
	for _, team := range []models.SportsTeam{g.AwayTeam, g.HomeTeam} {
		var a mmaAthlete
		if err := s.readSportsJSON(ctx, "https://sports.core.api.espn.com/v2/sports/mma/athletes/"+team.ID, &a); err != nil {
			failed = true
		} else if a.ID == team.ID {
			profile := models.SportsPlayerGameStats{ID: team.ID, TeamID: team.ID, Name: team.Name, Category: "Fighter profile"}
			for _, field := range [][2]string{{"Height", a.Height}, {"Reach", a.Reach}, {"Weight", a.Weight}, {"Stance", a.Stance.Text}, {"Division", a.WeightClass.Text}} {
				if field[1] != "" {
					profile.Stats = append(profile.Stats, models.SportsPlayerStatistic{Label: field[0], Value: field[1]})
				}
			}
			if len(profile.Stats) > 0 {
				d.FighterProfiles = append(d.FighterProfiles, profile)
			}
		}
		if g.Status == models.SportsGameScheduled {
			continue
		}
		var stats mmaStatistics
		endpoint := "https://sports.core.api.espn.com/v2/sports/mma/leagues/ufc/events/" + ids[1] + "/competitions/" + ids[2] + "/competitors/" + team.ID + "/statistics"
		if err := s.readSportsJSON(ctx, endpoint, &stats); err != nil {
			failed = true
			continue
		}
		values := map[string]string{}
		for _, cat := range stats.Splits.Categories {
			for _, stat := range cat.Stats {
				values[stat.Name] = stat.DisplayValue
			}
		}
		allStats[team.ID] = values
	}
	for _, field := range [][2]string{{"sigStrikesLanded", "Significant strikes landed"}, {"sigStrikesAttempted", "Significant strikes attempted"}, {"totalStrikesLanded", "Total strikes landed"}, {"takedownsLanded", "Takedowns landed"}, {"takedownsAttempted", "Takedowns attempted"}, {"timeInControl", "Control time"}, {"knockDowns", "Knockdowns"}} {
		away, home := allStats[g.AwayTeam.ID][field[0]], allStats[g.HomeTeam.ID][field[0]]
		if away != "" && home != "" {
			d.Comparisons = append(d.Comparisons, models.SportsComparison{Label: field[1], Away: away, Home: home})
		}
	}
	if failed && ok && cached.game.Detail != nil {
		g = cached.game
		copy := *g.Detail
		copy.Stale = true
		g.Detail = &copy
	} else {
		d.Stale = failed
		d.Capabilities.Stats = len(d.Comparisons) > 0
		g.Detail = d
	}
	ttl := 30 * time.Second
	if g.Status == models.SportsGameFinal {
		ttl = 10 * time.Minute
	}
	if failed {
		ttl = 15 * time.Second
	}
	s.cacheDetail(key, detailCacheEntry{game: g, expires: time.Now().Add(ttl)})
	return g
}
