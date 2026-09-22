package mediaidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Discovery is limited to missing-IMDb TMDB series. This bounded process-local
// cache retains matches for a week, no-matches for a day, failures five minutes.
type discoveryCacheEntry struct {
	mapping *SeriesCrossMapping
	expires time.Time
	outcome string
}
type discoveryService struct {
	mu                       sync.Mutex
	logf                     func(string, ...any)
	selections               map[string]time.Time
	entries                  map[string]discoveryCacheEntry
	pending                  map[string]chan struct{}
	client                   *http.Client
	tmdb, wikidata, cinemeta string
}

func newDiscoveryService() *discoveryService {
	return &discoveryService{logf: log.Printf, selections: map[string]time.Time{}, entries: map[string]discoveryCacheEntry{}, pending: map[string]chan struct{}{}, client: &http.Client{}, tmdb: "https://api.themoviedb.org/3", wikidata: "https://www.wikidata.org/wiki/Special:EntityData", cinemeta: "https://v3-cinemeta.strem.io"}
}

var discoveries = newDiscoveryService()
var catalogIDPattern = regexp.MustCompile(`^tmdb:tv:([1-9][0-9]*)$`)
var wikidataIDPattern = regexp.MustCompile(`^Q[1-9][0-9]*$`)
var imdbIDPattern = regexp.MustCompile(`^tt[0-9]+$`)

