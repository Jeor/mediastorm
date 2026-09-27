package indexer

import (
	"strings"

	"novastream/services/debrid"
)

// Try each required language title before stopping on a successful daily tier.
// Without localized titles, retain the existing ID/date format learning order.
func reserveDailyTitleQueries(candidates []dailyUsenetCandidate, opts SearchOptions, parsed debrid.ParsedQuery) ([]dailyUsenetCandidate, int) {
	if len(opts.requiredSearchTitles) == 0 {
		return candidates, 0
	}
	titles := combineFilterTitles([]string{parsed.Title}, opts.requiredSearchTitles)
	var reserved, remaining []dailyUsenetCandidate
	wanted := make(map[string]bool)
	for _, title := range titles {
		wanted["structured-title-date:"+strings.ToLower(title)] = true
	}
	for _, candidate := range candidates {
		if wanted[candidate.key] {
			reserved = append(reserved, candidate)
		} else {
			remaining = append(remaining, candidate)
		}
	}
	return append(reserved, remaining...), len(reserved)
}
