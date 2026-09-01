package homedesigner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"novastream/config"
	"novastream/models"
	"novastream/services/user_settings"
)

type serviceTestDirectory struct {
	users map[string]models.User
}

func (d serviceTestDirectory) Get(id string) (models.User, bool) {
	user, ok := d.users[id]
	return user, ok
}

func (d serviceTestDirectory) ListAll() []models.User {
	users := make([]models.User, 0, len(d.users))
	for _, user := range d.users {
		users = append(users, user)
	}
	return users
}

func (d serviceTestDirectory) ListForAccount(accountID string) []models.User {
	users := make([]models.User, 0)
	for _, user := range d.users {
		if user.AccountID == accountID {
			users = append(users, user)
		}
	}
	return users
}

func (d serviceTestDirectory) BelongsToAccount(profileID, accountID string) bool {
	user, ok := d.users[profileID]
	return ok && user.AccountID == accountID
}

func newServiceLoadTest(t *testing.T) (*Service, *config.Manager, *user_settings.Service) {
	t.Helper()
	manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := manager.Save(config.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	profiles, err := user_settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := serviceTestDirectory{users: map[string]models.User{
		"owned":       {ID: "owned", AccountID: "account-a", Name: "Owner", IconURL: "/private/owner.png", TraktAccountID: "linked-foreign"},
		"owned-other": {ID: "owned-other", AccountID: "account-a", Name: "Other owner"},
		"unowned":     {ID: "unowned", AccountID: "account-b", Name: "Foreign", IconURL: "/private/foreign.png", TraktAccountID: "foreign"},
	}}
	return New(manager, profiles, directory), manager, profiles
}

type serviceTestCatalogProvider struct {
	capabilities CatalogCapabilities
}

func (p serviceTestCatalogProvider) CatalogCapabilities(context.Context, Actor, Scope) CatalogCapabilities {
	return p.capabilities
}

func TestServiceLoadAllowsAdministratorGlobalAccess(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)

	document, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Scope.Kind != "global" {
		t.Fatalf("scope = %#v, want global", document.Scope)
	}
	if document.Rows.Inherited || document.Theme.Inherited {
		t.Fatalf("global document inheritance = rows:%v theme:%v, want both false", document.Rows.Inherited, document.Theme.Inherited)
	}
	if document.Rows.Override == nil || document.Theme.Override == nil {
		t.Fatal("global document must expose its persisted rows and theme")
	}
}

func TestServiceLoadReturnsSafeScopeEnvelopeForActor(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)

	document, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	if !document.Permissions.CanEdit || document.Permissions.CanEditGlobal || !document.Permissions.CanEditProfiles {
		t.Fatalf("permissions = %#v, want account profile-only editor permissions", document.Permissions)
	}
	if len(document.PreviewProfiles) != 2 || !hasPreviewProfile(document.PreviewProfiles, "owned", "Owner") || !hasPreviewProfile(document.PreviewProfiles, "owned-other", "Other owner") || hasPreviewProfile(document.PreviewProfiles, "unowned", "Foreign") {
		t.Fatalf("preview profiles = %#v, want account-a profiles only", document.PreviewProfiles)
	}
	if len(document.ThemePresets) == 0 || document.ThemePresets[0].ID != "default-dark" || document.ThemePresets[0].Appearance.AccentColor != "#3f66ff" || document.ThemePresets[0].Appearance.FontScale == nil || *document.ThemePresets[0].Appearance.FontScale != 1 {
		t.Fatalf("theme presets = %#v, want normalized Default Dark", document.ThemePresets)
	}
	encoded, err := json.Marshal(document.PreviewProfiles)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "iconUrl") || strings.Contains(string(encoded), "traktAccountId") || strings.Contains(string(encoded), "/private/") {
		t.Fatalf("preview profiles leaked profile data: %s", encoded)
	}
}

func TestServiceLoadAdministratorMayPreviewAllProfiles(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)

	document, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if !document.Permissions.CanEditGlobal || !document.Permissions.CanEditProfiles || len(document.PreviewProfiles) != 3 || !hasPreviewProfile(document.PreviewProfiles, "unowned", "Foreign") {
		t.Fatalf("administrator envelope = %#v, want all safe profile choices", document)
	}
}

