package mediaidentity

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDiscoveryLogsVerificationCacheAndActualSource(t *testing.T) {
	d, _ := fixtureDiscovery(t, nil)
	var logs []string
	d.logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	old := discoveries
	discoveries = d
	t.Cleanup(func() { discoveries = old })
	DiscoverSeason(context.Background(), "secret-test-key", "tmdb:tv:123", "", 1)
	DiscoverSeason(context.Background(), "secret-test-key", "tmdb:tv:123", "", 1)
	for range 10 {
		KnownAnthologyEpisode("tmdb:tv:123", 1, 1)
		KnownAnthologyEpisode("tmdb:tv:299939", 1, 1)
	}
	output := strings.Join(logs, "\n")
	for _, want := range []string{"discovery started", "discovery verified title=tmdb:tv:123 season=1 source=wikidata imdb=tt1234 provider_season=4 verified_episodes=2", "cache_hit title=tmdb:tv:123 season=1 outcome=verified", "source=wikidata imdb=tt1234 provider=S04E01", "source=manual_fallback imdb=tt13207736 provider=S04E01"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in logs: %s", want, output)
		}
	}
	if strings.Count(output, "mapping selected") != 2 {
		t.Fatal("per-result identity lookups spam logs")
	}
	if strings.Contains(output, "secret-test-key") {
		t.Fatal("API key leaked")
	}
}
func TestDiscoveryLogsNoMatchAndFailure(t *testing.T) {
	for _, tc := range []struct{ body, outcome string }{{`{"external_ids":{"imdb_id":"tt900"}}`, "no_match"}, {`invalid-json`, "failed"}} {
		t.Run(tc.outcome, func(t *testing.T) {
			d, _ := fixtureDiscovery(t, func(path, body string) string {
				if path == "/3/tv/123" {
					return tc.body
				}
				return body
			})
			var logs []string
			d.logf = func(f string, args ...any) { logs = append(logs, fmt.Sprintf(f, args...)) }
			d.ensure(context.Background(), "secret-test-key", "tmdb:tv:123", "", 1)
			d.ensure(context.Background(), "secret-test-key", "tmdb:tv:123", "", 1)
			output := strings.Join(logs, "\n")
			if !strings.Contains(output, "discovery finished title=tmdb:tv:123 season=1 outcome="+tc.outcome) || !strings.Contains(output, "cache_hit title=tmdb:tv:123 season=1 outcome="+tc.outcome) {
				t.Fatal(output)
			}
			if strings.Contains(output, "secret-test-key") {
				t.Fatal("API key leaked")
			}
		})
	}
}
