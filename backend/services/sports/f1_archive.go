package sports

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"novastream/models"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Final-session snapshots only. A successful archive request never establishes live coverage.
type F1Archive struct {
	State       string                     `json:"state"`
	Mode        string                     `json:"mode"`
	EventID     string                     `json:"eventId"`
	SessionName string                     `json:"sessionName,omitempty"`
	SourceURL   string                     `json:"sourceUrl,omitempty"`
	CheckedAt   time.Time                  `json:"checkedAt"`
	Reason      string                     `json:"reason,omitempty"`
	Channels    map[string]json.RawMessage `json:"channels,omitempty"`
}
type f1ArchiveEntry struct {
	value   F1Archive
	expires time.Time
}
type f1ArchiveCache struct {
	sync.Mutex
	entries map[string]f1ArchiveEntry
}
type f1ArchiveIndex struct {
	Meetings []struct {
		Sessions []struct {
			Name      string `json:"Name"`
			StartDate string `json:"StartDate"`
			GmtOffset string `json:"GmtOffset"`
			Path      string `json:"Path"`
		} `json:"Sessions"`
	} `json:"Meetings"`
}

var f1ArchivePath = regexp.MustCompile(`^[0-9]{4}/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+/$`)

func f1SessionName(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "fp1", "p1", "practice 1":
		return "Practice 1"
	case "fp2", "p2", "practice 2":
		return "Practice 2"
	case "fp3", "p3", "practice 3":
		return "Practice 3"
	case "qual", "qualifying":
		return "Qualifying"
	case "race":
		return "Race"
	case "sprint", "sprint race":
		return "Sprint"
	case "sprint qualifying", "sprint shootout", "sq":
		return "Sprint Qualifying"
	}
	return ""
}
func (s *Service) GetF1Archive(ctx context.Context, event models.SportsEvent) F1Archive {
	out := F1Archive{State: "unavailable", Mode: "archive", EventID: event.ID, CheckedAt: time.Now(), Reason: "Archived timing is available only for a completed F1 session."}
	if event.League != "f1" || event.Status != models.SportsGameFinal || event.StartTime.IsZero() {
		return out
	}
	name := f1SessionName(event.SessionType)
	if name == "" {
		out.Reason = "This session type has no verified archive mapping."
		return out
	}
	s.f1Archive.Lock()
	defer s.f1Archive.Unlock()
	if old, ok := s.f1Archive.entries[event.ID]; ok && time.Now().Before(old.expires) {
		return old.value
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	base := "https://livetiming.formula1.com/static/"
	var index f1ArchiveIndex
	if err := s.f1ArchiveJSON(ctx, fmt.Sprintf("%s%d/Index.json", base, event.StartTime.Year()), &index); err != nil {
		out.Reason = "F1 archive could not be reached."
		return out
	}
	path := ""
	for _, m := range index.Meetings {
		for _, session := range m.Sessions {
			offset := session.GmtOffset
			if len(offset) == 8 {
				offset = offset[:5]
			}
			if len(offset) == 5 {
				offset = "+" + offset
			}
			start, err := time.Parse("2006-01-02T15:04:05Z07:00", session.StartDate+offset)
			// Match session name and start time, never the nearest weekend or latest archive.
			if err == nil && session.Name == name && start.Equal(event.StartTime) && f1ArchivePath.MatchString(session.Path) {
				path = session.Path
			}
		}
	}
	if path == "" {
		out.Reason = "No archive matches this session's type and start time."
	} else {
		out.SourceURL = base + path
		out.SessionName = name
		out.Channels = map[string]json.RawMessage{}
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, topic := range []string{"SessionInfo", "SessionData", "DriverList", "TimingData", "TimingAppData", "RaceControlMessages", "WeatherData", "LapCount", "TrackStatus"} {
			wg.Add(1)
			go func(topic string) {
				defer wg.Done()
				var raw json.RawMessage
				if s.f1ArchiveJSON(ctx, out.SourceURL+topic+".json", &raw) == nil && len(raw) > 2 {
					mu.Lock()
					out.Channels[topic] = raw
					mu.Unlock()
				}
			}(topic)
		}
		wg.Wait()
		if len(out.Channels) > 0 {
			out.State = "available"
			out.Reason = ""
		} else {
			out.Reason = "No archived channels were returned."
		}
	}
	if s.f1Archive.entries == nil {
		s.f1Archive.entries = map[string]f1ArchiveEntry{}
	}
	if len(s.f1Archive.entries) >= 64 {
		s.f1Archive.entries = map[string]f1ArchiveEntry{}
	}
	s.f1Archive.entries[event.ID] = f1ArchiveEntry{out, time.Now().Add(5 * time.Minute)}
	return out
}

// F1 static files carry a UTF-8 BOM; the generic ESPN decoder intentionally does not.
func (s *Service) f1ArchiveJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("F1 archive status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}), target)
}
