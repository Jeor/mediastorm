package config

import (
	"path/filepath"
	"testing"
)

func TestSourceNameFilteringPersistsAndDefaultsOff(t *testing.T) {
	mgr := NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings := DefaultSettings()
	settings.TorrentScrapers = []TorrentScraperConfig{
		{Name: "Trusted", Type: "stremio-direct", Enabled: true, SkipNameFiltering: true},
		{Name: "Strict", Type: "torrentio", Enabled: true},
	}
	if err := mgr.Save(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.TorrentScrapers) != 2 || !loaded.TorrentScrapers[0].SkipNameFiltering || loaded.TorrentScrapers[1].SkipNameFiltering {
		t.Fatalf("sources = %+v", loaded.TorrentScrapers)
	}
	loaded.TorrentScrapers[0].SkipNameFiltering = false
	if err := mgr.Save(loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TorrentScrapers[0].SkipNameFiltering {
		t.Fatal("explicit off not retained")
	}
}
