package homedesigner

import (
	"reflect"
	"testing"

	"novastream/config"
)

func TestShelfConversion_RoundTripsEveryPersistedField(t *testing.T) {
	trueValue := true
	input := []config.ShelfConfig{{
		ID: "custom-stremio", Name: " Everything ", Enabled: true, Order: 7, Type: "stremio",
		LibraryID: "library-1", ListURL: "https://mdblist.com/lists/example/list/json",
		AddonManifestURL: "https://addon.example/manifest.json", AddonCatalogType: "movie", AddonCatalogID: "catalog-1", AddonName: "Example Addon",
		TMDBSourceType: "public-list", TMDBSourceID: "123", TMDBSourceName: "Example List", TMDBMediaType: "all", TMDBDiscoverQuery: "sort_by=popularity.desc",
		StreamingServices: []config.StreamingServiceLink{{ID: "netflix", Name: "Netflix", Enabled: true, Order: 1, LogoURL: "https://cdn.example/netflix.png", LogoScale: 1.2, TintColor: "#112233", Lists: []config.StreamingServiceListLink{{Key: "trending", Title: "Trending", URL: "https://example.com/trending"}}}},
		CollectionItems:   []config.CollectionHubLink{{ID: "collection-1", Name: "Collection", Enabled: true, Order: 2, SourceShelfID: "source-1", LogoURL: "https://cdn.example/logo.png", HeroArtURL: "https://cdn.example/hero.png", LogoScale: 1.1, TintColor: "#445566"}},
		TraktAccountID:    "trakt-1", TraktListType: "custom", TraktListID: "list-1",
		SimklAccountID: "simkl-1", SimklListType: "watching", SimklMediaType: "shows",
		LetterboxdListID: "letterboxd-1", LetterboxdListURL: "https://letterboxd.com/example/list/list/",
		Limit: 50, ActivityWindowDays: 30, MinimumProfiles: 2, MaxItemsPerProfile: 3,
		HideUnreleased: true, Sort: "popularity.desc", AnimateLogoOnlyOnFocus: true, ShowCollectionTitles: true, ShowCollectionCounts: true,
		CalendarSources: config.CalendarSourceSettings{Watchlist: &trueValue, History: &trueValue, Trending: &trueValue, TopTrending: &trueValue, MDBLists: &trueValue, MDBListShelves: map[string]bool{"custom-list": true}},
	}}

	got := ModelShelvesToConfig(ConfigShelvesToModels(input))
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("round trip = %#v, want %#v", got, input)
	}
}

func TestAppearanceConversion_RoundTripsEveryPersistedField(t *testing.T) {
	fontScale := 1.15
	highContrast := true
	reduceOverlays := true
	input := config.AppearanceSettings{
		FontScale: &fontScale, AccentColor: "#112233", TextColor: "#223344", SecondaryTextColor: "#334455",
		BackgroundColor: "#445566", ModalBackgroundColor: "#556677", ButtonStyle: "outlined", ButtonRadius: "pill",
		HighContrast: &highContrast, ReduceOverlays: &reduceOverlays,
	}

	got := ModelAppearanceToConfig(ConfigAppearanceToModel(input))
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("round trip = %#v, want %#v", got, input)
	}
}
