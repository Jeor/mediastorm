package homedesigner

import (
	"path"
	"strings"

	"novastream/config"
	"novastream/models"
)

// BuildCatalog preserves the original simple API while defaulting to the
// strictest safe behavior: no integrations are exposed. Use
// BuildCatalogForContext with explicit provider authorization decisions.
func BuildCatalog(settings config.Settings, _ []models.User) []CatalogEntry {
	return BuildCatalogForContext(settings, CatalogContext{})
}

// BuildCatalogForContext returns templates that the supplied actor and profile
// set may actually configure. It exposes no credentials, source URLs, or
// accounts belonging to another account owner.
func BuildCatalogForContext(settings config.Settings, context CatalogContext) []CatalogEntry {
	entries := builtInCatalogEntries()
	traktOptions := traktAccountOptions(settings.Trakt.Accounts, context)
	simklOptions := simklAccountOptions(settings.Simkl.Accounts, context)
	libraryOptions := libraryOptions(context.Libraries)
	tmdbAvailable, tmdbReason, tmdbSetupPath := tmdbAvailability(settings, context)

	entries = append(entries,
		repeatableEntry("genre", "Genre", "Browse a movie or TV genre.", "Discovery", models.ShelfConfig{Type: "genre"}, []CatalogField{{Path: "id", Type: "text", Label: "Genre row ID", Required: true}}, "shelf"),
		repeatableEntry("decade", "Decade", "Browse a movie or TV decade.", "Discovery", models.ShelfConfig{Type: "decade"}, []CatalogField{{Path: "id", Type: "text", Label: "Decade row ID", Required: true}}, "shelf"),
		streamingServiceEntry(),
		repeatableEntry("mdblist", "MDBList", "Show an MDBList URL.", "Lists", models.ShelfConfig{Type: "mdblist"}, []CatalogField{{Path: "listUrl", Type: "url", Label: "MDBList URL", Required: true}, limitField()}, "shelf"),
		repeatableEntry("stremio", "Stremio add-on", "Show a catalog from a Stremio add-on.", "Lists", models.ShelfConfig{Type: "stremio"}, []CatalogField{{Path: "addonManifestUrl", Type: "url", Label: "Manifest URL", Required: true}, {Path: "addonCatalogType", Type: "select", Label: "Catalog type", Required: true, Options: []Option{{Value: "movie", Label: "Movies"}, {Value: "series", Label: "Series"}}}, {Path: "addonCatalogId", Type: "text", Label: "Catalog", Required: true}}, "shelf"),
		withAvailability(repeatableEntry("tmdb", "TMDB", "Build a shelf from TMDB.", "Discovery", models.ShelfConfig{Type: "tmdb"}, []CatalogField{{Path: "tmdbSourceType", Type: "select", Label: "Source type", Required: true, Options: tmdbSourceTypeOptions()}, {Path: "tmdbSourceId", Type: "text", Label: "Source"}, {Path: "tmdbMediaType", Type: "select", Label: "Content", Required: true, Options: []Option{{Value: "movie", Label: "Movies"}, {Value: "tv", Label: "TV shows"}, {Value: "all", Label: "Movies and TV shows"}}}, {Path: "sort", Type: "select", Label: "Sort", Options: sortOptions()}}, "shelf"), tmdbAvailable, tmdbReason, tmdbSetupPath),
		withAvailability(repeatableEntry("trakt", "Trakt", "Show a Trakt watchlist or custom list.", "Lists", models.ShelfConfig{Type: "trakt"}, []CatalogField{{Path: "traktAccountId", Type: "select", Label: "Trakt account", Required: true, Options: traktOptions}, {Path: "traktListType", Type: "select", Label: "List type", Required: true, Options: []Option{{Value: "watchlist", Label: "Watchlist"}, {Value: "custom", Label: "Custom list"}}}, {Path: "traktListId", Type: "text", Label: "Custom list"}}, "shelf"), len(traktOptions) > 0, "Connect a Trakt account before adding this shelf.", setupPath(settings, context, "tools")),
		withAvailability(repeatableEntry("simkl", "Simkl", "Show a Simkl list.", "Lists", models.ShelfConfig{Type: "simkl"}, []CatalogField{{Path: "simklAccountId", Type: "select", Label: "Simkl account", Required: true, Options: simklOptions}, {Path: "simklMediaType", Type: "select", Label: "Content", Required: true, Options: []Option{{Value: "movies", Label: "Movies"}, {Value: "shows", Label: "Shows"}, {Value: "anime", Label: "Anime"}}}, {Path: "simklListType", Type: "select", Label: "List", Options: []Option{{Value: "plantowatch", Label: "Plan to watch"}, {Value: "watching", Label: "Watching"}, {Value: "completed", Label: "Completed"}, {Value: "hold", Label: "On hold"}, {Value: "dropped", Label: "Dropped"}}}}, "shelf"), len(simklOptions) > 0, "Connect a Simkl account before adding this shelf.", setupPath(settings, context, "tools")),
		repeatableEntry("letterboxd", "Letterboxd", "Show a public Letterboxd list.", "Lists", models.ShelfConfig{Type: "letterboxd"}, []CatalogField{{Path: "letterboxdListUrl", Type: "url", Label: "Public list URL"}, {Path: "letterboxdListId", Type: "text", Label: "Imported list ID"}}, "shelf"),
		repeatableEntry("collection-hub", "Collection hub", "Group shelves into a collection hub.", "Organization", models.ShelfConfig{Type: "collection-hub"}, []CatalogField{{Path: "collectionItems", Type: "collection", Label: "Collections"}}, "hub"),
		withAvailability(repeatableEntry("library", "Media library", "Show a configured media library.", "Libraries", models.ShelfConfig{Type: "library"}, []CatalogField{{Path: "libraryId", Type: "select", Label: "Media library", Required: true, Options: libraryOptions}}, "shelf"), len(libraryOptions) > 0, "Configure a media library before adding this shelf.", setupPath(settings, context, "library")),
	)
	return entries
}

