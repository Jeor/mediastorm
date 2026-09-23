package sports

import (
	"context"
	"testing"
	"time"
)

func TestPublishedCyclingCalendars(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	cro, err := publishedCyclingRace("cro-race", 2026, now)
	if err != nil || len(cro.Stages) != 6 || cro.Stages[1].Date != "2026-09-23" || cro.Stages[1].Departure != "Biograd na Moru" || !cro.ScheduleOnly {
		t.Fatalf("invalid CRO calendar: %+v %v", cro, err)
	}
	worlds, err := publishedCyclingRace("road-worlds", 2026, now)
	if err != nil || len(worlds.Stages) != 13 || len(worlds.RestDays) != 1 || worlds.RestDays[0] != "2026-09-23" {
		t.Fatalf("invalid Worlds calendar: %+v %v", worlds, err)
	}
	count := 0
	for _, stage := range worlds.Stages {
		if stage.Date == "2026-09-24" {
			count++
		}
		if stage.Date == "2026-09-23" || stage.Status == "live" || stage.Status == "final" || stage.Results.State != "unavailable" || len(stage.Results.Data) > 0 {
			t.Fatal("invented race status or results", stage)
		}
	}
	if count != 2 {
		t.Fatal("lost same-day championship races")
	}
	if _, err := publishedCyclingRace("road-worlds", 2027, now); err == nil {
		t.Fatal("old calendar reused for new year")
	}
	if worlds.Source.ObservedAt.Equal(now) {
		t.Fatal("calendar verification date replaced by request time")
	}
}
func TestPublishedCyclingThroughService(t *testing.T) {
	if time.Now().UTC().Year() != 2026 {
		t.Skip("versioned calendar is for 2026 only")
	}
	s := &Service{leagues: []League{{ID: "cro:cro-race"}, {ID: "uci:road-worlds"}}}
	feed := s.GetCycling(context.Background())
	if len(feed.Data) != 2 {
		t.Fatalf("missing enabled races: %+v", feed)
	}
	stage, err := s.GetCyclingStage(context.Background(), "road-worlds", 2026, 8)
	if err != nil || stage.Name != "Women U23 road race" {
		t.Fatal(stage, err)
	}
	if _, err := s.GetCyclingStage(context.Background(), "road-worlds", 2026, 40); err == nil {
		t.Fatal("accepted nonexistent race")
	}
}
