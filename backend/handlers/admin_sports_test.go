package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"novastream/config"
	"novastream/models"
	user_settings "novastream/services/user_settings"
	"novastream/services/users"
)

func sportsAdminRequest(method, path, body string, master bool, account string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), adminSessionContextKey{}, &models.Session{IsMaster: master, AccountID: account}))
}

func TestSportsAdminTemplateRendersBothScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	h := NewAdminUIHandler(path, "", nil, nil, nil, config.NewManager(path))
	for _, master := range []bool{true, false} {
		var out bytes.Buffer
		err := h.sportsTemplate.ExecuteTemplate(&out, "base", AdminPageData{CurrentPath: "/prefix/account/sports", IsAdmin: master, BasePath: "/prefix/account", ServerBasePath: "/prefix", Users: []models.User{{ID: "one", Name: "Alex"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Favorite teams") || !strings.Contains(out.String(), "Alex") {
			t.Fatal("missing profile editor")
		}
		if strings.Contains(out.String(), `id="sports-server"`) != master {
			t.Fatal("server settings must only render for master")
		}
		if dir := os.Getenv("SPORTS_UI_RENDER_DIR"); dir != "" {
			name := "sports-account.html"
			if master {
				name = "sports-admin.html"
			}
			if err := os.WriteFile(filepath.Join(dir, name), out.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSportsAdminSavesProfileFavoritesAndRejectsStaleRevision(t *testing.T) {
	dir := t.TempDir()
	profiles, err := users.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profiles.CreateForAccount("account-a", "Alex")
	if err != nil {
		t.Fatal(err)
	}
	other, err := profiles.CreateForAccount("account-b", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	preferences, err := user_settings.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &AdminUIHandler{usersService: profiles, userSettingsService: preferences}
	saved := models.EmptySportsPreferences()
	saved.PinnedSportIDs = []string{"soccer"}
	saved.CompetitionIDs = []string{"premier-league"}
	saved.TeamIDs = []string{"soccer-eng.1:359"}
	saved.AthleteIDs = []string{"keep-athlete"}
	payload, _ := json.Marshal(models.SportsPreferenceSnapshot{Revision: 0, Preferences: saved})
	put := func() int {
		r := sportsAdminRequest(http.MethodPut, "/account/api/sports/profiles/"+profile.ID, string(payload), false, "account-a")
		r = mux.SetURLVars(r, map[string]string{"userID": profile.ID})
		w := httptest.NewRecorder()
		h.SportsProfileAPI(w, r)
		return w.Code
	}
	if code := put(); code != http.StatusOK {
		t.Fatalf("save returned %d", code)
	}
	if code := put(); code != http.StatusConflict {
		t.Fatalf("stale save returned %d", code)
	}
	reloaded, err := user_settings.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reloaded.GetSportsPreferences(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 1 || len(result.Preferences.TeamIDs) != 1 || result.Preferences.TeamIDs[0] != "soccer-eng.1:359" || len(result.Preferences.AthleteIDs) != 1 {
		t.Fatalf("incorrect persisted preferences: %+v", result)
	}
	untouched, err := reloaded.GetSportsPreferences(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Revision != 0 || len(untouched.Preferences.PinnedSportIDs) != 0 {
		t.Fatal("another profile was changed")
	}
}

func TestSportsAdminRejectsServerChangesFromRegularAccounts(t *testing.T) {
	h := &AdminUIHandler{}
	w := httptest.NewRecorder()
	h.SportsSettingsAPI(w, sportsAdminRequest(http.MethodPut, "/account/api/sports/settings", `{}`, false, "account-a"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", w.Code)
	}
}

func TestSportsAdminRejectsOtherAccountsProfiles(t *testing.T) {
	svc, err := users.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := svc.CreateForAccount("account-b", "Other")
	if err != nil {
		t.Fatal(err)
	}
	h := &AdminUIHandler{usersService: svc}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		r := sportsAdminRequest(method, "/account/api/sports/profiles/other", `{}`, false, "account-a")
		r = mux.SetURLVars(r, map[string]string{"userID": profile.ID})
		w := httptest.NewRecorder()
		h.SportsProfileAPI(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s got %d, want 404", method, w.Code)
		}
	}
}
