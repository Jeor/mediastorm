package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"novastream/config"
	"novastream/models"
	"novastream/services/sessions"
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

func TestWatchPartyGuestSessionReusesRoomScopedIdentity(t *testing.T) {
	sessionService, err := sessions.NewService("", time.Hour)
	if err != nil {
		t.Fatalf("create session service: %v", err)
	}
	session, err := sessionService.CreateScopedWithResource(
		"account-1",
		false,
		"browser",
		"127.0.0.1",
		time.Hour,
		models.SessionScopeWatchParty,
		"room-1",
	)
	if err != nil {
		t.Fatalf("create guest session: %v", err)
	}

	handler := &WatchPartyHandler{sessions: sessionService}
	request := httptest.NewRequest(http.MethodPost, "/watch-party/token", nil)
	request.AddCookie(&http.Cookie{Name: watchPartyCookieName, Value: session.Token})

	gotSession, gotGuestID, ok := handler.guestSession(request, "room-1")
	if !ok {
		t.Fatal("guestSession() ok = false, want true")
	}
	if gotSession.Token != session.Token {
		t.Fatalf("guestSession() token = %q, want existing token", gotSession.Token)
	}
	if want := watchPartyGuestID(session.Token); gotGuestID != want {
		t.Fatalf("guestSession() guest ID = %q, want %q", gotGuestID, want)
	}
	if _, _, ok := handler.guestSession(request, "room-2"); ok {
		t.Fatal("guestSession() accepted a session scoped to another room")
	}
}

func TestJoinInvitationChecksExistingGuestBeforeCreatingSession(t *testing.T) {
	body, err := os.ReadFile("watch_party.go")
	if err != nil {
		t.Fatalf("read watch party handler: %v", err)
	}
	source := string(body)
	reuse := strings.Index(source, `if session, guestID, ok := h.guestSession(r, invite.RoomID); ok {`)
	create := strings.Index(source, `h.sessions.CreateScopedWithResource(invite.AccountID`)
	if reuse < 0 || create < 0 || reuse > create {
		t.Fatal("joinInvitation must reuse a valid room guest session before creating a new identity")
	}
}
