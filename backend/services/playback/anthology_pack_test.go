package playback

import (
	"testing"

	"novastream/internal/mediaresolve"
	"novastream/models"
	"novastream/utils/filter"
)

func TestAnthologyPackSelectsRequestedProviderEpisode(t *testing.T) {
	results := filter.Results([]models.NZBResult{{
		Title:      "Monster.The.Lizzie.Borden.Story.S04.1080p.Rus.ColdFilm",
		Attributes: map[string]string{"targetSeason": "1", "targetEpisode": "2", "absoluteEpisodeNumber": "2", "targetAbsoluteEpisode": "2"},
	}}, filter.Options{TitleID: "tmdb:tv:299939", ExpectedTitle: "Monster: The Lizzie Borden Story", TargetSeason: 1, TargetEpisode: 2, TargetAbsoluteEpisode: 2})
	if len(results) != 1 {
		t.Fatal("mapped season pack was rejected")
	}
	hints := buildSelectionHintsFromCandidate(results[0], "")
	if hints.TargetSeason != 4 || hints.TargetEpisode != 2 || hints.AbsoluteEpisodeNumber != 0 {
		t.Fatalf("wrong pack selection hints: %+v", hints)
	}
	files := []mediaresolve.Candidate{
		{Label: "Monster.The.Lizzie.Borden.Story.S04E01.mkv", Priority: 1},
		{Label: "Monster.The.Lizzie.Borden.Story.S01E02.mkv", Priority: 1},
		{Label: "Monster.The.Lizzie.Borden.Story.S04E02.mkv", Priority: 1},
	}
	if index, reason := mediaresolve.SelectBestCandidate(files, hints); index != 2 {
		t.Fatalf("selected %d: %s", index, reason)
	}
	if index, reason := mediaresolve.SelectBestCandidate(files[:2], hints); index != -1 {
		t.Fatalf("missing S04E02 must not fall back to another episode: %d, %s", index, reason)
	}
	if resolvedFileConflictsWithTargetEpisode(files[2].Label, results[0]) || !resolvedFileConflictsWithTargetEpisode(files[0].Label, results[0]) {
		t.Fatal("resolved-file validation did not use mapped episode coordinates")
	}
}
