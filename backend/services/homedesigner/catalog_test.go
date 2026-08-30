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
	entries := BuildCatalogForContext(cfg, CatalogContext{AuthorizedAccounts: []CatalogAccountAuthorization{{Provider: "trakt", AccountID: "trakt-1"}}})

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
	for _, shelfType := range []string{"genre", "decade", "streaming-service", "mdblist", "stremio", "tmdb", "trakt", "simkl", "letterboxd", "collection-hub", "library"} {
		entry, ok := byType[shelfType]
		if !ok {
			t.Fatalf("catalog missing %q template", shelfType)
		}
		if !entry.Multiple {
			t.Errorf("%q template Multiple = false, want true", shelfType)
		}
	}
	streaming := byType["streaming-service"]
	if !streaming.CatalogOnly || streaming.Expansion == nil || streaming.Expansion.OutputType != "mdblist" || streaming.Expansion.MinRows != 1 || streaming.Expansion.MaxRows != 2 || !hasOption(streaming.Fields, "media", "both") || !hasOption(streaming.Fields, "service", "netflix") {
		t.Fatalf("streaming-service template = %#v, want catalog-only MDBList expansion", streaming)
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
	for _, shelfType := range []string{"trakt", "library"} {
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
	if got := byType["tmdb"]; got.Available || got.SetupPath != "" || !strings.Contains(strings.ToLower(got.UnavailableReason), "administrator") {
		t.Fatalf("account TMDB template = %#v, want administrator guidance without unusable action", got)
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
		Actor:              Actor{AccountID: "account-a"},
		AuthorizedAccounts: []CatalogAccountAuthorization{{Provider: "trakt", AccountID: "owned"}, {Provider: "simkl", AccountID: "owned-simkl"}},
	}
	entries := BuildCatalogForContext(cfg, context)
	byType := catalogByType(entries)
	if !hasOption(byType["trakt"].Fields, "traktAccountId", "owned") || hasOption(byType["trakt"].Fields, "traktAccountId", "foreign") || hasOption(byType["trakt"].Fields, "traktAccountId", "shared") {
		t.Fatalf("Trakt options = %#v, want owned only", byType["trakt"].Fields)
	}
	if !hasOption(byType["simkl"].Fields, "simklAccountId", "owned-simkl") || hasOption(byType["simkl"].Fields, "simklAccountId", "foreign-simkl") {
		t.Fatalf("Simkl options = %#v, want owned only", byType["simkl"].Fields)
	}

	if hasOption(byType["trakt"].Fields, "traktAccountId", "shared") {
		t.Fatal("ownerless account leaked without an explicit provider authorization")
	}
	context.AuthorizedAccounts = append(context.AuthorizedAccounts, CatalogAccountAuthorization{Provider: "trakt", AccountID: "shared"})
	entries = BuildCatalogForContext(cfg, context)
	if !hasOption(catalogByType(entries)["trakt"].Fields, "traktAccountId", "shared") {
		t.Fatal("explicitly authorized account was not included")
	}
}

func TestHomeDesignerCatalog_ProfileLinksNeverAuthorizeForeignOrCrossLoginAccounts(t *testing.T) {
	cfg := config.DefaultSettings()
	cfg.Trakt.Accounts = []config.TraktAccount{{ID: "foreign", Name: "Foreign", OwnerAccountID: "account-b"}, {ID: "ownerless", Name: "Ownerless"}}
	entries := BuildCatalog(cfg, []models.User{
		{ID: "a-foreign", AccountID: "account-a", TraktAccountID: "foreign"},
		{ID: "a-ownerless", AccountID: "account-a", TraktAccountID: "ownerless"},
		{ID: "b-ownerless", AccountID: "account-b", TraktAccountID: "ownerless"},
	})
	trakt := catalogByType(entries)["trakt"]
	if hasOption(trakt.Fields, "traktAccountId", "foreign") || hasOption(trakt.Fields, "traktAccountId", "ownerless") {
		t.Fatalf("profile links authorized integration accounts: %#v", trakt.Fields)
	}
}

func TestHomeDesignerCatalog_AdminMaySeeUnlinkedForeignAccounts(t *testing.T) {
	cfg := config.DefaultSettings()
	cfg.Trakt.Accounts = []config.TraktAccount{{ID: "foreign", Name: "Foreign", OwnerAccountID: "account-b"}}
	entries := BuildCatalogForContext(cfg, CatalogContext{Actor: Actor{IsAdmin: true, AccountID: "admin"}})
	if !hasOption(catalogByType(entries)["trakt"].Fields, "traktAccountId", "foreign") {
		t.Fatal("admin catalog omitted authorized unlinked foreign account")
	}
}

func TestHomeDesignerCatalog_ExpandsStreamingServiceIntoRenderableMDBListRows(t *testing.T) {
	rows, errs := ExpandStreamingServiceSelection(StreamingServiceSelection{InstanceID: "netflix-a", Service: "netflix", Media: "both", Limit: 20})
	if len(errs) != 0 || len(rows) != 2 {
		t.Fatalf("expansion = %#v, %#v; want two rows without errors", rows, errs)
	}
	for _, row := range rows {
		if row.Type != "mdblist" || !strings.HasPrefix(row.ListURL, "https://mdblist.com/") || row.ID == "streaming-service" {
			t.Fatalf("expanded row = %#v, want renderable MDBList row", row)
		}
	}
}

func TestHomeDesignerCatalog_ExpandsAmazonPrimeToCanonicalMovieAndTVLists(t *testing.T) {
	rows, errs := ExpandStreamingServiceSelection(StreamingServiceSelection{InstanceID: "amazon-a", Service: "amazon", Media: "both"})
	if len(errs) != 0 || len(rows) != 2 {
		t.Fatalf("expansion = %#v, %#v; want Amazon movie and TV rows", rows, errs)
	}
	if got, want := rows[0].ListURL, "https://mdblist.com/lists/snoak/amazon-prime-top-10-movies/json"; got != want {
		t.Fatalf("Amazon movie URL = %q, want %q", got, want)
	}
	if got, want := rows[1].ListURL, "https://mdblist.com/lists/snoak/amazon-prime-top-10-tv-shows/json"; got != want {
		t.Fatalf("Amazon TV URL = %q, want %q", got, want)
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
	if got := byType["tmdb"].SetupPath; got != "/mediastorm/admin/settings" {
		t.Fatalf("admin TMDB setup path = %q, want /mediastorm/admin/settings", got)
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
