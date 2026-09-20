package sports

import (
	"encoding/json"
	"novastream/models"
	"strconv"
	"strings"
)

type teamContextRecord struct {
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

func applyTeamRecords(team *models.SportsTeam, records []teamContextRecord) {
	for _, r := range records {
		switch r.Type {
		case "total":
			team.Record = r.Summary
		case "vsconf":
			team.ConferenceRecord = r.Summary
		}
	}
}

// ESPN returns substitution flags either directly or wrapped with timing metadata.
type substitutionFlag bool

func (f *substitutionFlag) UnmarshalJSON(data []byte) error {
	var direct bool
	if err := json.Unmarshal(data, &direct); err == nil {
		*f = substitutionFlag(direct)
		return nil
	}
	var wrapped struct {
		DidSub bool `json:"didSub"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return err
	}
	*f = substitutionFlag(wrapped.DidSub)
	return nil
}

type teamContextRoster struct {
	Team struct {
		ID string `json:"id"`
	} `json:"team"`
	Formation string `json:"formation"`
	Roster    []struct {
		Athlete struct {
			ID          string         `json:"id"`
			DisplayName string         `json:"displayName"`
			Headshot    playerHeadshot `json:"headshot"`
		} `json:"athlete"`
		Position struct {
			DisplayName string `json:"displayName"`
		} `json:"position"`
		Stats []struct {
			Name         string `json:"name"`
			DisplayValue string `json:"displayValue"`
		} `json:"stats"`
		Jersey    string           `json:"jersey"`
		Starter   *bool            `json:"starter"`
		SubbedIn  substitutionFlag `json:"subbedIn"`
		SubbedOut substitutionFlag `json:"subbedOut"`
	} `json:"roster"`
}
type teamContextStandings struct {
	Groups []struct {
		Header    string `json:"header"`
		Standings struct {
			Entries []struct {
				ID    string `json:"id"`
				Team  string `json:"team"`
				Stats []struct {
					Name         string `json:"name"`
					DisplayValue string `json:"displayValue"`
				} `json:"stats"`
			} `json:"entries"`
		} `json:"standings"`
	} `json:"groups"`
}

func normalizeTeamContext(game *models.SportsGame, summary teamSportSummary) {
	soccer := strings.HasPrefix(game.League, "soccer-")
	if !soccer && !strings.Contains(game.League, "college") {
		return
	}
	if soccer {
		seenTeams := map[string]bool{}
		for _, roster := range summary.Rosters {
			id := roster.Team.ID
			if (id != game.HomeTeam.ID && id != game.AwayTeam.ID) || seenTeams[id] {
				continue
			}
			seenTeams[id] = true
			lineup := models.SportsLineup{TeamID: id, Formation: roster.Formation, Players: []models.SportsLineupPlayer{}}
			seen := map[string]bool{}
			for _, player := range roster.Roster {
				if player.Starter == nil || player.Athlete.ID == "" || player.Athlete.DisplayName == "" || seen[player.Athlete.ID] || len(lineup.Players) >= 40 {
					continue
				}
				seen[player.Athlete.ID] = true
				lineup.Players = append(lineup.Players, models.SportsLineupPlayer{ID: player.Athlete.ID, Name: player.Athlete.DisplayName, Number: player.Jersey, Position: player.Position.DisplayName, Starter: *player.Starter, SubbedIn: bool(player.SubbedIn), SubbedOut: bool(player.SubbedOut)})
			}
			if len(lineup.Players) > 0 {
				game.Detail.Lineups = append(game.Detail.Lineups, lineup)
			}
		}
	}
	for _, group := range summary.Standings.Groups {
		if group.Header == "" || len(game.Detail.Standings) >= 8 {
			continue
		}
		normalized := models.SportsStandingGroup{Title: group.Header, Rows: []models.SportsStandingRow{}}
		seen := map[string]bool{}
		for _, row := range group.Standings.Entries {
			if row.ID == "" || row.Team == "" || seen[row.ID] || len(normalized.Rows) >= 64 {
				continue
			}
			seen[row.ID] = true
			entry := models.SportsStandingRow{TeamID: row.ID, Name: row.Team}
			for _, stat := range row.Stats {
				switch stat.Name {
				case "rank":
					entry.Rank = stat.DisplayValue
				case "gamesPlayed":
					entry.Played = stat.DisplayValue
				case "points":
					entry.Points = stat.DisplayValue
				case "pointDifferential":
					entry.GoalDifference = stat.DisplayValue
				case "overall":
					entry.Record = stat.DisplayValue
				case "vs. Conf.":
					entry.ConferenceRecord = stat.DisplayValue
				}
			}
			normalized.Rows = append(normalized.Rows, entry)
		}
		if len(normalized.Rows) > 0 {
			game.Detail.Standings = append(game.Detail.Standings, normalized)
		}
	}
}

// Roster match stats are separate from season totals and require an appearance.
func normalizeSoccerPlayerStats(game models.SportsGame, rosters []teamContextRoster) []models.SportsPlayerGameStats {
	if game.Status == models.SportsGameScheduled || !strings.HasPrefix(game.League, "soccer-") {
		return nil
	}
	fields := []struct{ name, label string }{
		{"totalGoals", "G"}, {"goalAssists", "A"}, {"totalShots", "SH"}, {"shotsOnTarget", "SOT"},
		{"saves", "SV"}, {"foulsCommitted", "FC"}, {"yellowCards", "YC"}, {"redCards", "RC"},
	}
	seen := map[string]bool{}
	var rows []models.SportsPlayerGameStats
	for _, team := range rosters {
		if team.Team.ID != game.AwayTeam.ID && team.Team.ID != game.HomeTeam.ID {
			continue
		}
		for _, player := range team.Roster {
			key := team.Team.ID + ":" + player.Athlete.ID
			if player.Athlete.ID == "" || player.Athlete.DisplayName == "" || seen[key] {
				continue
			}
			values := map[string]string{}
			for _, stat := range player.Stats {
				values[stat.Name] = strings.TrimSpace(stat.DisplayValue)
			}
			if appearance, err := strconv.Atoi(values["appearances"]); err != nil || appearance != 1 {
				continue
			}
			row := models.SportsPlayerGameStats{ID: player.Athlete.ID, TeamID: team.Team.ID, Name: player.Athlete.DisplayName, HeadshotURL: string(player.Athlete.Headshot), Category: "Match stats"}
			for _, field := range fields {
				value := values[field.name]
				if n, err := strconv.Atoi(value); err == nil && n >= 0 {
					row.Stats = append(row.Stats, models.SportsPlayerStatistic{Label: field.label, Value: value})
				}
			}
			if len(row.Stats) > 0 {
				rows = append(rows, row)
				seen[key] = true
			}
		}
	}
	return rows
}
