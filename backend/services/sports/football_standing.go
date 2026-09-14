package sports

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"novastream/models"
	"time"
)

type footballStandingEntry struct {
	summary string
	expires time.Time
}

// Standings are optional current context. Never hold up a score/play refresh for
// them; an independently guarded cache also coalesces concurrent team lookups.
func (s *Service) applyFootballStanding(team *models.SportsTeam) {
	if team.ID == "" {
		return
	}
	s.footballStandingMu.Lock()
	if s.footballStandings == nil {
		s.footballStandings = map[string]footballStandingEntry{}
	}
	cached, ok := s.footballStandings[team.ID]
	if ok && time.Now().Before(cached.expires) {
		team.StandingSummary = cached.summary
		s.footballStandingMu.Unlock()
		return
	}
	if len(s.footballStandings) >= 64 {
		for id, entry := range s.footballStandings {
			if time.Now().After(entry.expires) {
				delete(s.footballStandings, id)
			}
		}
	}
	if len(s.footballStandings) >= 64 {
		s.footballStandingMu.Unlock()
		return
	}
	id := team.ID
	s.footballStandings[id] = footballStandingEntry{expires: time.Now().Add(5 * time.Minute)}
	s.footballStandingMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://site.api.espn.com/apis/site/v2/sports/football/nfl/teams/"+url.PathEscape(id), nil)
		if err != nil {
			return
		}
		response, err := s.client.Do(req)
		if err != nil {
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return
		}
		var payload struct {
			Team struct {
				ID              string `json:"id"`
				StandingSummary string `json:"standingSummary"`
			} `json:"team"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil || payload.Team.ID != id {
			return
		}
		s.footballStandingMu.Lock()
		s.footballStandings[id] = footballStandingEntry{summary: payload.Team.StandingSummary, expires: time.Now().Add(15 * time.Minute)}
		s.footballStandingMu.Unlock()
	}()
}