// DiscoverSeason warms the shared identity cache before sources build queries
// or filters. Failures leave normal searches and manual fallbacks intact.
func DiscoverSeason(ctx context.Context, apiKey, titleID, imdbID string, season int) {
	discoveries.ensure(ctx, apiKey, titleID, imdbID, season)
}
func discoveryKey(titleID string, season int) string { return fmt.Sprintf("%s/%d", titleID, season) }
func (d *discoveryService) lookup(titleID string, season, episode int) (AnthologyEpisode, bool) {
	d.mu.Lock()
	entry := d.entries[discoveryKey(titleID, season)]
	d.mu.Unlock()
	if entry.mapping == nil || time.Now().After(entry.expires) {
		return AnthologyEpisode{}, false
	}
	return lookupCrossMapping([]SeriesCrossMapping{*entry.mapping}, titleID, season, episode)
}
func (d *discoveryService) ensure(ctx context.Context, apiKey, titleID, imdbID string, season int) {
	id := catalogIDPattern.FindStringSubmatch(titleID)
	if strings.TrimSpace(imdbID) != "" || strings.TrimSpace(apiKey) == "" || len(id) != 2 || season < 1 || ctx.Err() != nil {
		return
	}
	key := discoveryKey(titleID, season)
	d.mu.Lock()
	if entry, ok := d.entries[key]; ok && time.Now().Before(entry.expires) {
		d.mu.Unlock()
		d.logf("[mediaidentity] discovery cache_hit title=%s season=%d outcome=%s", titleID, season, entry.outcome)
		return
	}
	if wait, ok := d.pending[key]; ok {
		d.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
		}
		return
	}
	if len(d.pending) >= 8 {
		d.mu.Unlock()
		return
	}
	done := make(chan struct{})
	d.pending[key] = done
	d.mu.Unlock()
	d.logf("[mediaidentity] discovery started title=%s season=%d source=tmdb_wikidata_cinemeta", titleID, season)
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	mapping, err := d.discover(lookupCtx, apiKey, id[1], titleID, season)
	cancel()
	outcome := "no_match"
	ttl := 24 * time.Hour
	if err != nil {
		outcome = "failed"
		ttl = 5 * time.Minute
	} else if mapping != nil {
		outcome = "verified"
		ttl = 7 * 24 * time.Hour
	}
	if ctx.Err() != nil {
		outcome = "canceled"
	}
	if mapping != nil && outcome == "verified" {
		d.logf("[mediaidentity] discovery verified title=%s season=%d source=wikidata imdb=%s provider_season=%d verified_episodes=%d cache_ttl=%s", titleID, season, mapping.Provider.IMDBID, mapping.Provider.Season, mapping.EpisodeCount, ttl)
	} else {
		d.logf("[mediaidentity] discovery finished title=%s season=%d outcome=%s", titleID, season, outcome)
	}
	d.mu.Lock()
	if len(d.entries) >= 512 {
		oldestKey := ""
		var oldest time.Time
		for k, e := range d.entries {
			if oldestKey == "" || e.expires.Before(oldest) {
				oldestKey = k
				oldest = e.expires
			}
		}
		delete(d.entries, oldestKey)
	}
	if ctx.Err() == nil {
		d.entries[key] = discoveryCacheEntry{mapping: mapping, expires: time.Now().Add(ttl), outcome: outcome}
	}
	delete(d.pending, key)
	close(done)
	d.mu.Unlock()
}
func (d *discoveryService) get(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("invalid metadata request")
	}
	req.Header.Set("User-Agent", "MediaStorm/1.0 (series identity discovery)")
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("metadata request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metadata HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}
func (d *discoveryService) tmdbGet(ctx context.Context, key, path string, out any) error {
	return d.get(ctx, d.tmdb+path+"?api_key="+url.QueryEscape(key)+"&append_to_response=external_ids", out)
}

type externalIDs struct {
	IMDB     string `json:"imdb_id"`
	TVDB     int64  `json:"tvdb_id"`
	Wikidata string `json:"wikidata_id"`
}
type tmdbIdentity struct {
	ID       int64       `json:"id"`
	External externalIDs `json:"external_ids"`
	Episodes []struct {
		Number int `json:"episode_number"`
	} `json:"episodes"`
}

func (d *discoveryService) discover(ctx context.Context, key, id, titleID string, season int) (*SeriesCrossMapping, error) {
	var show tmdbIdentity
	if err := d.tmdbGet(ctx, key, "/tv/"+id, &show); err != nil {
		return nil, err
	}
	// Missing search-request IDs alone are insufficient: confirm with TMDB.
	if strings.TrimSpace(show.External.IMDB) != "" {
		return nil, nil
	}
	path := fmt.Sprintf("/tv/%s/season/%d", id, season)
	var source tmdbIdentity
	if err := d.tmdbGet(ctx, key, path, &source); err != nil {
		return nil, err
	}
	if !wikidataIDPattern.MatchString(source.External.Wikidata) || len(source.Episodes) == 0 || len(source.Episodes) > 32 {
		return nil, nil
	}
	parent, ordinal, err := d.parent(ctx, source.External.Wikidata, id)
	if err != nil || parent == "" {
		return nil, err
	}
	entity, err := d.entity(ctx, parent)
	if err != nil {
		return nil, err
	}
	imdb := entity.singleString("P345")
	tvdb, _ := strconv.ParseInt(entity.singleString("P4835"), 10, 64)
	if !imdbIDPattern.MatchString(imdb) {
		return nil, nil
	}
	var provider struct {
		Meta struct {
			Name        string `json:"name"`
			ReleaseInfo string `json:"releaseInfo"`
			Videos      []struct {
				TVDB    int64 `json:"tvdb_id"`
				Season  int   `json:"season"`
				Episode int   `json:"episode"`
			} `json:"videos"`
		} `json:"meta"`
	}
	if err := d.get(ctx, d.cinemeta+"/meta/series/"+imdb+".json", &provider); err != nil {
		return nil, err
	}
	if strings.TrimSpace(provider.Meta.Name) == "" {
		return nil, nil
	}
	byID := map[int64]EpisodeCoordinate{}
	coordinates := map[EpisodeCoordinate]bool{}
	count := 0
	for _, v := range provider.Meta.Videos {
		if v.Season != ordinal || v.Episode < 1 {
			continue
		}
		count++
		coord := EpisodeCoordinate{v.Season, v.Episode}
		if coordinates[coord] {
			return nil, nil
		}
		coordinates[coord] = true
		if v.TVDB > 0 {
			if _, dup := byID[v.TVDB]; dup {
				return nil, nil
			}
			byID[v.TVDB] = coord
		}
	}
	if count != len(source.Episodes) {
		return nil, nil
	}
	overrides := map[int]EpisodeCoordinate{}
	used := map[EpisodeCoordinate]bool{}
	for _, ep := range source.Episodes {
		if ep.Number < 1 || ep.Number > len(source.Episodes) {
			return nil, nil
		}
		if _, dup := overrides[ep.Number]; dup {
			return nil, nil
		}
		var episode tmdbIdentity
		if err := d.tmdbGet(ctx, key, fmt.Sprintf("%s/episode/%d", path, ep.Number), &episode); err != nil {
			return nil, err
		}
		coord, ok := byID[episode.External.TVDB]
		if !ok || used[coord] {
			return nil, nil
		}
		used[coord] = true
		overrides[ep.Number] = coord
	}
	year := 0
	if len(provider.Meta.ReleaseInfo) >= 4 {
		year, _ = strconv.Atoi(provider.Meta.ReleaseInfo[:4])
	}
	return &SeriesCrossMapping{TitleID: titleID, CatalogSeason: season, FirstEpisode: 1, EpisodeCount: len(source.Episodes), Provider: AnthologyEpisode{IMDBID: imdb, TVDBID: tvdb, ReleaseTitle: provider.Meta.Name, Year: year, Season: ordinal, Episode: 1, SeasonEpisodeCount: count}, EpisodeOverrides: overrides}, nil
}
