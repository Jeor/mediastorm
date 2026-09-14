package sports

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MotoGPStanding struct {
	RiderID     string   `json:"riderId"`
	Name        string   `json:"name"`
	Number      *int     `json:"number,omitempty"`
	Team        string   `json:"team"`
	Constructor string   `json:"constructor"`
	Position    *int     `json:"position,omitempty"`
	Points      *float64 `json:"points,omitempty"`
	Wins        *int     `json:"wins,omitempty"`
	Podiums     *int     `json:"podiums,omitempty"`
}
type MotoGPCircuit struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	MapURL                string `json:"mapUrl,omitempty"`
	LengthMeters          *int   `json:"lengthMeters,omitempty"`
	LeftCorners           *int   `json:"leftCorners,omitempty"`
	RightCorners          *int   `json:"rightCorners,omitempty"`
	LongestStraightMeters *int   `json:"longestStraightMeters,omitempty"`
}
type MotoGPEnrichment struct {
	State     string           `json:"state"`
	Season    int              `json:"season"`
	Source    string           `json:"source"`
	SourceURL string           `json:"sourceUrl"`
	UpdatedAt *time.Time       `json:"updatedAt,omitempty"`
	Reason    string           `json:"reason,omitempty"`
	Standings []MotoGPStanding `json:"standings,omitempty"`
	Circuit   *MotoGPCircuit   `json:"circuit,omitempty"`
}
type motoGPEnrichmentCache struct {
	mu      sync.Mutex
	entries map[string]motoGPEnrichmentEntry
}
type motoGPEnrichmentEntry struct {
	value   MotoGPEnrichment
	expires time.Time
}
type motoGPBroadcast struct {
	ID      string `json:"id"`
	Circuit *struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Track *struct {
			Length   string `json:"lenght"`
			Left     string `json:"left_corners"`
			Right    string `json:"right_corners"`
			Straight string `json:"longest_straight"`
			Assets   struct {
				Simple struct {
					Path string `json:"path"`
				} `json:"simple"`
			} `json:"assets"`
		} `json:"track"`
	} `json:"circuit"`
}

