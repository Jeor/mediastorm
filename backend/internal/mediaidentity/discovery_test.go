package mediaidentity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureDiscovery(t *testing.T, mutate func(string, string) string) (*discoveryService, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	d := newDiscoveryService()
	fixtures := map[string]string{
		"/3/tv/123":                          `{"external_ids":{"imdb_id":""}}`,
		"/3/tv/123/season/1":                 `{"external_ids":{"wikidata_id":"Q100"},"episodes":[{"episode_number":1},{"episode_number":2}]}`,
		"/wiki/Special:EntityData/Q100.json": `{"entities":{"Q100":{"claims":{"P4983":[{"mainsnak":{"datavalue":{"value":"123"}}}],"P179":[{"mainsnak":{"datavalue":{"value":{"id":"Q200"}}},"qualifiers":{"P1545":[{"datavalue":{"value":"4"}}]}}]}}}}`,
		"/wiki/Special:EntityData/Q200.json": `{"entities":{"Q200":{"claims":{"P345":[{"mainsnak":{"datavalue":{"value":"tt1234"}}}],"P4835":[{"mainsnak":{"datavalue":{"value":"999"}}}]}}}}`,
		"/meta/series/tt1234.json":           `{"meta":{"name":"Example Anthology","releaseInfo":"2022–","videos":[{"season":4,"episode":1,"tvdb_id":101},{"season":4,"episode":2,"tvdb_id":102}]}}`,
		"/3/tv/123/season/1/episode/1":       `{"external_ids":{"tvdb_id":101}}`,
		"/3/tv/123/season/1/episode/2":       `{"external_ids":{"tvdb_id":102}}`,
	}
	d.client = &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body, ok := fixtures[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s", r.URL.Path)
			body = `{}`
		}
		if mutate != nil {
			body = mutate(r.URL.Path, body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	return d, calls
}
func TestDiscoverySkipsIdentifiedAndIneligibleSeries(t *testing.T) {
	d, calls := fixtureDiscovery(t, nil)
	for _, tc := range []struct {
		key, id, imdb string
		season        int
	}{
		{"key", "tmdb:tv:123", "tt1234", 1}, {"key", "tmdb:movie:123", "", 1}, {"key", "tvdb:123", "", 1}, {"", "tmdb:tv:123", "", 1}, {"key", "tmdb:tv:123", "", 0},
	} {
		d.ensure(context.Background(), tc.key, tc.id, tc.imdb, tc.season)
	}
	if calls.Load() != 0 {
		t.Fatal("ineligible requests contacted metadata")
	}
}
func TestDiscoveryVerifiesWholeSeasonAndCaches(t *testing.T) {
	d, calls := fixtureDiscovery(t, nil)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { d.ensure(context.Background(), "key", "tmdb:tv:123", "", 1) })
	}
	wg.Wait()
	if calls.Load() != 7 {
		t.Fatalf("expected one shared discovery (7 calls), got %d", calls.Load())
	}
	for episode := 1; episode <= 2; episode++ {
		got, ok := d.lookup("tmdb:tv:123", 1, episode)
		if !ok || got.IMDBID != "tt1234" || got.Season != 4 || got.Episode != episode || got.Year != 2022 || got.TVDBID != 999 || got.SeasonEpisodeCount != 2 {
			t.Fatalf("bad mapping: %+v %v", got, ok)
		}
	}
	if _, ok := d.lookup("tmdb:tv:123", 1, 3); ok {
		t.Fatal("unverified episode mapped")
	}
	if _, ok := d.lookup("tmdb:tv:124", 1, 1); ok {
		t.Fatal("unrelated series mapped")
	}
}
func TestDiscoveryRejectsIncompleteAmbiguousOrMismatchedData(t *testing.T) {
	for _, tc := range []struct{ name, path, from, to string }{
		{"show already identified", "/3/tv/123", `"imdb_id":""`, `"imdb_id":"tt900"`},
		{"no season wikidata", "/3/tv/123/season/1", `"Q100"`, `""`},
		{"wrong source", "/wiki/Special:EntityData/Q100.json", `"value":"123"`, `"value":"456"`},
		{"deprecated relationship", "/wiki/Special:EntityData/Q100.json", `"P179":[{`, `"P179":[{"rank":"deprecated",`},
		{"ambiguous parent id", "/wiki/Special:EntityData/Q200.json", `"P345":[`, `"P345":[{"mainsnak":{"datavalue":{"value":"tt900"}}},`},
		{"wrong ordinal", "/wiki/Special:EntityData/Q100.json", `"value":"4"`, `"value":"3"`},
		{"last episode mismatch", "/3/tv/123/season/1/episode/2", `102`, `9999`},
		{"missing episode id", "/3/tv/123/season/1/episode/2", `102`, `null`},
		{"duplicate provider id", "/meta/series/tt1234.json", `102`, `101`},
		{"duplicate provider coordinate", "/meta/series/tt1234.json", `"episode":2`, `"episode":1`},
		{"duplicate source episode", "/3/tv/123/season/1", `"episode_number":2`, `"episode_number":1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, calls := fixtureDiscovery(t, func(path, body string) string {
				if path == tc.path {
					return strings.ReplaceAll(body, tc.from, tc.to)
				}
				return body
			})
			d.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
			if _, ok := d.lookup("tmdb:tv:123", 1, 1); ok {
				t.Fatal("unverified mapping accepted")
			}
			before := calls.Load()
			d.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
			if calls.Load() != before {
				t.Fatal("negative result not cached")
			}
			if tc.name == "show already identified" && calls.Load() != 1 {
				t.Fatal("identified show triggered downstream discovery")
			}
		})
	}
}
func TestDiscoveryCacheExpiryAndFailure(t *testing.T) {
	d, calls := fixtureDiscovery(t, nil)
	d.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
	entry := d.entries["tmdb:tv:123/1"]
	entry.expires = time.Now().Add(-time.Second)
	d.entries["tmdb:tv:123/1"] = entry
	if _, ok := d.lookup("tmdb:tv:123", 1, 1); ok {
		t.Fatal("expired mapping used")
	}
	d.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
	if calls.Load() != 14 {
		t.Fatal("expired entry not refreshed")
	}
	failed := newDiscoveryService()
	var count int
	failed.client = &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	failed.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
	failed.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
	if count != 1 || time.Until(failed.entries["tmdb:tv:123/1"].expires) > 6*time.Minute {
		t.Fatal("failure backoff missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failed.ensure(ctx, "key", "tmdb:tv:124", "", 1)
	if count != 1 {
		t.Fatal("canceled request contacted metadata")
	}
}
func TestDiscoveryRetainsExactReorderedEpisodeCoordinates(t *testing.T) {
	d, _ := fixtureDiscovery(t, func(path, body string) string {
		if path == "/meta/series/tt1234.json" {
			return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(body, `101`, `999`), `102`, `101`), `999`, `102`)
		}
		return body
	})
	d.ensure(context.Background(), "key", "tmdb:tv:123", "", 1)
	got, ok := d.lookup("tmdb:tv:123", 1, 1)
	if !ok || got.Episode != 2 {
		t.Fatalf("exact coordinates lost: %+v", got)
	}
}
func TestDiscoveredMappingAvailableToSharedConsumers(t *testing.T) {
	old := discoveries
	d, _ := fixtureDiscovery(t, nil)
	discoveries = d
	t.Cleanup(func() { discoveries = old })
	DiscoverSeason(context.Background(), "key", "tmdb:tv:123", "", 1)
	got, ok := KnownAnthologyEpisode("tmdb:tv:123", 1, 2)
	if !ok || got.Season != 4 {
		t.Fatal("shared lookup did not see discovery")
	}
	// Existing manually verified mapping remains available on a lookup failure.
	got, ok = KnownAnthologyEpisode("tmdb:tv:299939", 1, 1)
	if !ok || got.IMDBID != "tt13207736" {
		t.Fatal("manual fallback lost")
	}
}

// Guard the fixture syntax so malformed JSON cannot masquerade as a no-match.
func TestWikidataStatementDecoding(t *testing.T) {
	var e wikidataEntity
	if err := json.Unmarshal([]byte(`{"claims":{"P345":[{"mainsnak":{"datavalue":{"value":"tt1234"}}}]}}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.singleString("P345") != "tt1234" {
		t.Fatal("IMDb claim missing")
	}
}
