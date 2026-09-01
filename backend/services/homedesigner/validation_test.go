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

func TestValidateHomeDesigner_MDBListOnlyAcceptsSafePublicListURLs(t *testing.T) {
	for _, test := range []struct {
		name, value string
		valid       bool
	}{
		{name: "canonical", value: "https://mdblist.com/lists/user/list-name/json", valid: true},
		{name: "canonical trailing slash", value: "https://mdblist.com/lists/user/list-name/json/", valid: true},
		{name: "documented shorthand", value: "https://mdblist.com/lists/user/list-name", valid: true},
		{name: "documented shorthand trailing slash", value: "https://mdblist.com/lists/user/list-name/", valid: true},
		{name: "http", value: "http://mdblist.com/lists/user/list/json"},
		{name: "loopback", value: "http://127.0.0.1/lists/user/list/json"},
		{name: "link local", value: "https://169.254.169.254/lists/user/list/json"},
		{name: "misleading host", value: "https://mdblist.com.evil.example/lists/user/list/json"},
		{name: "userinfo", value: "https://user:secret@mdblist.com/lists/user/list/json"},
		{name: "misleading path", value: "https://mdblist.com/redirect?next=/lists/user/list/json"},
		{name: "extra path", value: "https://mdblist.com/lists/user/list/json/extra"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{{ID: "list", Name: "List", Type: "mdblist", ListURL: test.value}}})
			errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
			if got := !hasFieldError(errs, "rows", "list", "listUrl"); got != test.valid {
				t.Fatalf("valid = %v, want %v; errors=%#v", got, test.valid, errs)
			}
		})
	}
}

func TestValidateHomeDesigner_NormalizesOrderAndWhitespace(t *testing.T) {
	shelfScale := 1.4
	heroScale := 0.25
	rows := &models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: " genre-35-movie ", Name: " Second ", Type: "genre", Order: 8},
		{ID: " genre-16-movie ", Name: " First ", Type: "genre", Order: 4},
	}, HomeShelfScale: &shelfScale, HomeHeroScale: &heroScale}
	request := customRowsRequest(*rows)
	if errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil)); len(errs) != 0 {
		t.Fatalf("errors = %#v, want normalization without errors", errs)
	}
	got := request.Rows.Value
	if got.Shelves[0].ID != "genre-16-movie" || got.Shelves[0].Name != "First" || got.Shelves[0].Order != 0 ||
		got.Shelves[1].ID != "genre-35-movie" || got.Shelves[1].Name != "Second" || got.Shelves[1].Order != 1 {
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

func TestValidateHomeDesigner_RejectsDuplicateInstanceIDsForRepeatableTypes(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-16-movie", Name: "Animation", Type: "genre"},
		{ID: "genre-16-movie", Name: "Animation again", Type: "genre"},
	}})
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	if !hasFieldError(errs, "rows", "genre-16-movie", "id") {
		t.Fatalf("errors = %#v, want duplicate repeatable id error", errs)
	}
}

func TestValidateHomeDesigner_RejectsUnavailableAndNonRenderableCatalogRows(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "tmdb-1", Name: "TMDB", Type: "tmdb", TMDBSourceType: "public-list", TMDBSourceID: "1", TMDBMediaType: "movie"},
		{ID: "streaming-1", Name: "Streaming", Type: "streaming-service"},
	}})
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	if !hasFieldError(errs, "rows", "tmdb-1", "type") || !hasFieldError(errs, "rows", "streaming-1", "type") {
		t.Fatalf("errors = %#v, want unavailable and non-renderable type errors", errs)
	}
}

func TestValidateHomeDesigner_EnforcesRenderableIdentifiersAndCatalogOptions(t *testing.T) {
	cfg := config.DefaultSettings()
	cfg.Metadata.TMDBAPIKey = "tmdb-key"
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-not-a-number-movie", Name: "Genre", Type: "genre"},
		{ID: "decade-1700-tv", Name: "Decade", Type: "decade"},
		{ID: "tmdb-1", Name: "TMDB", Type: "tmdb", TMDBSourceType: "unsupported", TMDBSourceID: "1", TMDBMediaType: "series", TMDBDiscoverQuery: "%%", Sort: "made-up"},
		{ID: "stremio-1", Name: "Stremio", Type: "stremio", AddonManifestURL: "not-a-url", AddonCatalogType: "podcast", AddonCatalogID: "catalog"},
	}})
	errs := ValidateApply(request, BuildCatalog(cfg, nil))
	for _, want := range []struct{ rowID, path string }{
		{"genre-not-a-number-movie", "id"}, {"decade-1700-tv", "id"}, {"tmdb-1", "tmdbSourceType"}, {"tmdb-1", "tmdbMediaType"}, {"tmdb-1", "tmdbDiscoverQuery"}, {"tmdb-1", "sort"}, {"stremio-1", "addonManifestUrl"}, {"stremio-1", "addonCatalogType"},
	} {
		if !hasFieldError(errs, "rows", want.rowID, want.path) {
			t.Errorf("errors = %#v, want %s error for %s", errs, want.path, want.rowID)
		}
	}
}

func TestValidateHomeDesigner_AcceptsLegacyProfileDashboardBuiltIn(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{{ID: "dashboard", Name: "Dashboard", Enabled: true}}})
	if errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil)); len(errs) != 0 {
		t.Fatalf("errors = %#v, want dashboard accepted as a legacy profile built-in", errs)
	}
}

