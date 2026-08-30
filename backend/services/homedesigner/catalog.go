package homedesigner

import (
	"strings"

	"novastream/config"
	"novastream/models"
)

// BuildCatalog returns the supported shelf templates and their current setup
// availability. It deliberately exposes display labels and stable IDs only.
func BuildCatalog(settings config.Settings, users []models.User) []CatalogEntry {
	entries := make([]CatalogEntry, 0, len(config.DefaultHomeShelfConfigs())+11)
	for _, shelf := range config.DefaultHomeShelfConfigs() {
		shelf.Type = "builtin"
		defaultShelf := ConfigShelvesToModels([]config.ShelfConfig{shelf})[0]
		entries = append(entries, CatalogEntry{
			Type: "builtin", Name: shelf.Name, Description: "Built-in home shelf", Category: "Built-in", Available: true,
			Default: defaultShelf, Fields: []CatalogField{{Path: "name", Type: "text", Label: "Name", Required: true}}, PreviewKind: "shelf",
		})
	}

	traktOptions := accountOptions(settings.Trakt.Accounts, users, func(user models.User) string { return user.TraktAccountID })
	simklOptions := accountOptions(settings.Simkl.Accounts, users, func(user models.User) string { return user.SimklAccountID })
	libraryAvailable := len(settings.Plex.Accounts) > 0 || len(settings.Jellyfin.Accounts) > 0

	entries = append(entries,
		repeatableEntry("genre", "Genre", "Browse a movie or TV genre.", "Discovery", models.ShelfConfig{Type: "genre"}, []CatalogField{{Path: "id", Type: "text", Label: "Genre", Required: true}, mediaTypeField("mediaType")}, "shelf"),
		repeatableEntry("decade", "Decade", "Browse a movie or TV decade.", "Discovery", models.ShelfConfig{Type: "decade"}, []CatalogField{{Path: "id", Type: "number", Label: "Decade", Required: true}, mediaTypeField("mediaType")}, "shelf"),
		repeatableEntry("streaming-service", "Streaming service", "Browse titles from a streaming service.", "Discovery", models.ShelfConfig{Type: "streaming-service"}, []CatalogField{{Path: "id", Type: "text", Label: "Service", Required: true}, {Path: "mediaType", Type: "select", Label: "Content", Options: []Option{{Value: "movies", Label: "Movies"}, {Value: "shows", Label: "Shows"}, {Value: "both", Label: "Both"}}}}, "shelf"),
		repeatableEntry("mdblist", "MDBList", "Show an MDBList URL.", "Lists", models.ShelfConfig{Type: "mdblist"}, []CatalogField{{Path: "listUrl", Type: "url", Label: "MDBList URL", Required: true}, limitField()}, "shelf"),
		repeatableEntry("stremio", "Stremio add-on", "Show a catalog from a Stremio add-on.", "Lists", models.ShelfConfig{Type: "stremio"}, []CatalogField{{Path: "addonManifestUrl", Type: "url", Label: "Manifest URL", Required: true}, {Path: "addonCatalogType", Type: "select", Label: "Catalog type", Required: true, Options: []Option{{Value: "movie", Label: "Movies"}, {Value: "series", Label: "Series"}}}, {Path: "addonCatalogId", Type: "text", Label: "Catalog", Required: true}}, "shelf"),
		withAvailability(repeatableEntry("tmdb", "TMDB", "Build a shelf from TMDB.", "Discovery", models.ShelfConfig{Type: "tmdb"}, []CatalogField{{Path: "tmdbSourceType", Type: "select", Label: "Source type", Required: true}, {Path: "tmdbSourceId", Type: "text", Label: "Source", Required: true}, mediaTypeField("tmdbMediaType")}, "shelf"), strings.TrimSpace(settings.Metadata.TMDBAPIKey) != "", "TMDB API access is not configured.", "/admin/settings#metadata"),
		withAvailability(repeatableEntry("trakt", "Trakt", "Show a Trakt watchlist or custom list.", "Lists", models.ShelfConfig{Type: "trakt"}, []CatalogField{{Path: "traktAccountId", Type: "select", Label: "Trakt account", Required: true, Options: traktOptions}, {Path: "traktListType", Type: "select", Label: "List type", Required: true, Options: []Option{{Value: "watchlist", Label: "Watchlist"}, {Value: "custom", Label: "Custom list"}}}, {Path: "traktListId", Type: "text", Label: "Custom list"}}, "shelf"), len(traktOptions) > 0, "Connect a Trakt account before adding this shelf.", "/admin/settings#trakt"),
		withAvailability(repeatableEntry("simkl", "Simkl", "Show a Simkl list.", "Lists", models.ShelfConfig{Type: "simkl"}, []CatalogField{{Path: "simklAccountId", Type: "select", Label: "Simkl account", Required: true, Options: simklOptions}, {Path: "simklMediaType", Type: "select", Label: "Content", Required: true, Options: []Option{{Value: "movies", Label: "Movies"}, {Value: "shows", Label: "Shows"}, {Value: "anime", Label: "Anime"}}}, {Path: "simklListType", Type: "select", Label: "List"}}, "shelf"), len(simklOptions) > 0, "Connect a Simkl account before adding this shelf.", "/admin/settings#simkl"),
		repeatableEntry("letterboxd", "Letterboxd", "Show a public Letterboxd list.", "Lists", models.ShelfConfig{Type: "letterboxd"}, []CatalogField{{Path: "letterboxdListUrl", Type: "url", Label: "Public list URL", Required: true}}, "shelf"),
		repeatableEntry("collection-hub", "Collection hub", "Group shelves into a collection hub.", "Organization", models.ShelfConfig{Type: "collection-hub"}, []CatalogField{{Path: "collectionItems", Type: "collection", Label: "Collections"}}, "hub"),
		withAvailability(repeatableEntry("library", "Media library", "Show a configured media library.", "Libraries", models.ShelfConfig{Type: "library"}, []CatalogField{{Path: "libraryId", Type: "text", Label: "Media library", Required: true}}, "shelf"), libraryAvailable, "Configure a Plex or Jellyfin library before adding this shelf.", "/admin/libraries"),
	)
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

func mediaTypeField(path string) CatalogField {
	return CatalogField{Path: path, Type: "select", Label: "Content", Required: true, Options: []Option{{Value: "movie", Label: "Movies"}, {Value: "tv", Label: "TV shows"}}}
}

func limitField() CatalogField {
	return CatalogField{Path: "limit", Type: "number", Label: "Item limit"}
}

func accountOptions[T interface{}](accounts []T, users []models.User, accountID func(models.User) string) []Option {
	labels := make(map[string][]string)
	for _, user := range users {
		if id := strings.TrimSpace(accountID(user)); id != "" {
			labels[id] = append(labels[id], strings.TrimSpace(user.Name))
		}
	}
	options := make([]Option, 0, len(accounts))
	for _, account := range accounts {
		id, name := accountLabel(account)
		if id == "" {
			continue
		}
		if linked := labels[id]; len(linked) > 0 {
			name = strings.Join(linked, ", ") + " · " + name
		}
		options = append(options, Option{Value: id, Label: name})
	}
	return options
}

func accountLabel[T interface{}](account T) (string, string) {
	switch value := any(account).(type) {
	case config.TraktAccount:
		return value.ID, value.Name
	case config.SimklAccount:
		return value.ID, value.Name
	default:
		return "", ""
	}
}
