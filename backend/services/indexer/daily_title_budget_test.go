package indexer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"novastream/config"
	"novastream/internal/providerbreaker"
	"novastream/models"
	"novastream/services/debrid"
)

func TestDailySearchReservesBothLanguageTitlesDespiteSuccessAndSmallBudget(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		queries = append(queries, q)
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<rss><channel><item><title>%s.2026.09.27.1080p.WEB-DL</title><guid>%s</guid></item></channel></rss>`, q, q)
	}))
	defer server.Close()
	svc := &Service{httpc: server.Client(), providerBreaker: providerbreaker.New()}
	settings := config.Settings{
		Indexers:  []config.IndexerConfig{{Name: "Fixture", URL: server.URL, Type: "newznab", Enabled: true}},
		Streaming: config.StreamingSettings{MaxDailyUsenetQueries: 1},
	}
	opts := SearchOptions{Query: "Rojst S01E01", MediaType: "series", IsDaily: true, TargetAirDate: "2026-09-27", SkipFilter: true, requiredSearchTitles: []string{"The Mire"}}
	results, err := svc.searchDailyUsenet(t.Context(), settings, opts, debrid.ParseQuery(opts.Query), []string{"The Mire"}, []string{"The Mire"}, models.FilterSettings{}, true)
	if err != nil || len(results) != 2 {
		t.Fatalf("want results from both language queries, got %v, %v", results, err)
	}
	if len(queries) != 2 || !slices.Contains(queries, "Rojst") || !slices.Contains(queries, "The Mire") {
		t.Fatalf("required queries missing: %v", queries)
	}
}
