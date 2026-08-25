package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"novastream/internal/torboxrate"
)

type thumbnailRoundTripFunc func(*http.Request) (*http.Response, error)

func (f thumbnailRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestThumbnailSourceBridgeForwardsRangeAndStoredAuth(t *testing.T) {
	key := strings.Repeat("a", 24)
	bridge := &thumbnailSourceBridge{
		secret: "test-secret",
		sessions: map[string]thumbnailSourceSession{
			key: {sourceURL: "https://media.example/movie.mkv", authHeader: "Authorization: Basic stored\r\n"},
		},
	}
	bridge.client = &http.Client{Transport: thumbnailRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Range"); got != "bytes=100-199" {
			t.Fatalf("upstream Range = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "Basic stored" {
			t.Fatalf("upstream Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Range":  []string{"bytes 100-102/1000"},
				"Content-Length": []string{"3"},
			},
			Body: io.NopCloser(strings.NewReader("abc")),
		}, nil
	})}

	req := httptest.NewRequest(http.MethodGet, "/thumbnail-source/test-secret/"+key, nil)
	req.Header.Set("Range", "bytes=100-199")
	req.Header.Set("Authorization", "Bearer client-must-not-pass-through")
	rec := httptest.NewRecorder()
	bridge.serveHTTP(rec, req)

	if rec.Code != http.StatusPartialContent || rec.Body.String() != "abc" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 100-102/1000" {
		t.Fatalf("response Content-Range = %q", got)
	}
}

func TestThumbnailRedirectPolicyPreservesAuthOnlyWithinOrigin(t *testing.T) {
	original := &http.Request{
		URL:    &url.URL{Scheme: "https", Host: "webdav.example", Path: "/movie.mkv"},
		Header: http.Header{"Authorization": []string{"Basic stored"}},
	}

	sameOrigin := &http.Request{
		URL:    &url.URL{Scheme: "https", Host: "webdav.example", Path: "/redirected/movie.mkv"},
		Header: make(http.Header),
	}
	if err := thumbnailRedirectPolicy(sameOrigin, []*http.Request{original}); err != nil {
		t.Fatalf("same-origin redirect policy: %v", err)
	}
	if got := sameOrigin.Header.Get("Authorization"); got != "Basic stored" {
		t.Fatalf("same-origin Authorization = %q", got)
	}

	crossOrigin := &http.Request{
		URL:    &url.URL{Scheme: "https", Host: "cdn.example", Path: "/movie.mkv"},
		Header: make(http.Header),
	}
	if err := thumbnailRedirectPolicy(crossOrigin, []*http.Request{original}); err != nil {
		t.Fatalf("cross-origin redirect policy: %v", err)
	}
	if got := crossOrigin.Header.Get("Authorization"); got != "" {
		t.Fatalf("cross-origin Authorization = %q, want empty", got)
	}
}

func TestThumbnailSourceBridgeRetriesTorBox429AfterSharedCooldown(t *testing.T) {
	originalCooldown := torboxrate.Downloads
	torboxrate.Downloads = &torboxrate.Cooldown{}
	defer func() { torboxrate.Downloads = originalCooldown }()

	var hits int64
	bridge := &thumbnailSourceBridge{}
	bridge.client = &http.Client{Transport: thumbnailRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if hit := atomic.AddInt64(&hits, 1); hit == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Status:     "429 Too Many Requests",
				Header:     http.Header{"Retry-After": []string{"0"}},
				Body:       io.NopCloser(strings.NewReader("Too many requests, retry in 0s")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Status:     "206 Partial Content",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("frame")),
		}, nil
	})}

	response, err := bridge.doRequest(context.Background(), http.MethodGet, thumbnailSourceSession{
		sourceURL: "https://store-073.wnam.tb-cdn.io/movie.mkv",
	}, http.Header{"Range": []string{"bytes=0-9"}})
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	defer response.Body.Close()
	if body, readErr := io.ReadAll(response.Body); readErr != nil || string(body) != "frame" {
		t.Fatalf("response body = %q, err=%v; want frame", body, readErr)
	}
	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Fatalf("upstream hits = %d, want 2", got)
	}
}
