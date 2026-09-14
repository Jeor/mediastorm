package sports

import (
	"encoding/json"
	"novastream/models"
	"testing"
)

func TestCurrentDriveOverridesPreviousDuplicateLocal(t *testing.T) {
	var p teamSportSummary
	json.Unmarshal([]byte(`{"drives":{"previous":[{"id":"d","team":{"id":"a"},"plays":[]}],"current":{"id":"d","team":{"id":"a"},"plays":[{"id":"p","type":{"text":"Rush"}}]}}}`), &p)
	g := models.SportsGame{League: "nfl", Status: models.SportsGameLive}
	g.AwayTeam.ID = "a"
	g.HomeTeam.ID = "h"
	rows := normalizeFootballDrives(g, p)
	if len(rows) != 1 || !rows[0].Current || len(rows[0].Plays) != 1 {
		t.Fatalf("current drive discarded: %+v", rows)
	}
}
