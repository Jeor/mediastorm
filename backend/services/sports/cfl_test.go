package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func cflFixtureTestData(t *testing.T) (cflFixtureResponse, map[int]cflTeam) {
	t.Helper()
	raw, err := os.ReadFile("testdata/cfl-fixtures-2026.json")
	if err != nil {
		t.Fatal(err)
	}
	var response cflFixtureResponse
	if err = json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("testdata/cfl-teams.json")
	if err != nil {
		t.Fatal(err)
	}
	var teams []cflTeam
	if err = json.Unmarshal(raw, &teams); err != nil {
		t.Fatal(err)
	}
	lookup := map[int]cflTeam{}
	for _, team := range teams {
		lookup[team.ID] = team
	}
	return response, lookup
}
func TestCFLActualFixturesPreserveStatusAndTeamIdentity(t *testing.T) {
	response, teams := cflFixtureTestData(t)
	games := normalizeCFLFixtures(response, teams, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	if len(games) != 6 {
		t.Fatalf("want six captured games, got %d", len(games))
	}
	byID := map[string]int{}
	for i, g := range games {
		byID[g.ID] = i
	}
	final := games[byID["cfl:6584"]]
	if final.Status != "final" || final.HomeTeam.Score != "10" || final.AwayTeam.Score != "20" || !final.AwayTeam.Winner || final.Clock != "" || final.FootballSituation != nil {
		t.Fatal(final)
	}
	scheduled := games[byID["cfl:6652"]]
	if scheduled.League != cflLeagueID || scheduled.HomeTeam.Name != "WINNIPEG Blue Bombers" || scheduled.AwayTeam.Abbreviation != "TOR" || scheduled.HomeTeam.Score != "" || scheduled.Status != "scheduled" || scheduled.EventContext != "Regular season · Week 17" || len(scheduled.Broadcasts) == 0 {
		t.Fatal(scheduled)
	}
	// Provider start_at is UTC midnight; venue-local text has no timezone and must
	// not replace it or shift the fixture a second time.
	if scheduled.StartTime.Format(time.RFC3339) != "2026-09-26T00:00:00Z" {
		t.Fatal(scheduled.StartTime)
	}
	tbd := games[byID["cfl:6676"]]
	if tbd.HomeTeam.Name != "TBD" || tbd.AwayTeam.Name != "TBD" || tbd.HomeTeam.ID != "" || tbd.EventContext != "Finals" {
		t.Fatal(tbd)
	}
}
func TestCFLUnknownStatusDoesNotInventFinalOrWinner(t *testing.T) {
	response, teams := cflFixtureTestData(t)
	f := response.Season[0]
	f.Status = ""
	zero := 0
	f.HomeScore = &zero
	f.AwayScore = &zero
	response = cflFixtureResponse{Season: []cflFixture{f, f, {ID: 999, StartAt: "not a timestamp"}}}
	games := normalizeCFLFixtures(response, teams, time.Now())
	if len(games) != 1 || games[0].Status == "final" || games[0].HomeTeam.Winner || games[0].HomeTeam.Score != "0" || games[0].StatusDetail != "Status unavailable" {
		t.Fatal(games)
	}
	f.Status = "Postponed"
	response.Season = []cflFixture{f}
	games = normalizeCFLFixtures(response, teams, time.Now())
	if games[0].StatusDetail != "Postponed" || games[0].Status == "final" {
		t.Fatal(games)
	}
}

type cflTestTransport struct {
	url  string
	base http.RoundTripper
}

func (r cflTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "api.stats.cfl.ca" {
		return nil, fmt.Errorf("unexpected host %s", req.URL)
	}
	clone := req.Clone(req.Context())
	u := *req.URL
	clone.URL = &u
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.url, "http://")
	return r.base.RoundTrip(clone)
}
func TestCFLFetchOptionalTeamsFailureKeepsFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/cfl-fixtures-2026.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fixtures/2026":
			w.Write(raw)
		case "/teams":
			w.WriteHeader(503)
		default:
			t.Errorf("unexpected path %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = cflTestTransport{server.URL, client.Transport}
	s := &Service{client: client}
	games, err := s.fetchCFLScoreboardDate(context.Background(), "2026-09-25")
	if err != nil || len(games) != 2 || games[0].HomeTeam.ID != "cfl:20" {
		t.Fatal(games, err)
	}
	if _, err = s.fetchCFLScoreboardDate(context.Background(), "../2026"); err == nil {
		t.Fatal("invalid date accepted")
	}
}
func TestCFLArtworkAllowlist(t *testing.T) {
	id := 1
	for _, logo := range []string{"http://content.cfl.ca/a.svg", "https://evil.invalid/logo.svg", "data:image/svg+xml,<svg/>", "https://user@content.cfl.ca/a.svg"} {
		if got := cflSportsTeam(&id, map[int]cflTeam{1: {Logo: logo}}); got.LogoURL != "" {
			t.Fatal(got)
		}
	}
}