func TestServiceUsesExplicitCatalogCapabilitiesForLoadAndApply(t *testing.T) {
	service, manager, _ := newServiceLoadTest(t)
	provider := serviceTestCatalogProvider{capabilities: CatalogCapabilities{
		BasePath:           "/mediastorm",
		AuthorizedAccounts: []CatalogAccountAuthorization{{Provider: "trakt", AccountID: "trakt-owned"}},
		Libraries:          []CatalogLibrary{{ID: "library-owned", Name: "Owned library"}},
	}}
	service = NewWithCatalogCapabilities(manager, service.profiles, service.directory, provider)
	if err := manager.Mutate(func(settings *config.Settings) error {
		settings.Trakt.Accounts = []config.TraktAccount{{ID: "trakt-owned", Name: "Owned"}, {ID: "foreign", Name: "Foreign"}}
		settings.HomeShelves.Shelves = []config.ShelfConfig{{ID: "trakt-row", Name: "Owned list", Enabled: true, Order: 0, Type: "trakt", TraktAccountID: "trakt-owned", TraktListType: "watchlist"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	before, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	byType := catalogByType(before.Catalog)
	if !hasOption(byType["trakt"].Fields, "traktAccountId", "trakt-owned") || hasOption(byType["trakt"].Fields, "traktAccountId", "foreign") || !hasOption(byType["library"].Fields, "libraryId", "library-owned") {
		t.Fatalf("catalog = %#v, want only explicitly authorized capabilities", byType)
	}
	rows := before.Rows.Effective
	rows.Shelves[0].Name = "Renamed list"
	if _, err := service.Apply(context.Background(), Actor{AccountID: "account-a"}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &rows},
	}); err != nil {
		t.Fatalf("Apply rejected an authorized existing integration row: %v", err)
	}
}

func TestServiceLoadRejectsNonAdministratorGlobalAccess(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)

	_, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "global"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Load error = %v, want ErrForbidden", err)
	}
}

func TestServiceLoadAllowsAdministratorAndOwnerProfileAccess(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)

	for _, actor := range []Actor{{IsAdmin: true}, {AccountID: "account-a"}} {
		document, err := service.Load(context.Background(), actor, Scope{Kind: "profile", ProfileID: "owned"})
		if err != nil {
			t.Fatalf("Load(%#v) error = %v", actor, err)
		}
		if document.Scope.ProfileID != "owned" {
			t.Fatalf("profile = %q, want owned", document.Scope.ProfileID)
		}
	}
}

func TestServiceLoadHidesUnownedProfiles(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)

	_, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "unowned"})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Load error = %v, want ErrProfileNotFound", err)
	}
}

func TestServiceLoadUsesRawProfileSectionsForIndependentInheritance(t *testing.T) {
	service, _, profiles := newServiceLoadTest(t)
	if err := profiles.Update("owned", models.UserSettings{
		Playback:    models.PlaybackSettings{PreferredPlayer: "vlc"},
		HomeShelves: models.HomeShelvesSettings{Shelves: []models.ShelfConfig{{ID: "watchlist", Name: "My list", Enabled: true, Order: 0}}},
	}); err != nil {
		t.Fatal(err)
	}

	document, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Rows.Inherited || document.Rows.Override == nil || document.Rows.Override.Shelves[0].Name != "My list" {
		t.Fatalf("rows = %#v, want explicit raw override", document.Rows)
	}
	if !document.Theme.Inherited || document.Theme.Override != nil {
		t.Fatalf("theme = %#v, want inherited with no override", document.Theme)
	}
	if !reflect.DeepEqual(document.Rows.Effective.Shelves, document.Rows.Override.Shelves) {
		t.Fatalf("effective rows = %#v, want exact stored snapshot %#v", document.Rows.Effective.Shelves, document.Rows.Override.Shelves)
	}
}

