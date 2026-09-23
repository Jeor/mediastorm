package sports

import (
	"context"
	"fmt"
	"golang.org/x/net/html"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type cyclingEvidence struct {
	Provider   string     `json:"provider"`
	ObservedAt time.Time  `json:"observedAt"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}
type cyclingResult struct {
	Bib         int    `json:"bib,omitempty"`
	Nationality string `json:"nationality,omitempty"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Team        string `json:"team,omitempty"`
	Rank        int    `json:"rank,omitempty"`
	Time        string `json:"time,omitempty"`
	Gap         string `json:"gap,omitempty"`
}
type cyclingResults struct {
	State  string          `json:"state"`
	Source cyclingEvidence `json:"source"`
	Data   []cyclingResult `json:"data,omitempty"`
	Reason string          `json:"reason,omitempty"`
}
type cyclingTeamResult struct {
	ID     string `json:"id"`
	TeamID string `json:"teamId"`
	Name   string `json:"name"`
	Rank   int    `json:"rank,omitempty"`
	Time   string `json:"time,omitempty"`
	Gap    string `json:"gap,omitempty"`
}
type cyclingTeamResults struct {
	State  string              `json:"state"`
	Source cyclingEvidence     `json:"source"`
	Data   []cyclingTeamResult `json:"data,omitempty"`
	Reason string              `json:"reason,omitempty"`
}
type CyclingStage struct {
	TeamResults                *cyclingTeamResults `json:"teamResults,omitempty"`
	RouteGuide                 *CyclingRouteGuide  `json:"routeGuide,omitempty"`
	Terrain                    string              `json:"terrain,omitempty"`
	ID                         string              `json:"id"`
	Name                       string              `json:"name"`
	Date                       string              `json:"date,omitempty"`
	Departure                  string              `json:"departure,omitempty"`
	Arrival                    string              `json:"arrival,omitempty"`
	Distance                   string              `json:"distance,omitempty"`
	Status                     string              `json:"status"`
	Results                    cyclingResults      `json:"results"`
	GeneralClassification      cyclingResults      `json:"generalClassification"`
	GeneralClassificationLabel string              `json:"generalClassificationLabel,omitempty"`
}
type CyclingRace struct {
	CalendarOnly   bool            `json:"calendarOnly,omitempty"`
	Country        string          `json:"country,omitempty"`
	Classification string          `json:"classification,omitempty"`
	ScheduleOnly   bool            `json:"scheduleOnly,omitempty"`
	RestDays       []string        `json:"restDays,omitempty"`
	RaceKind       string          `json:"raceKind"`
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Category       string          `json:"category"`
	StartDate      string          `json:"startDate,omitempty"`
	EndDate        string          `json:"endDate,omitempty"`
	Stages         []CyclingStage  `json:"stages"`
	Source         cyclingEvidence `json:"source"`
	SourceURL      string          `json:"sourceUrl"`
}
type CyclingCoverage struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}
type CyclingFeed struct {
	State    string            `json:"state"`
	Source   cyclingEvidence   `json:"source"`
	Data     []CyclingRace     `json:"data"`
	Reason   string            `json:"reason,omitempty"`
	Coverage []CyclingCoverage `json:"coverage"`
}
type cyclingCompetition struct {
	id, name, category, host string
	oneDay                   bool
}

