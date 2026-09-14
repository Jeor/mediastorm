package config

import "testing"

func TestLocalSportsNavigationUpgradeAndHide(t *testing.T) {
	raw := map[string]interface{}{"display": map[string]interface{}{"navigationTabVisibility": []interface{}{"home", "live"}}}
	migrateNavigationTabVisibilitySports(raw)
	d := raw["display"].(map[string]interface{})
	tabs := d["navigationTabVisibility"].([]interface{})
	if len(tabs) != 3 || tabs[2] != "sports" {
		t.Fatalf("upgrade: %v", tabs)
	}
	d["navigationTabVisibility"] = []interface{}{"live", "home"}
	migrateNavigationTabVisibilitySports(raw)
	if len(d["navigationTabVisibility"].([]interface{})) != 2 {
		t.Fatal("hidden Sports re-added")
	}
}
