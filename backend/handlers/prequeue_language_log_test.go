package handlers

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"

	"novastream/models"
	"novastream/services/indexer"
)

func TestPrequeueLanguageDiagnosticsIncludeEveryCandidate(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	results := make([]models.ScoredNZBResult, 12)
	for i := range results {
		results[i] = models.ScoredNZBResult{NZBResult: models.NZBResult{Title: fmt.Sprintf("Moana.2.release%d", i), Attributes: map[string]string{"languages": "pl"}}, FilterStatus: "passed", ScoreBreakdown: []models.ScoreBreakdownItem{{Criterion: "Preferred Audio Language", RankValue: 1, Reason: "fixed first priority: has preferred language 'pol'"}}}
	}
	logPrequeueCandidateList(results, "combined")
	for _, want := range []string{"showing all 12", "candidate #12", `languages="pl"`, "Preferred Audio Language", "has preferred language 'pol'"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q from log", want)
		}
	}
	if !combinedPrequeueSearchOptions(indexer.SearchOptions{}).IncludeScoreBreakdown {
		t.Fatal("combined search drops ranking diagnostics")
	}
}
