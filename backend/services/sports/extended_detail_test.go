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
	"time"
)

func TestHockeyShotsExcludeNonShotsAndMissingCoordinates(t *testing.T) {
	raw, _ := os.ReadFile("testdata/nhl-shots.json")
	var p teamSportSummary
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	got := normalizeHockeyShots(p.Plays, "6", "26")
	if len(got) != 4 {
		t.Fatalf("got %d shots", len(got))
	}
	for _, shot := range got {
		if shot.Kind == "blocked" && shot.TeamID != "6" {
			t.Fatal("blocked attempt credited to defender")
		}
	}
	p.Plays[0].Coordinate = nil
	if len(normalizeHockeyShots(p.Plays, "6", "26")) != 3 {
		t.Fatal("missing coordinate invented")
	}
	p.Plays[1].Coordinate.X = new(float64)
	*p.Plays[1].Coordinate.X = 200
	if len(normalizeHockeyShots(p.Plays, "6", "26")) != 2 {
		t.Fatal("invalid coordinate retained")
	}
}
func TestRugbyNestedComparisons(t *testing.T) {
	raw, _ := os.ReadFile("testdata/rugby-summary.json")
	var p teamSportSummary
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	g := models.SportsGame{ID: p.Header.ID, League: "rugby-180659", HomeTeam: models.SportsTeam{ID: "20"}, AwayTeam: models.SportsTeam{ID: "3"}}
	for _, c := range p.Header.Competitions[0].Competitors {
		if c.HomeAway == "away" {
			g.AwayTeam.ID = c.ID
		}
	}
	g.Detail = &models.SportsGameDetail{Plays: []models.SportsDetailPlay{{ID: "try", Title: "try"}}}
	got, err := normalizeTeamDetail(g, p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Detail.Plays) != 1 || got.Detail.Plays[0].ID != "try" {
		t.Fatal("lost scoreboard timeline")
	}
	if len(got.Detail.Comparisons) < 5 {
		t.Fatalf("lost nested stats: %+v", got.Detail.Comparisons)
	}
}
func TestMMAProfilesAndStatsAreBoundedAndCached(t *testing.T) {
	athlete, _ := os.ReadFile("testdata/mma-athlete.json")
	stats, _ := os.ReadFile("testdata/mma-statistics.json")
	calls := 0
	s := NewService(t.TempDir())
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := string(stats)
		if strings.Contains(r.URL.Path, "/athletes/") {
			body = string(athlete)
			if strings.HasSuffix(r.URL.Path, "/2") {
				body = strings.ReplaceAll(body, "3166126", "2")
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	g := models.SportsGame{ID: "ufc:600054045:401772654", League: "ufc", Status: models.SportsGameFinal, AwayTeam: models.SportsTeam{ID: "3166126", Name: "A"}, HomeTeam: models.SportsTeam{ID: "2", Name: "B"}}
	got := s.EnrichGame(context.Background(), g)
	if got.Detail == nil || len(got.Detail.FighterProfiles) != 2 || len(got.Detail.Comparisons) < 4 {
		t.Fatalf("missing enrichment: %+v", got.Detail)
	}
	s.EnrichGame(context.Background(), g)
	if calls != 4 {
		t.Fatalf("cache missed: %d calls", calls)
	}
	g.ID = "ufc:../../evil:1"
	s.EnrichGame(context.Background(), g)
	if calls != 4 {
		t.Fatal("unsafe provider identifier requested")
	}
}
