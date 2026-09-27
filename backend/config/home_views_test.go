package config

import "testing"

func TestMergeHomeViews(t *testing.T) {
	global := map[string]HomeViewSettings{"movies": {Mode: "custom"}, "shows": {Mode: "custom"}}
	profile := map[string]HomeViewSettings{"movies": {Mode: "inherit"}}
	merged := MergeHomeViews(global, profile)
	if merged["movies"].Mode != "inherit" || merged["shows"].Mode != "custom" {
		t.Fatal(merged)
	}
	merged["movies"] = HomeViewSettings{Mode: "custom"}
	if profile["movies"].Mode != "inherit" {
		t.Fatal("mutated profile")
	}
}
