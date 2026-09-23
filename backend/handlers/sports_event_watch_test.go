package handlers

import (
	"novastream/models"
	"novastream/services/sports"
	"testing"
	"time"
)

func TestRaceStreamIdentity(t *testing.T) {
	events := []models.SportsEvent{{ID: "f1:weekend", Title: "Monaco Grand Prix", League: "f1", SubEvents: []models.SportsEvent{{ID: "f1:weekend:race", SessionType: "Race", Status: "live"}, {ID: "f1:weekend:qualifying", SessionType: "Qualifying"}}}}
	game, ok := raceStreamEvent(events, "f1:weekend:race", "f1:weekend")
	if !ok || game.Title != "Monaco Grand Prix" || game.EventContext != "Race" {
		t.Fatal(game, ok)
	}
	if _, ok := raceStreamEvent(events, "f1:weekend:race", "untrusted"); ok {
		t.Fatal("accepted wrong parent")
	}
	events[0].Stale = true
	if _, ok := raceStreamEvent(events, "f1:weekend:race", "f1:weekend"); ok {
		t.Fatal("accepted stale feed")
	}
}
func TestCyclingStreamIdentity(t *testing.T) {
	races := []sports.CyclingRace{{ID: "aso:tour:2026", Name: "Tour de France", Stages: []sports.CyclingStage{{ID: "aso:tour:2026:3", Name: "Stage 3"}}}}
	game, ok := cyclingStreamEvent(races, "aso:tour:2026:3", "aso:tour:2026")
	if !ok || !game.StartTime.IsZero() || game.EventContext != "Stage 3" {
		t.Fatal(game, ok)
	}
	races[0].Stages[0].Status = "cancelled"
	if _, ok := cyclingStreamEvent(races, "aso:tour:2026:3", "aso:tour:2026"); ok {
		t.Fatal("accepted cancelled stage")
	}
}
func TestNonMatchStreamCandidates(t *testing.T) {
	race := models.SportsGame{Title: "Monaco Grand Prix", League: "f1", EventKind: "race-session", EventContext: "Race"}
	cycle := models.SportsGame{Title: "Tour de France", League: "cycling", EventKind: "cycling-stage", EventContext: "Stage 3"}
	for _, tc := range []struct {
		name  string
		game  models.SportsGame
		value string
		want  bool
	}{
		{"race", race, "F1 Monaco Grand Prix Race 1080p", true},
		{"qualifying conflict", race, "F1 Monaco Grand Prix Qualifying", false},
		{"different race", race, "F1 Italian Grand Prix", false},
		{"different series", race, "MotoGP Monaco Grand Prix", false},
		{"generic channel", race, "Sky Sports F1", false},
		{"stage", cycle, "Cycling Tour de France Stage 3", true},
		{"wrong stage", cycle, "Tour de France Stage 4", false},
		{"wrong competition", cycle, "Tour de France Femmes Stage 3", false},
		{"unrelated france", cycle, "France news", false},
		{"race only selection", cycle, "Tour de France Live", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := scoreWatchEvent(tc.value, tc.game)
			if (e.score > 0) != tc.want {
				t.Fatalf("%+v", e)
			}
			if e.score >= strongSportsConfidence {
				t.Fatal("must require explicit stream selection")
			}
		})
	}
	for _, source := range []string{"iptv", "addon"} {
		matches := selectableSportsMatches(matchGameToChannels(race, []LiveChannel{{ID: "a", Name: "F1 Monaco Grand Prix Race", URL: "https://example.com/live", SourceID: source}, {ID: "b", Name: "F1 Italian Grand Prix", URL: "https://example.com/wrong", SourceID: source}}, nil, ""))
		if len(matches) != 1 || matches[0].SourceID != source {
			t.Fatalf("%s: %+v", source, matches)
		}
	}
}

type watchEPG struct{ program *models.EPGProgram }

func (e watchEPG) GetNowPlaying(_ []string, _ ...time.Duration) []models.EPGNowPlaying {
	return []models.EPGNowPlaying{{ChannelID: "race", Current: e.program}}
}
func TestRaceStreamRejectsContradictoryCurrentProgram(t *testing.T) {
	game := models.SportsGame{Title: "Monaco Grand Prix", League: "f1", EventKind: "race-session", EventContext: "Race"}
	channels := []LiveChannel{{ID: "one", TvgID: "race", Name: "F1 Monaco Grand Prix Race", URL: "https://example.com/live"}}
	matches := matchGameToChannels(game, channels, watchEPG{&models.EPGProgram{Title: "F1 Monaco Grand Prix Qualifying"}}, "")
	if len(matches) != 0 {
		t.Fatalf("wrong session promoted: %+v", matches)
	}
	matches = matchGameToChannels(game, channels, watchEPG{&models.EPGProgram{Title: "F1 Monaco Grand Prix Race"}}, "")
	if len(matches) != 1 || matches[0].Confidence >= strongSportsConfidence {
		t.Fatalf("expected manual candidate: %+v", matches)
	}
	practice := game
	practice.EventContext = "Free Practice 1"
	if scoreWatchEvent("F1 Monaco Grand Prix FP2", practice).score > 0 {
		t.Fatal("wrong practice session")
	}
}

func TestCyclingCalendarStreamIdentity(t *testing.T) {
	races := []sports.CyclingRace{{ID: "calendar:road-abc:2026", Name: "Tour de Langkawi", CalendarOnly: true, Stages: []sports.CyclingStage{{ID: "calendar:road-abc:2026:1", Name: "Race schedule", Status: "scheduled"}}}}
	game, ok := cyclingStreamEvent(races, "calendar:road-abc:2026:1", "calendar:road-abc:2026")
	if !ok || !game.StartTime.IsZero() || game.Title != "Tour de Langkawi" {
		t.Fatal(game, ok)
	}
	if scoreWatchEvent("Cycling Tour de Langkawi Stage 2 Live", game).score == 0 {
		t.Fatal("calendar event cannot find stage broadcasts")
	}
	if _, ok := cyclingStreamEvent(races, "calendar:road-forged:2026:1", "calendar:road-abc:2026"); ok {
		t.Fatal("accepted unknown stream target")
	}
}
