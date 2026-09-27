package config

// HomeViewSettings is an explicit alternate layout. A missing view inherits Home;
// mode "inherit" also lets a profile opt out of a global custom view. Nil shelves
// inherit, whereas an empty array is intentionally empty. View lists never merge.
type HomeViewSettings struct {
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
	for _, key := range []string{"movies", "shows"} {
		if v, ok := global[key]; ok {
			result[key] = v
		}
		if v, ok := profile[key]; ok {
			result[key] = v
		}
	}
	return result
}
