package sports

import (
	"context"
	"fmt"
	"novastream/models"
	"sort"
	"strings"
	"sync"
	"time"
)

type LeagueAvailability struct {
	League      string    `json:"league"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Stale       bool      `json:"stale"`
	Unavailable bool      `json:"unavailable"`
}
type DatedScoreboard struct {
	Leagues   []LeagueAvailability `json:"leagues"`
	Games     []models.SportsGame  `json:"games"`
	UpdatedAt time.Time            `json:"updatedAt"`
	Stale     bool                 `json:"stale"`
	Date      string               `json:"date"`
}
type datedEntry struct {
	board   DatedScoreboard
	expires time.Time
}

func ValidateScoreboardDate(date string, now time.Time) error {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	// Two UTC days accommodates yesterday/tomorrow in the viewer's local zone.
	if parsed.Before(today.AddDate(0, 0, -2)) || parsed.After(today.AddDate(0, 0, 2)) {
		return fmt.Errorf("date outside schedule window")
	}
	return nil
}

func (s *Service) GetDatedScoreboard(ctx context.Context, date, leagueID string) (DatedScoreboard, error) {
	if err := ValidateScoreboardDate(date, time.Now()); err != nil {
		return DatedScoreboard{}, err
	}
	s.mu.RLock()
	leagues := append([]League(nil), s.leagues...)
	s.mu.RUnlock()
	// Only enabled matchup competitions enter this scoreboard; races use the event contract.
	selected := []League{}
	ids := []string{}
	for _, l := range leagues {
		if supportsHubLeague(l.ID) && (leagueID == "" || leagueID == l.ID) {
			selected = append(selected, l)
			ids = append(ids, l.ID)
		}
	}
	if leagueID != "" && len(selected) == 0 {
		return DatedScoreboard{}, fmt.Errorf("league not enabled")
	}
	key := "board:" + date + ":" + strings.Join(ids, ",")
	s.dateMu.Lock()
	defer s.dateMu.Unlock()
	if s.dated == nil {
		s.dated = map[string]datedEntry{}
	}
	cached, exists := s.dated[key]
	if exists && time.Now().Before(cached.expires) {
		return cached.board, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	type result struct {
		games []models.SportsGame
		err   error
	}
	results := make([]result, len(selected))
	slots := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, l := range selected {
		wg.Add(1)
		go func(i int, l League) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				results[i].err = ctx.Err()
				return
			}
			defer func() { <-slots }()
			results[i].games, results[i].err = s.fetchLeagueScoreboardDate(ctx, l, date)
		}(i, l)
	}
	wg.Wait()

	board := DatedScoreboard{Games: []models.SportsGame{}, Leagues: []LeagueAvailability{}, Date: date, UpdatedAt: time.Now()}
	successful := 0
	for i, r := range results {
		leagueKey := "league:" + date + ":" + selected[i].ID
		previous, hasPrevious := s.dated[leagueKey]
		availability := LeagueAvailability{League: selected[i].ID, UpdatedAt: board.UpdatedAt}
		if r.err != nil {
			availability.Stale = true
			board.Stale = true
			if hasPrevious {
				board.Games = append(board.Games, previous.board.Games...)
				availability.UpdatedAt = previous.board.UpdatedAt
			} else {
				availability.Unavailable = true
				availability.UpdatedAt = time.Time{}
			}
		} else {
			successful++
			board.Games = append(board.Games, r.games...)
			s.dated[leagueKey] = datedEntry{board: DatedScoreboard{Games: r.games, UpdatedAt: board.UpdatedAt}, expires: time.Now().Add(30 * time.Second)}
		}
		board.Leagues = append(board.Leagues, availability)
	}
	if successful == 0 && len(selected) > 0 {
		if exists {
			board.UpdatedAt = cached.board.UpdatedAt
		} else if len(board.Games) == 0 {
			return DatedScoreboard{}, fmt.Errorf("sports scoreboards unavailable")
		}
	}
	sort.SliceStable(board.Games, func(i, j int) bool { return board.Games[i].StartTime.Before(board.Games[j].StartTime) })
	for len(s.dated) >= 128 {
		oldestKey := ""
		var oldest time.Time
		for k, entry := range s.dated {
			if oldestKey == "" || entry.expires.Before(oldest) {
				oldestKey = k
				oldest = entry.expires
			}
		}
		delete(s.dated, oldestKey)
	}
	ttl := 30 * time.Second
	if board.Stale {
		ttl = 15 * time.Second
	}
	s.dated[key] = datedEntry{board: board, expires: time.Now().Add(ttl)}
	return board, nil
}

func supportsHubLeague(id string) bool {
	for _, league := range LeagueCatalog {
		if league.ID == id {
			return league.EventKind == "matchup" || league.EventKind == "fight-card"
		}
	}
	return false
}
