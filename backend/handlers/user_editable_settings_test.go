package handlers

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"novastream/config"
	"novastream/models"
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

	nestedFiltering := SettingsSchema["filtering.debrid"].(map[string]interface{})["fields"].(map[string]interface{})
	if nestedFiltering["hdrDvPolicy"].(map[string]interface{})["userEditableEligible"] != true {
		t.Fatal("profile-compatible nested filtering fields should be eligible")
	}

	metadata := SettingsSchema["metadata"].(map[string]interface{})["fields"].(map[string]interface{})
	if metadata["primaryLanguage"].(map[string]interface{})["userEditableEligible"] != true {
		t.Fatal("metadata.primaryLanguage should be eligible")
	}

	filtering := SettingsSchema["filtering"].(map[string]interface{})["fields"].(map[string]interface{})
	for _, field := range []string{"adaptivePlaybackEnabled", "adaptiveTargetBufferFactor", "realDebridRestrictedTermsFilterEnabled", "preferredScraper", "servicePriority"} {
		if filtering[field].(map[string]interface{})["userEditableEligible"] != true {
			t.Fatalf("filtering.%s should be eligible", field)
		}
	}
}

func TestScalarProfileSchemaFieldsAreEligible(t *testing.T) {
	modelType := reflect.TypeOf(models.UserSettings{})
	for sectionKey := range userEditableFields {
		section := SettingsSchema[sectionKey].(map[string]interface{})
		fields := section["fields"].(map[string]interface{})
		for fieldKey, rawField := range fields {
			field := rawField.(map[string]interface{})
			fieldType := stringValue(field["type"])
			if field["hidden"] == true || field["globalOnly"] == true || field["readonly"] == true || fieldType == "password" || fieldType == "file_upload" {
				continue
			}
			leafType, ok := jsonModelLeafType(modelType, sectionKey+"."+fieldKey)
			if !ok {
				continue
			}
			for leafType.Kind() == reflect.Pointer {
				leafType = leafType.Elem()
			}
			if leafType.Kind() == reflect.Struct || leafType.Kind() == reflect.Map || leafType.Kind() == reflect.Slice || leafType.Kind() == reflect.Array {
				continue
			}
			if field["userEditableEligible"] != true {
				t.Errorf("scalar profile field %s.%s is not frontend-editable", sectionKey, fieldKey)
			}
		}
	}
}

func jsonModelLeafType(root reflect.Type, path string) (reflect.Type, bool) {
	current := root
	for _, part := range strings.Split(path, ".") {
		for current.Kind() == reflect.Pointer {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return nil, false
		}
		found := false
		for index := 0; index < current.NumField(); index++ {
			field := current.Field(index)
			if strings.Split(field.Tag.Get("json"), ",")[0] == part {
				current = field.Type
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return current, true
}

func TestUserEditableSettingsSchemaIncludesSupportedScopes(t *testing.T) {
	schema := userEditableSettingsSchema([]string{
		"display.badgeVisibility",
		"display.enableAnimations",
		"display.simpleMode",
		"display.simpleModeHomeShelves",
		"filtering.debrid.hdrDvPolicy",
		"filtering.preferredScraper",
		"filtering.realDebridRestrictedTermsFilterEnabled",
	})
	if got := strings.Join(schema["display.badgeVisibility"].Scopes, ","); got != "profile" {
		t.Fatalf("badge visibility scopes = %q", got)
	}
	for _, path := range []string{"display.enableAnimations", "display.simpleMode", "display.simpleModeHomeShelves", "filtering.debrid.hdrDvPolicy"} {
		if got := strings.Join(schema[path].Scopes, ","); got != "profile,device" {
			t.Fatalf("%s scopes = %q", path, got)
		}
	}
	if got := strings.Join(schema["filtering.preferredScraper"].Scopes, ","); got != "profile" {
		t.Fatalf("preferred scraper scopes = %q", got)
	}
	if got := strings.Join(schema["filtering.realDebridRestrictedTermsFilterEnabled"].Scopes, ","); got != "profile,device" {
		t.Fatalf("Real-Debrid restriction filter scopes = %q", got)
	}
}

func TestUserEditableSettingsSchemaResolvesScraperOptions(t *testing.T) {
	schema := userEditableSettingsSchemaForSettings([]string{"filtering.preferredScraper"}, config.Settings{
		TorrentScrapers: []config.TorrentScraperConfig{{Name: "Torrentio"}, {Name: "Comet"}},
	})
	options, ok := schema["filtering.preferredScraper"].Options.([]map[string]string)
	if !ok || len(options) != 2 || options[0]["value"] != "Torrentio" || options[1]["value"] != "Comet" {
		t.Fatalf("unexpected scraper options: %#v", schema["filtering.preferredScraper"].Options)
	}
}

func TestUserEditableSettingsSchemaResolvesSimpleModeHomeShelves(t *testing.T) {
	schema := userEditableSettingsSchemaForSettings([]string{"display.simpleModeHomeShelves"}, config.Settings{
		HomeShelves: config.HomeShelvesSettings{
			Shelves: []config.ShelfConfig{
				{ID: "top-ten", Name: "Top 10 Today"},
				{ID: "tonight", Name: "Tonight"},
				{ID: "custom-list", Name: "Family Favorites"},
			},
		},
	})
	field := schema["display.simpleModeHomeShelves"]
	if field.Type != "ordered-multiselect" {
		t.Fatalf("simple mode home shelves type = %q", field.Type)
	}
	options, ok := field.Options.([]map[string]string)
	if !ok || len(options) != 2 || options[0]["value"] != "top-ten" || options[1]["value"] != "custom-list" {
		t.Fatalf("unexpected simple mode shelf options: %#v", field.Options)
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
		`aria-label="Settings scope"`,
		"Settings cascade: Server defaults → Person overrides → Device overrides",
		"ordered-multiselect",
		"openMultiselectModal",
		"moveMultiselectOption",
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
