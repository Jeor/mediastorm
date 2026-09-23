package indexer

import (
	"novastream/services/debrid"
	"reflect"
	"testing"
)

func TestCrossMappingAddsOneProviderQueryWithoutReplacingCatalog(t *testing.T) {
	opts := SearchOptions{TitleID: "tmdb:tv:299939", Query: "Monster: The Lizzie Borden Story S01E01", MediaType: "series", IMDBID: "tt9990001", TVDBID: 123, Year: 2026}
	if crossMappingSourceLimit(opts, 50) != 0 {
		t.Fatal("mapped identities capped before ranking")
	}
	parsed := debrid.ParseQuery(opts.Query)
	queries := buildSearchQueries(opts, parsed, nil)
	seen := map[string]int{}
	for _, q := range queries {
		seen[q]++
	}
	if seen[opts.Query] != 1 || seen["Monster S04E01"] != 1 || seen["Monster S04"] != 1 {
		t.Fatalf("queries=%v", queries)
	}
	provider := mappedQueryOptions(opts, "Monster S04E01")
	if provider.IMDBID != "tt13207736" || provider.TVDBID != 389492 || provider.Year != 2022 {
		t.Fatalf("wrong provider IDs: %+v", provider)
	}
	if got := mappedQueryOptions(opts, opts.Query); !reflect.DeepEqual(got, opts) {
		t.Fatal("catalog query mutated")
	}
	opts.TitleID = "tmdb:tv:999"
	if crossMappingSourceLimit(opts, 50) != 50 {
		t.Fatal("unmapped cap changed")
	}
	ordinary := buildSearchQueries(opts, parsed, nil)
	if len(queries) != len(ordinary)+2 {
		t.Fatalf("extra queries: mapped=%v unmapped=%v", queries, ordinary)
	}
	for _, q := range ordinary {
		if q == "Monster S04E01" {
			t.Fatal("unmapped title gained query")
		}
	}
}
