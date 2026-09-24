package mediaresolve

import "testing"

func TestMappedSeasonAbsoluteCannotSelectDifferentSeason(t *testing.T) {
	hints := SelectionHints{TargetSeason: 2, TargetEpisode: 1, AbsoluteEpisodeNumber: 1, MappedSeason: true}
	for _, label := range []string{"Kaiju.No.8.S01E001.mkv", "Kaiju.No.8.S03E001.mkv", "Kaiju.No.8.S02E002.mkv"} {
		if index, reason := SelectBestCandidate([]Candidate{{Label: label}}, hints); index != -1 {
			t.Fatalf("selected wrong season/episode %s: %d %s", label, index, reason)
		}
	}
	if index, reason := SelectBestCandidate([]Candidate{{Label: "Kaiju.No.8 - 01 [1080p].mkv"}}, hints); index != 0 {
		t.Fatalf("source-local absolute rejected: %d %s", index, reason)
	}
}
func TestEpisodeAliasesRequireExplicitSeason(t *testing.T) {
	aliases := []EpisodeCode{{Season: 2, Episode: 1}}
	if CandidateMatchesEpisodeAlias("Kaiju - 01.mkv", aliases) {
		t.Fatal("bare episode accepted as alternate season")
	}
	if !CandidateMatchesEpisodeAlias("Kaiju.S02E01.mkv", aliases) {
		t.Fatal("verified code rejected")
	}
	for _, value := range []string{`bad json`, `[{"Season":0,"Episode":1}]`, `[{"Season":2,"Episode":0}]`} {
		if len(ReadEpisodeAliases(value)) > 0 {
			t.Fatalf("accepted malformed aliases: %s", value)
		}
	}
}
