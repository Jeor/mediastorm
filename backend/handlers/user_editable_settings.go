package handlers

import (
	"sort"
	"strings"
)

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
