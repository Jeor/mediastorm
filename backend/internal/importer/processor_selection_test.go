package importer

import (
	"testing"

	"novastream/config"
)

func TestIsContentVideoPath(t *testing.T) {
	proc := &Processor{}

	tests := []struct {
		name         string
		internalPath string
		want         bool
	}{
		{
			name:         "main episode",
			internalPath: "Release/Show.S01E08.1080p.WEB.mkv",
			want:         true,
		},
		{
			name:         "sample filename before main",
			internalPath: "Release/Sample/Show.S01E08.sample.mkv",
			want:         false,
		},
		{
			name:         "sample directory with neutral filename",
			internalPath: "Release/Samples/clip.mkv",
			want:         false,
		},
		{
			name:         "extras filename",
			internalPath: "Release/Show.S01E08.extras.mp4",
			want:         false,
		},
		{
			name:         "trailer directory with neutral filename",
			internalPath: `Release\Trailers\clip.mkv`,
			want:         false,
		},
		{
			name:         "non-video",
			internalPath: "Release/Show.S01E08.srt",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := proc.isContentVideoPath(tt.internalPath); got != tt.want {
				t.Fatalf("isContentVideoPath(%q) = %t, want %t", tt.internalPath, got, tt.want)
			}
		})
	}
}

func TestArchiveEarlyPlaybackSkipsSampleBeforeMain(t *testing.T) {
	proc := &Processor{}
	archiveEntries := []string{
		"Release/Sample/Show.S01E08.sample.mkv",
		"Release/Show.S01E08.1080p.WEB.mkv",
	}

	var selected string
	for _, entry := range archiveEntries {
		if selected == "" && proc.isContentVideoPath(entry) {
			selected = entry
		}
	}

	if want := archiveEntries[1]; selected != want {
		t.Fatalf("selected %q, want main content %q", selected, want)
	}
}

func TestEnsureArchiveProcessorConfigRefreshesBothProcessors(t *testing.T) {
	cfg := &config.AltMountConfig{Import: config.ImportConfig{
		RarMaxWorkers:          2,
		RarMaxCacheSizeMB:      8,
		RarEnableMemoryPreload: false,
		RarMaxMemoryGB:         1,
	}}
	proc := NewProcessor(nil, nil, func() *config.AltMountConfig { return cfg })

	cfg.Import.RarMaxWorkers = 3
	cfg.Import.RarMaxCacheSizeMB = 16
	cfg.Import.RarEnableMemoryPreload = true
	cfg.Import.RarMaxMemoryGB = 2
	proc.ensureRarProcessorConfig()

	rp := proc.rarProcessor.(*rarProcessor)
	if rp.maxWorkers != 3 || rp.maxCacheSizeMB != 16 || !rp.enableMemoryPreload || rp.maxMemoryGB != 2 {
		t.Fatalf("RAR processor was not refreshed: %#v", rp)
	}
	sp := proc.sevenZipProcessor.(*sevenZipProcessor)
	if sp.maxWorkers != 3 || sp.maxCacheSizeMB != 16 {
		t.Fatalf("7z processor was not refreshed: %#v", sp)
	}
}
