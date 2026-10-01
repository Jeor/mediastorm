package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"novastream/models"
)

func TestStremioShelfCatalogPagination(t *testing.T) {
	for _, test := range []struct {
		name         string
		pageSize     int
		supportsSkip bool
		omitTitle    bool
		repeatPage   bool
		titleOnly    bool
		total        int
		wantCount    int
		wantOffsets  []int
	}{
		{name: "undeclared 20 item pages", supportsSkip: true, total: 60, wantCount: 60, wantOffsets: []int{0, 20, 40, 60}},
		{name: "short middle page", supportsSkip: true, omitTitle: true, total: 60, wantCount: 59, wantOffsets: []int{0, 20, 40, 60}},
		{name: "declared page size with short first page", pageSize: 20, supportsSkip: true, omitTitle: true, total: 60, wantCount: 59, wantOffsets: []int{0, 20, 40, 60}},
		{name: "invalid declared page size", pageSize: 501, supportsSkip: true, total: 60, wantCount: 60, wantOffsets: []int{0, 20, 40, 60}},
		{name: "ignores skip", supportsSkip: true, repeatPage: true, total: 60, wantCount: 20, wantOffsets: []int{0, 20}},
		{name: "title only repeated page", supportsSkip: true, repeatPage: true, titleOnly: true, total: 60, wantCount: 20, wantOffsets: []int{0, 20}},
		{name: "does not advertise skip", total: 60, wantCount: 20, wantOffsets: []int{0}},
		{name: "empty catalog", supportsSkip: true, wantCount: 0, wantOffsets: []int{0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var offsets []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				offset := 0
				if _, extra, ok := strings.Cut(r.URL.Path, "/skip="); ok {
					var err error
					offset, err = strconv.Atoi(strings.TrimSuffix(extra, ".json"))
					if err != nil {
						t.Errorf("invalid skip URL %q", r.URL.Path)
					}
				}
				offsets = append(offsets, offset)
				if test.repeatPage {
					offset = 0
				}
				var metas []stremioMeta
				for i := offset; i < offset+20 && i < test.total; i++ {
					// With a declared size, exercise a short first page. Otherwise
					// shorten the second page to test the inferred skip stride.
					if test.omitTitle && ((test.pageSize == 20 && i == 5) || (test.pageSize == 0 && i == 25)) {
						continue
					}
					meta := stremioMeta{ID: fmt.Sprintf("tt%07d", i+1), Type: "movie", Name: fmt.Sprintf("Movie %d", i+1)}
					if test.titleOnly {
						meta.ID = ""
					}
					metas = append(metas, meta)
				}
				_ = json.NewEncoder(w).Encode(stremioCatalogResponse{Metas: metas})
			}))
			defer server.Close()
			catalog := stremioCatalogDef{Type: "movie", ID: "movies", PageSize: test.pageSize}
			if test.supportsSkip {
				catalog.Extra = []stremioExtraProp{{Name: "skip"}}
			}
			metas, err := fetchStremioShelfCatalog(context.Background(), server.Client(), server.URL, catalog)
			if err != nil {
				t.Fatal(err)
			}
			if len(metas) != test.wantCount {
				t.Fatalf("catalog items=%d, want %d", len(metas), test.wantCount)
			}
			if !reflect.DeepEqual(offsets, test.wantOffsets) {
				t.Fatalf("requested offsets=%v, want %v", offsets, test.wantOffsets)
			}
			if test.wantCount == 59 && metas[len(metas)-1].ID != "tt0000060" {
				t.Fatal("pagination skipped titles after the short page")
			}
		})
	}
}

func TestStremioShelfCanceledPaginationDoesNotCachePartialCatalog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelNextPage := true
	handler := NewMetadataHandler(&fakeMetadataService{}, nil)
	handler.stremioHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch req.URL.Path {
		case "/manifest.json":
			body = `{"id":"test.cancel","catalogs":[{"type":"movie","id":"movies","extra":[{"name":"skip"}]}]}`
		case "/catalog/movie/movies.json":
			body = `{"metas":[{"id":"tt0000001"},{"id":"tt0000002"}]}`
		case "/catalog/movie/movies/skip=2.json":
			if cancelNextPage {
				cancelNextPage = false
				cancel()
				return nil, req.Context().Err()
			}
			body = `{"metas":[{"id":"tt0000003"}]}`
		default:
			body = `{"metas":[]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	_, err := handler.loadStremioShelfCatalog(ctx, "https://addon.example/manifest.json", "movie", "movies")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context cancellation", err)
	}
	if len(handler.stremioCatalogCache) != 0 {
		t.Fatal("canceled fetch cached a partial catalog")
	}
	metas, err := handler.loadStremioShelfCatalog(context.Background(), "https://addon.example/manifest.json", "movie", "movies")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 3 {
		t.Fatalf("retried catalog items=%d, want 3", len(metas))
	}
}

func TestStremioListLimitWithUndeclaredPageSize(t *testing.T) {
	var source []stremioMeta
	var enriched []models.TrendingItem
	for i := 0; i < 520; i++ {
		id := fmt.Sprintf("tt%07d", i+1)
		source = append(source, stremioMeta{ID: id, Type: "movie", Name: id})
		if i < stremioShelfMaxCatalogItems {
			enriched = append(enriched, models.TrendingItem{Rank: i + 1, Title: models.Title{ID: id, Name: id, MediaType: "movie"}})
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = w.Write([]byte(`{"id":"test.nuvio","catalogs":[{"type":"movie","id":"movies","extra":[{"name":"skip"}]}]}`))
			return
		}
		offset := 0
		if _, extra, ok := strings.Cut(r.URL.Path, "/skip="); ok {
			offset, _ = strconv.Atoi(strings.TrimSuffix(extra, ".json"))
		}
		if offset >= len(source) {
			_ = json.NewEncoder(w).Encode(stremioCatalogResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode(stremioCatalogResponse{Metas: source[offset:min(offset+20, len(source))]})
	}))
	defer server.Close()
	service := &fakeMetadataService{curatedResp: enriched}
	handler := NewMetadataHandler(service, nil)
	handler.stremioHTTPClient = server.Client()
	for _, limit := range []int{50, 100, 500, 0} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/list?manifestUrl=%s/manifest.json&catalogType=movie&catalogId=movies&limit=%d", server.URL, limit), nil)
			response := httptest.NewRecorder()
			handler.StremioList(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var payload CustomListResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			want := limit
			if want == 0 {
				want = stremioShelfMaxCatalogItems
			}
			if len(payload.Items) != want || payload.Total != stremioShelfMaxCatalogItems || len(service.lastCuratedItems) != stremioShelfMaxCatalogItems {
				t.Fatalf("items=%d total=%d source=%d, want %d/%d/%d", len(payload.Items), payload.Total, len(service.lastCuratedItems), want, stremioShelfMaxCatalogItems, stremioShelfMaxCatalogItems)
			}
		})
	}
}
