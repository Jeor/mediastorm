package filter

import (
	"testing"

	"novastream/models"
)

func TestNormalizeCountryCodeUsesISOAndReleaseAliases(t *testing.T) {
	tests := map[string]string{
		"UK":             "GB",
		"GB":             "GB",
		"GBR":            "GB",
		"United Kingdom": "GB",
		"US":             "US",
		"USA":            "US",
		"AUS":            "AU",
		"NZL":            "NZ",
		"CAN":            "CA",
	}
	for input, want := range tests {
		if got := NormalizeCountryCode(input); got != want {
			t.Errorf("NormalizeCountryCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResultsWithDetailsFiltersExplicitCountryMismatch(t *testing.T) {
	results := []models.NZBResult{
		{Title: "Shameless.UK.S01E01.DVDRip.x264"},
		{Title: "Shameless.US.S01E01.1080p.BluRay.x264"},
		{Title: "Shameless.S01E01.720p.HDTV.x264"},
	}
	detailed := ResultsWithDetails(results, Options{
		ExpectedTitle:   "Shameless",
		ExpectedYear:    2004,
		ExpectedCountry: "GBR",
		TargetSeason:    1,
		TargetEpisode:   1,
	})
	if len(detailed) != 3 {
		t.Fatalf("got %d detailed results, want 3", len(detailed))
	}
	if !detailed[0].Passed || detailed[0].Result.Attributes["countryMatch"] != "true" {
		t.Fatalf("UK result should pass with country match: %+v", detailed[0])
	}
	if detailed[1].Passed || detailed[1].RejectReason != "explicit country US does not match expected GB" {
		t.Fatalf("US result should be rejected as explicit mismatch: %+v", detailed[1])
	}
	if !detailed[2].Passed || detailed[2].Result.Attributes["countryMatch"] != "" {
		t.Fatalf("unmarked result should remain neutral and pass: %+v", detailed[2])
	}

	usDetailed := ResultsWithDetails(results[:2], Options{
		ExpectedTitle:   "Shameless",
		ExpectedYear:    2011,
		ExpectedCountry: "USA",
		TargetSeason:    1,
		TargetEpisode:   1,
	})
	if usDetailed[0].Passed || !usDetailed[1].Passed || usDetailed[1].Result.Attributes["countryMatch"] != "true" {
		t.Fatalf("reciprocal US search did not reject UK and accept US: %+v", usDetailed)
	}
}
