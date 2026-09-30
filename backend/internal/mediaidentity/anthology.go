package mediaidentity

import (
	"novastream/models"
	"strings"
)

// AnthologyEpisode is the provider identity of a catalog episode. ReleaseTitle
// and Year apply only together with these coordinates, never as global aliases.
type AnthologyEpisode struct {
	Numbering          *models.EpisodeNumbering
	Source             string // Empty for legacy anthology identities.
	AbsoluteEpisode    int    // Verified release-system absolute number, when available.
	IMDBID             string
	TVDBID             int64
	ReleaseTitle       string
	Year               int
	Season             int
	Episode            int
	SeasonEpisodeCount int
}

// EpisodeCoordinate supports exceptions within an otherwise season-wide map.
type EpisodeCoordinate struct{ Season, Episode int }

// SeriesCrossMapping describes a verified catalog-to-provider relationship.
// Discovered mappings and manual fallbacks share the same search/filter logic.
type SeriesCrossMapping struct {
	TitleID          string
	CatalogSeason    int
	FirstEpisode     int
	EpisodeCount     int
	Provider         AnthologyEpisode
	EpisodeOverrides map[int]EpisodeCoordinate
}

// Verified 2026-09-19 against TMDB episode external IDs and Cinemeta:
// TVDB episodes 11934436, 11963721–11963727 are identical across both orders.
var seriesCrossMappings = []SeriesCrossMapping{{
	TitleID: "tmdb:tv:299939", CatalogSeason: 1, FirstEpisode: 1, EpisodeCount: 8,
	Provider: AnthologyEpisode{IMDBID: "tt13207736", TVDBID: 389492, ReleaseTitle: "Monster", Year: 2022, Season: 4, Episode: 1, SeasonEpisodeCount: 8},
}, {
	// Verified 2026-09-30: TMDB 323903 S01E01–06 and Cinemeta
	// tt2708480 S03E01–06 have the same titles, synopses and weekly air dates
	// (Cinemeta dates are one day later). TMDB has no episode TVDB IDs or
	// season Wikidata ID, so automatic cross-provider discovery cannot prove it.
	TitleID: "tmdb:tv:323903", CatalogSeason: 1, FirstEpisode: 1, EpisodeCount: 6,
	Provider: AnthologyEpisode{IMDBID: "tt2708480", TVDBID: 322191, ReleaseTitle: "The Terror", Year: 2018, Season: 3, Episode: 1, SeasonEpisodeCount: 6},
}}

// UnscopedReleaseTitles excludes names whose identity requires mapped episode
// coordinates. Providers sometimes also publish these names as global aliases.
// Keep the catalog's own title; only distinct aliases need this restriction.
func UnscopedReleaseTitles(titles []string, catalogTitle, titleID string, season, episode int, numbering *models.EpisodeNumbering) []string {
	mappings := ReleaseEpisodeAliases(titleID, season, episode, numbering)
	if len(mappings) == 0 {
		return titles
	}
	out := make([]string, 0, len(titles))
	for _, title := range titles {
		scoped := false
		for _, m := range mappings {
			if strings.EqualFold(strings.TrimSpace(title), m.ReleaseTitle) && !strings.EqualFold(strings.TrimSpace(catalogTitle), m.ReleaseTitle) {
				scoped = true
				break
			}
		}
		if !scoped {
			out = append(out, title)
		}
	}
	return out
}

// KnownAnthologyEpisode performs only an in-memory exact-identity lookup.
func KnownAnthologyEpisode(titleID string, season, episode int) (AnthologyEpisode, bool) {
	if mapped, ok := discoveries.lookup(strings.TrimSpace(titleID), season, episode); ok {
		discoveries.logSelection(titleID, season, episode, mapped, "wikidata")
		return mapped, true
	}
	mapped, ok := lookupCrossMapping(seriesCrossMappings, titleID, season, episode)
	if ok {
		discoveries.logSelection(titleID, season, episode, mapped, "manual_fallback")
	}
	return mapped, ok
}

func lookupCrossMapping(mappings []SeriesCrossMapping, titleID string, season, episode int) (AnthologyEpisode, bool) {
	for _, m := range mappings {
		if strings.TrimSpace(titleID) != m.TitleID || season != m.CatalogSeason || episode < m.FirstEpisode || episode >= m.FirstEpisode+m.EpisodeCount {
			continue
		}
		mapped := m.Provider
		mapped.Episode += episode - m.FirstEpisode
		if override, ok := m.EpisodeOverrides[episode]; ok {
			mapped.Season, mapped.Episode = override.Season, override.Episode
		}
		return mapped, true
	}
	return AnthologyEpisode{}, false
}
