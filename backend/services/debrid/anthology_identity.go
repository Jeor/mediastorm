package debrid

import (
	"fmt"
	"strings"

	"novastream/internal/mediaidentity"
)

// mappedSearchRequests keeps the catalog request and adds a distinct provider
// request only for an explicit cross mapping. No metadata/network discovery.
func mappedSearchRequests(req SearchRequest, imdbBased bool) []SearchRequest {
	if req.Parsed.MediaType != MediaTypeSeries {
		return []SearchRequest{req}
	}
	mapped, ok := mediaidentity.KnownAnthologyEpisode(req.TitleID, req.Parsed.Season, req.Parsed.Episode)
	if !ok {
		return []SearchRequest{req}
	}
	alternate := req
	alternate.IMDBID = mapped.IMDBID
	alternate.Parsed.Title = mapped.ReleaseTitle
	alternate.Parsed.Year = mapped.Year
	alternate.Parsed.Season = mapped.Season
	alternate.Parsed.Episode = mapped.Episode
	alternate.Query = fmt.Sprintf("%s S%02dE%02d", mapped.ReleaseTitle, mapped.Season, mapped.Episode)
	if imdbBased {
		// An absent catalog IMDb ID has no stream endpoint. The mapped endpoint
		// can return both catalog- and anthology-named releases.
		if strings.TrimSpace(req.IMDBID) == "" {
			return []SearchRequest{alternate}
		}
		if strings.TrimSpace(req.IMDBID) == alternate.IMDBID {
			return []SearchRequest{alternate}
		}
	} else if strings.EqualFold(strings.TrimSpace(req.Query), alternate.Query) {
		return []SearchRequest{req}
	}
	return []SearchRequest{req, alternate}
}
