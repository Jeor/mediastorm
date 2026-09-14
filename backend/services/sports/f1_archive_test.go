package sports

import (
	"context"
	"io"
	"net/http"
	"novastream/models"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type f1ArchiveTransport func(*http.Request) (*http.Response, error)

func (f f1ArchiveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestF1ArchiveIdentityAndCache(t *testing.T) {
	var calls atomic.Int32
	s := &Service{client: &http.Client{Transport: f1ArchiveTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := `{"Lines":{}}`
		if strings.HasSuffix(r.URL.Path, "Index.json") {
			body = `{"Meetings":[{"Sessions":[{"Name":"Qualifying","StartDate":"2026-09-05T16:00:00","GmtOffset":"02:00:00","Path":"2026/2026-09-06_Italian_Grand_Prix/2026-09-05_Qualifying/"}]}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("\xef\xbb\xbf" + body))}, nil
	})}}
	event := models.SportsEvent{ID: "f1:qual", League: "f1", SessionType: "Qual", Status: models.SportsGameFinal, StartTime: time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)}
	result := s.GetF1Archive(context.Background(), event)
	if result.State != "available" || result.Mode != "archive" || result.EventID != event.ID {
		t.Fatal(result)
	}
	count := calls.Load()
	s.GetF1Archive(context.Background(), event)
	if calls.Load() != count {
		t.Fatal("cache missed")
	}
	event.ID = "other"
	event.StartTime = event.StartTime.Add(time.Hour)
	if s.GetF1Archive(context.Background(), event).State != "unavailable" {
		t.Fatal("wrong session archive attached")
	}
	event.Status = models.SportsGameLive
	before := calls.Load()
	if s.GetF1Archive(context.Background(), event).State != "unavailable" || calls.Load() != before {
		t.Fatal("archive used as live feed")
	}
}
