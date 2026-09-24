package playback

import (
	"novastream/internal/mappingtest"
	"novastream/internal/mediaresolve"
	"novastream/models"
	"novastream/utils/filter"
	"testing"
)

func TestAnimeUsenetPackUsesMappedEpisode(t *testing.T) {
	mappingtest.Install(t)
	mappingtest.Warm(t.Context(), "tmdb:tv:207468", 1, 13)
	results := filter.Results([]models.NZBResult{{Title: "Kaiju No 8 S02 COMPLETE 1080p WEB", Attributes: map[string]string{"absoluteEpisodeNumber": "13"}}}, filter.Options{TitleID: "tmdb:tv:207468", ExpectedTitle: "Kaiju No 8", TargetSeason: 1, TargetEpisode: 13, TargetAbsoluteEpisode: 13, IsAnime: true})
	if len(results) != 1 {
		t.Fatal("pack missing")
	}
	h := buildSelectionHintsFromCandidate(results[0], "")
	files := []mediaresolve.Candidate{{Label: "Kaiju.No.8.S01E13.mkv"}, {Label: "Kaiju.No.8.S02E02.mkv"}, {Label: "Kaiju.No.8.S02E01.mkv"}}
	if i, r := mediaresolve.SelectBestCandidate(files, h); i != 2 {
		t.Fatalf("selected %d: %s", i, r)
	}
	if resolvedFileConflictsWithTargetEpisode(files[2].Label, results[0]) || !resolvedFileConflictsWithTargetEpisode(files[0].Label, results[0]) {
		t.Fatal("resolved file verification ignored source coordinates")
	}
}
