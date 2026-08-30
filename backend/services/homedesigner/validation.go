package homedesigner

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"novastream/models"
)

var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidateApply normalizes safe shelf edits and returns structured errors for
// values that cannot be represented by the existing persisted settings.
func ValidateApply(request ApplyRequest, catalog []CatalogEntry) []FieldError {
	errs := make([]FieldError, 0)
	switch request.Scope.Kind {
	case "global", "profile":
	default:
		errs = append(errs, FieldError{Section: "scope", Path: "kind", Message: "scope kind must be global or profile"})
	}
	if request.Scope.Kind == "profile" && strings.TrimSpace(request.Scope.ProfileID) == "" {
		errs = append(errs, FieldError{Section: "scope", Path: "profileId", Message: "profile scope requires a profile id"})
	}

	if request.Rows != nil {
		errs = append(errs, validateRowsMutation(request.Scope, request.Rows, catalog)...)
	}
	if request.Theme != nil {
		errs = append(errs, validateThemeMutation(request.Scope, request.Theme)...)
	}
	return errs
}

func validateRowsMutation(scope Scope, mutation *SectionMutation[models.HomeShelvesSettings], catalog []CatalogEntry) []FieldError {
	errs := validateMutation("rows", scope, mutation.Mode, mutation.Value != nil)
	if mutation.Mode != ModeCustom || mutation.Value == nil {
		return errs
	}

	rows := mutation.Value
	normalizeRows(rows)
	entriesByType, builtinIDs := catalogTypes(catalog)
	rowIDs := make(map[string]bool, len(rows.Shelves))
	for _, shelf := range rows.Shelves {
		if shelf.ID != "" {
			rowIDs[shelf.ID] = true
		}
	}
	seenRowIDs := make(map[string]struct{}, len(rows.Shelves))
	for i := range rows.Shelves {
		shelf := &rows.Shelves[i]
		rowID := shelf.ID
		if shelf.Name == "" {
			errs = append(errs, rowError(rowID, "name", "name is required"))
		}
		if rowID == "" {
			errs = append(errs, rowError(rowID, "id", "id is required"))
		} else if _, exists := seenRowIDs[rowID]; exists {
			errs = append(errs, rowError(rowID, "id", "row id must be unique"))
		} else {
			seenRowIDs[rowID] = struct{}{}
		}
		if shelf.Type == "" && builtinIDs[rowID] {
			shelf.Type = "builtin"
		}
		entry, known := entriesByType[shelf.Type]
		if !known {
			errs = append(errs, rowError(rowID, "type", "unknown shelf type"))
			continue
		}
		if !entry.Available {
			errs = append(errs, rowError(rowID, "type", "shelf type is not available"))
			continue
		}
		if shelf.Type == "builtin" {
			if !builtinIDs[rowID] {
				errs = append(errs, rowError(rowID, "id", "unknown built-in shelf"))
			}
		}
		errs = append(errs, validateShelf(rowID, *shelf, entry)...)
		if shelf.Type == "collection-hub" {
			errs = append(errs, validateCollectionItems(rowID, shelf.CollectionItems, rowIDs)...)
		}
	}
	errs = append(errs, validateRowsSettings(*rows)...)
	return errs
}

func validateThemeMutation(scope Scope, mutation *SectionMutation[models.AppearanceSettings]) []FieldError {
	errs := validateMutation("theme", scope, mutation.Mode, mutation.Value != nil)
	if mutation.Mode != ModeCustom || mutation.Value == nil {
		return errs
	}
	theme := mutation.Value
	theme.AccentColor = strings.TrimSpace(theme.AccentColor)
	theme.TextColor = strings.TrimSpace(theme.TextColor)
	theme.SecondaryTextColor = strings.TrimSpace(theme.SecondaryTextColor)
	theme.BackgroundColor = strings.TrimSpace(theme.BackgroundColor)
	theme.ModalBackgroundColor = strings.TrimSpace(theme.ModalBackgroundColor)
	theme.ButtonStyle = strings.TrimSpace(theme.ButtonStyle)
	theme.ButtonRadius = strings.TrimSpace(theme.ButtonRadius)
	for _, color := range []struct{ path, value string }{
		{"accentColor", theme.AccentColor}, {"textColor", theme.TextColor}, {"secondaryTextColor", theme.SecondaryTextColor}, {"backgroundColor", theme.BackgroundColor}, {"modalBackgroundColor", theme.ModalBackgroundColor},
	} {
		if color.value != "" && !hexColorPattern.MatchString(color.value) {
			errs = append(errs, FieldError{Section: "theme", Path: color.path, Message: "color must use #RRGGBB"})
		}
	}
	if theme.FontScale != nil && (*theme.FontScale < 0.1 || *theme.FontScale > 2.0) {
		errs = append(errs, FieldError{Section: "theme", Path: "fontScale", Message: "font scale must be between 0.1 and 2.0"})
	}
	if theme.ButtonStyle != "" && theme.ButtonStyle != "soft" && theme.ButtonStyle != "outlined" && theme.ButtonStyle != "filled" {
		errs = append(errs, FieldError{Section: "theme", Path: "buttonStyle", Message: "button style must be soft, outlined, or filled"})
	}
	if theme.ButtonRadius != "" && theme.ButtonRadius != "square" && theme.ButtonRadius != "rounded" && theme.ButtonRadius != "pill" {
		errs = append(errs, FieldError{Section: "theme", Path: "buttonRadius", Message: "button radius must be square, rounded, or pill"})
	}
	return errs
}