func TestServiceProfileRowsRemainACompleteSnapshotAcrossReloadAndGlobalChanges(t *testing.T) {
	service, manager, profiles := newServiceLoadTest(t)
	before, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	rows := before.Rows.Effective
	removedID := rows.Shelves[len(rows.Shelves)-1].ID
	rows.Shelves = rows.Shelves[:len(rows.Shelves)-1]
	for i := range rows.Shelves {
		rows.Shelves[i].Order = i
	}
	after, err := service.Apply(context.Background(), Actor{AccountID: "account-a"}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &rows},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Rows.Effective.Shelves) != len(rows.Shelves) || shelfIDPresent(after.Rows.Effective.Shelves, removedID) {
		t.Fatalf("apply reintroduced removed shelf %q: %#v", removedID, after.Rows.Effective.Shelves)
	}

	if err := manager.Mutate(func(settings *config.Settings) error {
		settings.HomeShelves.Shelves = append(settings.HomeShelves.Shelves, config.ShelfConfig{ID: "later-global", Name: "Later global", Enabled: true, Order: len(settings.HomeShelves.Shelves)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, before.Scope)
	if err != nil {
		t.Fatal(err)
	}
	native, err := profiles.GetWithDefaults("owned", models.UserSettings{HomeShelves: configHomeShelvesToModel(mustLoadSettings(t, manager).HomeShelves)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Rows.Effective.Shelves, rows.Shelves) || !reflect.DeepEqual(native.HomeShelves.Shelves, rows.Shelves) || shelfIDPresent(reloaded.Rows.Effective.Shelves, "later-global") {
		t.Fatalf("snapshot leaked global rows: editor=%#v native=%#v want=%#v", reloaded.Rows.Effective.Shelves, native.HomeShelves.Shelves, rows.Shelves)
	}
}

func TestServicePersistsAnExplicitEmptyRowsSnapshot(t *testing.T) {
	service, _, profiles := newServiceLoadTest(t)
	before, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	empty := before.Rows.Effective
	empty.Shelves = []models.ShelfConfig{}
	after, err := service.Apply(context.Background(), Actor{AccountID: "account-a"}, ApplyRequest{Scope: before.Scope, ExpectedRevision: before.Revision, Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &empty}})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := profiles.Get("owned")
	if err != nil {
		t.Fatal(err)
	}
	if after.Rows.Inherited || len(after.Rows.Effective.Shelves) != 0 || stored == nil || stored.HomeShelves.ShelvesOverride == nil || !*stored.HomeShelves.ShelvesOverride {
		t.Fatalf("empty snapshot = after %#v stored %#v", after.Rows, stored)
	}
}

func shelfIDPresent(shelves []models.ShelfConfig, id string) bool {
	for _, shelf := range shelves {
		if shelf.ID == id {
			return true
		}
	}
	return false
}

func mustLoadSettings(t *testing.T, manager *config.Manager) config.Settings {
	t.Helper()
	settings, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestServiceApplyGlobalRowsOnlyPreservesTheme(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)
	before, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	rows := before.Rows.Effective
	rows.Shelves[0].Name = "Pinned first"

	after, err := service.Apply(context.Background(), Actor{IsAdmin: true}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &rows},
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.Rows.Effective.Shelves[0].Name != "Pinned first" {
		t.Fatalf("first row = %q, want saved row", after.Rows.Effective.Shelves[0].Name)
	}
	if !reflect.DeepEqual(after.Theme.Effective, before.Theme.Effective) {
		t.Fatalf("theme changed from %#v to %#v", before.Theme.Effective, after.Theme.Effective)
	}
	if len(after.PreviewProfiles) != 3 || len(after.ThemePresets) == 0 || !after.Permissions.CanEditGlobal {
		t.Fatalf("Apply reload omitted editor envelope: %#v", after)
	}
}

func TestServiceApplyGlobalRowsPreservesUnrelatedHomeSettings(t *testing.T) {
	service, manager, _ := newServiceLoadTest(t)
	if err := manager.Mutate(func(settings *config.Settings) error {
		settings.HomeShelves.PopularOnServerWindowDays = 123
		settings.HomeShelves.RecentlyWatchedCapPerProfile = 9
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	rows := before.Rows.Effective
	rows.Shelves[0].Name = "Pinned first"
	if _, err := service.Apply(context.Background(), Actor{IsAdmin: true}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &rows},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.HomeShelves.PopularOnServerWindowDays != 123 || stored.HomeShelves.RecentlyWatchedCapPerProfile != 9 {
		t.Fatalf("unrelated home settings = %#v, want window 123 and cap 9", stored.HomeShelves)
	}
}

func TestServiceApplyProfileRowsPreservesPlaybackAndInheritedTheme(t *testing.T) {
	service, _, profiles := newServiceLoadTest(t)
	if err := profiles.Update("owned", models.UserSettings{Playback: models.PlaybackSettings{PreferredPlayer: "vlc"}}); err != nil {
		t.Fatal(err)
	}
	before, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	rows := before.Rows.Effective
	rows.Shelves[0].Name = "Profile first"

	after, err := service.Apply(context.Background(), Actor{AccountID: "account-a"}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &rows},
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.Rows.Inherited || after.Theme.Inherited == false {
		t.Fatalf("inheritance = rows:%v theme:%v, want rows custom and theme inherited", after.Rows.Inherited, after.Theme.Inherited)
	}
	stored, err := profiles.Get("owned")
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Playback.PreferredPlayer != "vlc" || !appearanceInherited(stored.Display.Appearance) {
		t.Fatalf("stored settings = %#v, want playback and inherited theme preserved", stored)
	}
}

func TestServiceApplyProfileThemeKeepsRowsInherited(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)
	before, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	theme := before.Theme.Effective
	theme.AccentColor = "#112233"

	after, err := service.Apply(context.Background(), Actor{AccountID: "account-a"}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Theme: &SectionMutation[models.AppearanceSettings]{Mode: ModeCustom, Value: &theme},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !after.Rows.Inherited || after.Theme.Inherited || after.Theme.Override == nil || after.Theme.Override.AccentColor != "#112233" {
		t.Fatalf("document = %#v, want inherited rows and custom theme", after)
	}
}

func TestServiceApplyProfileResetKeepsOtherSection(t *testing.T) {
	service, _, profiles := newServiceLoadTest(t)
	rows := models.DefaultUserSettings().HomeShelves
	theme := models.DefaultUserSettings().Display.Appearance
	theme.AccentColor = "#112233"
	if err := profiles.Update("owned", models.UserSettings{HomeShelves: rows, Display: models.DisplaySettings{Appearance: theme}}); err != nil {
		t.Fatal(err)
	}
	before, err := service.Load(context.Background(), Actor{AccountID: "account-a"}, Scope{Kind: "profile", ProfileID: "owned"})
	if err != nil {
		t.Fatal(err)
	}

	after, err := service.Apply(context.Background(), Actor{AccountID: "account-a"}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeInherit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !after.Rows.Inherited || after.Theme.Inherited || after.Theme.Override == nil || after.Theme.Override.AccentColor != "#112233" {
		t.Fatalf("document = %#v, want rows reset and theme retained", after)
	}
}

func TestServiceApplyRejectsStaleRevisionWithoutWriting(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)
	before, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	theme := before.Theme.Effective
	theme.AccentColor = "#112233"

	_, err = service.Apply(context.Background(), Actor{IsAdmin: true}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: "stale",
		Theme: &SectionMutation[models.AppearanceSettings]{Mode: ModeCustom, Value: &theme},
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Apply error = %v, want ErrRevisionConflict", err)
	}
	after, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || !reflect.DeepEqual(after.Theme.Effective, before.Theme.Effective) {
		t.Fatalf("stale apply wrote document %#v", after)
	}
}

func TestServiceApplyRejectsInvalidRequestWithoutWriting(t *testing.T) {
	service, _, _ := newServiceLoadTest(t)
	before, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	invalidTheme := before.Theme.Effective
	invalidTheme.AccentColor = "not-a-color"

	_, err = service.Apply(context.Background(), Actor{IsAdmin: true}, ApplyRequest{
		Scope: before.Scope, ExpectedRevision: before.Revision,
		Theme: &SectionMutation[models.AppearanceSettings]{Mode: ModeCustom, Value: &invalidTheme},
	})
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Apply error = %v, want ValidationError", err)
	}
	after, err := service.Load(context.Background(), Actor{IsAdmin: true}, Scope{Kind: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("invalid apply wrote revision %q, want %q", after.Revision, before.Revision)
	}
}

func hasPreviewProfile(profiles []PreviewProfile, id, displayName string) bool {
	for _, profile := range profiles {
		if profile.ID == id && profile.DisplayName == displayName {
			return true
		}
	}
	return false
}
