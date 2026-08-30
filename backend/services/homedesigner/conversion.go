package homedesigner

import (
	"novastream/config"
	"novastream/models"
)

// ConfigShelvesToModels converts persisted global shelf definitions into the
// profile-facing representation without dropping source-specific fields.
func ConfigShelvesToModels(shelves []config.ShelfConfig) []models.ShelfConfig {
	if shelves == nil {
		return nil
	}
	converted := make([]models.ShelfConfig, len(shelves))
	for i, shelf := range shelves {
		converted[i] = models.ShelfConfig{
			ID: shelf.ID, Name: shelf.Name, Enabled: shelf.Enabled, Order: shelf.Order, Type: shelf.Type,
			LibraryID: shelf.LibraryID, ListURL: shelf.ListURL,
			AddonManifestURL: shelf.AddonManifestURL, AddonCatalogType: shelf.AddonCatalogType, AddonCatalogID: shelf.AddonCatalogID, AddonName: shelf.AddonName,
			TMDBSourceType: shelf.TMDBSourceType, TMDBSourceID: shelf.TMDBSourceID, TMDBSourceName: shelf.TMDBSourceName, TMDBMediaType: shelf.TMDBMediaType, TMDBDiscoverQuery: shelf.TMDBDiscoverQuery,
			StreamingServices: configStreamingServicesToModels(shelf.StreamingServices), CollectionItems: configCollectionItemsToModels(shelf.CollectionItems),
			TraktAccountID: shelf.TraktAccountID, TraktListType: shelf.TraktListType, TraktListID: shelf.TraktListID,
			SimklAccountID: shelf.SimklAccountID, SimklListType: shelf.SimklListType, SimklMediaType: shelf.SimklMediaType,
			LetterboxdListID: shelf.LetterboxdListID, LetterboxdListURL: shelf.LetterboxdListURL,
			Limit: shelf.Limit, ActivityWindowDays: shelf.ActivityWindowDays, MinimumProfiles: shelf.MinimumProfiles, MaxItemsPerProfile: shelf.MaxItemsPerProfile,
			HideUnreleased: shelf.HideUnreleased, Sort: shelf.Sort, AnimateLogoOnlyOnFocus: shelf.AnimateLogoOnlyOnFocus, ShowCollectionTitles: shelf.ShowCollectionTitles, ShowCollectionCounts: shelf.ShowCollectionCounts,
			CalendarSources: models.CalendarSettings{Watchlist: shelf.CalendarSources.Watchlist, History: shelf.CalendarSources.History, Trending: shelf.CalendarSources.Trending, TopTrending: shelf.CalendarSources.TopTrending, MDBLists: shelf.CalendarSources.MDBLists, MDBListShelves: shelf.CalendarSources.MDBListShelves},
		}
	}
	return converted
}

// ModelShelvesToConfig converts profile-facing shelf definitions to the
// persisted global representation without dropping source-specific fields.
func ModelShelvesToConfig(shelves []models.ShelfConfig) []config.ShelfConfig {
	if shelves == nil {
		return nil
	}
	converted := make([]config.ShelfConfig, len(shelves))
	for i, shelf := range shelves {
		converted[i] = config.ShelfConfig{
			ID: shelf.ID, Name: shelf.Name, Enabled: shelf.Enabled, Order: shelf.Order, Type: shelf.Type,
			LibraryID: shelf.LibraryID, ListURL: shelf.ListURL,
			AddonManifestURL: shelf.AddonManifestURL, AddonCatalogType: shelf.AddonCatalogType, AddonCatalogID: shelf.AddonCatalogID, AddonName: shelf.AddonName,
			TMDBSourceType: shelf.TMDBSourceType, TMDBSourceID: shelf.TMDBSourceID, TMDBSourceName: shelf.TMDBSourceName, TMDBMediaType: shelf.TMDBMediaType, TMDBDiscoverQuery: shelf.TMDBDiscoverQuery,
			StreamingServices: modelStreamingServicesToConfig(shelf.StreamingServices), CollectionItems: modelCollectionItemsToConfig(shelf.CollectionItems),
			TraktAccountID: shelf.TraktAccountID, TraktListType: shelf.TraktListType, TraktListID: shelf.TraktListID,
			SimklAccountID: shelf.SimklAccountID, SimklListType: shelf.SimklListType, SimklMediaType: shelf.SimklMediaType,
			LetterboxdListID: shelf.LetterboxdListID, LetterboxdListURL: shelf.LetterboxdListURL,
			Limit: shelf.Limit, ActivityWindowDays: shelf.ActivityWindowDays, MinimumProfiles: shelf.MinimumProfiles, MaxItemsPerProfile: shelf.MaxItemsPerProfile,
			HideUnreleased: shelf.HideUnreleased, Sort: shelf.Sort, AnimateLogoOnlyOnFocus: shelf.AnimateLogoOnlyOnFocus, ShowCollectionTitles: shelf.ShowCollectionTitles, ShowCollectionCounts: shelf.ShowCollectionCounts,
			CalendarSources: config.CalendarSourceSettings{Watchlist: shelf.CalendarSources.Watchlist, History: shelf.CalendarSources.History, Trending: shelf.CalendarSources.Trending, TopTrending: shelf.CalendarSources.TopTrending, MDBLists: shelf.CalendarSources.MDBLists, MDBListShelves: shelf.CalendarSources.MDBListShelves},
		}
	}
	return converted
}