func validateMutation(section string, scope Scope, mode string, hasValue bool) []FieldError {
	if mode != ModeCustom && mode != ModeInherit {
		return []FieldError{{Section: section, Path: "mode", Message: "mode must be custom or inherit"}}
	}
	if scope.Kind == "global" && mode != ModeCustom {
		return []FieldError{{Section: section, Path: "mode", Message: "global settings only support custom mode"}}
	}
	if mode == ModeCustom && !hasValue {
		return []FieldError{{Section: section, Path: "value", Message: "custom mode requires a value"}}
	}
	if mode == ModeInherit && hasValue {
		return []FieldError{{Section: section, Path: "value", Message: "inherit mode cannot include a value"}}
	}
	return nil
}

func normalizeRows(rows *models.HomeShelvesSettings) {
	for i := range rows.Shelves {
		shelf := &rows.Shelves[i]
		shelf.ID = strings.TrimSpace(shelf.ID)
		shelf.Name = strings.TrimSpace(shelf.Name)
		shelf.Type = strings.ToLower(strings.TrimSpace(shelf.Type))
		shelf.LibraryID = strings.TrimSpace(shelf.LibraryID)
		shelf.ListURL = strings.TrimSpace(shelf.ListURL)
		shelf.AddonManifestURL = strings.TrimSpace(shelf.AddonManifestURL)
		shelf.AddonCatalogType = strings.TrimSpace(shelf.AddonCatalogType)
		shelf.AddonCatalogID = strings.TrimSpace(shelf.AddonCatalogID)
		shelf.AddonName = strings.TrimSpace(shelf.AddonName)
		shelf.TMDBSourceType = strings.TrimSpace(shelf.TMDBSourceType)
		shelf.TMDBSourceID = strings.TrimSpace(shelf.TMDBSourceID)
		shelf.TMDBSourceName = strings.TrimSpace(shelf.TMDBSourceName)
		shelf.TMDBMediaType = strings.TrimSpace(shelf.TMDBMediaType)
		shelf.TraktAccountID = strings.TrimSpace(shelf.TraktAccountID)
		shelf.TraktListType = strings.TrimSpace(shelf.TraktListType)
		shelf.TraktListID = strings.TrimSpace(shelf.TraktListID)
		shelf.SimklAccountID = strings.TrimSpace(shelf.SimklAccountID)
		shelf.SimklListType = strings.TrimSpace(shelf.SimklListType)
		shelf.SimklMediaType = strings.TrimSpace(shelf.SimklMediaType)
		shelf.LetterboxdListID = strings.TrimSpace(shelf.LetterboxdListID)
		shelf.LetterboxdListURL = strings.TrimSpace(shelf.LetterboxdListURL)
		normalizeCollectionItems(shelf.CollectionItems)
	}
	sort.SliceStable(rows.Shelves, func(i, j int) bool { return rows.Shelves[i].Order < rows.Shelves[j].Order })
	for i := range rows.Shelves {
		rows.Shelves[i].Order = i
	}
	rows.HomeShelfScale = clampHomeScale(rows.HomeShelfScale)
	rows.HomeHeroScale = clampHomeScale(rows.HomeHeroScale)
}

func clampHomeScale(value *float64) *float64 {
	if value == nil {
		return nil
	}
	next := *value
	if next <= 0 {
		next = 1.0
	} else if next < 0.5 {
		next = 0.5
	} else if next > 1.0 {
		next = 1.0
	}
	return &next
}

func catalogTypes(catalog []CatalogEntry) (map[string]CatalogEntry, map[string]bool) {
	types, builtins := make(map[string]CatalogEntry), make(map[string]bool)
	for _, entry := range catalog {
		if entry.CatalogOnly {
			continue
		}
		if _, exists := types[entry.Type]; !exists {
			types[entry.Type] = entry
		}
		if entry.Type == "builtin" {
			builtins[entry.Default.ID] = true
		}
	}
	return types, builtins
}

