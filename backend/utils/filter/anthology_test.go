package filter

import (
	"strings"
	"testing"

	"novastream/models"
)

func TestKnownAnthologyEpisodeFiltering(t *testing.T) {
	for _, tc := range []struct {
		name, titleID, release string
		season, episode        int
		keep                   bool
	}{
		{"provider title", "tmdb:tv:299939", "Monster.2022.S04E01.2160p.WEB", 1, 1, true},
		{"provider pack", "tmdb:tv:299939", "Monster.2022.S04.COMPLETE.2160p.WEB", 1, 1, true},
		{"provider wrong season", "tmdb:tv:299939", "Monster.2022.S01E01.2160p.WEB", 1, 1, false},
		{"provider wrong episode", "tmdb:tv:299939", "Monster.2022.S04E02.2160p.WEB", 1, 1, false},
		{"provider wrong year", "tmdb:tv:299939", "Monster.2004.S04E01.2160p.WEB", 1, 1, false},
		{"provider unrelated identity", "tmdb:tv:999", "Monster.2022.S04E01.2160p.WEB", 1, 1, false},
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
		{"season pack", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04.COMPLETE.1080p.WEB", 1, 1, true},
		{"reported pack", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S04.1080p.Rus.ColdFilm", 1, 1, true},
		{"unrelated pack", "tmdb:tv:113988", "Monster.The.Lizzie.Borden.Story.S04.1080p.Rus.ColdFilm", 1, 1, false},
		{"wrong season pack", "tmdb:tv:299939", "Monster.The.Lizzie.Borden.Story.S03.1080p.Rus.ColdFilm", 1, 1, false},
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

func TestAnthologyPackSizeUsesVerifiedSeasonCount(t *testing.T) {
	opts := Options{TitleID: "tmdb:tv:299939", ExpectedTitle: "Monster: The Lizzie Borden Story",
		TargetSeason: 1, TargetEpisode: 8, TargetAbsoluteEpisode: 8,
		EpisodeResolver: NewSeriesEpisodeResolver(map[int]int{1: 8}), MaxSizeEpisodeGB: 5}
	for _, sizeGB := range []int64{32, 80} {
		results := Results([]models.NZBResult{{Title: "Monster.The.Lizzie.Borden.Story.S04.1080p.Rus.ColdFilm", SizeBytes: sizeGB * 1024 * 1024 * 1024}}, opts)
		if sizeGB == 80 {
			if len(results) != 0 {
				t.Fatal("oversized pack must still be rejected")
			}
			continue
		}
		if len(results) != 1 || results[0].EpisodeCount != 8 || results[0].EffectiveItemSizeBytes() != 4*1024*1024*1024 {
			t.Fatalf("expected eight-episode pack at 4 GB per episode: %+v", results)
		}
	}
	if opts.EpisodeResolver.GetEpisodesForSeasons([]int{1}) != 8 || opts.EpisodeResolver.GetEpisodesForSeasons([]int{4}) != 0 {
		t.Fatal("original TMDB episode resolver was mutated")
	}
}

func TestDevilInSilverRejectsOriginalTerrorDespiteProviderAlias(t *testing.T) {
	for _, tc := range []struct {
		release string
		keep    bool
		mapped  bool
	}{
		{"The.Terror.S01E01.Go.for.Broke.1080p.BluRay", false, false},
		{"The.Terror.S01.COMPLETE.1080p.BluRay", false, false},
		{"The.Terror.2018.S01E01.1080p.WEB", false, false},
		{"The.Terror.S02E01.1080p.WEB", false, false},
		{"The.Terror.S03E01.1080p.WEB", true, true},
		{"The.Terror.2018.S03E01.1080p.WEB", true, true},
		{"The.Terror.S03E02.1080p.WEB", false, false},
		{"The.Terror.S03.COMPLETE.1080p.WEB", true, true},
		{"The.Terror.Devil.in.Silver.S01E01.1080p.WEB", true, false},
		{"The.Terror.Devil.in.Silver.S03E01.1080p.WEB", true, true},
	} {
		t.Run(tc.release, func(t *testing.T) {
			opts := Options{TitleID: "tmdb:tv:323903", ExpectedTitle: "The Terror: Devil in Silver",
				AlternateTitles: []string{"The Terror"}, ExpectedYear: 2026, TargetSeason: 1, TargetEpisode: 1,
				Numbering:       &models.EpisodeNumbering{SeriesID: "tmdb:tv:323903", Ordering: "official"},
				EpisodeResolver: NewSeriesEpisodeResolver(map[int]int{1: 6})}
			results := Results([]models.NZBResult{{Title: tc.release, SizeBytes: 6 * 1024 * 1024 * 1024}}, opts)
			if (len(results) == 1) != tc.keep {
				t.Fatalf("kept %d results, want keep=%v", len(results), tc.keep)
			}
			if tc.mapped && (results[0].Attributes["targetSeason"] != "3" || results[0].Attributes["targetEpisode"] != "1" || results[0].Attributes["mappedCatalogEpisode"] != "S01E01") {
				t.Fatalf("incorrect file selection/catalog hints: %v", results[0].Attributes)
			}
			if tc.keep && strings.Contains(tc.release, "COMPLETE") && results[0].EpisodeCount != 6 {
				t.Fatalf("expected six-episode pack: %+v", results[0])
			}
		})
	}
}
