package mediaidentity

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mappingTransport func(*http.Request) (*http.Response, error)

func (f mappingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type memoryMappings struct {
	mu     sync.Mutex
	values map[string]MappingSnapshot
}

func (m *memoryMappings) Get(_ context.Context, k string) (*MappingSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[k]
	if !ok {
		return nil, nil
	}
	return &v, nil
}
func (m *memoryMappings) Put(_ context.Context, v MappingSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[v.Key] = v
	return nil
}

const cacheXML = `<anime-list><anime anidbid="1" tmdbtv="1" tmdbseason="1" tmdboffset="12" tvdbid="2" defaulttvdbseason="2"/></anime-list>`

func mappingResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Etag": []string{`"v1"`}}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestMappingCachePersistsAndRevalidates(t *testing.T) {
	repo := &memoryMappings{values: map[string]MappingSnapshot{}}
	var calls atomic.Int32
	client := &http.Client{Transport: mappingTransport(func(r *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		if n > 1 {
			if r.Header.Get("If-None-Match") != `"v1"` {
				t.Error("missing ETag validator")
			}
			return mappingResponse(304, ""), nil
		}
		return mappingResponse(200, cacheXML), nil
	})}
	s := NewEpisodeMappingService(repo, client)
	s.load(t.Context(), "anime-lists", s.animeURL, parseAnimeMappings)
	restored := NewEpisodeMappingService(repo, client)
	restored.load(t.Context(), "anime-lists", restored.animeURL, parseAnimeMappings)
	if calls.Load() != 1 {
		t.Fatal("fresh persistent snapshot fetched again")
	}
	old := restored.entries["anime-lists"]
	old.snapshot.CheckedAt = time.Now().Add(-48 * time.Hour)
	restored.refresh(t.Context(), "anime-lists", restored.animeURL, old, parseAnimeMappings)
	if calls.Load() != 2 || restored.entries["anime-lists"].data == nil {
		t.Fatal("304 lost dataset")
	}
	snapshot, _ := repo.Get(t.Context(), "anime-lists")
	if time.Since(snapshot.CheckedAt) > time.Minute {
		t.Fatal("304 not persisted")
	}
}
func TestMappingCacheKeepsLastGoodOnInvalidResponse(t *testing.T) {
	for _, response := range []struct {
		code int
		body string
	}{{500, "unavailable"}, {200, "<html>bad</html>"}, {200, strings.Repeat("x", maxMappingBytes+1)}} {
		repo := &memoryMappings{values: map[string]MappingSnapshot{}}
		s := NewEpisodeMappingService(repo, &http.Client{Transport: mappingTransport(func(*http.Request) (*http.Response, error) { return mappingResponse(response.code, response.body), nil })})
		data, _ := parseAnimeMappings([]byte(cacheXML))
		old := mappingEntry{snapshot: MappingSnapshot{Key: "anime-lists", Body: []byte(cacheXML), CheckedAt: time.Now().Add(-48 * time.Hour)}, data: data}
		repo.Put(t.Context(), old.snapshot)
		s.refresh(t.Context(), "anime-lists", s.animeURL, old, parseAnimeMappings)
		if s.entries["anime-lists"].data == nil || s.entries["anime-lists"].retry.Before(time.Now()) {
			t.Fatal("last good data/backoff lost")
		}
		saved, _ := repo.Get(t.Context(), "anime-lists")
		if string(saved.Body) != cacheXML {
			t.Fatal("invalid upstream overwrote persisted data")
		}
	}
}
func TestMappingCacheCoalescesColdRequests(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	s := NewEpisodeMappingService(nil, &http.Client{Transport: mappingTransport(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return mappingResponse(200, cacheXML), nil
	})})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.load(t.Context(), "anime-lists", s.animeURL, parseAnimeMappings) }()
	}
	<-entered
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("got %d duplicate requests", calls.Load())
	}
}
func TestStaleMappingLookupDoesNotWaitForRefresh(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := NewEpisodeMappingService(nil, &http.Client{Transport: mappingTransport(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return mappingResponse(200, cacheXML), nil
	})})
	data, _ := parseAnimeMappings([]byte(cacheXML))
	s.entries["anime-lists"] = mappingEntry{snapshot: MappingSnapshot{Body: []byte(cacheXML), CheckedAt: time.Now().Add(-48 * time.Hour)}, data: data}
	s.load(t.Context(), "anime-lists", s.animeURL, parseAnimeMappings)
	<-entered
	if _, c, ok := s.tvdbEpisode("tmdb:tv:1", EpisodeCoordinate{1, 13}); !ok || c != (EpisodeCoordinate{2, 1}) {
		t.Fatal("stale lookup lost")
	}
	s.mu.Lock()
	done := s.pending["anime-lists"]
	s.mu.Unlock()
	close(release)
	<-done
}
func TestXEMComposesOnlyVerifiedCatalogBridge(t *testing.T) {
	s := NewEpisodeMappingService(nil, nil)
	data, _ := parseAnimeMappings([]byte(cacheXML))
	s.entries["anime-lists"] = mappingEntry{data: data}
	x, err := parseXEMMappings([]byte(`{"result":"success","data":[{"tvdb":{"season":2,"episode":1},"scene":{"season":3,"episode":1,"absolute":13}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s.entries["xem:2"] = mappingEntry{data: x}
	old := SetEpisodeMappingService(s)
	defer SetEpisodeMappingService(old)
	got := ReleaseEpisodeAliases("tmdb:tv:1", 1, 13)
	if len(got) != 2 || got[0].Season != 2 || got[1].Season != 3 || got[1].AbsoluteEpisode != 13 {
		t.Fatalf("aliases=%+v", got)
	}
	if got := ReleaseEpisodeAliases("tmdb:tv:999", 1, 13); len(got) != 0 {
		t.Fatal("guessed unknown TMDB->TVDB bridge")
	}
	if got := ReleaseEpisodeAliases("tvdb:series:2", 2, 1); len(got) != 2 || got[0].Season != 1 || got[0].Episode != 13 || got[1].Season != 3 {
		t.Fatalf("TVDB catalog aliases=%+v", got)
	}
	if _, ok := KnownAnthologyEpisode("tmdb:tv:1", 1, 13); ok {
		t.Fatal("release mapping changed scrobble identity")
	}
}
func TestXEMRejectsAmbiguousSplitAndCombinedEpisodes(t *testing.T) {
	for _, body := range []string{
		`{"result":"success","data":[{"tvdb":{"season":1,"episode":1},"scene":{"season":2,"episode":1}},{"tvdb":{"season":1,"episode":1},"scene":{"season":2,"episode":2}}]}`,
		`{"result":"success","data":[{"tvdb":{"season":1,"episode":1},"scene":{"season":2,"episode":1}},{"tvdb":{"season":1,"episode":2},"scene":{"season":2,"episode":1}}]}`,
	} {
		data, err := parseXEMMappings([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if len(data.(xemIndex)) != 0 {
			t.Fatalf("ambiguous map accepted: %+v", data)
		}
	}
	for _, body := range []string{`{"result":"failure","message":"no show with the tvdb_id 2"}`, `{"result":"success","data":[]}`} {
		data, err := parseXEMMappings([]byte(body))
		if err != nil || len(data.(xemIndex)) != 0 {
			t.Fatal("valid negative result rejected")
		}
	}
	if _, err := parseXEMMappings([]byte(`{"result":"failure","message":"database unavailable"}`)); err == nil {
		t.Fatal("upstream error treated as missing map")
	}
}

func TestXEMEnrichesSameAliasWithVerifiedAbsoluteNumber(t *testing.T) {
	s := NewEpisodeMappingService(nil, nil)
	data, _ := parseAnimeMappings([]byte(cacheXML))
	s.entries["anime-lists"] = mappingEntry{data: data}
	x, err := parseXEMMappings([]byte(`{"result":"success","data":[{"tvdb":{"season":2,"episode":1},"scene":{"season":2,"episode":1,"absolute":13}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s.entries["xem:2"] = mappingEntry{data: x}
	old := SetEpisodeMappingService(s)
	defer SetEpisodeMappingService(old)
	got := ReleaseEpisodeAliases("tmdb:tv:1", 1, 13)
	if len(got) != 1 || got[0].AbsoluteEpisode != 13 {
		t.Fatalf("verified absolute alias lost: %+v", got)
	}
	if got := ReleaseEpisodeAliases("tmdb:tv:1", 0, 13); len(got) != 0 {
		t.Fatal("special episode treated as regular")
	}
}

func TestPublishedKaijuXEMResponse(t *testing.T) {
	body, err := os.ReadFile("../mappingtest/testdata/kaiju-xem.json")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseXEMMappings(body)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parsed.(xemIndex)[EpisodeCoordinate{2, 1}]
	if !ok || got.Coordinate != (EpisodeCoordinate{2, 1}) || got.Absolute != 1 {
		t.Fatalf("published Kaiju mapping=%+v present=%t", got, ok)
	}
}

func TestXEMRejectsConflictingAbsoluteNumbers(t *testing.T) {
	data, err := parseXEMMappings([]byte(`{"result":"success","data":[{"tvdb":{"season":2,"episode":1},"scene":{"season":2,"episode":1,"absolute":1}},{"tvdb":{"season":2,"episode":1},"scene":{"season":2,"episode":1,"absolute":13}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(data.(xemIndex)) != 0 {
		t.Fatal("conflicting scene absolute numbering accepted")
	}
}

func TestXEMCannotBridgeThroughSpecials(t *testing.T) {
	anime, err := parseAnimeMappings([]byte(`<anime-list><anime anidbid="1" tmdbtv="1" tmdbseason="1" tvdbid="2" defaulttvdbseason="0"/></anime-list>`))
	if err != nil {
		t.Fatal(err)
	}
	xem, err := parseXEMMappings([]byte(`{"result":"success","data":[{"tvdb":{"season":0,"episode":1},"scene":{"season":2,"episode":1,"absolute":1}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s := NewEpisodeMappingService(nil, nil)
	s.entries["anime-lists"] = mappingEntry{data: anime}
	s.entries["xem:2"] = mappingEntry{data: xem}
	old := SetEpisodeMappingService(s)
	defer SetEpisodeMappingService(old)
	if got := ReleaseEpisodeAliases("tmdb:tv:1", 1, 1); len(got) != 0 {
		t.Fatalf("special-to-regular bridge accepted: %+v", got)
	}
}
