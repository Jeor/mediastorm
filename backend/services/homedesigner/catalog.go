package homedesigner

import (
	"path"
	"strings"

	"novastream/config"
	"novastream/models"
)

// BuildCatalog preserves the original simple API while defaulting to the
// strictest safe behavior: only integrations explicitly linked to supplied
// profiles are exposed. Use BuildCatalogForContext when actor, libraries, or
// intentionally shared integrations need to be represented.
func BuildCatalog(settings config.Settings, users []models.User) []CatalogEntry {
	return BuildCatalogForContext(settings, CatalogContext{Profiles: users})
}

// BuildCatalogForContext returns templates that the supplied actor and profile
// set may actually configure. It exposes no credentials, source URLs, or
// accounts belonging to another account owner.
func BuildCatalogForContext(settings config.Settings, context CatalogContext) []CatalogEntry {
	profiles := authorizedProfiles(context)
	entries := builtInCatalogEntries()
	traktOptions := traktAccountOptions(settings.Trakt.Accounts, profiles, context)
	simklOptions := simklAccountOptions(settings.Simkl.Accounts, profiles, context)
	libraryOptions := libraryOptions(context.Libraries)

	entries = append(entries,
		repeatableEntry("genre", "Genre", "Browse a movie or TV genre.", "Discovery", models.ShelfConfig{Type: "genre"}, []CatalogField{{Path: "id", Type: "text", Label: "Genre row ID", Required: true}}, "shelf"),
		repeatableEntry("decade", "Decade", "Browse a movie or TV decade.", "Discovery", models.ShelfConfig{Type: "decade"}, []CatalogField{{Path: "id", Type: "text", Label: "Decade row ID", Required: true}}, "shelf"),
		repeatableEntry("mdblist", "MDBList", "Show an MDBList URL.", "Lists", models.ShelfConfig{Type: "mdblist"}, []CatalogField{{Path: "listUrl", Type: "url", Label: "MDBList URL", Required: true}, limitField()}, "shelf"),
		repeatableEntry("stremio", "Stremio add-on", "Show a catalog from a Stremio add-on.", "Lists", models.ShelfConfig{Type: "stremio"}, []CatalogField{{Path: "addonManifestUrl", Type: "url", Label: "Manifest URL", Required: true}, {Path: "addonCatalogType", Type: "select", Label: "Catalog type", Required: true, Options: []Option{{Value: "movie", Label: "Movies"}, {Value: "series", Label: "Series"}}}, {Path: "addonCatalogId", Type: "text", Label: "Catalog", Required: true}}, "shelf"),
		withAvailability(repeatableEntry("tmdb", "TMDB", "Build a shelf from TMDB.", "Discovery", models.ShelfConfig{Type: "tmdb"}, []CatalogField{{Path: "tmdbSourceType", Type: "select", Label: "Source type", Required: true, Options: tmdbSourceTypeOptions()}, {Path: "tmdbSourceId", Type: "text", Label: "Source"}, {Path: "tmdbMediaType", Type: "select", Label: "Content", Required: true, Options: []Option{{Value: "movie", Label: "Movies"}, {Value: "tv", Label: "TV shows"}, {Value: "all", Label: "Movies and TV shows"}}}, {Path: "sort", Type: "select", Label: "Sort", Options: sortOptions()}}, "shelf"), strings.TrimSpace(settings.Metadata.TMDBAPIKey) != "", "TMDB API access is not configured.", setupPath(settings, context, "settings")),
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

func authorizedProfiles(context CatalogContext) []models.User {
	profiles := make([]models.User, 0, len(context.Profiles))
	for _, profile := range context.Profiles {
		if context.Actor.AccountID != "" && !context.Actor.IsAdmin && profile.AccountID != context.Actor.AccountID {
			continue
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func traktAccountOptions(accounts []config.TraktAccount, profiles []models.User, context CatalogContext) []Option {
	return accountOptions(accounts, profiles, context, func(user models.User) string { return user.TraktAccountID }, func(account config.TraktAccount) (string, string, string) {
		return account.ID, account.Name, account.OwnerAccountID
	})
}

func simklAccountOptions(accounts []config.SimklAccount, profiles []models.User, context CatalogContext) []Option {
	return accountOptions(accounts, profiles, context, func(user models.User) string { return user.SimklAccountID }, func(account config.SimklAccount) (string, string, string) {
		return account.ID, account.Name, account.OwnerAccountID
	})
}

func accountOptions[T any](accounts []T, profiles []models.User, context CatalogContext, profileAccountID func(models.User) string, details func(T) (id, name, ownerID string)) []Option {
	linkedNames := make(map[string][]string)
	for _, profile := range profiles {
		if id := strings.TrimSpace(profileAccountID(profile)); id != "" {
			linkedNames[id] = append(linkedNames[id], strings.TrimSpace(profile.Name))
		}
	}
	options := make([]Option, 0, len(accounts))
	for _, account := range accounts {
		id, name, ownerID := details(account)
		id, ownerID = strings.TrimSpace(id), strings.TrimSpace(ownerID)
		if id == "" || !accountAllowed(id, ownerID, linkedNames, context) {
			continue
		}
		if linked := linkedNames[id]; len(linked) > 0 {
			name = strings.Join(linked, ", ") + " · " + name
		}
		options = append(options, Option{Value: id, Label: strings.TrimSpace(name)})
	}
	return options
}

func accountAllowed(id, ownerID string, linkedNames map[string][]string, context CatalogContext) bool {
	if len(linkedNames[id]) > 0 {
		return true
	}
	if context.Actor.AccountID != "" && ownerID == context.Actor.AccountID {
		return true
	}
	return ownerID == "" && context.IncludeSharedAccounts
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
