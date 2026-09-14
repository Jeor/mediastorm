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

func TestMotoGPEnrichmentIdentityAndAssets(t *testing.T) {
	var events []struct {
		ID          string `json:"id"`
		BroadcastID string `json:"toad_api_uuid"`
	}
	var broadcasts []motoGPBroadcast
	motoFixture(t, "circuit-events", &events)
	motoFixture(t, "circuit-broadcast", &broadcasts)
	c := joinedMotoGPCircuit("23b7a561-17e4-4aa6-9be3-2529a5b69938", events, broadcasts)
	if c == nil || c.MapURL == "" || c.LengthMeters == nil {
		t.Fatal("verified Aragon guide missing")
	}
	events[0].BroadcastID = ""
	if joinedMotoGPCircuit(events[0].ID, events, broadcasts) != nil {
		t.Fatal("guessed identity")
	}
	for _, u := range []string{"http://photos.motogp.com/events-admin/x.png", "https://evil.test/events-admin/x.png", "https://photos.motogp.com.evil.test/events-admin/x.png", "https://photos.motogp.com/events-admin/x.svg", "https://photos.motogp.com/events-admin/x.png?redirect=evil"} {
		if officialMotoGPMap(u) != "" {
			t.Fatal("unsafe asset", u)
		}
	}
}
func TestMotoGPEnrichmentCacheAndFailure(t *testing.T) {
	s := NewService(t.TempDir())
	failed := false
	calls := 0
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if failed {
			return nil, fmt.Errorf("offline")
		}
		name := "standings"
		switch {
		case strings.HasSuffix(r.URL.Path, "/seasons"):
			name = "seasons"
		case strings.HasSuffix(r.URL.Path, "/categories"):
			name = "categories"
		}
		data, e := os.ReadFile("testdata/motogp-" + name + ".json")
		if e != nil {
			t.Fatal(e)
		}
		if name == "seasons" {
			data = []byte(fmt.Sprintf(`[{"id":"e88b4e43-2209-47aa-8e83-0e0b1cedde6e","year":%d}]`, time.Now().UTC().Year()))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	first := s.GetMotoGPEnrichment(context.Background(), "")
	if first.State != "available" || len(first.Standings) != 29 || first.Standings[0].Position == nil || *first.Standings[0].Position != 1 || first.Standings[0].Points == nil {
		t.Fatalf("standings missing: %+v", first)
	}
	s.GetMotoGPEnrichment(context.Background(), "")
	if calls != 3 {
		t.Fatal("cache missed")
	}
	for k, v := range s.motoGPExtra.entries {
		v.expires = time.Time{}
		s.motoGPExtra.entries[k] = v
	}
	failed = true
	stale := s.GetMotoGPEnrichment(context.Background(), "")
	if stale.State != "stale" || len(stale.Standings) != 29 || !stale.UpdatedAt.Equal(*first.UpdatedAt) {
		t.Fatal("last good lost")
	}
	callsBefore := calls
	s.GetMotoGPEnrichment(context.Background(), "")
	if calls != callsBefore {
		t.Fatal("failure retry storm")
	}
	invalid := s.GetMotoGPEnrichment(context.Background(), "../../bad")
	if invalid.State != "unavailable" || calls != callsBefore {
		t.Fatal("invalid identity fetched")
	}
}
