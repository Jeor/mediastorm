package sports

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestEnsureLeagueTeamCatalogIncludesDisabledLeagueAndCaches(t *testing.T) {
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"nba"})
	calls := 0
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.Contains(r.URL.Path, "/basketball/wnba/teams") {
			t.Errorf("unexpected URL: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"sports":[{"leagues":[{"teams":[{"team":{"id":"1","displayName":"Atlanta Dream"}}]}]}]}`)), Header: make(http.Header)}, nil
	})}
	for i := 0; i < 2; i++ {
		teams, err := s.EnsureLeagueTeamCatalog(context.Background(), "wnba")
		if err != nil || len(teams) != 1 || teams[0].ID != "wnba:1" {
			t.Fatalf("teams=%v err=%v", teams, err)
		}
	}
	if calls != 1 {
		t.Fatalf("wanted cached catalog, got %d requests", calls)
	}
	for _, l := range s.Leagues() {
		if l.ID == "wnba" && l.Enabled {
			t.Fatal("favorites lookup enabled scoreboard polling")
		}
	}
	if _, err := s.EnsureLeagueTeamCatalog(context.Background(), "unknown"); err == nil {
		t.Fatal("accepted unknown league")
	}
	teams, err := s.EnsureLeagueTeamCatalog(context.Background(), "ufc")
	if err != nil || len(teams) != 0 || calls != 1 {
		t.Fatal("non-team sport should not request a team list")
	}
}

func TestEnsureLeagueTeamCatalogRetriesFailure(t *testing.T) {
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"nba"})
	calls := 0
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		code, body := 503, "unavailable"
		if calls > 1 {
			code = 200
			body = `{"sports":[{"leagues":[{"teams":[{"team":{"id":"2","displayName":"Team"}}]}]}]}`
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	if _, err := s.EnsureLeagueTeamCatalog(context.Background(), "wnba"); err == nil {
		t.Fatal("missing provider error")
	}
	teams, err := s.EnsureLeagueTeamCatalog(context.Background(), "wnba")
	if err != nil || len(teams) != 1 || calls != 2 {
		t.Fatalf("retry failed: %v %v", teams, err)
	}
}
