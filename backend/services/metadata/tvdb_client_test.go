package metadata

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"novastream/internal/mdblisturl"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchMDBListJSONRejectsRedirectsOutsideCanonicalMDBListPaths(t *testing.T) {
	requests := 0
	client := &tvdbClient{httpc: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host != "mdblist.com" {
			t.Fatalf("redirect transport reached unsafe host %q", req.URL.Host)
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://169.254.169.254/latest/meta-data"}},
			Body:       io.NopCloser(bytes.NewBuffer(nil)),
			Request:    req,
		}, nil
	})}}

	var destination []mdblistItem
	err := client.fetchMDBListJSON("https://mdblist.com/lists/user/list/json", &destination)
	if !errors.Is(err, mdblisturl.ErrUnsafeRedirect) {
		t.Fatalf("fetch error = %v, want unsafe redirect error", err)
	}
	if requests != 2 {
		t.Fatalf("canonical endpoint requests = %d, want two retry attempts", requests)
	}
}

func TestTVDBClientSetsAcceptLanguageHeader(t *testing.T) {
	var (
		mu        sync.Mutex
		captured  string
		loginDone bool
	)

	httpc := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			defer mu.Unlock()
			if req.URL.Path == "/v4/login" {
				loginDone = true
				body := bytes.NewBufferString(`{"data":{"token":"abc"}}`)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
			}
			captured = req.Header.Get("Accept-Language")
			if req.Header.Get("Authorization") == "" {
				t.Fatalf("expected bearer token on authorized request")
			}
			body := bytes.NewBufferString(`{"ok":true}`)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
		}),
	}

	client := newTVDBClient("apikey", "en", httpc, 24)
	client.minInterval = 0

	var dest map[string]any
	if err := client.doGET("https://api4.thetvdb.com/v4/test", nil, &dest); err != nil {
		t.Fatalf("doGET failed: %v", err)
	}
	if !loginDone {
		t.Fatalf("expected login request to occur")
	}
	if captured != "eng" {
		t.Fatalf("expected Accept-Language header 'eng', got %q", captured)
	}
}

func TestTVDBClientEpisodeTranslationCaching(t *testing.T) {
	var (
		mu               sync.Mutex
		translationCalls int
	)

	httpc := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			db := func(status int, body string) (*http.Response, error) {
				mu.Unlock()
				return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
			}
			switch req.URL.Path {
			case "/v4/login":
				return db(http.StatusOK, `{"data":{"token":"abc"}}`)
			case "/v4/episodes/123/translations/eng":
				translationCalls++
				return db(http.StatusOK, `{"data":{"language":"eng","name":"Episode Title","overview":"Episode Overview"}}`)
			default:
				mu.Unlock()
				t.Fatalf("unexpected request path: %s", req.URL.Path)
			}
			return nil, nil
		}),
	}

	client := newTVDBClient("apikey", "en", httpc, 24)
	client.minInterval = 0

	translation, err := client.episodeTranslation(123, "eng")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if translation == nil {
		t.Fatalf("expected translation, got nil")
	}
	if translation.Name != "Episode Title" || translation.Overview != "Episode Overview" {
		t.Fatalf("unexpected translation payload: %+v", translation)
	}

	// Second call should be served from cache
	translation, err = client.episodeTranslation(123, "eng")
	if err != nil {
		t.Fatalf("unexpected error on cache read: %v", err)
	}
	if translation == nil {
		t.Fatalf("expected cached translation, got nil")
	}
	if translationCalls != 1 {
		t.Fatalf("expected one translation HTTP call, got %d", translationCalls)
	}
}

func TestTVDBClientSeriesEpisodesBySeasonType(t *testing.T) {
	var (
		mu        sync.Mutex
		pageCalls []string
	)

	httpc := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			defer mu.Unlock()
			if req.URL.Path == "/v4/login" {
				body := bytes.NewBufferString(`{"data":{"token":"abc"}}`)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
			}
			if req.URL.Path == "/v4/series/42/episodes/official/eng" {
				page := req.URL.Query().Get("page")
				pageCalls = append(pageCalls, page)
				if req.Header.Get("Accept-Language") != "eng" {
					t.Fatalf("unexpected Accept-Language: %q", req.Header.Get("Accept-Language"))
				}
				var payload string
				switch page {
				case "", "0":
					payload = `{"data":{"episodes":[{"id":1,"seriesId":42,"name":"Episode 1","overview":"One"}]},"links":{"next":"/v4/series/42/episodes/official/eng?page=1"}}`
				case "1":
					payload = `{"data":{"episodes":[{"id":2,"seriesId":42,"name":"Episode 2","overview":"Two"}]},"links":{"next":null}}`
				default:
					t.Fatalf("unexpected page: %s", page)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(payload)), Header: make(http.Header)}, nil
			}
			t.Fatalf("unexpected request: %s", req.URL.String())
			return nil, nil
		}),
	}

	client := newTVDBClient("apikey", "en", httpc, 24)
	client.minInterval = 0

	episodes, err := client.seriesEpisodesBySeasonType(42, "official", "en")
	if err != nil {
		t.Fatalf("seriesEpisodesBySeasonType returned error: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(episodes))
	}
	if episodes[0].Name != "Episode 1" || episodes[1].Name != "Episode 2" {
		t.Fatalf("unexpected episodes: %+v", episodes)
	}
	if len(pageCalls) != 2 || pageCalls[0] != "0" || pageCalls[1] != "1" {
		t.Fatalf("unexpected page calls: %v", pageCalls)
	}
}