func normalizeCollectionItems(items []models.CollectionHubLink) {
	for i := range items {
		item := &items[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.SourceShelfID = strings.TrimSpace(item.SourceShelfID)
		item.LogoURL = strings.TrimSpace(item.LogoURL)
		item.HeroArtURL = strings.TrimSpace(item.HeroArtURL)
		item.TintColor = strings.TrimSpace(item.TintColor)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Order < items[j].Order })
	for i := range items {
		items[i].Order = i
	}
}

func validateCollectionItems(rowID string, items []models.CollectionHubLink, rowIDs map[string]bool) []FieldError {
	errs := make([]FieldError, 0)
	ids, names, sources := make(map[string]bool), make(map[string]bool), make(map[string]bool)
	for _, item := range items {
		itemID := item.ID
		if itemID == "" {
			errs = append(errs, collectionItemError(rowID, itemID, "id", "item id is required"))
		} else if ids[itemID] {
			errs = append(errs, collectionItemError(rowID, itemID, "id", "item id must be unique"))
		} else {
			ids[itemID] = true
		}
		if item.Name == "" {
			errs = append(errs, collectionItemError(rowID, itemID, "name", "item name is required"))
		} else if names[item.Name] {
			errs = append(errs, collectionItemError(rowID, itemID, "name", "item name must be unique"))
		} else {
			names[item.Name] = true
		}
		if item.SourceShelfID == "" || !rowIDs[item.SourceShelfID] || item.SourceShelfID == rowID {
			errs = append(errs, collectionItemError(rowID, itemID, "sourceShelfId", "source must reference another configured shelf"))
		} else if sources[item.SourceShelfID] {
			errs = append(errs, collectionItemError(rowID, itemID, "sourceShelfId", "source shelf may only be used once"))
		} else {
			sources[item.SourceShelfID] = true
		}
		if item.LogoURL != "" && !isHTTPURL(item.LogoURL) {
			errs = append(errs, collectionItemError(rowID, itemID, "logoUrl", "must be an http or https URL"))
		}
		if item.HeroArtURL != "" && !isHTTPURL(item.HeroArtURL) {
			errs = append(errs, collectionItemError(rowID, itemID, "heroArtUrl", "must be an http or https URL"))
		}
		if item.LogoScale != 0 && (item.LogoScale < 0.5 || item.LogoScale > 2.0) {
			errs = append(errs, collectionItemError(rowID, itemID, "logoScale", "logo scale must be between 0.5 and 2.0"))
		}
		if item.TintColor != "" && !hexColorPattern.MatchString(item.TintColor) {
			errs = append(errs, collectionItemError(rowID, itemID, "tintColor", "color must use #RRGGBB"))
		}
	}
	return errs
}

func validateShelf(rowID string, shelf models.ShelfConfig, entry CatalogEntry) []FieldError {
	errs := make([]FieldError, 0)
	require := func(path, value string) {
		if value == "" {
			errs = append(errs, rowError(rowID, path, "field is required"))
		}
	}
	if shelf.Limit < 0 || shelf.Limit > 500 {
		errs = append(errs, rowError(rowID, "limit", "limit must be between 0 and 500"))
	}
	for _, field := range entry.Fields {
		value := shelfFieldValue(shelf, field.Path)
		if field.Required && value == "" {
			errs = append(errs, rowError(rowID, field.Path, "field is required"))
		}
		if value != "" && len(field.Options) > 0 && !isCatalogOption(field.Options, value) {
			errs = append(errs, rowError(rowID, field.Path, "value is not supported"))
		}
		if value != "" && field.Type == "url" && !isHTTPURL(value) {
			errs = append(errs, rowError(rowID, field.Path, "must be an http or https URL"))
		}
	}
	switch shelf.Type {
	case "genre":
		if !validGenreShelfID(shelf.ID) {
			errs = append(errs, rowError(rowID, "id", "genre id must use genre-<positive id>-<movie|tv>"))
		}
	case "decade":
		if !validDecadeShelfID(shelf.ID) {
			errs = append(errs, rowError(rowID, "id", "decade id must use decade-<year>-<movie|tv>"))
		}
	case "tmdb":
		if shelf.TMDBSourceType != "custom-discover" && !validTMDBSourceID(shelf.TMDBSourceID) {
			errs = append(errs, rowError(rowID, "tmdbSourceId", "source id must be a positive integer or a URL containing one"))
		}
		if !validTMDBDiscoverQuery(shelf.TMDBDiscoverQuery) {
			errs = append(errs, rowError(rowID, "tmdbDiscoverQuery", "discover query is invalid"))
		}
	case "trakt":
		require("traktAccountId", shelf.TraktAccountID)
		require("traktListType", shelf.TraktListType)
		if shelf.TraktListType == "custom" {
			require("traktListId", shelf.TraktListID)
		}
	case "simkl":
		require("simklAccountId", shelf.SimklAccountID)
		require("simklMediaType", shelf.SimklMediaType)
	case "letterboxd":
		if shelf.LetterboxdListID == "" && shelf.LetterboxdListURL == "" {
			errs = append(errs, rowError(rowID, "letterboxdListUrl", "a Letterboxd list is required"))
		}
	}
	return errs
}

