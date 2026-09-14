package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cyclingFixture(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
func TestCyclingCapturedSchedulesAndSeparateClassifications(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for _, c := range cyclingCompetitions[:3] {
		t.Run(c.id, func(t *testing.T) {
			var stages []asoStage
			cyclingFixture(t, "aso-"+c.id+"-stages.json", &stages)
			race := normalizeCyclingSchedule(stages, c, 2026, now)
			want := 21
			number := 21
			if c.id == "tour-femmes" {
				want = 9
				number = 9
			}
			if c.id == "vuelta" {
				number = 16
			}
			if len(race.Stages) != want || race.Stages[0].Name != "Stage 1" || race.StartDate == "" {
				t.Fatal("schedule mapping lost", race)
			}
			var raw []asoRanking
			cyclingFixture(t, "aso-"+c.id+"-results.json", &raw)
			stage := race.Stages[number-1]
			normalizeCyclingResults(raw, &stage, 2026, number, now)
			if stage.Results.State != "available" || stage.GeneralClassification.State != "available" || len(stage.Results.Data) < 100 || len(stage.GeneralClassification.Data) < 100 {
				t.Fatal("missing classification", stage.Results.State, stage.GeneralClassification.State)
			}
			if stage.Results.Data[0].Rank != 1 || stage.GeneralClassification.Data[0].Rank != 1 || stage.Results.Data[0].Time == stage.GeneralClassification.Data[0].Time {
				t.Fatal("stage and overall results conflated")
			}
			if stage.Status == "final" || stage.Status == "live" {
				t.Fatal("unverified lifecycle inferred")
			}
			if c.id == "vuelta" {
				last := stage.Results.Data[len(stage.Results.Data)-1]
				if last.Rank != 0 || last.Time != "" || last.Gap != "" {
					t.Fatal("unranked competitor turned into result", last)
				}
			}
			// Intermediate checkpoints must not be promoted to finish classifications.
			for i := range raw {
				for j := range raw[i].CheckpointTypes {
					raw[i].CheckpointTypes[j].Type = "intermediate"
				}
			}
			normalizeCyclingResults(raw, &stage, 2026, number, now)
			if stage.Results.State != "pending" || stage.GeneralClassification.State != "pending" {
				t.Fatal("accepted intermediate timing")
			}
		})
	}
}
func TestCyclingDurationsAndDateOnly(t *testing.T) {
	total := 266186000.0
	zero := 0.0
	fraction := 62345.0
	if cyclingTime(&total, false) != "73:56:26" || cyclingTime(&zero, false) != "" || cyclingTime(&zero, true) != "+0:00" || cyclingTime(&fraction, false) != "0:01:02.345" {
		t.Fatal("duration mapping")
	}
	var raw []asoStage
	cyclingFixture(t, "aso-tour-stages.json", &raw)
	race := normalizeCyclingSchedule(raw, cyclingCompetitions[0], 2026, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if race.Stages[0].Date != "2026-07-04" || race.Stages[0].Status != "scheduled" || race.Stages[0].Terrain != "Team time trial" {
		t.Fatal("calendar day shifted")
	}
}
func TestCyclingBoardFailureIsolationAndBackoff(t *testing.T) {
	now := time.Now()
	s := NewService(t.TempDir())
	s.cycling.boards = map[string]cyclingBoardCache{}
	key := fmt.Sprintf("tour:%d", now.Year())
	s.cycling.boards[key] = cyclingBoardCache{race: CyclingRace{ID: "saved", Source: cyclingSource(now.Add(-time.Hour), 0)}}
	var calls atomic.Int32
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("offline"))}, nil
	})}
	feed := s.GetCycling(context.Background())
	s.GetCycling(context.Background())
	if feed.State != "stale" || len(feed.Data) != 1 || feed.Data[0].ID != "saved" || calls.Load() != int32(len(cyclingCompetitions)) {
		t.Fatal("lost last good race or repeated failures", feed, calls.Load())
	}
	if feed.Coverage[0].State != "stale" || feed.Coverage[1].State != "unavailable" {
		t.Fatal("failure isolation lost")
	}
}

func TestCyclingExpansionCapturedSources(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for _, c := range cyclingCompetitions[3:11] {
		t.Run(c.id, func(t *testing.T) {
			var schedule []asoStage
			cyclingFixture(t, "aso-"+c.id+"-stages.json", &schedule)
			race := normalizeCyclingSchedule(schedule, c, 2026, now)
			want := 1
			if c.id == "paris-nice" {
				want = 8
			}
			if c.id == "vuelta-femenina" {
				want = 7
			}
			if len(race.Stages) != want {
				t.Fatalf("want %d stages, got %d", want, len(race.Stages))
			}
			stage := race.Stages[want-1]
			var raw []asoRanking
			cyclingFixture(t, "aso-"+c.id+"-results.json", &raw)
			normalizeCyclingResults(raw, &stage, 2026, want, now)
			if stage.Results.State != "available" || len(stage.Results.Data) < 80 || stage.Results.Data[0].Rank != 1 || stage.Results.Data[0].Name == "" || stage.Results.Data[0].Team == "" {
				t.Fatalf("missing arrival riders: %s %d", stage.Results.State, len(stage.Results.Data))
			}
			if c.oneDay {
				markCyclingOneDay(&stage)
				if race.RaceKind != "one-day" || stage.Name != "Race" || stage.GeneralClassification.State != "unavailable" || len(stage.GeneralClassification.Data) > 0 || stage.GeneralClassificationLabel != "" {
					t.Fatal("one-day race treated as stage race")
				}
			} else if race.RaceKind != "stage-race" || stage.GeneralClassification.State != "available" || len(stage.GeneralClassification.Data) < 80 {
				t.Fatal("missing separate GC")
			}
			if stage.Status != "unknown" {
				t.Fatal("inferred live/final from finish results")
			}
		})
	}
}

func TestCyclingBoardParallelBudgetAndProviderLimit(t *testing.T) {
	s := NewService(t.TempDir())
	var active, maximum atomic.Int32
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) {
				break
			}
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	feed := s.GetCycling(ctx)
	if time.Since(start) > time.Second || maximum.Load() < 2 || maximum.Load() > 4 || len(feed.Coverage) != len(cyclingCompetitions) || feed.State != "unavailable" {
		t.Fatalf("lost deadline/concurrency/failure isolation: max=%d, feed=%+v", maximum.Load(), feed)
	}
}
