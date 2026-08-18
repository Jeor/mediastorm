package indexer

import (
	"testing"

	"novastream/config"
	"novastream/models"
)

func TestSortResultsByScoreCountryMatchSupersedesQualityCriteria(t *testing.T) {
	results := []models.NZBResult{
		{Title: "Shameless.S01E01.2160p.WEB-DL", Attributes: map[string]string{}},
		{Title: "Shameless.UK.S01E01.720p.HDTV", Attributes: map[string]string{"countryMatch": "true"}},
	}
	ctx := ScoringContext{RankingCriteria: []config.RankingCriterion{
		{ID: config.RankingResolution, Name: "Resolution", Enabled: true, Order: 0},
	}}
	(&Service{}).sortResultsByScore(results, ctx)
	if results[0].Attributes["countryMatch"] != "true" {
		t.Fatalf("explicit matching country should rank first, got %q", results[0].Title)
	}
}

func TestScoreResultIncludesCountryMatchDiagnostic(t *testing.T) {
	_, breakdown := ScoreResult(models.NZBResult{Attributes: map[string]string{
		"countryMatch":   "true",
		"releaseCountry": "GB",
	}}, ScoringContext{})
	if len(breakdown) != 1 || breakdown[0].Criterion != "Country Match" || breakdown[0].Points != 0 {
		t.Fatalf("unexpected country breakdown: %+v", breakdown)
	}
}
