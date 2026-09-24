package debrid

import (
	"fmt"
	"strings"

	"novastream/internal/mediaidentity"
)

// mappedSearchRequests keeps the catalog request and adds a distinct provider
// request only for a verified cross mapping. Discovery runs before this helper.
func mappedSearchRequests(req SearchRequest, imdbBased bool) []SearchRequest {
	if req.Parsed.MediaType != MediaTypeSeries {
		return []SearchRequest{req}
	}

	requests := []SearchRequest{req}
	for _, mapped := range mediaidentity.ReleaseEpisodeAliases(req.TitleID, req.Parsed.Season, req.Parsed.Episode, req.Numbering) {
		alternate := req
		if mapped.IMDBID != "" {
			alternate.IMDBID = mapped.IMDBID
		}
		if mapped.ReleaseTitle != "" {
			alternate.Parsed.Title = mapped.ReleaseTitle
		}
		if mapped.Year > 0 {
			alternate.Parsed.Year = mapped.Year
		}
		alternate.Parsed.Season, alternate.Parsed.Episode = mapped.Season, mapped.Episode
		alternate.Query = fmt.Sprintf("%s S%02dE%02d", alternate.Parsed.Title, mapped.Season, mapped.Episode)
		// Legacy anthology IMDb identity replaces an invalid catalog endpoint.
		// Numbering aliases keep both endpoints, including on the same IMDb ID.
		if imdbBased && mapped.Source == "" && (strings.TrimSpace(req.IMDBID) == "" || req.IMDBID == mapped.IMDBID) {
			requests = nil
		}
		duplicate := false
		for _, prior := range requests {
			if prior.IMDBID == alternate.IMDBID && prior.Parsed.Season == alternate.Parsed.Season && prior.Parsed.Episode == alternate.Parsed.Episode && (imdbBased || strings.EqualFold(prior.Parsed.Title, alternate.Parsed.Title)) {
				duplicate = true
			}
		}
		if !duplicate {
			requests = append(requests, alternate)
		}
	}
	return requests
}
