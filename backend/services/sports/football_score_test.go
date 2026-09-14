package sports

import (
	"encoding/json"
	"novastream/models"
	"testing"
)

func TestFootballScoringSideLocal(t *testing.T) {
	g := models.SportsGame{}
	g.AwayTeam.ID = "a"
	g.HomeTeam.ID = "h"
	var before, score teamDetailPlay
	json.Unmarshal([]byte(`{"id":"a","awayScore":7,"homeScore":3}`), &before)
	json.Unmarshal([]byte(`{"id":"b","scoringPlay":true,"awayScore":7,"homeScore":9,"team":{"id":"a"}}`), &score)
	if footballScoringTeam(score, []teamDetailPlay{before}, g) != "h" {
		t.Fatal("defensive score assigned to possession")
	}
	if footballScoringTeam(score, nil, g) != "" {
		t.Fatal("invented scorer without score baseline")
	}
	score.HomeScore = nil
	if footballScoringTeam(score, []teamDetailPlay{before}, g) != "" {
		t.Fatal("invented scorer with missing score")
	}
}
