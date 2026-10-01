package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"novastream/models"
	"novastream/services/letterboxd"
	"novastream/services/metadata"
)

func TestLetterboxdFacetsDoNotBlockFirstPage(t *testing.T) {
	service := &progressiveTestService{}
	client := letterboxd.NewClient()
	pages := 0
	client.SetHTTPClientForTest(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		pages++
		var body strings.Builder
		body.WriteString(`<html><head><meta name="description" content="A list of 500 films"></head><body>`)
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&body, `<div data-item-name="Movie %d (2020)" data-item-slug="movie-%d" data-target-link="/film/movie-%d/"></div>`, i, i, i)
		}
		body.WriteString(`<a class="next" href="/official/list/letterboxds-top-500-films/page/2/">Next</a></body></html>`)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body.String()))}, nil
	})})
	h := &MetadataHandler{Service: service, LetterboxdClient: client}
	request := httptest.NewRequest("GET", "/?listUrl=https://letterboxd.com/official/list/letterboxds-top-500-films/&limit=20&shelfPhase=cards&includeFacets=true&sortBy=default&filterMediaType=all", nil)
	recorder := httptest.NewRecorder()
	h.LetterboxdList(recorder, request)
	var response CustomListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || len(response.Items) != 20 || response.Total != 500 || !response.MetadataPending || service.cards != 20 || service.full != 0 || pages != 1 {
		t.Fatalf("first page blocked on index: status=%d cards=%d full=%d pages=%d response=%+v", recorder.Code, service.cards, service.full, pages, response)
	}
}

func TestLetterboxdTop500Live(t *testing.T) {
	key := os.Getenv("STRMR_TMDB_BENCH_API_KEY")
	if key == "" {
		t.Skip("opt-in live Letterboxd/TMDB probe")
	}
	service := metadata.NewService("", key, "eng", t.TempDir(), 24, false, metadata.MDBListConfig{})
	h := &MetadataHandler{Service: service, LetterboxdClient: letterboxd.NewClient()}
	q := url.Values{"listUrl": {"https://letterboxd.com/official/list/letterboxds-top-500-films/"}, "limit": {"20"}, "shelfPhase": {"cards"}, "includeFacets": {"true"}, "filterMediaType": {"all"}, "sortBy": {"default"}, "name": {"Top 500"}}
	for _, phase := range []string{"cold", "warm", "background"} {
		if phase == "background" {
			q.Set("shelfPhase", "complete")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		r := httptest.NewRequest("GET", "/?"+q.Encode(), nil).WithContext(ctx)
		w := httptest.NewRecorder()
		start := time.Now()
		h.LetterboxdList(w, r)
		cancel()
		var response CustomListResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal("invalid list response")
		}
		if w.Code != 200 || len(response.Items) != 20 || response.Total != 500 {
			t.Fatalf("status=%d items=%d total=%d", w.Code, len(response.Items), response.Total)
		}
		posters := 0
		for _, item := range response.Items {
			if item.Title.Poster != nil {
				posters++
			}
		}
		t.Logf("Top 500 %s: %d ms, %d cards, %d posters, total=%d, metadataPending=%v", phase, time.Since(start).Milliseconds(), len(response.Items), posters, response.Total, response.MetadataPending)
		if phase == "background" && (response.MetadataPending || len(response.Genres) == 0) {
			t.Fatal("background metadata/facets incomplete")
		}
	}
}

type letterboxdNumberedService struct{ progressiveTestService }

func (s *letterboxdNumberedService) GetShelfCards(ctx context.Context, source []metadata.CuratedItem) ([]models.TrendingItem, error) {
	items := append([]metadata.CuratedItem(nil), source...)
	for i := range items {
		fmt.Sscanf(items[i].Title, "Movie %d", &items[i].TMDBID)
	}
	return s.progressiveTestService.GetShelfCards(ctx, items)
}

func TestLetterboxdProgressiveFiltersBackfillAndKeepPagination(t *testing.T) {
	service := &letterboxdNumberedService{progressiveTestService: progressiveTestService{hideOdd: true}}
	client := letterboxd.NewClient()
	client.SetHTTPClientForTest(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body strings.Builder
		body.WriteString(`<html><meta name="description" content="A list of 60 films"><body>`)
		for i := 1; i <= 60; i++ {
			fmt.Fprintf(&body, `<div data-item-name="Movie %d (2020)" data-target-link="/film/movie-%d/"></div>`, i, i)
		}
		body.WriteString(`</body></html>`)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body.String()))}, nil
	})})
	h := &MetadataHandler{Service: service, LetterboxdClient: client}
	request := func(phase string, offset int) CustomListResponse {
		w := httptest.NewRecorder()
		h.LetterboxdList(w, httptest.NewRequest("GET", fmt.Sprintf("/?listUrl=https://letterboxd.com/official/list/test/&limit=20&offset=%d&shelfPhase=%s&includeFacets=true&hideUnreleased=true", offset, phase), nil))
		var result CustomListResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
			t.Fatalf("status=%d err=%v body=%s", w.Code, err, w.Body.String())
		}
		return result
	}
	first := request("cards", 0)
	if len(first.Items) != 20 || first.Items[0].Title.TMDBID != 2 || first.Items[19].Title.TMDBID != 40 || !first.TotalPending || first.UnfilteredTotal != 60 {
		t.Fatalf("first page: %+v", first)
	}
	last := request("cards", 20)
	if len(last.Items) != 10 || last.Items[0].Title.TMDBID != 42 || last.Total != 30 || last.TotalPending {
		t.Fatalf("last page: %+v", last)
	}
	full := request("complete", 0)
	if len(full.Items) != 20 || full.Total != 30 || full.TotalPending || service.full != 20 {
		t.Fatalf("completed page: %+v", full)
	}
}