func officialMotoGPMap(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "photos.motogp.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/events-admin/") || !strings.HasSuffix(strings.ToLower(u.Path), ".png") {
		return ""
	}
	return raw
}
func motoGPPositive(raw string, zero bool) *int {
	n, e := strconv.Atoi(raw)
	if e != nil || n < 0 || (!zero && n == 0) {
		return nil
	}
	return &n
}
func joinedMotoGPCircuit(eventID string, events []struct {
	ID          string `json:"id"`
	BroadcastID string `json:"toad_api_uuid"`
}, broadcasts []motoGPBroadcast) *MotoGPCircuit {
	broadcastID := ""
	for _, e := range events {
		if e.ID == eventID {
			broadcastID = e.BroadcastID
			break
		}
	}
	if !motoGPUUID.MatchString(broadcastID) {
		return nil
	}
	for _, b := range broadcasts {
		if b.ID != broadcastID || b.Circuit == nil || b.Circuit.Track == nil {
			continue
		}
		c, t := b.Circuit, b.Circuit.Track
		return &MotoGPCircuit{ID: c.ID, Name: c.Name, MapURL: officialMotoGPMap(t.Assets.Simple.Path), LengthMeters: motoGPPositive(t.Length, false), LeftCorners: motoGPPositive(t.Left, true), RightCorners: motoGPPositive(t.Right, true), LongestStraightMeters: motoGPPositive(t.Straight, false)}
	}
	return nil
}
func (s *Service) motoGPSeasonIDs(ctx context.Context, year int) (string, string, error) {
	var seasons []motoGPSeason
	if e := s.racingJSON(ctx, motoGPBase+"/results/seasons", &seasons); e != nil {
		return "", "", e
	}
	seasonID := ""
	for _, v := range seasons {
		if v.Year == year && motoGPUUID.MatchString(v.ID) {
			seasonID = v.ID
			break
		}
	}
	if seasonID == "" {
		return "", "", fmt.Errorf("current season unavailable")
	}
	var categories []motoGPCategory
	if e := s.racingJSON(ctx, motoGPBase+"/results/categories?seasonUuid="+seasonID, &categories); e != nil {
		return "", "", e
	}
	for _, v := range categories {
		if v.LegacyID == 3 && motoGPUUID.MatchString(v.ID) {
			return seasonID, v.ID, nil
		}
	}
	return "", "", fmt.Errorf("MotoGP category unavailable")
}
func (s *Service) GetMotoGPEnrichment(ctx context.Context, eventID string) MotoGPEnrichment {
	year := time.Now().UTC().Year()
	value := MotoGPEnrichment{State: "unavailable", Season: year, Source: "MotoGP", SourceURL: "https://www.motogp.com/en/world-standing", Reason: "Current-season championship data unavailable"}
	if eventID != "" {
		value.SourceURL = motoGPBase + "/events?seasonYear=" + strconv.Itoa(year)
		value.Reason = "Official circuit guide unavailable"
		if !motoGPUUID.MatchString(eventID) {
			return value
		}
	}
	s.motoGPExtra.mu.Lock()
	defer s.motoGPExtra.mu.Unlock()
	key := strconv.Itoa(year) + ":" + eventID
	if s.motoGPExtra.entries == nil {
		s.motoGPExtra.entries = map[string]motoGPEnrichmentEntry{}
	}
	old := s.motoGPExtra.entries[key]
	if time.Now().Before(old.expires) {
		return old.value
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	seasonID, categoryID, err := s.motoGPSeasonIDs(ctx, year)
	if err == nil && eventID == "" {
		var raw struct {
			Classification []struct {
				Position *int     `json:"position"`
				Points   *float64 `json:"points"`
				Wins     *int     `json:"race_wins"`
				Podiums  *int     `json:"podiums"`
				Rider    struct {
					ID     string `json:"id"`
					Name   string `json:"full_name"`
					Number *int   `json:"number"`
				} `json:"rider"`
				Team struct {
					Name string `json:"name"`
				} `json:"team"`
				Constructor struct {
					Name string `json:"name"`
				} `json:"constructor"`
			} `json:"classification"`
		}
		err = s.racingJSON(ctx, motoGPBase+"/results/standings?seasonUuid="+seasonID+"&categoryUuid="+categoryID, &raw)
		if err == nil {
			if len(raw.Classification) == 0 || len(raw.Classification) > 100 {
				err = fmt.Errorf("unexpected standings size")
			} else {
				for _, r := range raw.Classification {
					if !motoGPUUID.MatchString(r.Rider.ID) || strings.TrimSpace(r.Rider.Name) == "" {
						err = fmt.Errorf("invalid rider")
						break
					}
					value.Standings = append(value.Standings, MotoGPStanding{r.Rider.ID, r.Rider.Name, r.Rider.Number, r.Team.Name, r.Constructor.Name, r.Position, r.Points, r.Wins, r.Podiums})
				}
			}
		}
	} else if err == nil {
		var events []struct {
			ID          string `json:"id"`
			BroadcastID string `json:"toad_api_uuid"`
		}
		var broadcasts []motoGPBroadcast
		err = s.racingJSON(ctx, motoGPBase+"/results/events?seasonUuid="+seasonID, &events)
		if err == nil {
			err = s.racingJSON(ctx, value.SourceURL, &broadcasts)
		}
		if err == nil {
			value.Circuit = joinedMotoGPCircuit(eventID, events, broadcasts)
			if value.Circuit == nil {
				err = fmt.Errorf("circuit identity unavailable")
			}
		}
	}
	ttl := 15 * time.Minute
	if err == nil {
		now := time.Now().UTC()
		value.State = "available"
		value.Reason = ""
		value.UpdatedAt = &now
	} else {
		value.Standings = nil
		value.Circuit = nil
		ttl = time.Minute
		if old.value.UpdatedAt != nil {
			value = old.value
			value.State = "stale"
			value.Reason = "Showing previously retrieved MotoGP data; refresh unavailable"
		}
	}
	// Only current-season entries, with a fixed maximum independent of user-supplied UUIDs.
	if len(s.motoGPExtra.entries) >= 64 {
		for k := range s.motoGPExtra.entries {
			delete(s.motoGPExtra.entries, k)
			break
		}
	}
	s.motoGPExtra.entries[key] = motoGPEnrichmentEntry{value, time.Now().Add(ttl)}
	return value
}
