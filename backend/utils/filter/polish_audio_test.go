package filter

import (
	"novastream/models"
	"testing"
)

func TestExplicitPolishAudioLabels(t *testing.T) {
	for _, tc := range []struct{ label, provider, want string }{
		{"PLDUB.DUAL", "", "pol"}, {"Dubbing PL", "", "pol"}, {"Lektor PL", "", "pol"},
		{"PL.SUBS", "", ""}, {"MULTI", "", ""}, {"PLDUB", "eng", "eng"},
	} {
		t.Run(tc.label+tc.provider, func(t *testing.T) {
			r := models.NZBResult{Title: "Vaiana.2.2024.1080p.BluRay." + tc.label, Attributes: map[string]string{"languages": tc.provider}}
			results := ResultsWithDetails([]models.NZBResult{r}, Options{ExpectedTitle: "Vaiana 2", ExpectedYear: 2024, IsMovie: true})
			if len(results) != 1 || !results[0].Passed {
				t.Fatalf("unexpected filtered result: %+v", results)
			}
			if got := results[0].Result.Attributes["languages"]; got != tc.want {
				t.Fatalf("languages=%q want %q", got, tc.want)
			}
		})
	}
}