var cyclingCompetitions = []cyclingCompetition{
	{"tour", "Tour de France", "men", "racecenter.letour.fr", false},
	{"vuelta", "La Vuelta", "men", "racecenter.lavuelta.es", false},
	{"tour-femmes", "Tour de France Femmes", "women", "racecenter.letourfemmes.fr", false},
	{"paris-nice", "Paris-Nice", "men", "racecenter.paris-nice.fr", false},
	{"vuelta-femenina", "La Vuelta Femenina", "women", "racecenter.lavueltafemenina.es", false},
	{"paris-roubaix", "Paris-Roubaix", "men", "racecenter.paris-roubaix.fr", true},
	{"paris-roubaix-femmes", "Paris-Roubaix Femmes", "women", "racecenter.paris-roubaix-femmes.fr", true},
	{"liege-bastogne-liege", "Liège-Bastogne-Liège", "men", "racecenter.liege-bastogne-liege.be", true},
	{"liege-bastogne-liege-femmes", "Liège-Bastogne-Liège Femmes", "women", "racecenter.liege-bastogne-liege-femmes.be", true},
	{"fleche-wallonne", "La Flèche Wallonne", "men", "racecenter.la-fleche-wallonne.be", true},
	{"fleche-wallonne-femmes", "La Flèche Wallonne Femmes", "women", "racecenter.la-fleche-wallonne-femmes.be", true},
	{"giro", "Giro d’Italia", "men", "www.giroditalia.it", false},
	{"cro-race", "CRO Race", "men", "crorace.com", false},
	{"road-worlds", "UCI Road World Championships", "unknown", "www.montreal2026.org", false},
}

type cyclingBoardCache struct {
	race    CyclingRace
	expires time.Time
	stale   bool
}
type cyclingStageCache struct {
	stage   CyclingStage
	expires time.Time
}
type cyclingCache struct {
	subscription cyclingSubscriptionCache
	mu           sync.Mutex
	detailMu     sync.Mutex
	boards       map[string]cyclingBoardCache
	stages       map[string]cyclingStageCache
	guides       map[string]cyclingGuideCache
}

type asoStage struct {
	Type      string  `json:"type"`
	ID        string  `json:"_id"`
	Stage     int     `json:"stage"`
	Date      string  `json:"date"`
	Length    float64 `json:"lengthDisplay"`
	Cancelled bool    `json:"isCancelled"`
	UpdatedAt int64   `json:"_updatedAt"`
	Departure struct {
		Label string `json:"label"`
	} `json:"departureCity"`
	Arrival struct {
		Label string `json:"label"`
	} `json:"arrivalCity"`
}
type asoRanking struct {
	Bib             int      `json:"bib"`
	Nationality     string   `json:"nationality"`
	ID              string   `json:"_id"`
	Bind            string   `json:"_bind"`
	Parent          string   `json:"_parent"`
	Type            string   `json:"type"`
	Types           []string `json:"types"`
	Checkpoint      string   `json:"$cp"`
	UpdatedAt       int64    `json:"_updatedAt"`
	Firstname       string   `json:"firstname"`
	Lastname        string   `json:"lastname"`
	Name            string   `json:"name"`
	Team            string   `json:"$team"`
	CheckpointTypes []struct {
		Type string `json:"type"`
		Code string `json:"code"`
	} `json:"checkpointTypes"`
	Rankings []struct {
		Position int      `json:"position"`
		Absolute *float64 `json:"absolute"`
		Relative *float64 `json:"relative"`
		Rider    string   `json:"$rider"`
	} `json:"rankings"`
}

