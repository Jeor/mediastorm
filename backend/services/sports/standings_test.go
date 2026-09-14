package sports

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const standingsFixture = `{"season":{"year":2027,"displayName":"2026-27"},"children":[{"id":"east","name":"Eastern Conference","standings":{"season":2026,"entries":[{"team":{"id":"1","displayName":"Team One"},"stats":[{"name":"wins","displayValue":"0"},{"name":"wins","displayValue":"15"},{"name":"overall","displayValue":"0-0"},{"name":"playoffSeed","displayValue":"0"}]},{"team":{"id":"1","displayName":"Duplicate"},"stats":[{"name":"wins","displayValue":"99"}]}]}}]}`

func TestLeagueStandingsNormalization(t *testing.T) {
	var raw standingsResponse
	if err := json.Unmarshal([]byte(standingsFixture), &raw); err != nil {
		t.Fatal(err)
	}
	groups := normalizeLeagueStandings(raw, "nba")
	if len(groups) != 1 || groups[0].SeasonLabel != "2026" || len(groups[0].Rows) != 1 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	if groups[0].Rows[0].Values["wins"] != "0" {
		t.Fatal("overwrote overall stats with a later split")
	}
	raw.Children[0].Standings.Season = 0
	if len(normalizeLeagueStandings(raw, "nba")) != 0 {
		t.Fatal("accepted an unidentifiable table season")
	}
}

type standingsTestTransport func(*http.Request) (*http.Response, error)

func (f standingsTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLeagueStandingsCacheAndFailureRetention(t *testing.T) {
	calls := 0
	service := &Service{client: &http.Client{Transport: standingsTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		code := 200
		body := standingsFixture
		if calls > 1 {
			code = 503
			body = "unavailable"
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	a := service.GetLeagueStandings(context.Background(), "nba")
	b := service.GetLeagueStandings(context.Background(), "nba")
	if a.State != "available" || b.State != "available" || calls != 1 {
		t.Fatal("cache miss")
	}
	service.standings.entries["nba"].expires = time.Time{}
	stale := service.GetLeagueStandings(context.Background(), "nba")
	if stale.State != "stale" || len(stale.Groups) != 1 || stale.UpdatedAt == nil {
		t.Fatal("lost previous table")
	}
	service.GetLeagueStandings(context.Background(), "nba")
	if calls != 2 {
		t.Fatal("missing failure backoff")
	}
	if service.GetLeagueStandings(context.Background(), "../../private").State != "unavailable" || calls != 2 {
		t.Fatal("unvalidated league fetched")
	}
}

func TestLeagueStandingsRacingStringSeason(t *testing.T) {
	var raw standingsResponse
	if err := json.Unmarshal([]byte(strings.Replace(standingsFixture, `"season":2026`, `"season":"2026"`, 1)), &raw); err != nil {
		t.Fatal(err)
	}
	if groups := normalizeLeagueStandings(raw, "f1"); len(groups) != 1 || groups[0].Season != 2026 {
		t.Fatal("lost string-valued racing season")
	}
}
