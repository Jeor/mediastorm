package handlers

import (
	"context"
	"net/http"
	"novastream/models"
	"novastream/services/sports"
	"regexp"
	"strings"
)

// Resolve identifiers against provider-owned feeds, never client-supplied titles.
func (h *SportsHandler) streamEvent(ctx context.Context, id, kind, parentID string) (models.SportsGame, bool) {
	if kind == "" {
		return h.service.GetGame(id)
	}
	if kind == "racing" {
		for _, prefix := range []string{"f1:", "nascar:", "indycar:", "motogp:"} {
			if strings.HasPrefix(id, prefix) {
				return raceStreamEvent(h.service.GetRaceBoard(ctx).Events, id, parentID)
			}
		}
	}
	if kind == "cycling" && (strings.HasPrefix(id, "aso:") || strings.HasPrefix(id, "rcs:") || strings.HasPrefix(id, "cro:") || strings.HasPrefix(id, "uci:") || strings.HasPrefix(id, "calendar:")) {
		return cyclingStreamEvent(h.service.GetCycling(ctx).Data, id, parentID)
	}
	return models.SportsGame{}, false
}
func raceStreamEvent(events []models.SportsEvent, id, parentID string) (models.SportsGame, bool) {
	for _, parent := range events {
		if parent.ID != parentID {
			continue
		}
		sessions := parent.SubEvents
		if parent.EventKind == "race-session" {
			sessions = []models.SportsEvent{parent}
		}
		for _, session := range sessions {
			if session.ID != id || parent.Stale || session.Stale {
				continue
			}
			return models.SportsGame{ID: id, Title: parent.Title, EventContext: session.SessionType, League: parent.League, Sport: "racing", EventKind: "race-session", StartTime: session.StartTime, Status: session.Status}, true
		}
	}
	return models.SportsGame{}, false
}
func cyclingStreamEvent(races []sports.CyclingRace, id, parentID string) (models.SportsGame, bool) {
	for _, race := range races {
		if race.ID != parentID {
			continue
		}
		for _, stage := range race.Stages {
			if stage.ID != id || stage.Status == "cancelled" {
				continue
			}
			// Date-only stages deliberately have no invented start time.
			return models.SportsGame{ID: id, Title: race.Name, EventContext: stage.Name, League: "cycling", Sport: "cycling", EventKind: "cycling-stage", Status: models.SportsGameStatus("scheduled")}, true
		}
	}
	return models.SportsGame{}, false
}

var watchYear = regexp.MustCompile(`^20[0-9]{2}$`)
var watchStage = regexp.MustCompile(`(?i)\b(?:stage|etape|étape)\s*([0-9]+)\b`)

var watchPractice = regexp.MustCompile(`(?i)\b(?:fp|(?:free\s+)?practice\s*)([1-3])\b`)

func watchSession(value string) string {
	v := strings.ToLower(value)
	if m := watchPractice.FindStringSubmatch(v); len(m) > 1 {
		return "practice" + m[1]
	}
	for _, name := range []string{"qualifying", "practice", "sprint", "race"} {
		if strings.Contains(v, name) {
			return name
		}
	}
	return ""
}
func conflictingWatchSegment(value string, game models.SportsGame) bool {
	if game.EventKind == "race-session" {
		actual, wanted := watchSession(value), watchSession(game.EventContext)
		if actual == "" || wanted == "" {
			return false
		}
		a, w := strings.TrimRight(actual, "123"), strings.TrimRight(wanted, "123")
		return a != w || (actual != a && wanted != w && actual != wanted)
	}
	if game.EventKind == "cycling-stage" {
		expected, actual := watchStage.FindStringSubmatch(game.EventContext), watchStage.FindStringSubmatch(value)
		return len(actual) > 1 && len(expected) > 1 && actual[1] != expected[1]
	}
	return false
}
func scoreWatchEvent(value string, game models.SportsGame) sportsEvidence {
	if game.EventKind != "race-session" && game.EventKind != "cycling-stage" {
		return scoreSportsEventTitle(value, game.Title)
	}
	if conflictingWatchSegment(value, game) {
		return sportsEvidence{}
	}
	tokens := sportsTokens(value)
	set := map[string]bool{}
	for _, t := range tokens {
		set[t] = true
	}
	meaningful := 0
	for _, t := range sportsTokens(game.Title) {
		switch t {
		case "the", "and", "de", "d", "a", "championship":
			continue
		}
		if len(t) < 3 || watchYear.MatchString(t) {
			continue
		}
		meaningful++
		if !set[t] {
			return sportsEvidence{}
		}
	}
	generic := strings.ToLower(strings.TrimSpace(game.Title))
	if meaningful == 0 || generic == "race" || generic == "grand prix" || generic == "qualifying" || generic == "practice" {
		return sportsEvidence{}
	}
	if game.EventKind == "race-session" {
		v := normalizeForMatch(value)
		series := map[string][]string{"f1": {"formula1", "formulaone"}, "motogp": {"motogp"}, "nascar": {"nascar"}, "indycar": {"indycar"}}
		found := game.League == "f1" && set["f1"]
		for _, alias := range series[game.League] {
			if strings.Contains(v, alias) {
				found = true
			}
		}
		if !found {
			return sportsEvidence{}
		}

	} else {
		expected, actual := watchStage.FindStringSubmatch(game.EventContext), watchStage.FindStringSubmatch(value)
		if len(actual) > 1 && len(expected) > 1 && actual[1] != expected[1] {
			return sportsEvidence{}
		}
		women := func(v string) bool {
			v = strings.ToLower(v)
			return strings.Contains(v, "femmes") || strings.Contains(v, "femenina") || strings.Contains(v, "women")
		}
		if women(value) != women(game.Title) {
			return sportsEvidence{}
		}
	}
	// Race identity is plausible evidence, not proof of a live broadcast: require selection.
	return sportsEvidence{score: 0.78, reason: "Race / session title match — verify broadcast", terms: sportsTokens(game.Title), on: "event-title"}
}

func streamEventQuery(r *http.Request) (string, string) {
	return r.URL.Query().Get("kind"), r.URL.Query().Get("parentId")
}
