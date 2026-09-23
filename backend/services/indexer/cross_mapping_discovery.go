package indexer

import (
	"context"
	"novastream/config"
	"novastream/internal/mediaidentity"
	"novastream/services/debrid"
	"strings"
)

func (s *Service) discoverCrossMapping(ctx context.Context, settings config.Settings, opts SearchOptions) {
	if opts.IsAnime || strings.TrimSpace(opts.IMDBID) != "" {
		return
	}
	parsed := debrid.ParseQuery(opts.Query)
	mediaType := strings.ToLower(opts.MediaType)
	if mediaType == "" {
		mediaType = string(parsed.MediaType)
	}
	if mediaType != "series" {
		return
	}
	season := parsed.Season
	mediaidentity.DiscoverSeason(ctx, settings.Metadata.TMDBAPIKey, opts.TitleID, opts.IMDBID, season)
}
