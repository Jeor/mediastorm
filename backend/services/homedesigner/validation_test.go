package homedesigner

import (
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestValidateHomeDesigner_RejectsDuplicateUniqueRows(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "top-ten", Name: "Top 10", Type: "builtin"},
		{ID: "top-ten", Name: "Top 10 again", Type: "builtin"},
	}})
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	if !hasFieldError(errs, "rows", "top-ten", "id") {
		t.Fatalf("errors = %#v, want duplicate built-in id error", errs)
	}
}

func TestValidateHomeDesigner_AllowsRepeatedGenreRows(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-16-movie", Name: "Animation", Type: "genre"},
		{ID: "genre-35-movie", Name: "Comedy", Type: "genre"},
	}})
	if errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil)); len(errs) != 0 {
		t.Fatalf("errors = %#v, want repeated genre rows allowed", errs)
	}
}

func TestValidateHomeDesigner_RejectsInvalidRowsAndTheme(t *testing.T) {
	fontScale := 2.5
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "unknown-1", Name: "Unknown", Type: "unknown"},
		{ID: "list-1", Name: " ", Type: "mdblist", Limit: -1},
		{ID: "stremio-1", Name: "Stremio", Type: "stremio"},
		{ID: "list-2", Name: "Too many", Type: "mdblist", ListURL: "https://mdblist.com/lists/a/b/json", Limit: 501},
	}})
	request.Theme = &SectionMutation[models.AppearanceSettings]{Mode: ModeCustom, Value: &models.AppearanceSettings{
		FontScale: &fontScale, AccentColor: "blue", ButtonStyle: "raised", ButtonRadius: "round",
	}}
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	for _, want := range []struct{ rowID, path string }{
		{"unknown-1", "type"}, {"list-1", "name"}, {"list-1", "limit"}, {"stremio-1", "addonManifestUrl"}, {"list-2", "limit"},
		{"", "accentColor"}, {"", "fontScale"}, {"", "buttonStyle"}, {"", "buttonRadius"},
	} {
		if !hasFieldError(errs, "rows", want.rowID, want.path) && !hasFieldError(errs, "theme", want.rowID, want.path) {
			t.Errorf("errors = %#v, want path %q for row %q", errs, want.path, want.rowID)
		}
	}
}

func TestValidateHomeDesigner_NormalizesOrderAndWhitespace(t *testing.T) {
	shelfScale := 1.4
	heroScale := 0.25
	rows := &models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: " second ", Name: " Second ", Type: "genre", Order: 8},
		{ID: " first ", Name: " First ", Type: "genre", Order: 4},
	}, HomeShelfScale: &shelfScale, HomeHeroScale: &heroScale}
	request := customRowsRequest(*rows)
	if errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil)); len(errs) != 0 {
		t.Fatalf("errors = %#v, want normalization without errors", errs)
	}
	got := request.Rows.Value
	if got.Shelves[0].ID != "first" || got.Shelves[0].Name != "First" || got.Shelves[0].Order != 0 ||
		got.Shelves[1].ID != "second" || got.Shelves[1].Name != "Second" || got.Shelves[1].Order != 1 {
		t.Fatalf("normalized shelves = %#v", got.Shelves)
	}
	if *got.HomeShelfScale != 1.0 || *got.HomeHeroScale != 0.5 {
		t.Fatalf("normalized scales = %v, %v; want 1.0, 0.5", *got.HomeShelfScale, *got.HomeHeroScale)
	}
}

func TestValidateHomeDesigner_RejectsGlobalInheritance(t *testing.T) {
	request := ApplyRequest{Scope: Scope{Kind: "global"}, Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeInherit}}
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	if !hasFieldError(errs, "rows", "", "mode") {
		t.Fatalf("errors = %#v, want global inheritance error", errs)
	}
}

func customRowsRequest(rows models.HomeShelvesSettings) ApplyRequest {
	return ApplyRequest{Scope: Scope{Kind: "profile", ProfileID: "profile-1"}, Rows: &SectionMutation[models.HomeShelvesSettings]{Mode: ModeCustom, Value: &rows}}
}

func hasFieldError(errs []FieldError, section, rowID, path string) bool {
	for _, err := range errs {
		if err.Section == section && err.RowID == rowID && err.Path == path {
			return true
		}
	}
	return false
}
