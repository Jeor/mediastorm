package filter

import (
	"testing"

	"novastream/models"
)

func TestKnownAnthologyEpisodeFiltering(t *testing.T) {
	for _, tc := range []struct {
		name, titleID, release string
		season, episode        int
		keep                   bool
	}{
		{"TMDB numbering", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S01E01.1080p.WEB", 1, 1, true},
		{"anthology numbering", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04E01.1080p.WEB", 1, 1, true},
		{"last verified episode", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04E08.1080p.WEB", 1, 8, true},
		{"wrong episode", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04E02.1080p.WEB", 1, 1, false},
		{"wrong season", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S03E01.1080p.WEB", 1, 1, false},
		{"other identity", "tmdb:tv:113988", "Monster.The.Lizzie.Borden.Story.S04E01.1080p.WEB", 1, 1, false},
		{"missing identity", "", "Monster.The.Lizzie.Borden.Story.S04E01.1080p.WEB", 1, 1, false},
		{"unverified target", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04E09.1080p.WEB", 1, 9, false},
		{"wrong title", "tmdb:tv:299939", "Some.Other.Series.S04E01.1080p.WEB", 1, 1, false},
		{"wrong year", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.2015.S04E01.1080p.WEB", 1, 1, false},
		{"pack unchanged", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04.COMPLETE.1080p.WEB", 1, 1, false},
		{"special unchanged", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04E01.1080p.WEB", 0, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{TitleID: tc.titleID, ExpectedTitle: "Monster: The Lizzie Borden Story", ExpectedYear: 2026, TargetSeason: tc.season, TargetEpisode: tc.episode}
			results := Results([]models.NZBResult{{Title: tc.release}}, opts)
			if (len(results) == 1) != tc.keep {
				t.Fatalf("kept %d results, want keep=%v", len(results), tc.keep)
			}
			if tc.keep && tc.name != "TMDB numbering" && results[0].Attributes["targetSeason"] != "4" {
				t.Fatalf("release file-selection hints must use season 4: %v", results[0].Attributes)
			}
			if opts.TargetSeason != tc.season {
				t.Fatal("original target changed")
			}
		})
	}
}