func builtInCatalogEntries() []CatalogEntry {
	configShelves := config.DefaultHomeShelfConfigs()
	profileShelves := models.DefaultHomeShelfConfigs()
	seen := make(map[string]struct{}, len(configShelves)+len(profileShelves))
	entries := make([]CatalogEntry, 0, len(configShelves)+len(profileShelves))
	add := func(shelf models.ShelfConfig) {
		if _, exists := seen[shelf.ID]; exists {
			return
		}
		seen[shelf.ID] = struct{}{}
		shelf.Type = "builtin"
		entries = append(entries, CatalogEntry{Type: "builtin", Name: shelf.Name, Description: "Built-in home shelf", Category: "Built-in", Available: true, Default: shelf, Fields: []CatalogField{{Path: "name", Type: "text", Label: "Name", Required: true}}, PreviewKind: "shelf"})
	}
	for _, shelf := range ConfigShelvesToModels(configShelves) {
		add(shelf)
	}
	for _, shelf := range profileShelves {
		add(shelf)
	}
	return entries
}

func repeatableEntry(shelfType, name, description, category string, defaultShelf models.ShelfConfig, fields []CatalogField, previewKind string) CatalogEntry {
	return CatalogEntry{Type: shelfType, Name: name, Description: description, Category: category, Multiple: true, Available: true, Default: defaultShelf, Fields: fields, PreviewKind: previewKind}
}

func withAvailability(entry CatalogEntry, available bool, reason, setupPath string) CatalogEntry {
	entry.Available = available
	if !available {
		entry.UnavailableReason, entry.SetupPath = reason, setupPath
	}
	return entry
}

func tmdbAvailability(settings config.Settings, context CatalogContext) (bool, string, string) {
	if strings.TrimSpace(settings.Metadata.TMDBAPIKey) != "" {
		return true, "", ""
	}
	if !context.Actor.IsAdmin {
		return false, "TMDB API access must be configured by an administrator.", ""
	}
	return false, "TMDB API access is not configured.", setupPath(settings, context, "settings")
}

