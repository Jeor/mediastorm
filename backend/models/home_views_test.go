package models

import (
	"novastream/config"
	"testing"
)

func TestResolveHomeViewInheritanceAndReplacement(t *testing.T) {
	shelves := []config.ShelfConfig{{ID: "custom", Name: "Custom", Enabled: true}}
	home := HomeShelvesSettings{Shelves: []ShelfConfig{{ID: "watchlist", Enabled: true}}, ItemCap: 20, Views: map[string]config.HomeViewSettings{"movies": {Mode: "custom", Shelves: &shelves}}}
	resolved := ResolveHomeView(home, "movies")
	if len(resolved.Shelves) != 1 || resolved.Shelves[0].ID != "custom" || resolved.ItemCap != 20 {
		t.Fatalf("unexpected layout: %+v", resolved)
	}
	if home.Shelves[0].ID != "watchlist" {
		t.Fatal("mutated Home")
	}
	empty := []config.ShelfConfig{}
	home.Views["movies"] = config.HomeViewSettings{Mode: "custom", Shelves: &empty}
	if len(ResolveHomeView(home, "movies").Shelves) != 0 {
		t.Fatal("empty custom layout must not inherit")
	}
	home.Views["movies"] = config.HomeViewSettings{Mode: "inherit"}
	if ResolveHomeView(home, "movies").Shelves[0].ID != "watchlist" {
		t.Fatal("explicit inheritance failed")
	}
	if ResolveHomeView(home, "invalid").Shelves[0].ID != "watchlist" {
		t.Fatal("unknown view must use Home")
	}
}
