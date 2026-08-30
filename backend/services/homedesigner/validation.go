package homedesigner

import (
	"regexp"
	"sort"
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
	knownTypes, builtinIDs := catalogTypes(catalog)
	seenBuiltins := make(map[string]struct{})
	for i := range rows.Shelves {
		shelf := &rows.Shelves[i]
		rowID := shelf.ID
		if shelf.Name == "" {
			errs = append(errs, rowError(rowID, "name", "name is required"))
		}
		if rowID == "" {
			errs = append(errs, rowError(rowID, "id", "id is required"))
		}
		if shelf.Type == "" && builtinIDs[rowID] {
			shelf.Type = "builtin"
		}
		if !knownTypes[shelf.Type] {
			errs = append(errs, rowError(rowID, "type", "unknown shelf type"))
			continue
		}
		if shelf.Type == "builtin" {
			if !builtinIDs[rowID] {
				errs = append(errs, rowError(rowID, "id", "unknown built-in shelf"))
			} else if _, exists := seenBuiltins[rowID]; exists {
				errs = append(errs, rowError(rowID, "id", "built-in shelf may only be added once"))
			} else {
				seenBuiltins[rowID] = struct{}{}
			}
		}
		errs = append(errs, validateShelf(rowID, *shelf)...)
	}
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

func catalogTypes(catalog []CatalogEntry) (map[string]bool, map[string]bool) {
	types, builtins := make(map[string]bool), make(map[string]bool)
	for _, entry := range catalog {
		types[entry.Type] = true
		if entry.Type == "builtin" {
			builtins[entry.Default.ID] = true
		}
	}
	return types, builtins
}

func validateShelf(rowID string, shelf models.ShelfConfig) []FieldError {
	errs := make([]FieldError, 0)
	require := func(path, value string) {
		if value == "" {
			errs = append(errs, rowError(rowID, path, "field is required"))
		}
	}
	if shelf.Limit < 0 || shelf.Limit > 500 {
		errs = append(errs, rowError(rowID, "limit", "limit must be between 0 and 500"))
	}
	switch shelf.Type {
	case "mdblist":
		require("listUrl", shelf.ListURL)
	case "stremio":
		require("addonManifestUrl", shelf.AddonManifestURL)
		require("addonCatalogType", shelf.AddonCatalogType)
		require("addonCatalogId", shelf.AddonCatalogID)
	case "tmdb":
		require("tmdbSourceType", shelf.TMDBSourceType)
		if shelf.TMDBSourceType != "custom-discover" {
			require("tmdbSourceId", shelf.TMDBSourceID)
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
	case "library":
		require("libraryId", shelf.LibraryID)
	}
	return errs
}

func rowError(rowID, path, message string) FieldError {
	return FieldError{Section: "rows", RowID: rowID, Path: path, Message: message}
}
