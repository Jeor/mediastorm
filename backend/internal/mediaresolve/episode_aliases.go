package mediaresolve

import "encoding/json"

// ReadEpisodeAliases accepts only the explicit coordinates bound by filtering.
func ReadEpisodeAliases(value string) []EpisodeCode {
	var aliases []EpisodeCode
	if len(value) > 4096 || json.Unmarshal([]byte(value), &aliases) != nil || len(aliases) > 8 {
		return nil
	}
	for _, a := range aliases {
		if a.Season < 1 || a.Episode < 1 {
			return nil
		}
	}
	return aliases
}

// CandidateMatchesEpisodeAlias requires an explicit code. Bare E01 filenames
// cannot establish which numbering system a multi-season pack uses.
func CandidateMatchesEpisodeAlias(label string, aliases []EpisodeCode) bool {
	code, ok := ExtractEpisodeCode(label)
	if !ok {
		return false
	}
	for _, a := range aliases {
		if code == a {
			return true
		}
	}
	return false
}

// XEM absolute numbers can restart each season (Kaiju S02E01 has scene
// absolute 1, TVDB absolute 13). Never use that 1 to accept an explicit S01E001.
func CandidateMatchesSelectionAbsolute(label string, hints SelectionHints) bool {
	if hints.MappedSeason {
		if code, ok := ExtractEpisodeCode(label); ok && code.Season != hints.TargetSeason {
			return false
		}
	}
	return CandidateMatchesAbsoluteEpisode(label, hints.AbsoluteEpisodeNumber)
}
