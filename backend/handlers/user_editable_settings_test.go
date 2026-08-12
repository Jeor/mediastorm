package handlers

import (
	"os"
	"strings"
	"testing"
)

func TestUserEditableSettingsSchemaEligibility(t *testing.T) {
	playback := SettingsSchema["playback"].(map[string]interface{})
	playbackFields := playback["fields"].(map[string]interface{})
	preferredPlayer := playbackFields["preferredPlayer"].(map[string]interface{})
	if preferredPlayer["userEditableEligible"] != true {
		t.Fatal("playback.preferredPlayer should be eligible for frontend exposure")
	}
	if preferredPlayer["userEditablePath"] != "playback.preferredPlayer" {
		t.Fatalf("unexpected editable path: %v", preferredPlayer["userEditablePath"])
	}

	server := SettingsSchema["server"].(map[string]interface{})
	serverFields := server["fields"].(map[string]interface{})
	if serverFields["port"].(map[string]interface{})["userEditableEligible"] == true {
		t.Fatal("server.port must not be eligible for frontend exposure")
	}

	playbackCookies := playbackFields["youtubeProxyUrl"].(map[string]interface{})
	if playbackCookies["userEditableEligible"] == true {
		t.Fatal("password fields must not be eligible for frontend exposure")
	}
}

func TestFilterUserEditableSettings(t *testing.T) {
	got := filterUserEditableSettings([]string{
		"server.port",
		"display.enableAnimations",
		"playback.preferredPlayer",
		"display.enableAnimations",
	})
	want := []string{"display.enableAnimations", "playback.preferredPlayer"}
	if len(got) != len(want) {
		t.Fatalf("filterUserEditableSettings() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filterUserEditableSettings() = %#v, want %#v", got, want)
		}
	}
}

func TestSettingsTemplateUsesCompactEditablePencilToggle(t *testing.T) {
	source, err := os.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	template := string(source)
	for _, expected := range []string{
		"user-editable-setting-toggle",
		"renderUserEditableSettingToggle(fieldDef)",
		`aria-pressed="`,
		`<path d="M12 20h9"/>`,
		"@media (hover: hover) and (pointer: fine)",
	} {
		if !strings.Contains(template, expected) {
			t.Fatalf("settings template missing %q", expected)
		}
	}
	if strings.Contains(template, "user-editable-setting-control") {
		t.Fatal("settings template should not render the old full-width editable checkbox")
	}
	if strings.Contains(template, ".user-editable-setting-toggle:hover,\n.user-editable-setting-toggle:focus-visible") {
		t.Fatal("touchscreen hover styling must not share the keyboard focus rule")
	}
}
