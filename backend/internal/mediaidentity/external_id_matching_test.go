package mediaidentity

import "testing"

func TestHasMatchingExternalIDIgnoresEpisodeNumbering(t *testing.T) {
	primal := map[string]string{"tmdb": "89456", "tvdb": "364007", "absoluteEpisode": "28"}
	for _, key := range []string{"absoluteEpisode", "ABSOLUTEEPISODE"} {
		unrelated := map[string]string{"tmdb": "78191", "tvdb": "336924", key: "28"}
		if HasMatchingExternalID(primal, unrelated) || HasMatchingExternalID(unrelated, primal) {
			t.Fatalf("shared numbering %q matched unrelated series", key)
		}
	}
	alias := map[string]string{"tvdb": "364007", "absoluteEpisode": "8"}
	if !HasMatchingExternalID(primal, alias) {
		t.Fatal("shared provider ID must still match when episode numbering differs")
	}
}
