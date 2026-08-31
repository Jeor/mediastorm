package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"novastream/config"
	"novastream/models"
	"novastream/services/homedesigner"
	"novastream/services/user_settings"
	"novastream/services/users"
)

// TestHomeDesignerPageRendersForAdminAndAccount protects the dedicated editor
// shell from regressing into an admin-only page or losing its scoped base path.
func TestHomeDesignerPageRendersForAdminAndAccount(t *testing.T) {
	handler, _ := newHomeDesignerHandler(t)

	for _, test := range []struct {
		name    string
		path    string
		session models.Session
		want    string
	}{
		{name: "admin", path: "/admin/settings/home-designer", session: models.Session{IsMaster: true}, want: `data-base-path="/admin"`},
		{name: "account", path: "/account/settings/home-designer", session: models.Session{AccountID: "account-a"}, want: `data-base-path="/account"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := homeDesignerRequest(http.MethodGet, test.path, test.session, nil)

			handler.HomeDesignerPage(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
			}
			for _, marker := range []string{"Home Designer", test.want, `data-home-designer-status`, "/assets/home-designer/dev/app.js", "/assets/home-designer/dev/home_designer.css", `class="form-select" data-home-designer-scope`, `data-home-designer-theme></div>`} {
				if !strings.Contains(recorder.Body.String(), marker) {
					t.Fatalf("page is missing %q", marker)
				}
			}
		})
	}
}

// TestHomeDesignerAssetsServeOnlyEmbeddedAllowlistEntries protects the editor
// asset route from becoming a filesystem path reader.
func TestHomeDesignerAssetsServeOnlyEmbeddedAllowlistEntries(t *testing.T) {
	for _, test := range []struct {
		name            string
		asset           string
		want            int
		wantContentType string
	}{
		{name: "module", asset: "app.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "document store module", asset: "store.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "catalog library module", asset: "library.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "outline module", asset: "outline.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "api module", asset: "api.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "theme module", asset: "theme.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "preview module", asset: "preview.js", want: http.StatusOK, wantContentType: "text/javascript; charset=utf-8"},
		{name: "stylesheet", asset: "home_designer.css", want: http.StatusOK, wantContentType: "text/css; charset=utf-8"},
		{name: "traversal", asset: "../app.js", want: http.StatusNotFound},
		{name: "unknown", asset: "other.js", want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/admin/assets/home-designer/"+test.asset, nil), map[string]string{"asset": test.asset})

			HomeDesignerAsset(recorder, request)

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
			if test.wantContentType != "" && recorder.Header().Get("Content-Type") != test.wantContentType {
				t.Fatalf("Content-Type = %q, want %q", recorder.Header().Get("Content-Type"), test.wantContentType)
			}
			if test.want == http.StatusOK {
				if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("X-Content-Type-Options = %q, want nosniff", recorder.Header().Get("X-Content-Type-Options"))
				}
				if recorder.Header().Get("Cache-Control") == "" {
					t.Fatal("asset response did not set a cache policy")
				}
			}
		})
	}
}

// TestHomeDesignerPageExplainsHowAnAccountWithoutProfilesCanProceed protects
// account owners from a blank editor that has no authorized scope to load.
func TestHomeDesignerPageExplainsHowAnAccountWithoutProfilesCanProceed(t *testing.T) {
	handler, _ := newHomeDesignerHandler(t)
	recorder := httptest.NewRecorder()
	handler.HomeDesignerPage(recorder, homeDesignerRequest(http.MethodGet, "/account/settings/home-designer", models.Session{AccountID: "account-empty"}, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Create a profile first") {
		t.Fatalf("page did not explain the empty profile state: %s", recorder.Body.String())
	}
}

// TestHomeDesignerNavigationKeepsSettingsOpenAndOnlyActivatesDesigner
// protects the dedicated page from being mistaken for the legacy Home form.
func TestHomeDesignerNavigationKeepsSettingsOpenAndOnlyActivatesDesigner(t *testing.T) {
	handler, _ := newHomeDesignerHandler(t)
	for _, test := range []struct {
		name    string
		path    string
		session models.Session
		base    string
	}{
		{name: "admin", path: "/admin/settings/home-designer", session: models.Session{IsMaster: true}, base: "/admin"},
		{name: "account", path: "/account/settings/home-designer", session: models.Session{AccountID: "account-a"}, base: "/account"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.HomeDesignerPage(recorder, homeDesignerRequest(http.MethodGet, test.path, test.session, nil))
			body := recorder.Body.String()
			activeDesigner := `href="` + test.base + `/settings/home-designer" class="sidebar-nav-link active"`
			if !strings.Contains(body, activeDesigner) {
				t.Fatalf("Home Designer link was not uniquely active: missing %q", activeDesigner)
			}
			legacyHome := `href="` + test.base + `/settings?category=interface" class="sidebar-nav-link active"`
			if strings.Contains(body, legacyHome) {
				t.Fatalf("legacy Home link was active on the Home Designer page: %q", legacyHome)
			}
			if !strings.Contains(body, `<details class="sidebar-group current" open>`) {
				t.Fatal("Settings group was not open for Home Designer")
			}
		})
	}
}

// TestGetHomeDesignerEnforcesScopeOwnership protects global settings and
// profile documents from an account crossing its authorized boundary.
func TestGetHomeDesignerEnforcesScopeOwnership(t *testing.T) {
	handler, owned := newHomeDesignerHandler(t)

	for _, test := range []struct {
		name    string
		session models.Session
		query   string
		want    int
	}{
		{name: "account cannot load global", session: models.Session{AccountID: "account-a"}, query: "scope=global", want: http.StatusForbidden},
		{name: "account loads owned profile", session: models.Session{AccountID: "account-a"}, query: "scope=profile&profileId=" + owned.ID, want: http.StatusOK},
		{name: "account cannot load unowned profile", session: models.Session{AccountID: "account-a"}, query: "scope=profile&profileId=" + profileIDForAccount(t, handler, "account-b"), want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.GetHomeDesigner(recorder, homeDesignerRequest(http.MethodGet, "/account/api/home-designer?"+test.query, test.session, nil))

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
			if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want JSON", contentType)
			}
		})
	}
}

// TestPutHomeDesignerMapsValidationConflictAndSuccessToJSON protects clients
// from HTML error responses and verifies writes remain explicit applies.
func TestPutHomeDesignerMapsValidationConflictAndSuccessToJSON(t *testing.T) {
	handler, _ := newHomeDesignerHandler(t)
	admin := models.Session{IsMaster: true}

	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "validation", body: `{"scope":{"kind":"global"},"expectedRevision":"unused","theme":{"mode":"custom"}}`, want: http.StatusUnprocessableEntity},
		{name: "stale revision", body: `{"scope":{"kind":"global"},"expectedRevision":"stale","theme":{"mode":"custom","value":{"accentColor":"#112233"}}}`, want: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := homeDesignerRequest(http.MethodPut, "/admin/api/home-designer", admin, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			handler.PutHomeDesigner(recorder, request)

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
			var response homeDesignerErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode JSON error response: %v", err)
			}
			if response.Code == "" {
				t.Fatalf("error response did not include a code: %s", recorder.Body.String())
			}
		})
	}

	load := httptest.NewRecorder()
	handler.GetHomeDesigner(load, homeDesignerRequest(http.MethodGet, "/admin/api/home-designer?scope=global", admin, nil))
	if load.Code != http.StatusOK {
		t.Fatalf("load global document: status = %d: %s", load.Code, load.Body.String())
	}
	var document homedesigner.Document
	if err := json.Unmarshal(load.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	payload, err := json.Marshal(homedesigner.ApplyRequest{
		Scope: document.Scope, ExpectedRevision: document.Revision,
		Theme: &homedesigner.SectionMutation[models.AppearanceSettings]{Mode: homedesigner.ModeCustom, Value: &models.AppearanceSettings{AccentColor: "#112233"}},
	})
	if err != nil {
		t.Fatalf("marshal apply request: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := homeDesignerRequest(http.MethodPut, "/admin/api/home-designer", admin, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	handler.PutHomeDesigner(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("successful apply status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

// TestPutHomeDesignerRejectsAmbiguousJSON protects the mutation boundary from
// silently accepting misspelled fields or a second update document.
func TestPutHomeDesignerRejectsAmbiguousJSON(t *testing.T) {
	handler, _ := newHomeDesignerHandler(t)
	for _, body := range []string{
		`{"scope":{"kind":"global"},"unexpected":true}`,
		`{"scope":{"kind":"global"}} {"scope":{"kind":"global"}}`,
	} {
		recorder := httptest.NewRecorder()
		request := homeDesignerRequest(http.MethodPut, "/admin/api/home-designer", models.Session{IsMaster: true}, strings.NewReader(body))
		handler.PutHomeDesigner(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
		}
	}
}

func newHomeDesignerHandler(t *testing.T) (*AdminUIHandler, models.User) {
	t.Helper()
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	manager := config.NewManager(settingsPath)
	if err := manager.Save(config.DefaultSettings()); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	usersService, err := users.NewService(directory)
	if err != nil {
		t.Fatalf("new users service: %v", err)
	}
	owned, err := usersService.CreateForAccount("account-a", "Owned")
	if err != nil {
		t.Fatalf("create owned profile: %v", err)
	}
	if _, err := usersService.CreateForAccount("account-b", "Unowned"); err != nil {
		t.Fatalf("create unowned profile: %v", err)
	}
	profileSettings, err := user_settings.NewService(directory)
	if err != nil {
		t.Fatalf("new user settings service: %v", err)
	}
	return NewAdminUIHandler(settingsPath, "", nil, usersService, profileSettings, manager), owned
}

func homeDesignerRequest(method, target string, session models.Session, body *strings.Reader) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	return request.WithContext(context.WithValue(request.Context(), adminSessionContextKey{}, &session))
}

func profileIDForAccount(t *testing.T, handler *AdminUIHandler, accountID string) string {
	t.Helper()
	profiles := handler.usersService.ListForAccount(accountID)
	if len(profiles) != 1 {
		t.Fatalf("profiles for %q = %#v, want one", accountID, profiles)
	}
	return profiles[0].ID
}