func TestValidateHomeDesigner_NormalizesCollectionHubItems(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-1-movie", Name: "Source A", Type: "genre"},
		{ID: "genre-2-movie", Name: "Source B", Type: "genre"},
		{ID: "hub", Name: "Hub", Type: "collection-hub", CollectionItems: []models.CollectionHubLink{
			{ID: " item-b ", Name: " B ", SourceShelfID: " genre-2-movie ", Order: 8, LogoURL: "https://example.test/b.png", HeroArtURL: "https://example.test/b.jpg", LogoScale: 1.2, TintColor: " #112233 "},
			{ID: " item-a ", Name: " A ", SourceShelfID: " genre-1-movie ", Order: 4},
		}},
	}})
	if errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil)); len(errs) != 0 {
		t.Fatalf("errors = %#v, want valid normalized collection hub", errs)
	}
	items := request.Rows.Value.Shelves[2].CollectionItems
	if items[0].ID != "item-a" || items[0].Name != "A" || items[0].SourceShelfID != "genre-1-movie" || items[0].Order != 0 || items[1].ID != "item-b" || items[1].Order != 1 || items[1].TintColor != "#112233" {
		t.Fatalf("normalized collection items = %#v", items)
	}
}

func TestValidateHomeDesigner_RejectsMalformedCollectionHubItems(t *testing.T) {
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-1-movie", Name: "Source", Type: "genre"},
		{ID: "hub", Name: "Hub", Type: "collection-hub", CollectionItems: []models.CollectionHubLink{
			{ID: " ", Name: "", SourceShelfID: "missing", Order: 0, LogoURL: "bad-url", LogoScale: 3, TintColor: "blue"},
			{ID: "same", Name: "Same", SourceShelfID: "genre-1-movie", Order: 1},
			{ID: "same", Name: "Same", SourceShelfID: "genre-1-movie", Order: 2},
		}},
	}})
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	for _, want := range []struct{ itemID, path string }{{"", "id"}, {"", "name"}, {"", "sourceShelfId"}, {"", "logoUrl"}, {"", "logoScale"}, {"", "tintColor"}, {"same", "id"}, {"same", "name"}, {"same", "sourceShelfId"}} {
		if !hasCollectionItemError(errs, "hub", want.itemID, want.path) {
			t.Errorf("errors = %#v, want item %q %s error", errs, want.itemID, want.path)
		}
	}
}

func TestValidateHomeDesigner_PreservesAdvancedCollectionHubDefaultTint(t *testing.T) {
	const advancedDefaultTint = "rgba(148,163,184,0.18)"
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-1-movie", Name: "Source", Type: "genre"},
		{ID: "hub", Name: "Hub", Type: "collection-hub", CollectionItems: []models.CollectionHubLink{{ID: "collection-1", Name: "Collection", SourceShelfID: "genre-1-movie", TintColor: advancedDefaultTint}}},
	}})
	if errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil)); len(errs) != 0 {
		t.Fatalf("errors = %#v, want persisted advanced default tint accepted", errs)
	}
	if got := request.Rows.Value.Shelves[1].CollectionItems[0].TintColor; got != advancedDefaultTint {
		t.Fatalf("tint = %q, want lossless %q", got, advancedDefaultTint)
	}
}

func TestValidateHomeDesigner_RejectsInvalidCollectionHubSourcesAndItemOverflow(t *testing.T) {
	items := make([]models.CollectionHubLink, 0, 21)
	for i := 0; i < 21; i++ {
		items = append(items, models.CollectionHubLink{ID: "item-" + string(rune('a'+i)), Name: "Item " + string(rune('a'+i)), SourceShelfID: "genre-1-movie", Order: i})
	}
	request := customRowsRequest(models.HomeShelvesSettings{Shelves: []models.ShelfConfig{
		{ID: "genre-1-movie", Name: "Source", Type: "genre"},
		{ID: "streaming-services", Name: "Streaming Services", Type: "builtin"},
		{ID: "hub-a", Name: "Hub A", Type: "collection-hub", CollectionItems: []models.CollectionHubLink{{ID: "from-hub", Name: "From hub", SourceShelfID: "hub-b"}}},
		{ID: "hub-b", Name: "Hub B", Type: "collection-hub", CollectionItems: []models.CollectionHubLink{{ID: "cycle", Name: "Cycle", SourceShelfID: "hub-a"}}},
		{ID: "hub-stream", Name: "Hub stream", Type: "collection-hub", CollectionItems: []models.CollectionHubLink{{ID: "from-streaming", Name: "From streaming", SourceShelfID: "streaming-services"}}},
		{ID: "hub-overflow", Name: "Overflow", Type: "collection-hub", CollectionItems: items},
	}})
	errs := ValidateApply(request, BuildCatalog(config.DefaultSettings(), nil))
	if !hasCollectionItemError(errs, "hub-a", "from-hub", "sourceShelfId") || !hasCollectionItemError(errs, "hub-b", "cycle", "sourceShelfId") || !hasCollectionItemError(errs, "hub-stream", "from-streaming", "sourceShelfId") || !hasCollectionItemError(errs, "hub-overflow", "", "collectionItems") {
		t.Fatalf("errors = %#v, want invalid source-type and item-cap errors", errs)
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

func hasCollectionItemError(errs []FieldError, rowID, itemID, path string) bool {
	for _, err := range errs {
		if err.Section == "rows" && err.RowID == rowID && err.ItemID == itemID && err.Path == path {
			return true
		}
	}
	return false
}
