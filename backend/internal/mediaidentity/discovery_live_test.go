package mediaidentity

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// Opt in with MEDIAIDENTITY_LIVE_SETTINGS pointing at local settings.json.
// Credentials are read in-process and never included in test output.
func TestDiscoveryLiveTMDBWikidata(t *testing.T) {
	path := os.Getenv("MEDIAIDENTITY_LIVE_SETTINGS")
	if path == "" {
		t.Skip("live metadata verification is opt-in")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read live settings")
	}
	var settings struct {
		Metadata struct {
			Key string `json:"tmdbApiKey"`
		} `json:"metadata"`
	}
	if json.Unmarshal(data, &settings) != nil || settings.Metadata.Key == "" {
		t.Fatal("missing live TMDB key")
	}
	d := newDiscoveryService()
	d.ensure(context.Background(), settings.Metadata.Key, "tmdb:tv:299939", "", 1)
	for ep := 1; ep <= 8; ep++ {
		got, ok := d.lookup("tmdb:tv:299939", 1, ep)
		if !ok || got.IMDBID != "tt13207736" || got.TVDBID != 389492 || got.Season != 4 || got.Episode != ep || got.Year != 2022 {
			t.Fatalf("live automatic discovery failed for episode %d: %+v", ep, got)
		}
	}
	// Even if a caller omits IMDb, the authoritative show ID stops discovery.
	d.ensure(context.Background(), settings.Metadata.Key, "tmdb:tv:1396", "", 1)
	if _, ok := d.lookup("tmdb:tv:1396", 1, 1); ok {
		t.Fatal("ordinary series mapped")
	}
}
