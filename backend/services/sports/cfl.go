package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"novastream/models"
	"sort"
	"strconv"
	"strings"
	"time"
)

const cflAPIBase = "https://api.stats.cfl.ca"
const cflLeagueID = "espn:football:cfl" // Stable catalog identity; the provider is CFL, not ESPN.
const cflMaxResponseBytes = 4 << 20

type cflFixture struct {
	ID         int      `json:"ID"`
	Week       int      `json:"week"`
	HomeTeamID *int     `json:"home_team_id"`
	AwayTeamID *int     `json:"away_team_id"`
	StartAt    string   `json:"start_at"`
	HomeScore  *int     `json:"home_team_score"`
	AwayScore  *int     `json:"away_team_score"`
	Status     string   `json:"game_status"`
	Broadcasts []string `json:"broadcasting_options"`
}
type cflFixtureResponse struct {
	Year       string       `json:"year"`
	Preseason  []cflFixture `json:"preseason"`
	Season     []cflFixture `json:"season"`
	Semifinals []cflFixture `json:"semiFinals"`
	Finals     []cflFixture `json:"finals"`
}
type cflTeam struct {
	ID             int    `json:"ID"`
	Name           string `json:"name"`
	Region         string `json:"region_label"`
	Abbreviation   string `json:"abbreviation"`
	Color          string `json:"primary_color"`
	AlternateColor string `json:"accent_color"`
	Logo           string `json:"logo_primary"`
}

func (s *Service) readCFLJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cflAPIBase+path, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CFL %s HTTP %d", path, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cflMaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > cflMaxResponseBytes {
		return fmt.Errorf("CFL response exceeds limit")
	}
	if err = json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode CFL %s: %w", path, err)
	}
	return nil
}

// fetchCFLScoreboardDate uses the public API configured by stats.cfl.ca.
// Adjacent UTC dates preserve fixtures around a viewer's local midnight. The
// scoreboard caller performs final local-day selection, as with ESPN responses.
func (s *Service) fetchCFLScoreboardDate(ctx context.Context, date string) ([]models.SportsGame, error) {
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid CFL date: %w", err)
	}
	// Team artwork/names are optional. A short independent request cannot turn
	// valid fixtures into an error when the metadata endpoint is unavailable.
	teamCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	teamsCh := make(chan []cflTeam, 1)
	go func() {
		var teams []cflTeam
		if s.readCFLJSON(teamCtx, "/teams", &teams) != nil {
			teams = nil
		}
		teamsCh <- teams
	}()
	var response cflFixtureResponse
	if err = s.readCFLJSON(ctx, fmt.Sprintf("/fixtures/%d", day.Year()), &response); err != nil {
		return nil, err
	}
	if response.Year != strconv.Itoa(day.Year()) {
		return nil, fmt.Errorf("CFL returned season %q for %d", response.Year, day.Year())
	}
	var teams []cflTeam
	select {
	case teams = <-teamsCh:
	case <-teamCtx.Done():
	}
	lookup := map[int]cflTeam{}
	for _, team := range teams {
		if team.ID > 0 {
			lookup[team.ID] = team
		}
	}
	games := normalizeCFLFixtures(response, lookup, time.Now())
	result := make([]models.SportsGame, 0, len(games))
	from, to := day.AddDate(0, 0, -1), day.AddDate(0, 0, 2)
	for _, g := range games {
		if !g.StartTime.Before(from) && g.StartTime.Before(to) {
			result = append(result, g)
		}
	}
	return result, nil
}

func cflSportsTeam(id *int, teams map[int]cflTeam) models.SportsTeam {
	if id == nil || *id <= 0 {
		return models.SportsTeam{Name: "TBD"}
	}
	team := teams[*id]
	result := models.SportsTeam{ID: fmt.Sprintf("cfl:%d", *id), Name: strings.TrimSpace(team.Region + " " + team.Name), Location: team.Region, Nickname: team.Name, Abbreviation: team.Abbreviation, Color: team.Color, AlternateColor: team.AlternateColor}
	if result.Name == "" {
		result.Name = fmt.Sprintf("Team %d", *id)
	}
	// Inline SVG, arbitrary hosts and private URLs are never promoted to artwork.
	if u, err := url.Parse(team.Logo); err == nil && u.Scheme == "https" && u.Host == "content.cfl.ca" && u.User == nil {
		result.LogoURL = u.String()
	}
	return result
}

func normalizeCFLFixtures(response cflFixtureResponse, teams map[int]cflTeam, now time.Time) []models.SportsGame {
	result := []models.SportsGame{}
	seen := map[int]bool{}
	groups := []struct {
		label    string
		fixtures []cflFixture
	}{{"Preseason", response.Preseason}, {"Regular season", response.Season}, {"Semifinals", response.Semifinals}, {"Finals", response.Finals}}
	for _, group := range groups {
		for _, f := range group.fixtures {
			start, err := time.Parse(time.RFC3339, f.StartAt)
			if f.ID <= 0 || err != nil || seen[f.ID] {
				continue
			}
			seen[f.ID] = true
			g := models.SportsGame{ID: fmt.Sprintf("cfl:%d", f.ID), League: cflLeagueID, Sport: "football", EventKind: "matchup", StartTime: start, Status: models.SportsGameScheduled, StatusDetail: "Scheduled", EventContext: group.label, HomeTeam: cflSportsTeam(f.HomeTeamID, teams), AwayTeam: cflSportsTeam(f.AwayTeamID, teams), Broadcasts: f.Broadcasts}
			if group.label == "Regular season" && f.Week > 0 {
				g.EventContext = fmt.Sprintf("Regular season · Week %d", f.Week)
			}
			g.Title = g.AwayTeam.Name + " at " + g.HomeTeam.Name
			status := strings.TrimSpace(f.Status)
			// The official feed has both Finished and the nested-quoted "Finished".
			if unquoted, err := strconv.Unquote(status); err == nil {
				status = unquoted
			}
			if strings.EqualFold(status, "Finished") {
				g.Status = models.SportsGameFinal
				g.StatusDetail = "Final"
			} else if status != "" {
				g.StatusDetail = status
			} else if start.Before(now) {
				g.StatusDetail = "Status unavailable"
			}
			if f.HomeScore != nil && *f.HomeScore >= 0 {
				g.HomeTeam.Score = strconv.Itoa(*f.HomeScore)
			}
			if f.AwayScore != nil && *f.AwayScore >= 0 {
				g.AwayTeam.Score = strconv.Itoa(*f.AwayScore)
			}
			if g.Status == models.SportsGameFinal && f.HomeScore != nil && f.AwayScore != nil && *f.HomeScore >= 0 && *f.AwayScore >= 0 {
				g.HomeTeam.Winner = *f.HomeScore > *f.AwayScore
				g.AwayTeam.Winner = *f.AwayScore > *f.HomeScore
			}
			// total_periods and game_clock in completed fixtures are not dependable
			// current-quarter data. Do not reuse NFL field/down diagrams for CFL.
			result = append(result, g)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].StartTime.Before(result[j].StartTime) })
	return result
}