func cyclingSource(now time.Time, updated int64) cyclingEvidence {
	e := cyclingEvidence{Provider: "ASO race center", ObservedAt: now}
	if updated > 0 {
		t := time.UnixMilli(updated)
		e.UpdatedAt = &t
	}
	return e
}
func cyclingPending(e cyclingEvidence, reason string) cyclingResults {
	return cyclingResults{State: "pending", Source: e, Reason: reason}
}
func normalizeCyclingSchedule(raw []asoStage, c cyclingCompetition, year int, now time.Time) CyclingRace {
	race := CyclingRace{RaceKind: "stage-race", ID: fmt.Sprintf("aso:%s:%d", c.id, year), Name: c.name, Category: c.category, Stages: []CyclingStage{}, Source: cyclingSource(now, 0), SourceURL: "https://" + c.host + "/en/"}
	if c.oneDay {
		race.RaceKind = "one-day"
	}
	sort.SliceStable(raw, func(i, j int) bool { return raw[i].Stage < raw[j].Stage })
	seen := map[int]bool{}
	latest := int64(0)
	for _, item := range raw {
		if item.Stage < 1 || item.Stage > 40 || item.ID == "" || len(item.Date) < 10 || seen[item.Stage] {
			continue
		}
		date := item.Date[:10]
		if _, err := time.Parse("2006-01-02", date); err != nil || !strings.HasPrefix(date, strconv.Itoa(year)+"-") {
			continue
		}
		seen[item.Stage] = true
		e := cyclingSource(now, item.UpdatedAt)
		terrain := map[string]string{"PLN": "Flat", "VAL": "Hilly", "HMG": "Mountain", "PAS": "Individual time trial", "EQU": "Team time trial", "REP": "Rest day"}[item.Type]
		stage := CyclingStage{Terrain: terrain, ID: race.ID + ":" + strconv.Itoa(item.Stage), Name: fmt.Sprintf("Stage %d", item.Stage), Date: date, Departure: item.Departure.Label, Arrival: item.Arrival.Label, Status: "unknown", Results: cyclingPending(e, "Select this stage to load published results."), GeneralClassification: cyclingPending(e, "Select this stage to load the overall classification."), GeneralClassificationLabel: fmt.Sprintf("After stage %d", item.Stage)}
		if site, ok := asoGuideSites[c.id]; ok {
			stage.RouteGuide = &CyclingRouteGuide{State: "pending", Source: e, SourceURL: fmt.Sprintf("https://%s/en/stage-%d", site.host, item.Stage)}
		}
		if c.oneDay {
			stage.Name = "Race"
			markCyclingOneDay(&stage)
		}
		if item.Cancelled {
			stage.Status = "cancelled"
		} else if date > now.UTC().Format("2006-01-02") {
			stage.Status = "scheduled"
		}
		if item.Length > 0 && item.Length < 1000 {
			stage.Distance = strconv.FormatFloat(item.Length, 'f', -1, 64) + " km"
		}
		if item.UpdatedAt > latest {
			latest = item.UpdatedAt
		}
		race.Stages = append(race.Stages, stage)
	}
	if len(race.Stages) > 0 {
		race.StartDate = race.Stages[0].Date
		race.EndDate = race.Stages[len(race.Stages)-1].Date
	}
	race.Source = cyclingSource(now, latest)
	return race
}
func cyclingLeagueID(id string) string {
	if id == "cro-race" {
		return "cro:cro-race"
	}
	if id == "road-worlds" {
		return "uci:road-worlds"
	}
	if id == "giro" {
		return "rcs:" + id
	}
	return "aso:" + id
}

func (s *Service) enabledCyclingCompetitions() []cyclingCompetition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// A zero-value service is used by provider tests; production initializes its league list.
	if s.leagues == nil {
		return cyclingCompetitions
	}
	enabled := map[string]bool{}
	for _, league := range s.leagues {
		enabled[league.ID] = true
	}
	out := []cyclingCompetition{}
	for _, competition := range cyclingCompetitions {
		if enabled[cyclingLeagueID(competition.id)] {
			out = append(out, competition)
		}
	}
	return out
}

