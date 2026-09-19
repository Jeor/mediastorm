package debrid

import (
	"log"
	"strings"
)

type streamEpisodeIdentity struct {
	imdbID  string
	season  int
	episode int
}

// anthologyStreamIdentity is deliberately an allowlist, not a fuzzy title or
// year fallback. TMDB splits Lizzie Borden into its own series, while IMDb and
// Cinemeta place it in Monster season 4. Verified 2026-09-19: all eight TMDB
// episode external TVDB IDs match Cinemeta tt13207736 season 4 (11934436,
// 11963721–11963727). Do not apply the parent IMDb ID to TMDB season 1 globally.
func anthologyStreamIdentity(titleID, imdbID string, parsed ParsedQuery) (streamEpisodeIdentity, bool) {
	if strings.TrimSpace(titleID) != "tmdb:tv:299939" || strings.TrimSpace(imdbID) != "" ||
		parsed.MediaType != MediaTypeSeries || parsed.Season != 1 || parsed.Episode < 1 || parsed.Episode > 8 {
		return streamEpisodeIdentity{}, false
	}
	return streamEpisodeIdentity{imdbID: "tt13207736", season: 4, episode: parsed.Episode}, true
}

// Only IMDb stream providers use this copy. The original query, result filters,
// text-search providers, display identity, and watch history retain TMDB order.
func (req SearchRequest) forIMDBStreamProvider() SearchRequest {
	if identity, ok := anthologyStreamIdentity(req.TitleID, req.IMDBID, req.Parsed); ok {
		log.Printf("[debrid] anthology stream mapping titleId=%s S%02dE%02d -> %s:%d:%d",
			req.TitleID, req.Parsed.Season, req.Parsed.Episode, identity.imdbID, identity.season, identity.episode)
		req.IMDBID = identity.imdbID
		req.Parsed.Season = identity.season
		req.Parsed.Episode = identity.episode
	}
	return req
}
