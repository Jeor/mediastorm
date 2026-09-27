package config

import "regexp"

// HomeViewSettings is an explicit alternate layout. A missing view inherits Home;
// mode "inherit" also lets a profile opt out of a global custom view. Nil shelves
// inherit, whereas an empty array is intentionally empty. View lists never merge.
type HomeViewSettings struct {
	Name        string `json:"name,omitempty"`
	Icon        string `json:"icon,omitempty"`
	MediaFilter string `json:"mediaFilter,omitempty"` // all, movies, shows; independent of page identity
	Deleted     bool   `json:"deleted,omitempty"`     // profile hides an inherited page

	HomeShelfFocusModel             *string        `json:"homeShelfFocusModel,omitempty"`
	ExcludeUpcomingFromContinue     *bool          `json:"excludeUpcomingFromContinue,omitempty"`
	DisableTvLandscapeCardExpansion *bool          `json:"disableTvLandscapeCardExpansion,omitempty"`
	Mode                            string         `json:"mode"`
	Shelves                         *[]ShelfConfig `json:"shelves,omitempty"`
	MobileTopShelfMode              *string        `json:"mobileTopShelfMode,omitempty"`
	MobileTopShelfSourceID          *string        `json:"mobileTopShelfSourceId,omitempty"`
	TVTopShelfMode                  *string        `json:"tvTopShelfMode,omitempty"`
	TVTopShelfSourceID              *string        `json:"tvTopShelfSourceId,omitempty"`
	ExploreCardPosition             *string        `json:"exploreCardPosition,omitempty"`
	ItemCap                         *int           `json:"itemCap,omitempty"`
	HomeShelfScale                  *float64       `json:"homeShelfScale,omitempty"`
	HomeHeroScale                   *float64       `json:"homeHeroScale,omitempty"`
}

// MergeHomeViews resolves profile view definitions without mutating either input.
func MergeHomeViews(global, profile map[string]HomeViewSettings) map[string]HomeViewSettings {
	if len(global) == 0 && len(profile) == 0 {
		return nil
	}
	result := make(map[string]HomeViewSettings)
	for key, v := range global {
		if IsHomeViewID(key) {
			result[key] = v
		}
	}
	for key, v := range profile {
		if !IsHomeViewID(key) {
			continue
		}
		inherited := result[key]
		if v.Name == "" {
			v.Name = inherited.Name
		}
		if v.Icon == "" {
			v.Icon = inherited.Icon
		}
		if v.MediaFilter == "" {
			v.MediaFilter = inherited.MediaFilter
		}
		result[key] = v
	}
	return result
}

var customHomeViewID = regexp.MustCompile(`^page-[a-zA-Z0-9-]{1,80}$`)

func IsHomeViewID(id string) bool {
	return id == "movies" || id == "shows" || customHomeViewID.MatchString(id)
}

// HomeViewFilter resolves media scope independently from layout inheritance.
func HomeViewFilter(views map[string]HomeViewSettings, id string) string {
	if id == "movies" || id == "shows" {
		return id
	}
	if v, ok := views[id]; ok && IsHomeViewID(id) && !v.Deleted {
		if v.MediaFilter == "movies" || v.MediaFilter == "shows" {
			return v.MediaFilter
		}
	}
	return "all"
}
