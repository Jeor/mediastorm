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
	for _, shelfType := range []string{"genre", "decade", "streaming-service", "mdblist", "stremio", "tmdb", "trakt", "simkl", "letterboxd", "collection-hub", "library"} {
		entry, ok := byType[shelfType]
		if !ok {
			t.Fatalf("catalog missing %q template", shelfType)
		}
		if !entry.Multiple {
			t.Errorf("%q template Multiple = false, want true", shelfType)
		}
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
