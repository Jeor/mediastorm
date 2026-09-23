package sports

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Numeric provider IDs overlap between leagues. Artwork must remain attached
// to the participant supplied in each event, never a global ID lookup.
func TestArtworkIdentityAcrossESPNLeagues(t *testing.T) {
	for _, league := range LeagueCatalog {
		if league.Slug == "" {
			continue
		}
		t.Run(league.ID, func(t *testing.T) {
			for _, kind := range []string{"team", "athlete", "roster"} {
				name := league.ID + " " + kind
				logo := "https://example.test/" + league.ID + "/participant.png"
				var competitor espnCompetitor
				raw := map[string]any{"id": "1", kind: map[string]any{"id": "1", "displayName": name, "shortName": name, "shortDisplayName": name, "logo": logo, "headshot": map[string]string{"href": logo}}}
				data, _ := json.Marshal(raw)
				if err := json.Unmarshal(data, &competitor); err != nil {
					t.Fatal(err)
				}
				got := competitorTeam(competitor)
				if got.ID != "1" || got.Name != name {
					t.Fatalf("%s: identity changed: %+v", kind, got)
				}
				if kind == "roster" {
					logo = ""
				}
				if got.LogoURL != logo {
					t.Fatalf("%s: artwork mismatch: %+v", kind, got)
				}
			}
			if !league.SupportsTeams {
				return
			}
			var payload espnTeamsResponse
			raw := fmt.Sprintf(`{"sports":[{"leagues":[{"teams":[{"team":{"id":"1","displayName":%q,"logos":[{"href":%q}]}}]}]}]}`, league.ID, "https://example.test/"+league.ID+".png")
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatal(err)
			}
			records := espnTeamsToRecords(payload, league)
			if len(records) != 1 || records[0].ID != league.ID+":1" || records[0].Name != league.ID || records[0].LogoURL != "https://example.test/"+league.ID+".png" {
				t.Fatalf("catalog identity lost: %+v", records)
			}
		})
	}
}
