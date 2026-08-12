package handlers

import "testing"

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
