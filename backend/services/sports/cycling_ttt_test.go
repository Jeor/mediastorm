package sports

import (
	"testing"
	"time"
)

func TestCyclingCapturedTeamTimeTrial(t *testing.T) {
	var raw []asoRanking
	cyclingFixture(t, "aso-tour-ttt-stage1.json", &raw)
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	stage := CyclingStage{ID: "aso:tour:2026:1", Terrain: "Team time trial", Status: "unknown"}
	normalizeCyclingResults(raw, &stage, 2026, 1, now)
	if stage.TeamResults == nil || stage.TeamResults.State != "available" || len(stage.TeamResults.Data) != 23 {
		t.Fatalf("team classification: %+v", stage.TeamResults)
	}
	winner := stage.TeamResults.Data[0]
	if winner.Name != "TEAM VISMA | LEASE A BIKE" || winner.Rank != 1 || winner.Time != "0:21:47.870" || winner.Gap != "+0:00" || winner.TeamID != "team-2026:58c4faf080bb352fad62ec06627f2c64da293e753f2fc4589f682522f234742a" {
		t.Fatalf("winner: %+v", winner)
	}
	if stage.Results.State != "pending" || len(stage.Results.Data) != 0 || len(stage.GeneralClassification.Data) != 184 || stage.Status != "unknown" {
		t.Fatal("team results must remain separate from individual results and GC without inferring final status")
	}
	if stage.TeamResults.Source.UpdatedAt == nil || stage.TeamResults.Source.UpdatedAt.UnixMilli() != 1786959946074 {
		t.Fatal("missing provider update time")
	}
	// Ambiguous arrival captures cannot select one checkpoint by row order.
	for _, r := range raw {
		if r.Type == "ttt" && len(r.Types) == 1 && r.Types[0] == "A" {
			raw = append(raw, r)
			break
		}
	}
	normalizeCyclingResults(raw, &stage, 2026, 1, now)
	if stage.TeamResults.State != "pending" || len(stage.TeamResults.Data) != 0 {
		t.Fatal("ambiguous arrival must not produce teams")
	}
}

func TestCyclingTeamResolutionAndScope(t *testing.T) {
	var raw []asoRanking
	cyclingFixture(t, "aso-tour-ttt-stage1.json", &raw)
	stage := CyclingStage{ID: "aso:tour:2026:1", Terrain: "Team time trial"}
	for i := range raw {
		if raw[i].Bind == "team-2026" && raw[i].Name == "TEAM VISMA | LEASE A BIKE" {
			raw[i].Name = ""
		}
	}
	normalizeCyclingResults(raw, &stage, 2026, 1, time.Now())
	if stage.TeamResults.State != "stale" || len(stage.TeamResults.Data) != 22 {
		t.Fatal("unresolved teams must not become representative rider rows")
	}
	road := CyclingStage{ID: stage.ID, Terrain: "Flat"}
	normalizeCyclingResults(raw, &road, 2026, 1, time.Now())
	if road.TeamResults != nil {
		t.Fatal("TTT module must be scoped to a team time trial")
	}
}