func (s *Service) GetCycling(ctx context.Context) CyclingFeed {
	competitions := s.enabledCyclingCompetitions()
	now := time.Now()
	year := now.UTC().Year()
	cache := &s.cycling
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.boards == nil {
		cache.boards = map[string]cyclingBoardCache{}
	}
	feed := CyclingFeed{State: "available", Source: cyclingEvidence{Provider: "Cycling organizers", ObservedAt: now}, Data: []CyclingRace{}, Coverage: []CyclingCoverage{}}
	// A single board request has one shared network budget. Slow providers cannot
	// turn multiple feeds into sequential timeouts. Workers update disjoint
	// result slots; the cache is committed only after they finish.
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	boards := make([]cyclingBoardCache, len(competitions))
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	var subscription cyclingSubscriptionCache
	if len(competitions) > 0 {
		workers.Add(1)
		slots <- struct{}{}
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			subscription = s.cyclingSubscription(requestCtx, year, now)
		}()
	}
	for i, c := range competitions {
		key := fmt.Sprintf("%s:%d", c.id, year)
		old, exists := cache.boards[key]
		boards[i] = old
		if exists && now.Before(old.expires) {
			continue
		}
		// Local, verified calendars must not wait behind remote provider timeouts.
		if c.id == "cro-race" || c.id == "road-worlds" {
			race, err := publishedCyclingRace(c.id, year, now)
			if err == nil {
				old = cyclingBoardCache{race: race, expires: now.Add(5 * time.Minute)}
			} else {
				old.stale = true
				old.expires = now.Add(30 * time.Second)
			}
			boards[i] = old
			continue
		}
		workers.Add(1)
		go func(i int, c cyclingCompetition, old cyclingBoardCache) {
			defer workers.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-requestCtx.Done():
				old.stale = true
				old.expires = now.Add(30 * time.Second)
				boards[i] = old
				return
			}
			providerCtx, providerCancel := context.WithTimeout(requestCtx, 6*time.Second)
			var race CyclingRace
			var err error
			if c.id == "giro" {
				var doc *html.Node
				doc, err = s.giroHTML(providerCtx, "the-route/")
				if err == nil {
					race, err = normalizeGiroSchedule(doc, year, now)
				}
			} else {
				var raw []asoStage
				err = s.racingJSON(providerCtx, fmt.Sprintf("https://%s/api/stage-%d", c.host, year), &raw)
				if err == nil {
					race = normalizeCyclingSchedule(raw, c, year, now)
				}
			}
			providerCancel()
			if err == nil {
				if len(race.Stages) == 0 {
					err = fmt.Errorf("no valid stages")
				} else {
					old = cyclingBoardCache{race: race, expires: now.Add(5 * time.Minute)}
				}
			}

			if err != nil {
				old.stale = true
				old.expires = now.Add(30 * time.Second)
			}
			boards[i] = old
		}(i, c, old)
	}
	workers.Wait()
	for i, c := range competitions {
		old := boards[i]
		cache.boards[fmt.Sprintf("%s:%d", c.id, year)] = old
		state := "available"
		if old.stale {
			state = "stale"
			feed.State = "stale"
		}
		if old.race.ID == "" {
			state = "unavailable"
		} else {
			feed.Data = append(feed.Data, old.race)
		}
		feed.Coverage = append(feed.Coverage, CyclingCoverage{ID: c.id, Name: c.name, State: state})
	}
	if len(competitions) > 0 {
		cache.subscription = subscription
		mergeCyclingSubscription(&feed, subscription, competitions, year)
	}
	if len(feed.Data) == 0 {
		feed.State = "unavailable"
		feed.Reason = "Cycling race schedules could not be reached."
		if len(competitions) == 0 {
			feed.Reason = "Cycling is disabled in server league availability."
		}
	}
	// Retain current-season cache entries only.
	for key := range cache.boards {
		if !strings.HasSuffix(key, ":"+strconv.Itoa(year)) {
			delete(cache.boards, key)
		}
	}
	return feed
}
func markCyclingOneDay(stage *CyclingStage) {
	stage.GeneralClassification = cyclingResults{State: "unavailable", Source: stage.Results.Source, Reason: "This one-day race has no overall stage classification."}
	stage.GeneralClassificationLabel = ""
}
func cyclingTime(ms *float64, gap bool) string {
	if ms == nil || *ms < 0 || math.IsNaN(*ms) || math.IsInf(*ms, 0) || *ms > 1e12 || (!gap && *ms == 0) {
		return ""
	}
	seconds := int64(*ms / 1000)
	h := seconds / 3600
	m := (seconds % 3600) / 60
	sec := seconds % 60
	value := fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	if gap {
		if h == 0 {
			value = fmt.Sprintf("%d:%02d", m, sec)
		}
		value = "+" + value
	}
	if remainder := int64(*ms) % 1000; remainder != 0 {
		value += fmt.Sprintf(".%03d", remainder)
	}
	return value
}
func normalizeCyclingResults(raw []asoRanking, stage *CyclingStage, year int, number int, now time.Time) {
	records := map[string]asoRanking{}
	for _, r := range raw {
		records[r.Bind+":"+r.ID] = r
	}
	for _, kind := range []string{"ite", "itg", "ttt"} {
		if kind == "ttt" && stage.Terrain != "Team time trial" {
			continue
		}
		result := cyclingResults{State: "pending", Source: cyclingSource(now, 0), Reason: "No finish classification was found in this feed capture."}
		matches := []asoRanking{}
		for _, r := range raw {
			if r.Bind != fmt.Sprintf("rankingType-%d-%d", year, number) || r.Type != kind {
				continue
			}
			cp, ok := records[r.Checkpoint]
			if !ok {
				continue
			}
			arrival := false
			for _, t := range cp.CheckpointTypes {
				if t.Type == "arrival" && t.Code == "A" {
					arrival = true
				}
			}
			hasArrivalType := false
			for _, kind := range r.Types {
				if kind == "A" {
					hasArrivalType = true
				}
			}
			if arrival && hasArrivalType {
				matches = append(matches, r)
			}
		}
		// Never guess between conflicting checkpoint classifications.
		if len(matches) == 1 {
			r := matches[0]
			result.Source = cyclingSource(now, r.UpdatedAt)
			result.Data = []cyclingResult{}
			seen := map[string]bool{}
			partial := false
			for _, entry := range r.Rankings {
				rider, ok := records[entry.Rider]
				identity := entry.Rider
				if kind == "ttt" {
					// ASO TTT rows reference a representative rider; resolve the team
					// without presenting the representative as an individual result.
					team, teamOK := records[rider.Team]
					identity = rider.Team
					if !ok || !teamOK || team.Bind != fmt.Sprintf("team-%d", year) || team.Name == "" || seen[identity] {
						partial = true
						continue
					}
					rider.Firstname, rider.Lastname = team.Name, ""
				}
				if !ok || rider.Firstname == "" || (kind != "ttt" && rider.Lastname == "") || seen[identity] {
					partial = true
					continue
				}
				seen[identity] = true
				rank := entry.Position
				if rank < 0 {
					rank = 0
				}
				gap := ""
				if rank > 0 {
					gap = cyclingTime(entry.Relative, true)
				}
				bib, nationality := 0, ""
				if kind != "ttt" {
					if rider.Bib > 0 && rider.Bib < 10000 {
						bib = rider.Bib
					}
					code := strings.ToUpper(strings.TrimSpace(rider.Nationality))
					if len(code) == 3 && strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") == "" {
						nationality = code
					}
				}
				result.Data = append(result.Data, cyclingResult{Bib: bib, Nationality: nationality, ID: stage.ID + ":" + identity, Name: strings.TrimSpace(rider.Firstname + " " + rider.Lastname), Team: records[rider.Team].Name, Rank: rank, Time: cyclingTime(entry.Absolute, false), Gap: gap})
			}
			if len(result.Data) > 0 {
				result.State = "available"
				result.Reason = ""
				if partial {
					result.State = "stale"
					result.Reason = "Some riders could not be resolved."
				}
				sort.SliceStable(result.Data, func(i, j int) bool {
					a, b := result.Data[i].Rank, result.Data[j].Rank
					if a == 0 {
						return false
					}
					if b == 0 {
						return true
					}
					return a < b
				})
			}
		}
		if kind == "ite" {
			stage.Results = result
		} else if kind == "ttt" {
			stage.TeamResults = &cyclingTeamResults{State: result.State, Source: result.Source, Reason: result.Reason}
			for _, row := range result.Data {
				stage.TeamResults.Data = append(stage.TeamResults.Data, cyclingTeamResult{ID: row.ID, TeamID: strings.TrimPrefix(row.ID, stage.ID+":"), Name: row.Name, Rank: row.Rank, Time: row.Time, Gap: row.Gap})
			}
			if result.State == "stale" {
				stage.TeamResults.Reason = "Some teams could not be resolved."
			}
		} else {
			stage.GeneralClassification = result
		}
	}
}
func (s *Service) GetCyclingStage(ctx context.Context, raceID string, year, number int) (CyclingStage, error) {
	if year != time.Now().UTC().Year() || number < 1 || number > 40 {
		return CyclingStage{}, fmt.Errorf("unsupported cycling date or stage")
	}
	// Resolve calendar-only entries from the trusted board before organizer lookup.
	if number == 1 {
		for _, race := range s.GetCycling(ctx).Data {
			parts := strings.Split(race.ID, ":")
			if len(parts) == 3 && parts[1] == raceID && race.CalendarOnly && len(race.Stages) > 0 {
				return race.Stages[0], nil
			}
		}
	}
	var c cyclingCompetition
	for _, candidate := range cyclingCompetitions {
		if candidate.id == raceID {
			c = candidate
		}
	}
	if c.id == "" {
		return CyclingStage{}, fmt.Errorf("unsupported cycling race")
	}
	board := s.GetCycling(ctx)
	id := fmt.Sprintf("%s:%d:%d", cyclingLeagueID(raceID), year, number)
	var stage CyclingStage
	for _, race := range board.Data {
		for _, candidate := range race.Stages {
			if candidate.ID == id {
				stage = candidate
			}
		}
	}
	if stage.ID == "" {
		return stage, fmt.Errorf("stage not found")
	}
	if raceID == "cro-race" || raceID == "road-worlds" {
		return stage, nil
	}
	cache := &s.cycling
	cache.detailMu.Lock()
	defer cache.detailMu.Unlock()
	if cache.stages == nil {
		cache.stages = map[string]cyclingStageCache{}
	}
	now := time.Now()
	old, exists := cache.stages[id]
	if exists && now.Before(old.expires) {
		return old.stage, nil
	}
	if stage.Status == "cancelled" || stage.Status == "scheduled" {
		s.enrichCyclingGuide(ctx, &stage, year, number, now)
		return stage, nil
	}
	var raw []asoRanking
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var err error
	if c.id == "giro" {
		err = s.enrichGiroStage(requestCtx, &stage, year, number, now)
	} else {
		err = s.racingJSON(requestCtx, fmt.Sprintf("https://%s/api/rankingType-%d-%d", c.host, year, number), &raw)
	}
	ttl := 5 * time.Minute
	if err != nil {
		ttl = 30 * time.Second
		if exists {
			stage = old.stage
		}
		if stage.TeamResults != nil {
			if len(stage.TeamResults.Data) > 0 {
				stage.TeamResults.State = "stale"
			} else {
				stage.TeamResults.State = "unavailable"
			}
			stage.TeamResults.Reason = "The cycling results feed could not be reached."
		}
		for _, module := range []*cyclingResults{&stage.Results, &stage.GeneralClassification} {
			if len(module.Data) > 0 {
				module.State = "stale"
			} else {
				module.State = "unavailable"
			}
			module.Reason = "The cycling results feed could not be reached."
		}
	} else if c.id != "giro" {
		normalizeCyclingResults(raw, &stage, year, number, now)
	}
	if c.id == "giro" && err == nil && stage.GeneralClassification.State == "unavailable" && exists && len(old.stage.GeneralClassification.Data) > 0 {
		reason := stage.GeneralClassification.Reason
		stage.GeneralClassification = old.stage.GeneralClassification
		stage.GeneralClassification.State = "stale"
		stage.GeneralClassification.Reason = reason
		ttl = 30 * time.Second
	}
	if c.oneDay {
		markCyclingOneDay(&stage)
	}
	s.enrichCyclingGuide(ctx, &stage, year, number, now)
	if stage.RouteGuide != nil && (stage.RouteGuide.State == "stale" || stage.RouteGuide.State == "unavailable") {
		ttl = 30 * time.Second
	}
	if len(cache.stages) >= 32 {
		oldest := ""
		var expiry time.Time
		for key, value := range cache.stages {
			if oldest == "" || value.expires.Before(expiry) {
				oldest = key
				expiry = value.expires
			}
		}
		delete(cache.stages, oldest)
	}
	cache.stages[id] = cyclingStageCache{stage: stage, expires: now.Add(ttl)}
	return stage, nil
}
