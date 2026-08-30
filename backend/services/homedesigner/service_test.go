package homedesigner

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
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
		"owned":   {ID: "owned", AccountID: "account-a"},
		"unowned": {ID: "unowned", AccountID: "account-b"},
	}}
	return New(manager, profiles, directory), manager, profiles
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
	if len(document.Rows.Effective.Shelves) <= len(document.Rows.Override.Shelves) {
		t.Fatalf("effective rows = %#v, want defaults merged with raw override", document.Rows.Effective)
	}
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
