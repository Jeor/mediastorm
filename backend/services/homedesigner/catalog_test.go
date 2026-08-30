package homedesigner

import (
	"strings"
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestHomeDesignerCatalog_ContainsUniqueBuiltinsAndRepeatableTemplates(t *testing.T) {
	cfg := config.DefaultSettings()
	cfg.Metadata.TMDBAPIKey = "tmdb-key"
	cfg.Trakt.Accounts = []config.TraktAccount{{ID: "trakt-1", Name: "Shared Trakt"}}
	cfg.Plex.Accounts = []config.PlexAccount{{ID: "plex-1", Name: "Home Plex"}}
	entries := BuildCatalog(cfg, []models.User{{ID: "profile-1", Name: "Avery", TraktAccountID: "trakt-1"}})

	seenBuiltinIDs := map[string]bool{}
	byType := map[string]CatalogEntry{}
	for _, entry := range entries {
		if entry.Type == "builtin" {
			if seenBuiltinIDs[entry.Default.ID] {
				t.Fatalf("built-in %q was listed more than once", entry.Default.ID)
			}
			seenBuiltinIDs[entry.Default.ID] = true
		}
		if _, exists := byType[entry.Type]; !exists {
			byType[entry.Type] = entry
		}
	}
	if len(seenBuiltinIDs) == 0 {
		t.Fatal("catalog did not include built-in shelves")
	}
	for _, shelfType := range []string{"genre", "decade", "mdblist", "stremio", "tmdb", "trakt", "simkl", "letterboxd", "collection-hub", "library"} {
		entry, ok := byType[shelfType]
		if !ok {
			t.Fatalf("catalog missing %q template", shelfType)
		}
		if !entry.Multiple {
			t.Errorf("%q template Multiple = false, want true", shelfType)
		}
	}
	if _, exists := byType["streaming-service"]; exists {
		t.Fatal("catalog exposes streaming-service as a non-renderable persisted row")
	}
	trakt := byType["trakt"]
	if !hasOption(trakt.Fields, "traktAccountId", "trakt-1") {
		t.Fatalf("Trakt account options = %#v, want linked account without credentials", trakt.Fields)
	}
}

func TestHomeDesignerCatalog_MarksMissingCapabilitiesUnavailableWithSetupLinks(t *testing.T) {
	entries := BuildCatalog(config.DefaultSettings(), nil)
	byType := map[string]CatalogEntry{}
	for _, entry := range entries {
		if _, exists := byType[entry.Type]; !exists {
			byType[entry.Type] = entry
		}
	}
	for _, shelfType := range []string{"trakt", "tmdb", "library"} {
		entry, ok := byType[shelfType]
		if !ok {
			t.Fatalf("catalog missing %q template", shelfType)
		}
		if entry.Available {
			t.Errorf("%q Available = true, want false without configured capability", shelfType)
		}
		if strings.TrimSpace(entry.UnavailableReason) == "" {
			t.Errorf("%q UnavailableReason is empty", shelfType)
		}
		if strings.TrimSpace(entry.SetupPath) == "" {
			t.Errorf("%q SetupPath is empty", shelfType)
		}
	}
}

func TestHomeDesignerCatalog_DoesNotExposeForeignIntegrationAccounts(t *testing.T) {
	cfg := config.DefaultSettings()
	cfg.Trakt.Accounts = []config.TraktAccount{
		{ID: "owned", Name: "Owned", OwnerAccountID: "account-a"},
		{ID: "foreign", Name: "Foreign", OwnerAccountID: "account-b"},
		{ID: "shared", Name: "Shared"},
	}
	cfg.Simkl.Accounts = []config.SimklAccount{
		{ID: "owned-simkl", Name: "Owned Simkl", OwnerAccountID: "account-a"},
		{ID: "foreign-simkl", Name: "Foreign Simkl", OwnerAccountID: "account-b"},
	}
	context := CatalogContext{
		Actor:    Actor{AccountID: "account-a"},
		Profiles: []models.User{{ID: "profile-a", AccountID: "account-a", Name: "Avery", TraktAccountID: "owned", SimklAccountID: "owned-simkl"}},
	}
	entries := BuildCatalogForContext(cfg, context)
	byType := catalogByType(entries)
	if !hasOption(byType["trakt"].Fields, "traktAccountId", "owned") || hasOption(byType["trakt"].Fields, "traktAccountId", "foreign") || hasOption(byType["trakt"].Fields, "traktAccountId", "shared") {
		t.Fatalf("Trakt options = %#v, want owned only", byType["trakt"].Fields)
	}
	if !hasOption(byType["simkl"].Fields, "simklAccountId", "owned-simkl") || hasOption(byType["simkl"].Fields, "simklAccountId", "foreign-simkl") {
		t.Fatalf("Simkl options = %#v, want owned only", byType["simkl"].Fields)
	}

	context.IncludeSharedAccounts = true
	entries = BuildCatalogForContext(cfg, context)
	if !hasOption(catalogByType(entries)["trakt"].Fields, "traktAccountId", "shared") {
		t.Fatal("intentionally authorized shared account was not included")
	}
}

func TestHomeDesignerCatalog_UsesAuthorizedLibrariesAndRoleAwareSetupPaths(t *testing.T) {
	ctx := CatalogContext{
		Actor:     Actor{AccountID: "account-a"},
		BasePath:  "/mediastorm",
		Libraries: []CatalogLibrary{{ID: "library-a", Name: "Avery's library"}},
	}
	entries := BuildCatalogForContext(config.DefaultSettings(), ctx)
	byType := catalogByType(entries)
	library := byType["library"]
	if !library.Available || !hasOption(library.Fields, "libraryId", "library-a") {
		t.Fatalf("library template = %#v, want authorized concrete library", library)
	}
	if got := byType["trakt"].SetupPath; got != "/mediastorm/account/tools" {
		t.Fatalf("account Trakt setup path = %q, want /mediastorm/account/tools", got)
	}

	ctx.Actor.IsAdmin = true
	ctx.Libraries = nil
	entries = BuildCatalogForContext(config.DefaultSettings(), ctx)
	byType = catalogByType(entries)
	if got := byType["library"].SetupPath; got != "/mediastorm/admin/library" {
		t.Fatalf("admin library setup path = %q, want /mediastorm/admin/library", got)
	}
}

func hasOption(fields []CatalogField, path, value string) bool {
	for _, field := range fields {
		if field.Path != path {
			continue
		}
		for _, option := range field.Options {
			if option.Value == value {
				return true
			}
		}
	}
	return false
}

func catalogByType(entries []CatalogEntry) map[string]CatalogEntry {
	byType := make(map[string]CatalogEntry, len(entries))
	for _, entry := range entries {
		if _, exists := byType[entry.Type]; !exists {
			byType[entry.Type] = entry
		}
	}
	return byType
}
