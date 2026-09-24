package filter

import (
	"encoding/json"
	"fmt"
	"novastream/internal/mediaidentity"
	"novastream/internal/mediaresolve"
	"novastream/models"
	"novastream/utils/parsett"
	"strconv"
)

// Pick a source identity only when the release declares that numbering. In
// particular an absolute fansub episode must not become a cour-local episode.
func releaseMappingForResult(opts Options, parsed *parsett.ParsedTitle) (mediaidentity.AnthologyEpisode, bool) {
	if opts.IsMovie || len(parsed.Seasons) != 1 {
		return mediaidentity.AnthologyEpisode{}, false
	}
	var selected mediaidentity.AnthologyEpisode
	found := false
	for _, m := range mediaidentity.ReleaseEpisodeAliases(opts.TitleID, opts.TargetSeason, opts.TargetEpisode, opts.Numbering) {
		if m.Season != parsed.Seasons[0] {
			continue
		}
		if m.Source != "" && m.Season == opts.TargetSeason {
			// Same-season remapping cannot disambiguate a season pack. A single
			// explicitly numbered release can identify the alternate coordinate.
			if len(parsed.Episodes) != 1 || parsed.Episodes[0] != m.Episode {
				continue
			}
		}
		if found && (selected.Season != m.Season || selected.Episode != m.Episode) {
			return mediaidentity.AnthologyEpisode{}, false
		}
		selected = m
		found = true
	}
	return selected, found
}

// Multi-season packs can contain either catalog or release coordinates. Carry
// explicit alternatives into file selection without treating bare E01 as one.
func bindPackEpisodeAliases(result *models.NZBResult, opts Options, parsed *parsett.ParsedTitle) {
	if opts.IsMovie || len(parsed.Episodes) != 0 || len(parsed.Seasons) == 1 {
		return
	}
	var aliases []mediaresolve.EpisodeCode
	for _, m := range mediaidentity.ReleaseEpisodeAliases(opts.TitleID, opts.TargetSeason, opts.TargetEpisode, opts.Numbering) {
		if m.Source == "" {
			continue
		}
		contains := len(parsed.Seasons) == 0
		for _, season := range parsed.Seasons {
			if season == m.Season {
				contains = true
			}
		}
		if contains {
			aliases = append(aliases, mediaresolve.EpisodeCode{Season: m.Season, Episode: m.Episode})
		}
	}
	if len(aliases) == 0 {
		return
	}
	data, _ := json.Marshal(aliases)
	result.Attributes["episodeSelectionAliases"] = string(data)
	result.Attributes["mappedCatalogNumbering"] = models.EpisodeNumberingKey(opts.Numbering)
	result.Attributes["mappedCatalogEpisode"] = fmt.Sprintf("S%02dE%02d", opts.TargetSeason, opts.TargetEpisode)
	result.Attributes["targetSeason"] = strconv.Itoa(opts.TargetSeason)
	result.Attributes["targetEpisode"] = strconv.Itoa(opts.TargetEpisode)
	result.Attributes["targetEpisodeCode"] = result.Attributes["mappedCatalogEpisode"]
	if opts.TargetAbsoluteEpisode > 0 {
		result.Attributes["absoluteEpisodeNumber"] = strconv.Itoa(opts.TargetAbsoluteEpisode)
	}
}