func streamingServiceEntry() CatalogEntry {
	return CatalogEntry{
		Type: "streaming-service", Name: "Streaming service", Description: "Expand a streaming provider into MDBList movie and TV shelves.", Category: "Lists", Multiple: true, Available: true,
		Default: models.ShelfConfig{Type: "mdblist"}, Fields: []CatalogField{
			{Path: "service", Type: "select", Label: "Service", Required: true, Options: streamingServiceOptions()},
			{Path: "media", Type: "select", Label: "Content", Required: true, Options: []Option{{Value: "movies", Label: "Movies"}, {Value: "shows", Label: "TV shows"}, {Value: "both", Label: "Movies and TV shows"}}},
		}, PreviewKind: "shelf", CatalogOnly: true, Expansion: &CatalogExpansion{OutputType: "mdblist", MinRows: 1, MaxRows: 2},
	}
}

func streamingServiceOptions() []Option {
	return []Option{{Value: "netflix", Label: "Netflix"}, {Value: "disney", Label: "Disney+"}, {Value: "amazon", Label: "Amazon Prime"}, {Value: "appletv", Label: "Apple TV+"}, {Value: "paramount", Label: "Paramount+"}, {Value: "hbomax", Label: "HBO Max"}, {Value: "hulu", Label: "Hulu"}, {Value: "crunchyroll", Label: "Crunchyroll"}}
}

type streamingServiceLists struct{ movies, shows string }

var streamingServiceMDBLists = map[string]streamingServiceLists{
	"netflix":     {"https://mdblist.com/lists/snoak/netflix-top-10-movies/json", "https://mdblist.com/lists/snoak/netflix-top-10-shows/json"},
	"disney":      {"https://mdblist.com/lists/snoak/disney-plus-top-10-movies/json", "https://mdblist.com/lists/snoak/disney-plus-top-10-tv-shows/json"},
	"amazon":      {"https://mdblist.com/lists/snoak/amazon-prime-top-10-shows/json", "https://mdblist.com/lists/snoak/amazon-prime-top-10-tv-shows/json"},
	"appletv":     {"https://mdblist.com/lists/snoak/apple-tv-top-10-movies/json", "https://mdblist.com/lists/snoak/apple-tv-top-10-tv-shows/json"},
	"paramount":   {"https://mdblist.com/lists/snoak/paramount-plus-top-10-movies/json", "https://mdblist.com/lists/snoak/paramount-plus-top-10-tv-shows/json"},
	"hbomax":      {"https://mdblist.com/lists/snoak/hbo-top-10-movies-2/json", "https://mdblist.com/lists/snoak/hbo-top-10-tv-shows/json"},
	"hulu":        {"https://mdblist.com/lists/snoak/top-hulu-movies/json", "https://mdblist.com/lists/snoak/top-tv-shows-hulu/json"},
	"crunchyroll": {"https://mdblist.com/lists/snoak/trending-anime-movies/json", "https://mdblist.com/lists/snoak/trending-anime-shows/json"},
}

// ExpandStreamingServiceSelection creates one or two renderable MDBList
// shelves. Callers supply a stable instance ID so the generated row IDs remain
// distinct when a service is added more than once.
func ExpandStreamingServiceSelection(selection StreamingServiceSelection) ([]models.ShelfConfig, []FieldError) {
	selection.InstanceID = strings.TrimSpace(selection.InstanceID)
	selection.Service = strings.TrimSpace(selection.Service)
	selection.Media = strings.TrimSpace(selection.Media)
	lists, found := streamingServiceMDBLists[selection.Service]
	if selection.InstanceID == "" {
		return nil, []FieldError{{Section: "rows", Path: "instanceId", Message: "instance id is required"}}
	}
	if !found {
		return nil, []FieldError{{Section: "rows", Path: "service", Message: "service is not supported"}}
	}
	if selection.Media != "movies" && selection.Media != "shows" && selection.Media != "both" {
		return nil, []FieldError{{Section: "rows", Path: "media", Message: "media must be movies, shows, or both"}}
	}
	if selection.Limit < 0 || selection.Limit > 500 {
		return nil, []FieldError{{Section: "rows", Path: "limit", Message: "limit must be between 0 and 500"}}
	}
	name := strings.TrimSpace(selection.Name)
	serviceName := selection.Service
	for _, option := range streamingServiceOptions() {
		if option.Value == selection.Service {
			serviceName = option.Label
			break
		}
	}
	rows := make([]models.ShelfConfig, 0, 2)
	add := func(media, label, listURL string) {
		rowName := name
		if rowName == "" || selection.Media == "both" {
			rowName = serviceName + " " + label
		}
		rows = append(rows, models.ShelfConfig{ID: "streaming-service-" + selection.InstanceID + "-" + media, Name: rowName, Enabled: true, Order: len(rows), Type: "mdblist", ListURL: listURL, Limit: selection.Limit, HideUnreleased: selection.HideUnreleased})
	}
	if selection.Media == "movies" || selection.Media == "both" {
		add("movies", "Movies", lists.movies)
	}
	if selection.Media == "shows" || selection.Media == "both" {
		add("shows", "TV shows", lists.shows)
	}
	return rows, nil
}

