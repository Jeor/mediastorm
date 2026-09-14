package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"novastream/models"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRaceCapturedWeekendAndClassification(t *testing.T) {
	data, err := os.ReadFile("testdata/f1-racing-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw raceScoreboard
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	events := normalizeRaceBoard(raw, "f1", time.Now())
	if len(events) != 1 || len(events[0].SubEvents) != 5 {
		t.Fatalf("lost sessions: %+v", events)
	}
	e := events[0]
	race := e.SubEvents[4]
	if race.SessionID != "401839102" || !e.StartTime.Equal(race.StartTime) || e.ID == race.ID {
		t.Fatal("weekend/session identity or date lost")
	}
	if len(race.Participants) != 22 {
		t.Fatal("lost driver entries", len(race.Participants))
	}
	for _, p := range race.Participants {
		if p.Position != 0 || p.ID == "" {
			t.Fatal("invented rank or lost identity")
		}
	}
	data, err = os.ReadFile("testdata/f1-race-statistics-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	var stats raceStatistics
	if err = json.Unmarshal(data, &stats); err != nil {
		t.Fatal(err)
	}
	p := models.SportsParticipant{}
	applyRaceStatistics(&p, stats, "Race")
	if p.Position != 1 {
		t.Fatal("explicit place not used", p.Position)
	}
	found := false
	for _, v := range p.Statistics {
		if v.Name == "totalTime" && v.Label == "Elapsed time" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing elapsed race time")
	}
	p = models.SportsParticipant{}
	applyRaceStatistics(&p, stats, "Qual")
	for _, v := range p.Statistics {
		if v.Name == "totalTime" && v.Label != "Session time" {
			t.Fatal("qualifying mislabeled")
		}
	}
}
func TestRaceZeroTimingAndZeroLaps(t *testing.T) {
	var stats raceStatistics
	json.Unmarshal([]byte(`{"splits":{"categories":[{"stats":[{"name":"place","value":0},{"name":"lapsCompleted","value":0,"displayValue":"0"},{"name":"totalTime","value":0,"displayValue":".000"}]}]}}`), &stats)
	p := models.SportsParticipant{}
	applyRaceStatistics(&p, stats, "Race")
	if p.Position != 0 || len(p.Statistics) != 1 || p.Statistics[0].Name != "lapsCompleted" {
		t.Fatal("placeholder timing or zero laps mishandled", p)
	}
}
func TestRaceBoardRetainsLastGoodAndBacksOff(t *testing.T) {
	data, err := os.ReadFile("testdata/f1-racing-captured.json")
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"f1"})
	fail := false
	calls := 0
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if fail {
			return nil, fmt.Errorf("offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	first := s.GetRaceBoard(context.Background())
	if len(first.Events) != 1 {
		t.Fatal("missing initial board")
	}
	v := s.raceBoards["f1"]
	v.expires = time.Time{}
	s.raceBoards["f1"] = v
	fail = true
	last := s.GetRaceBoard(context.Background())
	s.GetRaceBoard(context.Background())
	if calls != 2 || len(last.Events) != 1 || !last.Events[0].Stale || last.Leagues[0].Unavailable || !last.Events[0].UpdatedAt.Equal(first.Events[0].UpdatedAt) {
		t.Fatal("last good board/backoff lost", calls, last)
	}
}
