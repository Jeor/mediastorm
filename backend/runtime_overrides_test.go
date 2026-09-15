package main

import (
	"path/filepath"
	"testing"

	"novastream/config"
)

func TestApplyRuntimeOverrides(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	hlsDir := filepath.Join(t.TempDir(), "hls")
	t.Setenv("STRMR_CACHE_DIR", cacheDir)
	t.Setenv("STRMR_HLS_TEMP_DIR", hlsDir)
	settings := config.DefaultSettings()

	applyRuntimeOverrides(&settings)

	if settings.Cache.Directory != cacheDir {
		t.Fatalf("cache directory = %q, want %q", settings.Cache.Directory, cacheDir)
	}
	if settings.Transmux.HLSTempDirectory != hlsDir {
		t.Fatalf("HLS directory = %q, want %q", settings.Transmux.HLSTempDirectory, hlsDir)
	}
}
