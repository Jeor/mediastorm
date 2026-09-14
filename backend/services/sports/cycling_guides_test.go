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

const giroTestGuideURL = "https://www.giroditalia.it/en/tappe/stage-21-of-the-giro-ditalia-2026-roma-roma/"

func TestGiroGuideAssociationAndAssets(t *testing.T) {
	now := time.Now()
	doc := giroFixture(t, "giro-stage21-guide")
	guide, err := normalizeGiroGuide(doc, giroTestGuideURL, 2026, 21, now)
	if err != nil || guide.State != "available" || !giroGuideImage(guide.MapURL) || !giroGuideImage(guide.ProfileURL) {
		t.Fatal("missing verified guide", guide, err)
	}
	for _, input := range []struct {
		url         string
		year, stage int
	}{{giroTestGuideURL, 2027, 21}, {giroTestGuideURL, 2026, 20}, {strings.Replace(giroTestGuideURL, "roma-roma", "other", 1), 2026, 21}} {
		if _, err := normalizeGiroGuide(doc, input.url, input.year, input.stage, now); err == nil {
			t.Fatal("accepted wrong guide association", input)
		}
	}
	for _, value := range []string{"http://static2.giroditalia.it/wp-content/uploads/x.jpg", "https://static2.giroditalia.it.evil.test/wp-content/uploads/x.jpg", "https://static2.giroditalia.it/wp-content/uploads/../x.jpg", "https://static2.giroditalia.it/wp-content/uploads/x.svg", "https://user@static2.giroditalia.it/wp-content/uploads/x.jpg"} {
		if giroGuideImage(value) {
			t.Fatal("accepted unsafe asset", value)
		}
	}
	race, err := normalizeGiroSchedule(giroFixture(t, "giro-route"), 2026, now)
	if err != nil || race.Stages[20].RouteGuide.SourceURL != giroTestGuideURL {
		t.Fatal("schedule lost exact guide URL", err)
	}
}
func TestCyclingGuideCacheAndFailureIsolation(t *testing.T) {
	now := time.Now()
	s := NewService(t.TempDir())
	body, _ := os.ReadFile("testdata/giro-stage21-guide.html")
	calls := 0
	failure := false
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.Contains(r.URL.Path, "/tappe/") {
			t.Fatal("unexpected guide request", r.URL)
		}
		if failure {
			return nil, fmt.Errorf("offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	stage := CyclingStage{ID: "rcs:giro:2026:21", Results: cyclingResults{State: "available", Data: []cyclingResult{{Name: "Retained rider"}}}, RouteGuide: &CyclingRouteGuide{SourceURL: giroTestGuideURL}}
	s.enrichCyclingGuide(context.Background(), &stage, 2026, 21, now)
	if stage.RouteGuide.State != "available" {
		t.Fatal(stage.RouteGuide)
	}
	s.enrichCyclingGuide(context.Background(), &stage, 2026, 21, now.Add(time.Minute))
	if calls != 1 {
		t.Fatal("guide not cached")
	}
	failure = true
	s.enrichCyclingGuide(context.Background(), &stage, 2026, 21, now.Add(31*time.Minute))
	if stage.RouteGuide.State != "stale" || stage.RouteGuide.MapURL == "" || stage.Results.State != "available" || len(stage.Results.Data) != 1 || !stage.RouteGuide.Source.ObservedAt.Equal(now) {
		t.Fatal("failed guide erased good data", stage)
	}
	s.enrichCyclingGuide(context.Background(), &stage, 2026, 21, now.Add(31*time.Minute+time.Second))
	if calls != 2 {
		t.Fatal("failure backoff ignored")
	}
}
