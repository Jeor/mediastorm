package models

import "encoding/json"

// ResolveHomeView applies explicit view fields to an already resolved Home layout.
// User/device restrictions are applied by callers independently of this layout.
func ResolveHomeView(home HomeShelvesSettings, view string) HomeShelvesSettings {
	override, ok := home.Views[view]
	if !ok || override.Mode != "custom" || (view != "movies" && view != "shows") {
		return home
	}
	// The shared config view uses the same JSON shelf schema as profile settings.
	base, _ := json.Marshal(home)
	var resolved HomeShelvesSettings
	_ = json.Unmarshal(base, &resolved)
	data, _ := json.Marshal(override)
	_ = json.Unmarshal(data, &resolved)
	if override.Shelves != nil && len(*override.Shelves) == 0 {
		if override.MobileTopShelfMode == nil {
			resolved.MobileTopShelfMode = "disabled"
		}
		if override.TVTopShelfMode == nil {
			resolved.TVTopShelfMode = "disabled"
		}
	}
	return resolved
}
