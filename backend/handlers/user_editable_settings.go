package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// UserEditableSettingSchema is the safe subset of the admin schema needed to
// render native profile/device editors. It deliberately excludes conditional
// admin UI behavior and any server-only metadata.
type UserEditableSettingSchema struct {
	Type        string      `json:"type"`
	Label       string      `json:"label"`
	Description string      `json:"description,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Options     interface{} `json:"options,omitempty"`
	Min         interface{} `json:"min,omitempty"`
	Max         interface{} `json:"max,omitempty"`
	Step        interface{} `json:"step,omitempty"`
}

// userEditableFields are scalar settings fields that have matching profile-level
// overrides and a frontend mirror. Array editors and server-only integration
// settings remain excluded until their client editing semantics are defined.
var userEditableFields = map[string]map[string]struct{}{
	"filtering": fieldSet(
		"maxSizeMovieGb", "maxSizeEpisodeGb", "maxResolution", "filterOutTerms",
		"preferredTerms", "splitByService",
	),
	"ranking": fieldSet("newestReleaseFirst", "splitByService"),
	"playback": fieldSet(
		"preferredPlayer", "preferredAudioLanguage", "preferredSubtitleLanguage", "allowedTrackLanguages",
		"preferredSubtitleMode", "pauseWhenAppInactive", "subtitleSize", "subtitleUseCropDetectPosition",
		"subtitleColor", "subtitleOpacity", "subtitleFont", "subtitleBold", "subtitleOutlineEnabled",
		"subtitleOutlineColor", "subtitleOutlineWeight", "subtitleBackgroundEnabled", "subtitleBackgroundColor",
		"subtitleBackgroundOpacity", "seekForwardSeconds", "seekBackwardSeconds", "forceAacTranscoding",
		"autoPlayTrailersTV", "rewindOnResumeFromPause", "rewindOnPlaybackStart", "disablePrequeue",
		"ignoreDolbyVisionCompatibilityCheck", "streamMigrationEnabled", "creditsDetectionEnabled",
		"creditsAutoSkip", "matchFrameRate", "liveClosedCaptionExtraction", "maxResultsPerResolution",
	),
	"homeShelves": nil, // All scalar fields in this section are profile-compatible.
	"display": fieldSet(
		"navigationTabVisibility", "includeUnreleasedMoviesInLists", "includeUnreleasedShowsInLists",
		"includeUnreleasedMoviesInSearch", "includeUnreleasedShowsInSearch", "enableAnimations",
		"enableHeroArtPanning", "enableHeroArtRotation", "hideContinueWatchingHeroMetadata",
		"moveDetailsRatingsToMetadata", "hideDetailsPoster", "hideTvDrawerRail",
	),
	"network": nil,
}

func fieldSet(paths ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		result[path] = struct{}{}
	}
	return result
}

func init() {
	annotateUserEditableSettingsSchema()
}

func annotateUserEditableSettingsSchema() {
	for sectionKey, rawSection := range SettingsSchema {
		allowedFields, ok := userEditableFields[sectionKey]
		if !ok {
			continue
		}
		section, ok := rawSection.(map[string]interface{})
		if !ok {
			continue
		}
		fields, ok := section["fields"].(map[string]interface{})
		if !ok {
			continue
		}
		for fieldKey, rawField := range fields {
			if allowedFields != nil {
				if _, allowed := allowedFields[fieldKey]; !allowed && !(sectionKey == "display" && strings.HasPrefix(fieldKey, "appearance.")) {
					continue
				}
			}
			field, ok := rawField.(map[string]interface{})
			if !ok || field["hidden"] == true || field["globalOnly"] == true {
				continue
			}
			fieldType, _ := field["type"].(string)
			if fieldType == "password" || fieldType == "file_upload" {
				continue
			}
			field["userEditableEligible"] = true
			field["userEditablePath"] = sectionKey + "." + fieldKey
		}
	}
}

func eligibleUserEditableSettings() map[string]struct{} {
	eligible := make(map[string]struct{})
	for _, rawSection := range SettingsSchema {
		section, ok := rawSection.(map[string]interface{})
		if !ok {
			continue
		}
		fields, ok := section["fields"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, rawField := range fields {
			field, ok := rawField.(map[string]interface{})
			if !ok || field["userEditableEligible"] != true {
				continue
			}
			path, _ := field["userEditablePath"].(string)
			if path != "" {
				eligible[path] = struct{}{}
			}
		}
	}
	return eligible
}

func filterUserEditableSettings(paths []string) []string {
	eligible := eligibleUserEditableSettings()
	seen := make(map[string]struct{}, len(paths))
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := eligible[path]; !ok {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		filtered = append(filtered, path)
	}
	sort.Strings(filtered)
	return filtered
}

func userEditableSettingsSchema(paths []string) map[string]UserEditableSettingSchema {
	result := make(map[string]UserEditableSettingSchema)
	for _, path := range filterUserEditableSettings(paths) {
		sectionKey, fieldKey, ok := strings.Cut(path, ".")
		if !ok {
			continue
		}
		section, _ := SettingsSchema[sectionKey].(map[string]interface{})
		fields, _ := section["fields"].(map[string]interface{})
		field, _ := fields[fieldKey].(map[string]interface{})
		if field == nil {
			continue
		}
		result[path] = UserEditableSettingSchema{
			Type:        stringValue(field["type"]),
			Label:       stringValue(field["label"]),
			Description: stringValue(field["description"]),
			Placeholder: stringValue(field["placeholder"]),
			Options:     field["options"],
			Min:         field["min"],
			Max:         field["max"],
			Step:        field["step"],
		}
	}
	return result
}

func stringValue(value interface{}) string {
	result, _ := value.(string)
	return result
}

func validateUserEditableSettingValue(path string, raw json.RawMessage) error {
	schema, ok := userEditableSettingsSchema([]string{path})[path]
	if !ok {
		return fmt.Errorf("setting %q is not eligible for frontend editing", path)
	}
	switch schema.Type {
	case "boolean":
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s must be true or false", schema.Label)
		}
	case "number":
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s must be a number", schema.Label)
		}
		if min, ok := numericSchemaValue(schema.Min); ok && value < min {
			return fmt.Errorf("%s must be at least %v", schema.Label, min)
		}
		if max, ok := numericSchemaValue(schema.Max); ok && value > max {
			return fmt.Errorf("%s must be at most %v", schema.Label, max)
		}
	case "tags", "checkboxes", "multiselect":
		var value []string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s must be a list of text values", schema.Label)
		}
		if schema.Type != "tags" && schema.Options != nil {
			for _, selected := range value {
				if !editableOptionAllowed(schema.Options, selected) {
					return fmt.Errorf("%s contains an unsupported value", schema.Label)
				}
			}
		}
	case "select":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s must be text", schema.Label)
		}
		if !editableOptionAllowed(schema.Options, value) {
			return fmt.Errorf("%s has an unsupported value", schema.Label)
		}
	default:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s must be text", schema.Label)
		}
	}
	return nil
}

func numericSchemaValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func editableOptionAllowed(options interface{}, value string) bool {
	if options == nil {
		return true
	}
	switch typed := options.(type) {
	case []string:
		for _, option := range typed {
			if option == value {
				return true
			}
		}
	case []map[string]string:
		for _, option := range typed {
			if option["value"] == value {
				return true
			}
		}
	case []interface{}:
		for _, rawOption := range typed {
			switch option := rawOption.(type) {
			case string:
				if option == value {
					return true
				}
			case map[string]interface{}:
				if stringValue(option["value"]) == value {
					return true
				}
			}
		}
	}
	return false
}