func shelfFieldValue(shelf models.ShelfConfig, path string) string {
	switch path {
	case "id":
		return shelf.ID
	case "name":
		return shelf.Name
	case "listUrl":
		return shelf.ListURL
	case "addonManifestUrl":
		return shelf.AddonManifestURL
	case "addonCatalogType":
		return shelf.AddonCatalogType
	case "addonCatalogId":
		return shelf.AddonCatalogID
	case "tmdbSourceType":
		return shelf.TMDBSourceType
	case "tmdbSourceId":
		return shelf.TMDBSourceID
	case "tmdbMediaType":
		return shelf.TMDBMediaType
	case "sort":
		return shelf.Sort
	case "traktAccountId":
		return shelf.TraktAccountID
	case "traktListType":
		return shelf.TraktListType
	case "traktListId":
		return shelf.TraktListID
	case "simklAccountId":
		return shelf.SimklAccountID
	case "simklMediaType":
		return shelf.SimklMediaType
	case "simklListType":
		return shelf.SimklListType
	case "letterboxdListId":
		return shelf.LetterboxdListID
	case "letterboxdListUrl":
		return shelf.LetterboxdListURL
	case "libraryId":
		return shelf.LibraryID
	default:
		return ""
	}
}

func isCatalogOption(options []Option, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func positiveInteger(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0
}

func validTMDBSourceID(value string) bool {
	if positiveInteger(value) {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	for _, part := range strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' || r == '-' }) {
		if positiveInteger(part) {
			return true
		}
	}
	return false
}

func validTMDBDiscoverQuery(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	if index := strings.Index(value, "?"); index >= 0 {
		value = value[index+1:]
	}
	_, err := url.ParseQuery(strings.TrimPrefix(value, "?"))
	return err == nil
}

func validGenreShelfID(value string) bool {
	parts := strings.Split(value, "-")
	return len(parts) == 3 && parts[0] == "genre" && positiveInteger(parts[1]) && (parts[2] == "movie" || parts[2] == "tv")
}

func validDecadeShelfID(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) != 3 || parts[0] != "decade" || (parts[2] != "movie" && parts[2] != "tv") {
		return false
	}
	decade, err := strconv.Atoi(parts[1])
	return err == nil && decade >= 1800
}

func validateRowsSettings(rows models.HomeShelvesSettings) []FieldError {
	errs := make([]FieldError, 0)
	if rows.ExploreCardPosition != "" && rows.ExploreCardPosition != "front" && rows.ExploreCardPosition != "end" {
		errs = append(errs, FieldError{Section: "rows", Path: "exploreCardPosition", Message: "explore card position must be front or end"})
	}
	if rows.ItemCap != 0 && (rows.ItemCap < 1 || rows.ItemCap > 100) {
		errs = append(errs, FieldError{Section: "rows", Path: "itemCap", Message: "item cap must be between 1 and 100"})
	}
	ids := make(map[string]bool, len(rows.Shelves))
	for _, shelf := range rows.Shelves {
		ids[shelf.ID] = true
	}
	errs = append(errs, validateTopShelfMode("mobileTopShelfMode", "mobileTopShelfSourceId", rows.MobileTopShelfMode, rows.MobileTopShelfSourceID, ids)...)
	errs = append(errs, validateTopShelfMode("tvTopShelfMode", "tvTopShelfSourceId", rows.TVTopShelfMode, rows.TVTopShelfSourceID, ids)...)
	return errs
}

func validateTopShelfMode(modePath, sourcePath, mode, source string, ids map[string]bool) []FieldError {
	if mode == "" || mode == "default" || mode == "disabled" {
		if source != "" {
			return []FieldError{{Section: "rows", Path: sourcePath, Message: "source is only valid when mode is shelf"}}
		}
		return nil
	}
	if mode != "shelf" {
		return []FieldError{{Section: "rows", Path: modePath, Message: "mode must be default, disabled, or shelf"}}
	}
	if source == "" || !ids[source] {
		return []FieldError{{Section: "rows", Path: sourcePath, Message: "source must reference a configured shelf"}}
	}
	return nil
}

func rowError(rowID, path, message string) FieldError {
	return FieldError{Section: "rows", RowID: rowID, Path: path, Message: message}
}

func collectionItemError(rowID, itemID, path, message string) FieldError {
	return FieldError{Section: "rows", RowID: rowID, ItemID: itemID, Path: path, Message: message}
}
