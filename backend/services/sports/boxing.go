package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"novastream/models"
	"strings"
	"time"
)

type boxingSchedule struct {
	Events []boxingEvent `json:"events"`
}
type boxingEvent struct {
	ID        string `json:"idEvent"`
	LeagueID  string `json:"idLeague"`
	Title     string `json:"strEvent"`
	Timestamp string `json:"strTimestamp"`
	Venue     string `json:"strVenue"`
	Status    string `json:"strStatus"`
	Result    string `json:"strResult"`
	Postponed string `json:"strPostponed"`
}

func (s *Service) fetchBoxingDate(ctx context.Context, date string) ([]models.SportsGame, error) {
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	endpoint := "https://www.thesportsdb.com/api/v1/json/123/eventsday.php?" + url.Values{"d": {date}, "l": {"4445"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("boxing schedule HTTP %d", resp.StatusCode)
	}
	var data boxingSchedule
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return nil, err
	}
	games := []models.SportsGame{}
	for _, e := range data.Events {
		if g, ok := normalizeBoxingEvent(e); ok {
			games = append(games, g)
		}
	}
	return games, nil
}
func normalizeBoxingEvent(e boxingEvent) (models.SportsGame, bool) {
	if e.ID == "" || e.LeagueID != "4445" || strings.TrimSpace(e.Title) == "" {
		return models.SportsGame{}, false
	}
	// TheSportsDB timestamps are UTC even when their ISO string omits Z.
	stamp := e.Timestamp
	if len(stamp) == 19 {
		stamp += "Z"
	}
	start := parseESPNDate(stamp)
	if start.IsZero() {
		return models.SportsGame{}, false
	}
	state := models.SportsGameScheduled
	label := "Scheduled · limited coverage"
	if strings.EqualFold(e.Postponed, "yes") {
		label = "Postponed"
	} else if strings.EqualFold(e.Status, "Match Finished") || strings.EqualFold(e.Status, "FT") {
		state = models.SportsGameFinal
		label = "Final"
		if e.Result != "" {
			label = e.Result
		}
	}
	// A start time in the past is not evidence of a live or completed bout.
	g := models.SportsGame{ID: "boxing:" + e.ID, League: "boxing", Sport: "boxing", EventKind: "fight-card", Title: e.Title, StartTime: start, Status: state, StatusDetail: label, VenueName: e.Venue, Detail: &models.SportsGameDetail{Source: "thesportsdb", UpdatedAt: time.Now(), Periods: []models.SportsPeriodScore{}, Plays: []models.SportsDetailPlay{}, Comparisons: []models.SportsComparison{}, CoverageNote: "Limited boxing schedule coverage. Live round and punch statistics are not supplied."}}
	return g, true
}
