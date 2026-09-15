package main

import (
	"os"
	"strings"

	"novastream/config"
)

// applyRuntimeOverrides lets a platform launcher keep mutable data outside the
// application directory without rewriting the user's settings file. This is
// especially important for ZIP-based Windows updates, which replace app files
// but preserve %LOCALAPPDATA% data.
func applyRuntimeOverrides(settings *config.Settings) {
	if value := strings.TrimSpace(os.Getenv("STRMR_CACHE_DIR")); value != "" {
		settings.Cache.Directory = value
	}
	if value := strings.TrimSpace(os.Getenv("STRMR_HLS_TEMP_DIR")); value != "" {
		settings.Transmux.HLSTempDirectory = value
	}
}
