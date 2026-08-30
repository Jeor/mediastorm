package homedesigner

import (
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestHomeDesignerRevision_GlobalIgnoresPlaybackAndChangesForRowsAndTheme(t *testing.T) {
	settings := config.DefaultSettings()
	settings.HomeShelves.Shelves = []config.ShelfConfig{{ID: "top-ten", Name: "Top 10", Enabled: true, Order: 0}}
	base := RevisionForGlobal(settings)
	if base == "" {
		t.Fatal("global revision is empty")
	}
	settings.Playback.PreferredPlayer = "vlc"
	if got := RevisionForGlobal(settings); got != base {
		t.Fatalf("revision after playback change = %q, want %q", got, base)
	}
	settings.HomeShelves.Shelves[0].Name = "Top Ten"
	if got := RevisionForGlobal(settings); got == base {
		t.Fatal("revision did not change after rows changed")
	}
	rowsRevision := RevisionForGlobal(settings)
	settings.Display.Appearance.AccentColor = "#112233"
	if got := RevisionForGlobal(settings); got == rowsRevision {
		t.Fatal("revision did not change after appearance changed")
	}
}

func TestHomeDesignerRevision_ProfileIgnoresPlaybackAndChangesForRowsAndTheme(t *testing.T) {
	fontScale := 1.0
	settings := &models.UserSettings{
		HomeShelves: models.HomeShelvesSettings{Shelves: []models.ShelfConfig{{ID: "genre-16-movie", Name: "Animation", Type: "genre"}}},
		Display:     models.DisplaySettings{Appearance: models.AppearanceSettings{FontScale: &fontScale}},
	}
	base := RevisionForProfile(settings)
	settings.Playback.PreferredPlayer = "vlc"
	if got := RevisionForProfile(settings); got != base {
		t.Fatalf("revision after playback change = %q, want %q", got, base)
	}
	settings.HomeShelves.Shelves[0].Name = "Animation movies"
	if got := RevisionForProfile(settings); got == base {
		t.Fatal("revision did not change after rows changed")
	}
	rowsRevision := RevisionForProfile(settings)
	settings.Display.Appearance.AccentColor = "#112233"
	if got := RevisionForProfile(settings); got == rowsRevision {
		t.Fatal("revision did not change after appearance changed")
	}
}
