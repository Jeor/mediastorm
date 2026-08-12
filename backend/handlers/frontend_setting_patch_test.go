package handlers

import (
	"encoding/json"
	"testing"

	"novastream/models"
)

func TestJSONModelPathOverridden(t *testing.T) {
	settings := models.UserSettings{
		Playback:  models.PlaybackSettings{PreferredPlayer: "vlc"},
		Filtering: models.FilterSettings{AdaptivePlaybackEnabled: models.BoolPtr(false)},
		Display: models.DisplaySettings{
			EnableAnimations: models.BoolPtr(false),
		},
	}
	for _, path := range []string{"playback.preferredPlayer", "display.enableAnimations", "filtering.adaptivePlaybackEnabled"} {
		if !jsonModelPathOverridden(settings, path) {
			t.Fatalf("%s should be reported as overridden", path)
		}
	}
	for _, path := range []string{"playback.subtitleSize", "display.hideDetailsPoster", "ranking.newestReleaseFirst"} {
		if jsonModelPathOverridden(settings, path) {
			t.Fatalf("%s should be reported as inherited", path)
		}
	}
}

func TestPatchJSONObjectPreservesSiblings(t *testing.T) {
	raw, err := patchJSONObject(
		[]byte(`{"playback":{"preferredPlayer":"native","subtitleSize":1.25}}`),
		"playback.preferredPlayer",
		json.RawMessage(`"vlc"`),
		false,
	)
	if err != nil {
		t.Fatalf("patchJSONObject: %v", err)
	}
	var got map[string]map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["playback"]["preferredPlayer"] != "vlc" || got["playback"]["subtitleSize"] != 1.25 {
		t.Fatalf("unexpected patched document: %#v", got)
	}
}

func TestPatchJSONObjectResetPreservesSiblings(t *testing.T) {
	raw, err := patchJSONObject(
		[]byte(`{"playback":{"preferredPlayer":"native","subtitleSize":1.25}}`),
		"playback.preferredPlayer",
		nil,
		true,
	)
	if err != nil {
		t.Fatalf("patchJSONObject: %v", err)
	}
	var got map[string]map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := got["playback"]["preferredPlayer"]; exists {
		t.Fatalf("reset setting was retained: %#v", got)
	}
	if got["playback"]["subtitleSize"] != 1.25 {
		t.Fatalf("sibling was not preserved: %#v", got)
	}
}

func TestValidateUserEditableSettingValueRejectsUnsupportedMultiOption(t *testing.T) {
	err := validateUserEditableSettingValue("playback.allowedTrackLanguages", json.RawMessage(`["not-a-language"]`))
	if err == nil {
		t.Fatal("expected unsupported option to be rejected")
	}
}

func TestClientSettingPathRejectsProfileOnlySettings(t *testing.T) {
	if _, ok := clientSettingPath("homeShelves.itemCap"); ok {
		t.Fatal("home shelf settings must not be accepted as device overrides")
	}
	if got, ok := clientSettingPath("display.enableAnimations"); !ok || got != "enableAnimations" {
		t.Fatalf("clientSettingPath() = %q, %v", got, ok)
	}
	if got, ok := clientSettingPath("filtering.debrid.hdrDvPolicy"); !ok || got != "debrid.hdrDvPolicy" {
		t.Fatalf("nested clientSettingPath() = %q, %v", got, ok)
	}
	if got, ok := clientSettingPath("animeFiltering.animePreferredLanguage"); !ok || got != "animePreferredLanguage" {
		t.Fatalf("anime clientSettingPath() = %q, %v", got, ok)
	}
}
