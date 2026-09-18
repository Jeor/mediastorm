package sports

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDateWindow(t *testing.T) {
	now := time.Date(2026, 9, 8, 23, 0, 0, 0, time.UTC)
	for _, date := range []string{"2026-09-06", "2026-09-08", "2026-09-10"} {
		if ValidateScoreboardDate(date, now) != nil {
			t.Fatal(date)
		}
	}
	for _, date := range []string{"2026-09-11", "2026-02-30", "20260908"} {
		if ValidateScoreboardDate(date, now) == nil {
			t.Fatal(date)
		}
	}
}
func TestDatedScoreboardQueriesAndRetainsLastGood(t *testing.T) {
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"mlb"})
	date := time.Now().UTC().Format("2006-01-02")
	calls := 0
	fail := false
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("dates") != strings.ReplaceAll(date, "-", "") {
			t.Error("missing date query")
		}
		if fail {
			return nil, fmt.Errorf("offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"events":[]}`))}, nil
	})}
	first, e := s.GetDatedScoreboard(context.Background(), date, "")
	if e != nil {
		t.Fatal(e)
	}
	s.GetDatedScoreboard(context.Background(), date, "")
	if calls != 1 {
		t.Fatal("not cached")
	}
	for k, v := range s.dated {
		v.expires = time.Time{}
		s.dated[k] = v
	}
	fail = true
	last, e := s.GetDatedScoreboard(context.Background(), date, "")
	if e != nil || !last.Stale || !last.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatal("lost freshness on failure")
	}
}

func TestDatedScoreboardIsolatesLeagueFailures(t *testing.T) {
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"nfl", "soccer-eng.1"})
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "soccer") {
			return nil, fmt.Errorf("soccer offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"events":[]}`))}, nil
	})}
	board, err := s.GetDatedScoreboard(context.Background(), time.Now().UTC().Format("2006-01-02"), "")
	if err != nil || !board.Stale || len(board.Leagues) != 2 {
		t.Fatalf("partial result lost: %+v %v", board, err)
	}
	for _, league := range board.Leagues {
		if league.League == "nfl" && (league.Stale || league.Unavailable) {
			t.Fatal("healthy league marked unavailable")
		}
		if league.League == "soccer-eng.1" && !league.Unavailable {
			t.Fatal("missing league concealed as empty")
		}
	}
}

// Group IDs are sport-specific: football 50 is CAA–South, not Division I.
func TestScoreboardUsesFullCollegeDivisionForLiveAndDatedRequests(t *testing.T) {
	for _, tc := range []struct{ id, group string }{
		{"college-football", "90"},
		{"mens-college-basketball", "50"},
		{"womens-college-basketball", "50"},
		{"nfl", ""},
	} {
		for _, date := range []string{"", "2026-09-17"} {
			t.Run(tc.id+"/"+date, func(t *testing.T) {
				s := NewService(t.TempDir())
				s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
					q := r.URL.Query()
					if q.Get("groups") != tc.group {
						t.Errorf("groups = %q, want %q", q.Get("groups"), tc.group)
					}
					if q.Get("limit") != "1000" {
						t.Error("scoreboard must request full event limit")
					}
					if q.Get("dates") != strings.ReplaceAll(date, "-", "") {
						t.Error("incorrect date")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"events":[]}`))}, nil
				})}
				for _, league := range LeagueCatalog {
					if league.ID == tc.id {
						if _, err := s.fetchLeagueScoreboardDate(context.Background(), league, date); err != nil {
							t.Fatal(err)
						}
					}
				}
			})
		}
	}
}
