package debrid

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type zileanRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn zileanRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestZileanSearchSendsIMDBID(t *testing.T) {
	var gotIMDBID string
	client := &http.Client{Transport: zileanRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotIMDBID = r.URL.Query().Get("ImdbId")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`[]`)),
			Request:    r,
		}, nil
	})}

	scraper := NewZileanScraper("https://zilean.test", "Zilean", client)
	_, err := scraper.Search(context.Background(), SearchRequest{
		Query:  "Captain Star S01E01",
		IMDBID: "tt0143031",
		Parsed: ParsedQuery{Title: "Captain Star", Season: 1, Episode: 1, MediaType: MediaTypeSeries},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotIMDBID != "tt0143031" {
		t.Fatalf("ImdbId query = %q, want tt0143031", gotIMDBID)
	}
}

func TestZileanReleasedEpisodeFallsBackToSeason(t *testing.T) {
	var episodes []string
	client := &http.Client{Transport: zileanRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		episodes = append(episodes, r.URL.Query().Get("Episode"))
		body := `[]`
		if len(episodes) == 2 {
			body = `[{"raw_title":"Captain Star S01-S02 Complete","info_hash":"0123456789abcdef0123456789abcdef01234567","season":1}]`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	scraper := NewZileanScraper("https://zilean.test", "Zilean", client)
	results, err := scraper.Search(context.Background(), SearchRequest{
		Query:           "Captain Star S01E01",
		EpisodeReleased: true,
		Parsed: ParsedQuery{
			Title: "Captain Star", Season: 1, Episode: 1, MediaType: MediaTypeSeries,
		},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if strings.Join(episodes, "|") != "1|" {
		t.Fatalf("Episode parameters = %v, want [1 empty]", episodes)
	}
	if len(results) != 1 || !strings.Contains(results[0].Title, "Complete") {
		t.Fatalf("results = %+v, want season fallback", results)
	}
}

func TestZileanParseResponsePreservesIngestedAt(t *testing.T) {
	scraper := NewZileanScraper("https://zilean.test", "Zilean", nil)
	results, err := scraper.parseResponse([]byte(`[
		{
			"raw_title": "Movie.2026.1080p.WEB-DL",
			"size": "123456789",
			"info_hash": "0123456789abcdef0123456789abcdef01234567",
			"resolution": "1080p",
			"languages": ["en"],
			"ingested_at": "2026-07-30T18:34:00.123456Z"
		}
	]`))
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	want := time.Date(2026, time.July, 30, 18, 34, 0, 123456000, time.UTC)
	if !results[0].PublishDate.Equal(want) {
		t.Fatalf("PublishDate = %v, want %v", results[0].PublishDate, want)
	}
}

func TestZileanParseResponseIgnoresInvalidIngestedAt(t *testing.T) {
	scraper := NewZileanScraper("https://zilean.test", "Zilean", nil)
	results, err := scraper.parseResponse([]byte(`[
		{
			"raw_title": "Movie.2026.1080p.WEB-DL",
			"size": 123456789,
			"info_hash": "0123456789abcdef0123456789abcdef01234567",
			"ingested_at": "not-a-timestamp"
		}
	]`))
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].PublishDate.IsZero() {
		t.Fatalf("PublishDate = %v, want zero time", results[0].PublishDate)
	}
}

func TestZileanLocalizedMovieFallsBackWithoutIMDB(t *testing.T) {
	var queries []string
	client := &http.Client{Transport: zileanRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("Query") != "Vaiana 2" || q.Get("Year") != "2024" {
			t.Fatalf("lost title/year constraints: %s", q.Encode())
		}
		queries = append(queries, q.Get("ImdbId"))
		body := `[]`
		if q.Get("ImdbId") == "" {
			body = `[
   {"raw_title":"Vaiana 2 2024 PLDUB 1080p","info_hash":"1111111111111111111111111111111111111111","languages":["pl"]},
   {"raw_title":"Vaiana 2 2024 Wrong ID","info_hash":"2222222222222222222222222222222222222222","imdb_id":"tt99999999"}
  ]`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	scraper := NewZileanScraper("https://zilean.test", "Zilean", client)
	results, err := scraper.Search(context.Background(), SearchRequest{Query: "Vaiana 2", IMDBID: "tt13622970", Parsed: ParsedQuery{Title: "Vaiana 2", Year: 2024, MediaType: MediaTypeMovie}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(queries, "|") != "tt13622970|" {
		t.Fatalf("expected constrained then title-only query: %v", queries)
	}
	if len(results) != 1 || len(results[0].Languages) != 1 || results[0].Languages[0] != "pl" {
		t.Fatalf("Polish fallback missing or conflicting IMDb kept: %+v", results)
	}
}

func TestZileanMovieDoesNotRetrySuccessfulIMDBSearch(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: zileanRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"raw_title":"Moana 2 2024","info_hash":"1111111111111111111111111111111111111111"}]`)), Request: r}, nil
	})}
	scraper := NewZileanScraper("https://zilean.test", "Zilean", client)
	results, err := scraper.searchMovie(context.Background(), "Moana 2", 2024, "tt13622970")
	if err != nil || len(results) != 1 || calls != 1 {
		t.Fatalf("results=%d calls=%d err=%v", len(results), calls, err)
	}
}
