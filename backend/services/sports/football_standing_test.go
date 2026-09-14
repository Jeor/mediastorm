package sports

import (
	"context"
	"io"
	"net/http"
	"novastream/models"
	"strings"
	"testing"
	"time"
)

type footballStandingTransportLocal func(*http.Request) (*http.Response, error)

func (f footballStandingTransportLocal) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
func TestFootballStandingNonblockingCacheLocal(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	s := Service{client: &http.Client{Transport: footballStandingTransportLocal(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, context.DeadlineExceeded
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"team":{"id":"21","standingSummary":"1st in NFC East"}}`))}, nil
	})}}
	team := models.SportsTeam{ID: "21"}
	done := make(chan struct{})
	go func() { s.applyFootballStanding(&team); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("optional lookup blocked detail")
	}
	<-started
	s.applyFootballStanding(&team)
	select {
	case <-started:
		t.Fatal("duplicate lookup")
	default:
	}
}
