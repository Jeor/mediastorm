package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func progressiveStremioTestHandler(t *testing.T, total int, svc *progressiveTestService) (*MetadataHandler, string, *[]int) {
	t.Helper()
	var offsets []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = w.Write([]byte(`{"catalogs":[{"type":"movie","id":"trending","extra":[{"name":"skip"}]}]}`))
			return
		}
		offset := 0
		if _, extra, ok := strings.Cut(r.URL.Path, "/skip="); ok {
			offset, _ = strconv.Atoi(strings.TrimSuffix(extra, ".json"))
		}
		offsets = append(offsets, offset)
		metas := []stremioMeta{}
		for i := offset; i < min(offset+20, total); i++ {
			metas = append(metas, stremioMeta{ID: fmt.Sprintf("tmdb:%d", i+1), Type: "movie", Name: fmt.Sprint(i + 1), Poster: "https://images.example/poster.jpg"})
		}
		_ = json.NewEncoder(w).Encode(stremioCatalogResponse{Metas: metas})
	}))
	t.Cleanup(server.Close)
	h := NewMetadataHandler(svc, nil)
	h.stremioHTTPClient = server.Client()
	return h, server.URL + "/manifest.json", &offsets
}

func progressiveStremioTestRequest(manifestURL, phase string, extra url.Values) *http.Request {
	query := url.Values{"manifestUrl": {manifestURL}, "catalogType": {"movie"}, "catalogId": {"trending"}, "shelfPhase": {phase}, "limit": {"24"}}
	for k, v := range extra {
		query[k] = v
	}
	return httptest.NewRequest(http.MethodGet, "/list?"+query.Encode(), nil)
}

