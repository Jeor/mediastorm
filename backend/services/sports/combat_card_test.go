package sports

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCombatCardCapturedProviderFields(t *testing.T) {
	raw, err := os.ReadFile("testdata/mma-card-20250913.json")
	if err != nil {
		t.Fatal(err)
	}
	var p espnScoreboardResponse
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	games := scoreboardEventGames(p.Events[0], League{ID: "ufc", Sport: "mma", EventKind: "fight-card"})
	if len(games) != 14 {
		t.Fatalf("bouts: %d", len(games))
	}
	first := games[0]
	if first.Combat == nil || first.Combat.Division != "Welterweight" || first.Combat.ScheduledRounds != 0 {
		t.Fatalf("invalid rounds or division: %+v", first.Combat)
	}
	if first.Period != "Round 1" || first.Clock != "4:27" {
		t.Fatalf("finish time lost: %+v", first)
	}
	if first.AwayTeam.Record == "" || first.HomeTeam.Record == "" {
		t.Fatal("fighter records lost")
	}
	for i, g := range games {
		if len(g.Combat.Bouts) != 14 || g.Combat.Bouts[i].ID != g.ID {
			t.Fatal("provider order lost")
		}
		if g.Combat.Bouts[0].Away.Winner != first.AwayTeam.Winner {
			t.Fatal("winner mismatch")
		}
	}
	if games[13].Combat.ScheduledRounds != 5 {
		t.Fatal("verified five-round bout lost")
	}
}
