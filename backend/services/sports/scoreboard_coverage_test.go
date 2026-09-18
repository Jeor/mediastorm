package sports

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"novastream/models"
	"os"
	"strings"
	"testing"
)

func TestNestedScoreboardCoverage(t *testing.T) {
	for _, tc := range []struct {
		league, sport, fixture string
		count                  int
	}{
		{"wta", "tennis", "tennis-scoreboard-nested.json", 3},
		{"atp", "tennis", "tennis-scoreboard-nested.json", 3},
		{"ufc", "mma", "ufc-scoreboard-bouts.json", 2},
	} {
		t.Run(tc.league, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			s := NewService(t.TempDir())
			s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})}
			games, err := s.fetchLeagueScoreboardDate(context.Background(), League{ID: tc.league, Sport: tc.sport, Slug: tc.league}, "2026-09-18")
			if err != nil || len(games) != tc.count {
				t.Fatalf("games=%d want=%d err=%v", len(games), tc.count, err)
			}
			ids := map[string]bool{}
			for _, g := range games {
				if ids[g.ID] || g.ID == "" {
					t.Fatal("lost distinct match identity")
				}
				ids[g.ID] = true
				if g.HomeTeam.ID == "" || g.AwayTeam.ID == "" || g.HomeTeam.Name == "" || g.AwayTeam.Name == "" {
					t.Fatalf("lost competitor: %+v", g)
				}
			}
			if tc.sport == "tennis" {
				if games[0].StartTime.Day() != 12 || games[0].AwayTeam.Score != "2" || games[0].HomeTeam.Score != "0" {
					t.Fatalf("lost match date or set scores: %+v", games[0])
				}
				if !strings.Contains(games[2].AwayTeam.Name, " / ") {
					t.Fatal("lost doubles roster")
				}
			}
		})
	}
}

func TestWNBAFetchesBasketballDetail(t *testing.T) {
	raw, err := os.ReadFile("testdata/nba-summary-final.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Game    models.SportsGame `json:"game"`
		Summary json.RawMessage   `json:"summary"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Game.League = "wnba"
	s := NewService(t.TempDir())
	called := false
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.URL.Path != "/apis/site/v2/sports/basketball/wnba/summary" {
			t.Error(r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(fixture.Summary)))}, nil
	})}
	got := s.EnrichGame(context.Background(), fixture.Game)
	if !called || got.Detail == nil || !got.Detail.Capabilities.Stats || len(got.Detail.Comparisons) == 0 {
		t.Fatal("WNBA detail was not fetched and normalized")
	}
}

func TestNestedScoreboardKeepsStableIDsAndSkipsDuplicateMatches(t *testing.T) {
	raw, err := os.ReadFile("testdata/tennis-scoreboard-nested.json")
	if err != nil {
		t.Fatal(err)
	}
	var payload espnScoreboardResponse
	if err = json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	event := payload.Events[0]
	// Providers can include a highlighted match both directly and in its group.
	event.Competitions = event.Groupings[0].Competitions[:1]
	league := League{ID: "wta", Sport: "tennis"}
	first := scoreboardEventGames(event, league)
	second := scoreboardEventGames(event, league)
	if len(first) != 3 || len(second) != 3 {
		t.Fatal("duplicate matches")
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatal("unstable match identity")
		}
	}
}
