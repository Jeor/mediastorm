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

func TestNamedHomeViews(t *testing.T) {
	global := map[string]HomeViewSettings{"page-docs": {Name: "Documentaries", Mode: "custom", MediaFilter: "movies"}, "search": {Mode: "custom"}}
	profile := map[string]HomeViewSettings{"page-docs": {Mode: "inherit"}, "page-local": {Name: "Local", Mode: "inherit"}}
	merged := MergeHomeViews(global, profile)
	if len(merged) != 2 || merged["page-docs"].Name != "Documentaries" || merged["page-docs"].Mode != "inherit" || HomeViewFilter(merged, "page-docs") != "movies" {
		t.Fatalf("bad merge: %+v", merged)
	}
	if HomeViewFilter(merged, "page-local") != "all" || HomeViewFilter(merged, "missing") != "all" {
		t.Fatal("default filter must be all")
	}
	profile["page-docs"] = HomeViewSettings{Mode: "inherit", Deleted: true}
	if HomeViewFilter(MergeHomeViews(global, profile), "page-docs") != "all" {
		t.Fatal("deleted page must not filter")
	}
	for _, id := range []string{"page-", "page-../bad", "search", "all"} {
		if IsHomeViewID(id) {
			t.Fatalf("accepted invalid page %q", id)
		}
	}
	if global["page-docs"].Mode != "custom" {
		t.Fatal("mutated global")
	}
}
