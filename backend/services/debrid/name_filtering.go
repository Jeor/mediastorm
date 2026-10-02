package debrid

import (
	"context"

	"novastream/utils/filter"
)

// nameFilteringScraper carries an explicit source preference through aggregation
// so normal searches and the search tester apply the same identity checks.
type nameFilteringScraper struct {
	Scraper
}

func (s *nameFilteringScraper) Search(ctx context.Context, req SearchRequest) ([]ScrapeResult, error) {
	// Daily scrapers normally inspect names while probing adjacent episodes.
	// Trust the primary requested episode only when name matching is disabled.
	req.IsDaily = false
	results, err := s.Scraper.Search(ctx, req)
	for i := range results {
		attrs := make(map[string]string, len(results[i].Attributes)+1)
		for key, value := range results[i].Attributes {
			attrs[key] = value
		}
		attrs[filter.SkipNameFilteringAttribute] = "true"
		results[i].Attributes = attrs
	}
	return results, err
}
