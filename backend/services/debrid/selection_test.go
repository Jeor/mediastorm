package debrid

import (
	"testing"

	"novastream/internal/mediaresolve"
)

func TestSelectMediaFilesPrefersEpisodeMatch(t *testing.T) {
	files := []File{
		{ID: 1, Path: "Series.S01E01.1080p.mkv"},
		{ID: 2, Path: "Series.S01E02.1080p.mkv"},
	}

	selection := selectMediaFiles(files, mediaresolve.SelectionHints{
		ReleaseTitle: "Series S01E02 1080p WEB-DL",
	})

	if selection == nil {
		t.Fatalf("expected selection, got nil")
	}
	if selection.PreferredID != "2" {
		t.Fatalf("expected preferred ID 2, got %s", selection.PreferredID)
	}
	if selection.OrderedIDs[0] != "2" {
		t.Fatalf("expected preferred ID to be first in ordered list, got %v", selection.OrderedIDs)
	}
	if selection.PreferredReason == "" {
		t.Fatalf("expected non-empty reason")
	}
}

func TestSelectMediaFilesPrefersTitleSimilarity(t *testing.T) {
	files := []File{
		{ID: 10, Path: "Sample.Extras.mkv"},
		{ID: 11, Path: "Movie.Title.2023.2160p.WEB-DL.x265.mkv"},
	}

	selection := selectMediaFiles(files, mediaresolve.SelectionHints{
		ReleaseTitle: "Movie.Title.2023.2160p.WEB-DL.x265-GROUP",
	})

	if selection == nil {
		t.Fatalf("expected selection, got nil")
	}
	if selection.PreferredID != "11" {
		t.Fatalf("expected preferred ID 11, got %s", selection.PreferredID)
	}
	if selection.OrderedIDs[0] != "11" {
		t.Fatalf("expected preferred ID to be first in ordered list, got %v", selection.OrderedIDs)
	}
	if selection.PreferredReason == "" {
		t.Fatalf("expected non-empty reason")
	}
}

func TestSelectMediaFilesExcludesEpisodeSamples(t *testing.T) {
	files := []File{
		{ID: 2, Path: "/Ted.Lasso.S01E01.2160p.UHD.BluRay.x265-BROADCAST/Sample/ted.lasso.s01e01.2160p.uhd.bluray.x265-broadcast.sample.mkv"},
		{ID: 52, Path: "/Ted.Lasso.S01E02.2160p.UHD.BluRay.x265-BROADCAST/Sample/ted.lasso.s01e02.2160p.uhd.bluray.x265-broadcast.sample.mkv"},
	}

	selection := selectMediaFiles(files, mediaresolve.SelectionHints{
		TargetSeason:  1,
		TargetEpisode: 1,
	})

	if selection != nil {
		t.Fatalf("expected sample-only torrent to have no playable media selection, got %+v", selection)
	}
}

func TestSelectMediaFilesPrefersEpisodeOverMatchingSample(t *testing.T) {
	files := []File{
		{ID: 1, Path: "/Sample/Show.S01E01.sample.mkv"},
		{ID: 2, Path: "/Show.S01E01.1080p.mkv"},
		{ID: 3, Path: "/Show.S01E02.1080p.mkv"},
	}

	selection := selectMediaFiles(files, mediaresolve.SelectionHints{
		TargetSeason:  1,
		TargetEpisode: 1,
	})

	if selection == nil {
		t.Fatal("expected a playable media selection")
	}
	if selection.PreferredID != "2" {
		t.Fatalf("expected full episode ID 2, got %s", selection.PreferredID)
	}
	for _, id := range selection.OrderedIDs {
		if id == "1" {
			t.Fatalf("expected sample ID 1 to be excluded from caching, got %v", selection.OrderedIDs)
		}
	}
}

func TestSelectMediaFilesUsesExplicitTargetEpisode(t *testing.T) {
	files := []File{
		{ID: 20, Path: "Show.S01E05.1080p.mkv"},
		{ID: 21, Path: "Show.S02E05.1080p.mkv"},
	}

	selection := selectMediaFiles(files, mediaresolve.SelectionHints{
		TargetSeason:  1,
		TargetEpisode: 5,
	})

	if selection == nil {
		t.Fatalf("expected selection, got nil")
	}
	if selection.PreferredID != "20" {
		t.Fatalf("expected preferred ID 20, got %s", selection.PreferredID)
	}
	if selection.PreferredReason == "" {
		t.Fatalf("expected reason to mention explicit target")
	}
}

func TestSelectMediaFilesSingleDailyFileMatchesCompactDate(t *testing.T) {
	files := []File{{ID: 1, Path: "/Coronation Street_20260817_20302100.mp4", Bytes: 400_000_000}}
	selection := selectMediaFiles(files, mediaresolve.SelectionHints{
		TargetSeason:  67,
		TargetEpisode: 150,
		IsDaily:       true,
		TargetAirDate: "2026-08-17",
	})

	if selection == nil {
		t.Fatal("selectMediaFiles returned nil")
	}
	if selection.RejectionReason != "" {
		t.Fatalf("selection rejected compact daily filename: %s", selection.RejectionReason)
	}
	if selection.PreferredID != "1" {
		t.Fatalf("PreferredID = %q, want 1", selection.PreferredID)
	}
}
