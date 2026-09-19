package mediaidentity

import "strings"

// AnthologyEpisode identifies the same episode in the provider's anthology.
type AnthologyEpisode struct {
	IMDBID             string
	Season             int
	Episode            int
	SeasonEpisodeCount int
}

// KnownAnthologyEpisode is an exact allowlist, never a title/year heuristic.
// Verified 2026-09-19: TMDB 299939 S01E01-E08 have the same external TVDB
// episode IDs as Cinemeta tt13207736 S04E01-E08 (11934436, 11963721–11963727).
func KnownAnthologyEpisode(titleID string, season, episode int) (AnthologyEpisode, bool) {
	if strings.TrimSpace(titleID) != "tmdb:tv:299939" || season != 1 || episode < 1 || episode > 8 {
		return AnthologyEpisode{}, false
	}
	return AnthologyEpisode{IMDBID: "tt13207736", Season: 4, Episode: episode, SeasonEpisodeCount: 8}, true
}
