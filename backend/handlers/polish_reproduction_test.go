package handlers

import "testing"

// MEDIASTORM-414: provider language codes must match profile preferences.
func TestPolishReproductionTrackSelection(t *testing.T) {
	for _, language := range []string{"pol", "Polish", "pl", "pl-PL", "pl_PL"} {
		t.Run(language, func(t *testing.T) {
			audio := []AudioStreamInfo{
				{Index: 1, Language: "eng", Codec: "aac"},
				{Index: 2, Language: language, Codec: "aac"},
			}
			if got := FindAudioTrackByLanguage(audio, "pol"); got != 2 {
				t.Errorf("audio language=%q: selected %d, want Polish track 2", language, got)
			}
			for _, mode := range []string{"on", "forced-only"} {
				subs := []SubtitleStreamInfo{
					{Index: 3, Language: "eng", Codec: "subrip", IsForced: mode == "forced-only"},
					{Index: 4, Language: language, Codec: "subrip", IsForced: mode == "forced-only"},
				}
				if got := FindSubtitleTrackByPreference(subs, "pol", mode, "eng"); got != 4 {
					t.Errorf("subtitle language=%q mode=%q: selected %d, want Polish track 4", language, mode, got)
				}
			}
		})
	}
}

func TestPolishTrackMatchingDoesNotUseSubstrings(t *testing.T) {
	if matchesLanguage("eng", "Napoleon", "pol") {
		t.Fatal("unrelated track title must not match Polish")
	}
	if !matchesLanguage("und", "Polish 5.1", "pl") {
		t.Fatal("descriptive Polish label should match two-letter preference")
	}
	if matchesLanguage("eng", "English", "") {
		t.Fatal("empty preference must not match every language")
	}
}

func TestPolishSubtitlePriorityRecognizesSameLanguage(t *testing.T) {
	streams := []SubtitleStreamInfo{
		{Index: 1, Language: "pol", Title: "Full Subs"},
		{Index: 2, Language: "pl", Title: "Dubtitles"},
	}
	if got := FindSubtitleTrackByPreference(streams, "pol", "on", "pl"); got != 2 {
		t.Fatalf("same-language audio should select dubtitles: got %d", got)
	}
}
