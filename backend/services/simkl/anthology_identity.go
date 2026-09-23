package simkl

import (
	"strconv"

	"novastream/internal/mediaidentity"
)

// EpisodeIdentity translates catalog numbering at the Simkl boundary. Simkl
// groups the separate TMDB Lizzie Borden series under Monster season 4.
// Keep the anthology IDs out of local history so other seasons stay distinct.
func EpisodeIdentity(ids IDs, season, episode int) (IDs, int, int) {
	mapped, ok := mediaidentity.KnownAnthologyEpisode("tmdb:tv:"+strconv.Itoa(ids.TMDB), season, episode)
	if !ok {
		return ids, season, episode
	}
	return IDs{IMDB: mapped.IMDBID}, mapped.Season, mapped.Episode
}

// CatalogEpisodeIdentity reverses only the verified anthology season. Other
// Monster seasons must retain their own identity and numbering.
func CatalogEpisodeIdentity(ids map[string]string, season, episode int) (string, map[string]string, int, bool) {
	if season != 4 || episode < 1 || episode > 8 ||
		(ids["simkl"] != "1446614" && ids["imdb"] != "tt13207736" && ids["tvdb"] != "389492") {
		return "", nil, 0, false
	}
	return "tmdb:tv:299939", map[string]string{"tmdb": "299939", "titleId": "tmdb:tv:299939"}, 1, true
}