func ConfigAppearanceToModel(appearance config.AppearanceSettings) models.AppearanceSettings {
	return models.AppearanceSettings{FontScale: appearance.FontScale, AccentColor: appearance.AccentColor, TextColor: appearance.TextColor, SecondaryTextColor: appearance.SecondaryTextColor, BackgroundColor: appearance.BackgroundColor, ModalBackgroundColor: appearance.ModalBackgroundColor, ButtonStyle: appearance.ButtonStyle, ButtonRadius: appearance.ButtonRadius, HighContrast: appearance.HighContrast, ReduceOverlays: appearance.ReduceOverlays}
}

func ModelAppearanceToConfig(appearance models.AppearanceSettings) config.AppearanceSettings {
	return config.AppearanceSettings{FontScale: appearance.FontScale, AccentColor: appearance.AccentColor, TextColor: appearance.TextColor, SecondaryTextColor: appearance.SecondaryTextColor, BackgroundColor: appearance.BackgroundColor, ModalBackgroundColor: appearance.ModalBackgroundColor, ButtonStyle: appearance.ButtonStyle, ButtonRadius: appearance.ButtonRadius, HighContrast: appearance.HighContrast, ReduceOverlays: appearance.ReduceOverlays}
}

func configCollectionItemsToModels(items []config.CollectionHubLink) []models.CollectionHubLink {
	if items == nil {
		return nil
	}
	converted := make([]models.CollectionHubLink, len(items))
	for i, item := range items {
		converted[i] = models.CollectionHubLink{ID: item.ID, Name: item.Name, Enabled: item.Enabled, Order: item.Order, SourceShelfID: item.SourceShelfID, LogoURL: item.LogoURL, HeroArtURL: item.HeroArtURL, LogoScale: item.LogoScale, TintColor: item.TintColor}
	}
	return converted
}

func modelCollectionItemsToConfig(items []models.CollectionHubLink) []config.CollectionHubLink {
	if items == nil {
		return nil
	}
	converted := make([]config.CollectionHubLink, len(items))
	for i, item := range items {
		converted[i] = config.CollectionHubLink{ID: item.ID, Name: item.Name, Enabled: item.Enabled, Order: item.Order, SourceShelfID: item.SourceShelfID, LogoURL: item.LogoURL, HeroArtURL: item.HeroArtURL, LogoScale: item.LogoScale, TintColor: item.TintColor}
	}
	return converted
}

func configStreamingServicesToModels(services []config.StreamingServiceLink) []models.StreamingServiceLink {
	if services == nil {
		return nil
	}
	converted := make([]models.StreamingServiceLink, len(services))
	for i, service := range services {
		converted[i] = models.StreamingServiceLink{ID: service.ID, Name: service.Name, Enabled: service.Enabled, Order: service.Order, LogoURL: service.LogoURL, LogoScale: service.LogoScale, TintColor: service.TintColor}
		if service.Lists != nil {
			converted[i].Lists = make([]models.StreamingServiceListLink, len(service.Lists))
			for j, list := range service.Lists {
				converted[i].Lists[j] = models.StreamingServiceListLink{Key: list.Key, Title: list.Title, URL: list.URL}
			}
		}
	}
	return converted
}

func modelStreamingServicesToConfig(services []models.StreamingServiceLink) []config.StreamingServiceLink {
	if services == nil {
		return nil
	}
	converted := make([]config.StreamingServiceLink, len(services))
	for i, service := range services {
		converted[i] = config.StreamingServiceLink{ID: service.ID, Name: service.Name, Enabled: service.Enabled, Order: service.Order, LogoURL: service.LogoURL, LogoScale: service.LogoScale, TintColor: service.TintColor}
		if service.Lists != nil {
			converted[i].Lists = make([]config.StreamingServiceListLink, len(service.Lists))
			for j, list := range service.Lists {
				converted[i].Lists[j] = config.StreamingServiceListLink{Key: list.Key, Title: list.Title, URL: list.URL}
			}
		}
	}
	return converted
}
