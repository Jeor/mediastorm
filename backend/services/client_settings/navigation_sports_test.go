package client_settings

import (
	"novastream/models"
	"testing"
)

func TestLocalSportsNavigationClientUpgradeAndHide(t *testing.T) {
	tabs := []string{"home", "live"}
	yes := true
	values := map[string]models.ClientFilterSettings{"qa": {NavigationTabVisibility: &tabs, NavigationTabVisibilityIncludesWatchlist: &yes, NavigationTabVisibilityIncludesSystemTabs: &yes}}
	normalizeNavigationTabVisibility(values)
	value := values["qa"]
	if len(*value.NavigationTabVisibility) != 3 {
		t.Fatal("Sports not migrated")
	}
	hidden := []string{"live", "home"}
	value.NavigationTabVisibility = &hidden
	values["qa"] = value
	normalizeNavigationTabVisibility(values)
	if len(*values["qa"].NavigationTabVisibility) != 2 {
		t.Fatal("hidden Sports re-added")
	}
}
