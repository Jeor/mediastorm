package sports

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func subscriptionFixture(year int) string {
	return fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:race-one\r\nDTSTART;VALUE=DATE:%d0922\r\nDTEND;VALUE=DATE:%d0928\r\nSUMMARY:Tour of Test (2.Pro)\r\nLOCATION:Croatia\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:race-two\r\nDTSTART;VALUE=DATE:%d0926\r\nDTEND;VALUE=DATE:%d0927\r\nSUMMARY:Paris-Roubaix (1.WWT)\r\nSTATUS:CANCELLED\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", year, year, year, year)
}
func TestCyclingSubscriptionDatesIdentityAndValidation(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	races, err := parseCyclingSubscription(subscriptionFixture(2026), 2026, now)
	if err != nil || len(races) != 2 {
		t.Fatal(races, err)
	}
	race := races[0]
	if race.StartDate != "2026-09-22" || race.EndDate != "2026-09-27" || !race.CalendarOnly || len(race.Stages) != 1 || race.Stages[0].Name != "Race schedule" || race.Stages[0].Status != "unknown" {
		t.Fatal("invented stage/lifecycle or incorrect exclusive end", race)
	}
	if races[1].Name != "Paris-Roubaix Women" || races[1].Category != "women" || races[1].Stages[0].Status != "cancelled" {
		t.Fatal("lost category/cancellation", races[1])
	}
	next, _ := parseCyclingSubscription(strings.ReplaceAll(subscriptionFixture(2026), "Tour of Test", "Renamed Tour"), 2026, now.Add(time.Hour))
	if next[0].ID != race.ID {
		t.Fatal("IDs depend on mutable names")
	}
	for _, body := range []string{"<html>blocked</html>", strings.ReplaceAll(subscriptionFixture(2026), "END:VCALENDAR", ""), subscriptionFixture(2025), strings.ReplaceAll(subscriptionFixture(2026), "DTSTART;VALUE=DATE:", "DTSTART:")} {
		if _, err := parseCyclingSubscription(body, 2026, now); err == nil {
			t.Fatal("accepted invalid or wrong-year calendar")
		}
	}
}
func TestCyclingSubscriptionCacheFailureRestartAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s := NewService(dir)
	calls := 0
	offline := false
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if offline {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("offline"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(subscriptionFixture(2026)))}, nil
	})}
	first := s.cyclingSubscription(context.Background(), 2026, now)
	if first.Stale || len(first.Races) != 2 {
		t.Fatal(first)
	}
	s.cycling.subscription = first
	_ = s.cyclingSubscription(context.Background(), 2026, now.Add(time.Hour))
	if calls != 1 {
		t.Fatal("overpolling", calls)
	}
	offline = true
	restarted := NewService(dir)
	restarted.client = s.client
	saved := restarted.cyclingSubscription(context.Background(), 2026, now.Add(time.Hour))
	if len(saved.Races) != 2 || calls != 1 {
		t.Fatal("lost disk cache")
	}
	restarted.cycling.subscription = saved
	stale := restarted.cyclingSubscription(context.Background(), 2026, now.Add(7*time.Hour))
	if !stale.Stale || len(stale.Races) != 2 || !stale.Races[0].Source.ObservedAt.Equal(now) {
		t.Fatal("lost last good evidence", stale)
	}
	restarted.cycling.subscription = stale
	_ = restarted.cyclingSubscription(context.Background(), 2026, now.Add(7*time.Hour+time.Minute))
	if calls != 2 {
		t.Fatal("failure backoff lost", calls)
	}
	expired := restarted.cyclingSubscription(context.Background(), 2026, now.Add(8*24*time.Hour))
	if len(expired.Races) != 0 {
		t.Fatal("kept expired calendar")
	}
	if next := restarted.cyclingSubscription(context.Background(), 2027, now.AddDate(1, 0, 0)); len(next.Races) != 0 {
		t.Fatal("rolled calendar to new year")
	}
}
func TestCyclingSubscriptionMergeAndTrustedStage(t *testing.T) {
	year := time.Now().UTC().Year()
	if year != 2026 {
		t.Skip("calendar source edition")
	}
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"cro:cro-race"})
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(subscriptionFixture(year)))}, nil
	})}
	feed := s.GetCycling(context.Background())
	if len(feed.Data) != 2 {
		t.Fatalf("missing calendar or re-enabled disabled Paris-Roubaix: %+v", feed.Data)
	}
	race := feed.Data[1]
	stage, err := s.GetCyclingStage(context.Background(), strings.Split(race.ID, ":")[1], year, 1)
	if err != nil || stage.ID != race.Stages[0].ID {
		t.Fatal(stage, err)
	}
	if _, err := s.GetCyclingStage(context.Background(), "road-invented", year, 1); err == nil {
		t.Fatal("accepted client identity")
	}
	// Organizer data wins even when the public calendar disagrees on dates.
	races, _ := parseCyclingSubscription(subscriptionFixture(year), year, time.Now())
	races[0].Name = "CRO Race"
	original := feed.Data[0]
	board := CyclingFeed{Data: []CyclingRace{original}}
	mergeCyclingSubscription(&board, cyclingSubscriptionCache{Races: races}, []cyclingCompetition{{id: "cro-race"}}, year)
	if len(board.Data) != 1 || board.Data[0].ID != original.ID {
		t.Fatal("duplicate organizer event", board)
	}
}
func TestCyclingDownloadedSubscription(t *testing.T) {
	path := os.Getenv("CYCLING_CALENDAR_VERIFY_FILE")
	if path == "" {
		t.Skip("optional real downloaded feed")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	races, err := parseCyclingSubscription(string(raw), 2026, time.Now())
	if err != nil || len(races) < 200 {
		t.Fatal(len(races), err)
	}
	t.Logf("Verified %d calendar entries", len(races))
}

func TestCyclingPublicSubscriptionLive(t *testing.T) {
	if os.Getenv("CYCLING_CALENDAR_VERIFY_LIVE") != "1" {
		t.Skip("explicit external source verification")
	}
	if time.Now().UTC().Year() != 2026 {
		t.Skip("2026 feed")
	}
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"cro:cro-race", "uci:road-worlds"})
	feed := s.GetCycling(context.Background())
	if len(feed.Data) < 200 {
		t.Fatalf("live subscription missing: %d races; coverage %+v", len(feed.Data), feed.Coverage)
	}
	ids := map[string]bool{}
	found := false
	for _, race := range feed.Data {
		if ids[race.ID] {
			t.Fatal("duplicate race", race.ID)
		}
		ids[race.ID] = true
		if race.Name == "Omloop Van Het Houtland" && race.StartDate == "2026-09-23" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected current calendar event absent")
	}
	t.Logf("Live feed returned %d unique races including organizer schedules and the current one-day event", len(feed.Data))
}

func TestCyclingSubscriptionRespectsDisabledSport(t *testing.T) {
	s := NewService(t.TempDir())
	s.SetEnabledLeagueIDs([]string{"nfl"})
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("disabled cycling fetched a source")
		return nil, nil
	})}
	if feed := s.GetCycling(context.Background()); len(feed.Data) != 0 {
		t.Fatal("disabled cycling returned events")
	}
}
