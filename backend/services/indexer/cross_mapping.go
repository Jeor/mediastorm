package indexer

import (
	"strings"

	"novastream/internal/mediaidentity"
	"novastream/services/debrid"
)

// Bind a mapped text query to the corresponding provider IDs without changing
// the catalog options used for filtering, display, history, or cache identity.
func mappedQueryOptions(opts SearchOptions, query string) SearchOptions {
	original := debrid.ParseQuery(opts.Query)
	requested := debrid.ParseQuery(query)
	out := opts
	out.Query = query
	mapped, ok := mediaidentity.KnownAnthologyEpisode(opts.TitleID, original.Season, original.Episode)
	if ok && original.MediaType == debrid.MediaTypeSeries && strings.EqualFold(requested.Title, mapped.ReleaseTitle) && requested.Season == mapped.Season && (requested.Episode == mapped.Episode || requested.Episode == 0) {
		out.IMDBID, out.TVDBID, out.Year = mapped.IMDBID, mapped.TVDBID, mapped.Year
	}
	return out
}

// Preserve both identities until the indexer applies its final ranking/limit.
func crossMappingSourceLimit(opts SearchOptions, limit int) int {
	parsed := debrid.ParseQuery(opts.Query)
	if parsed.MediaType == debrid.MediaTypeSeries {
		if _, ok := mediaidentity.KnownAnthologyEpisode(opts.TitleID, parsed.Season, parsed.Episode); ok {
			return 0
		}
	}
	return limit
}