func stremioTestResponse(t *testing.T, h *MetadataHandler, r *http.Request) CustomListResponse {
	t.Helper()
	w := httptest.NewRecorder()
	h.StremioList(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response CustomListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestStremioHomeCardsReturnBeforeRemainingCatalogPages(t *testing.T) {
	svc := &progressiveTestService{}
	h, manifest, offsets := progressiveStremioTestHandler(t, 60, svc)
	first := stremioTestResponse(t, h, progressiveStremioTestRequest(manifest, "cards", nil))
	if len(first.Items) != 24 || !first.TotalPending || !first.MetadataPending || svc.full != 0 {
		t.Fatalf("first shelf=%+v enriched=%d", first, svc.full)
	}
	if !reflect.DeepEqual(*offsets, []int{0, 20}) {
		t.Fatalf("foreground downloaded remaining catalog pages: %v", *offsets)
	}
	full := stremioTestResponse(t, h, progressiveStremioTestRequest(manifest, "complete", nil))
	if full.Total != 60 || full.TotalPending || full.MetadataPending || svc.full != 24 {
		t.Fatalf("completion=%+v enriched=%d", full, svc.full)
	}
	if !reflect.DeepEqual(*offsets, []int{0, 20, 40, 60}) {
		t.Fatalf("completion did not resume cached prefix: %v", *offsets)
	}
	for i := range first.Items {
		if first.Items[i].Title.ID != full.Items[i].Title.ID {
			t.Fatal("completion changed card identity")
		}
	}
	// Completing a prefix must leave a complete reusable catalog, not 40 items.
	warm := stremioTestResponse(t, h, progressiveStremioTestRequest(manifest, "cards", nil))
	if warm.TotalPending || warm.Total != 60 || len(*offsets) != 4 {
		t.Fatalf("warm shelf=%+v requests=%v", warm, *offsets)
	}
}

func TestStremioHomeReadsAnotherPageWhenVisibilityFilteringNeedsIt(t *testing.T) {
	svc := &progressiveTestService{hideOdd: true}
	h, manifest, offsets := progressiveStremioTestHandler(t, 100, svc)
	response := stremioTestResponse(t, h, progressiveStremioTestRequest(manifest, "cards", url.Values{"hideUnreleased": {"true"}}))
	if len(response.Items) != 24 || !response.TotalPending || svc.full != 0 {
		t.Fatalf("filtered shelf=%+v enriched=%d", response, svc.full)
	}
	if !reflect.DeepEqual(*offsets, []int{0, 20, 40}) {
		t.Fatalf("filter backfill requests=%v", *offsets)
	}
	for i, item := range response.Items {
		if item.Title.TMDBID != int64((i+1)*2) {
			t.Fatal("filtered source order changed or unreleased card was published")
		}
	}
}

func TestStremioHomeEmptyFilteredCatalogStopsWithExactTotal(t *testing.T) {
	svc := &progressiveTestService{hideOdd: true}
	h, manifest, offsets := progressiveStremioTestHandler(t, 1, svc)
	response := stremioTestResponse(t, h, progressiveStremioTestRequest(manifest, "cards", url.Values{"hideUnreleased": {"true"}}))
	if len(response.Items) != 0 || response.Total != 0 || response.TotalPending || response.MetadataPending {
		t.Fatalf("empty filtered shelf=%+v", response)
	}
	if !reflect.DeepEqual(*offsets, []int{0, 1}) {
		t.Fatalf("requests=%v", *offsets)
	}
}

func TestStremioHomeAdvancedSortStillReadsCompleteCatalog(t *testing.T) {
	svc := &progressiveTestService{}
	h, manifest, offsets := progressiveStremioTestHandler(t, 60, svc)
	response := stremioTestResponse(t, h, progressiveStremioTestRequest(manifest, "cards", url.Values{"sortBy": {"name"}}))
	if response.Total != 60 || response.TotalPending || svc.full != 60 || len(*offsets) != 4 {
		t.Fatalf("full-list sort=%+v enriched=%d requests=%v", response, svc.full, *offsets)
	}
}

func TestStremioShelfPrefixCacheReturnsPrivateSnapshots(t *testing.T) {
	h, manifest, offsets := progressiveStremioTestHandler(t, 60, &progressiveTestService{})
	entry, err := h.loadStremioShelfCatalogPrefix(context.Background(), manifest, "movie", "trending", 24)
	if err != nil {
		t.Fatal(err)
	}
	entry.metas[0].Name = "mutated"
	entry, err = h.loadStremioShelfCatalogPrefix(context.Background(), manifest, "movie", "trending", 24)
	if err != nil || entry.metas[0].Name != "1" || len(*offsets) != 2 {
		t.Fatalf("cache mutation or refetch: %+v %v requests=%v", entry, err, *offsets)
	}
}

func TestStremioLateForegroundPrefixDoesNotReplaceCompletedCatalog(t *testing.T) {
	h := NewMetadataHandler(&progressiveTestService{}, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	var skip20Calls atomic.Int32
	h.stremioHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"catalogs":[{"type":"movie","id":"trending","extra":[{"name":"skip"}]}]}`
		if r.URL.Path != "/manifest.json" {
			offset := 0
			if _, extra, ok := strings.Cut(r.URL.Path, "/skip="); ok {
				offset, _ = strconv.Atoi(strings.TrimSuffix(extra, ".json"))
			}
			if offset == 20 && skip20Calls.Add(1) == 1 {
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			metas := []stremioMeta{}
			for i := offset; i < min(offset+20, 60); i++ {
				metas = append(metas, stremioMeta{ID: fmt.Sprintf("tmdb:%d", i+1)})
			}
			data, _ := json.Marshal(stremioCatalogResponse{Metas: metas})
			body = string(data)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	manifest := "https://addon.example/manifest.json"
	if _, err := h.loadStremioShelfCatalogPrefix(t.Context(), manifest, "movie", "trending", 20); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := h.loadStremioShelfCatalogPrefix(t.Context(), manifest, "movie", "trending", 24)
		finished <- err
	}()
	<-started
	full, err := h.loadStremioShelfCatalog(t.Context(), manifest, "movie", "trending")
	close(release)
	if err != nil || len(full) != 60 {
		t.Fatalf("background completion=%d err=%v", len(full), err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	entry, err := h.loadStremioShelfCatalogPrefix(t.Context(), manifest, "movie", "trending", 24)
	if err != nil || len(entry.metas) != 60 || entry.more {
		t.Fatalf("late prefix replaced completed cache: count=%d more=%v err=%v", len(entry.metas), entry.more, err)
	}
}
