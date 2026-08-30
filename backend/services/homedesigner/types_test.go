package homedesigner

import (
	"encoding/json"
	"strings"
	"testing"

	"novastream/models"
)

func TestPreviewResponse_ProjectsOnlyPresentationSafeFieldsAndContext(t *testing.T) {
	request := PreviewRequest{Scope: Scope{Kind: "profile", ProfileID: "profile-a"}, PreviewProfileID: "profile-a", Platform: "tv"}
	rows := models.HomeShelvesSettings{Shelves: []models.ShelfConfig{{
		ID: "source-1", Name: "Private source", Enabled: true, Order: 2, Type: "stremio", Limit: 20, HideUnreleased: true,
		ListURL: "https://private.example/list", AddonManifestURL: "https://private.example/manifest", TraktAccountID: "secret-account",
		StreamingServices: []models.StreamingServiceLink{{LogoURL: "https://private.example/logo", Lists: []models.StreamingServiceListLink{{URL: "https://private.example/stream"}}}},
	}}}
	response := BuildPreviewResponse(request, rows, models.AppearanceSettings{AccentColor: "#112233"})
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"private.example", "secret-account", "listUrl", "addonManifestUrl", "traktAccountId", "streamingServices"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("preview JSON leaked %q: %s", forbidden, encoded)
		}
	}
	if response.ProfileID != "profile-a" || response.Platform != "tv" || len(response.Rows) != 1 || response.Rows[0].Name != "Private source" || response.Theme.AccentColor != "#112233" {
		t.Fatalf("preview response = %#v, want safe rows, theme, profile, and platform", response)
	}
}
