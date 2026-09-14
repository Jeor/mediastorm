package sports

import (
	"context"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func giroFixture(t *testing.T, name string) *html.Node {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".html")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func TestGiroCapturedCalendarAndClassifications(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	race, err := normalizeGiroSchedule(giroFixture(t, "giro-route"), 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(race.Stages) != 21 || race.StartDate != "2026-05-08" || race.EndDate != "2026-05-31" || race.Stages[0].ID != "rcs:giro:2026:1" || race.Stages[20].Distance != "131 km" || race.Stages[20].Status != "unknown" {
		t.Fatalf("bad Giro schedule: %+v", race)
	}
	stage, err := normalizeGiroClassification(giroFixture(t, "giro-stage21"), "js-tab-classifica-ORARR", race.Stages[20].ID, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	gc, err := normalizeGiroClassification(giroFixture(t, "giro-rankings"), "js-tab-classifica-CLGEN", race.Stages[20].ID, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if stage.State != "available" || gc.State != "available" || len(stage.Data) < 100 || len(gc.Data) < 100 || stage.Data[0].Name != "Jonathan MILAN" || stage.Data[0].Time != "3:05:50" || gc.Data[0].Name != "Jonas VINGEGAARD" || gc.Data[0].Time != "83:22:51" {
		t.Fatal("Giro result categories conflated", stage.State, gc.State, len(stage.Data), len(gc.Data))
	}
	if _, err = normalizeGiroSchedule(giroFixture(t, "giro-route"), 2027, now); err == nil {
		t.Fatal("accepted wrong edition")
	}
	doc := giroFixture(t, "giro-stage21")
	panel := giroClass(doc, "js-tab-classifica-ORARR")[0]
	for _, node := range giroClass(panel, "distacco") {
		for i, a := range node.Attr {
			if a.Key == "class" {
				node.Attr[i].Val = "unknown-column"
			}
		}
	}
	if _, err = normalizeGiroClassification(doc, "js-tab-classifica-ORARR", race.Stages[20].ID, 2026, now); err == nil {
		t.Fatal("accepted changed columns")
	}
}
func TestGiroCurrentGCIsNotAttachedToEarlierStage(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	s := NewService(t.TempDir())
	stageBody, _ := os.ReadFile("testdata/giro-stage21.html")
	gcBody, _ := os.ReadFile("testdata/giro-rankings.html")
	s.client = &http.Client{Transport: detailTransport(func(r *http.Request) (*http.Response, error) {
		body := gcBody
		if strings.Contains(r.URL.Path, "di-tappa") {
			body = stageBody
			if strings.Contains(r.URL.Path, "/20/") {
				body = []byte(strings.Replace(string(stageBody), `js-n-stage">21`, `js-n-stage">20`, 1))
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	current := CyclingStage{ID: "rcs:giro:2026:21"}
	if err := s.enrichGiroStage(context.Background(), &current, 2026, 21, now); err != nil {
		t.Fatal(err)
	}
	if current.GeneralClassification.State != "available" {
		t.Fatal("current GC unavailable")
	}
	stage := CyclingStage{ID: "rcs:giro:2026:20"}
	if err := s.enrichGiroStage(context.Background(), &stage, 2026, 20, now); err != nil {
		t.Fatal(err)
	}
	if stage.Results.State != "available" || stage.GeneralClassification.State != "unavailable" || len(stage.GeneralClassification.Data) != 0 {
		t.Fatal("latest GC mislabeled as earlier stage")
	}
}
