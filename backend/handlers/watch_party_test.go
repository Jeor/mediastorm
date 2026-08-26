package handlers

import (
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestWatchPartyLandingFormatsShortCode(t *testing.T) {
	handler := &WatchPartyHandler{serverBasePath: "/mediastorm"}
	recorder := httptest.NewRecorder()
	handler.Landing(recorder, httptest.NewRequest("GET", "/mediastorm/watch-party", nil))
	body := recorder.Body.String()
	for _, marker := range []string{
		`id="watch-party-code"`,
		`replace(/[^A-Z2-7]/g,'')`,
		`compact.slice(0,4)+'-'+compact.slice(4)`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("landing page missing code formatter %q", marker)
		}
	}
}

type fakeWatchPartySettings struct {
	settings config.Settings
	err      error
}

func (f fakeWatchPartySettings) Load() (config.Settings, error) {
	return f.settings, f.err
}

func TestWatchPartyExternalBaseURLRequired(t *testing.T) {
	handler := &WatchPartyHandler{settings: fakeWatchPartySettings{}}
	if _, err := handler.externalBaseURL(); err == nil {
		t.Fatal("externalBaseURL() error = nil, want missing-setting error")
	}
}

func TestWatchPartyExternalBaseURLNormalized(t *testing.T) {
	handler := &WatchPartyHandler{settings: fakeWatchPartySettings{settings: config.Settings{
		Server: config.ServerSettings{ExternalBackendURL: "https://watch.example.com/mediastorm/api/"},
	}}}
	got, err := handler.externalBaseURL()
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://watch.example.com/mediastorm"; got != want {
		t.Fatalf("externalBaseURL() = %q, want %q", got, want)
	}
}

func TestWatchPartyExternalBaseURLLoadFailure(t *testing.T) {
	handler := &WatchPartyHandler{settings: fakeWatchPartySettings{err: errors.New("load failed")}}
	if _, err := handler.externalBaseURL(); err == nil {
		t.Fatal("externalBaseURL() error = nil, want load error")
	}
}

func TestExternalRoomViewIncludesOnlyCurrentGuestReadiness(t *testing.T) {
	room := &models.WatchRoom{Members: []models.WatchRoomMember{
		{ProfileID: "host", Name: "Host", Joined: true, Ready: true, IsCreator: true},
		{ProfileID: "guest:mine", Name: "Me", Joined: true, Ready: true, IsGuest: true},
		{ProfileID: "guest:other", Name: "Other", Joined: true, Ready: false, IsGuest: true},
	}}

	view := externalRoomView(room, "mine")
	if !view.CurrentGuestReady {
		t.Fatal("currentGuestReady = false, want true")
	}
	for _, member := range view.Members {
		if member.ProfileID != "" {
			t.Fatalf("external room view exposed member profile ID %q", member.ProfileID)
		}
	}
}

func TestWatchPartyRoomPageIncludesGuestReadinessControls(t *testing.T) {
	body, err := os.ReadFile("watch_party.go")
	if err != nil {
		t.Fatalf("read watch party handler: %v", err)
	}
	rendered := string(body)
	for _, want := range []string{
		`id="readyButton"`,
		`currentGuestReady`,
		`body:JSON.stringify({ready:!currentGuestReady})`,
		`room.status==='playing'&&room.playbackReady`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("watch party room page missing guest readiness hook %q", want)
		}
	}
}
