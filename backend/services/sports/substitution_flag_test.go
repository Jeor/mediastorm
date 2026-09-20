package sports

import (
	"encoding/json"
	"novastream/models"
	"testing"
)

func TestSoccerSubstitutionPayloadVariants(t *testing.T) {
	for _, tc := range []struct {
		name, flag string
		want       bool
	}{
		{"boolean true", "true", true}, {"boolean false", "false", false},
		{"object true", `{"didSub":true,"clock":{"displayValue":"64'"}}`, true},
		{"object false", `{"didSub":false}`, false}, {"null", "null", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"rosters":[{"team":{"id":"home"},"roster":[{"athlete":{"id":"player","displayName":"Player"},"starter":true,"subbedIn":` + tc.flag + `,"subbedOut":` + tc.flag + `}]}]}`
			var summary teamSportSummary
			if err := json.Unmarshal([]byte(raw), &summary); err != nil {
				t.Fatal(err)
			}
			game := models.SportsGame{League: "soccer-por.1", HomeTeam: models.SportsTeam{ID: "home"}, Detail: &models.SportsGameDetail{}}
			normalizeTeamContext(&game, summary)
			if len(game.Detail.Lineups) != 1 || len(game.Detail.Lineups[0].Players) != 1 {
				t.Fatal("lineup dropped")
			}
			player := game.Detail.Lineups[0].Players[0]
			if player.SubbedIn != tc.want || player.SubbedOut != tc.want {
				t.Fatalf("incorrect substitution: %+v", player)
			}
		})
	}
}