func limitField() CatalogField {
	return CatalogField{Path: "limit", Type: "number", Label: "Item limit"}
}

func tmdbSourceTypeOptions() []Option {
	return []Option{{Value: "public-list", Label: "Public list"}, {Value: "production-company", Label: "Production company"}, {Value: "network", Label: "Network"}, {Value: "movie-collection", Label: "Movie collection"}, {Value: "person-credits", Label: "Person credits"}, {Value: "director-credits", Label: "Director credits"}, {Value: "custom-discover", Label: "Custom discover"}}
}

func sortOptions() []Option {
	values := []string{"air-date-asc", "air-date-desc", "recently-watched", "title", "original", "popularity.desc", "popularity.asc", "vote_average.desc", "vote_average.asc", "release_date.desc", "release_date.asc", "title.asc", "title.desc"}
	options := make([]Option, len(values))
	for i, value := range values {
		options[i] = Option{Value: value, Label: value}
	}
	return options
}

func traktAccountOptions(accounts []config.TraktAccount, context CatalogContext) []Option {
	return accountOptions(accounts, "trakt", context, func(account config.TraktAccount) (string, string) {
		return account.ID, account.Name
	})
}

func simklAccountOptions(accounts []config.SimklAccount, context CatalogContext) []Option {
	return accountOptions(accounts, "simkl", context, func(account config.SimklAccount) (string, string) {
		return account.ID, account.Name
	})
}

func accountOptions[T any](accounts []T, provider string, context CatalogContext, details func(T) (id, name string)) []Option {
	authorized := make(map[string]bool, len(context.AuthorizedAccounts))
	for _, account := range context.AuthorizedAccounts {
		if strings.EqualFold(strings.TrimSpace(account.Provider), provider) {
			authorized[strings.TrimSpace(account.AccountID)] = true
		}
	}
	options := make([]Option, 0, len(accounts))
	for _, account := range accounts {
		id, name := details(account)
		id = strings.TrimSpace(id)
		if id == "" || (!context.Actor.IsAdmin && !authorized[id]) {
			continue
		}
		options = append(options, Option{Value: id, Label: strings.TrimSpace(name)})
	}
	return options
}

func libraryOptions(libraries []CatalogLibrary) []Option {
	options := make([]Option, 0, len(libraries))
	for _, library := range libraries {
		if id := strings.TrimSpace(library.ID); id != "" {
			options = append(options, Option{Value: id, Label: strings.TrimSpace(library.Name)})
		}
	}
	return options
}

func setupPath(settings config.Settings, context CatalogContext, surface string) string {
	basePath := strings.TrimSpace(context.BasePath)
	if basePath == "" {
		basePath = settings.Server.BasePath
	}
	basePath = "/" + strings.Trim(strings.TrimSpace(basePath), "/")
	if basePath == "/" {
		basePath = ""
	}
	namespace := "account"
	if context.Actor.IsAdmin {
		namespace = "admin"
	}
	return path.Join(basePath, namespace, surface)
}
