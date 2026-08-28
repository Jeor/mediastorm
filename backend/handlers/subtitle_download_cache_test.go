package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testSubtitleVTT = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nTest subtitle\n"

func requestSubtitleDownload(t *testing.T, handler *SubtitlesHandler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/subtitles/download?subtitleId=subtitle-1&provider=test&title=Example&year=2026&language=eng", nil)
	recorder := httptest.NewRecorder()
	handler.Download(recorder, req)
	return recorder
}

func TestSubtitlesHandlerDownloadCachesSuccessfulVTT(t *testing.T) {
	handler := NewSubtitlesHandler()
	var calls atomic.Int32
	handler.downloadRunner = func(SubtitleDownloadParams) ([]byte, error) {
		calls.Add(1)
		return []byte(testSubtitleVTT), nil
	}

	for i := 0; i < 2; i++ {
		response := requestSubtitleDownload(t, handler)
		if response.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, response.Code, http.StatusOK)
		}
		if response.Body.String() != testSubtitleVTT {
			t.Fatalf("request %d body = %q, want %q", i+1, response.Body.String(), testSubtitleVTT)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("download runner calls = %d, want 1", got)
	}
}

func TestSubtitlesHandlerDownloadCoalescesConcurrentRequests(t *testing.T) {
	handler := NewSubtitlesHandler()
	var calls atomic.Int32
	runnerStarted := make(chan struct{})
	releaseRunner := make(chan struct{})
	var startOnce sync.Once
	handler.downloadRunner = func(SubtitleDownloadParams) ([]byte, error) {
		calls.Add(1)
		startOnce.Do(func() { close(runnerStarted) })
		<-releaseRunner
		return []byte(testSubtitleVTT), nil
	}

	const requestCount = 12
	startRequests := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, requestCount)
	for i := 0; i < requestCount; i++ {
		go func() {
			<-startRequests
			responses <- requestSubtitleDownload(t, handler)
		}()
	}

	close(startRequests)
	select {
	case <-runnerStarted:
	case <-time.After(time.Second):
		t.Fatal("download runner did not start")
	}
	// Keep the provider request in flight long enough for the other handlers to
	// join the singleflight call.
	time.Sleep(50 * time.Millisecond)
	close(releaseRunner)

	for i := 0; i < requestCount; i++ {
		select {
		case response := <-responses:
			if response.Code != http.StatusOK || response.Body.String() != testSubtitleVTT {
				t.Fatalf("response = status %d body %q", response.Code, response.Body.String())
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for coalesced response")
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("download runner calls = %d, want 1", got)
	}
}

func TestSubtitlesHandlerDownloadDoesNotCacheFailuresOrNonVTT(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		handler := NewSubtitlesHandler()
		var calls atomic.Int32
		handler.downloadRunner = func(SubtitleDownloadParams) ([]byte, error) {
			calls.Add(1)
			return nil, errors.New("provider unavailable")
		}

		for i := 0; i < 2; i++ {
			if response := requestSubtitleDownload(t, handler); response.Code != http.StatusInternalServerError {
				t.Fatalf("request %d status = %d, want %d", i+1, response.Code, http.StatusInternalServerError)
			}
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("download runner calls = %d, want 2", got)
		}
	})

	t.Run("non-vtt", func(t *testing.T) {
		handler := NewSubtitlesHandler()
		var calls atomic.Int32
		handler.downloadRunner = func(SubtitleDownloadParams) ([]byte, error) {
			calls.Add(1)
			return []byte(`{"error":"no subtitle"}`), nil
		}

		for i := 0; i < 2; i++ {
			if response := requestSubtitleDownload(t, handler); response.Code != http.StatusOK {
				t.Fatalf("request %d status = %d, want %d", i+1, response.Code, http.StatusOK)
			}
		}
		if got := calls.Load(); got != 2 {
			t.Fatalf("download runner calls = %d, want 2", got)
		}
	})
}

func TestSubtitleDownloadCacheExpiresAndEvictsOldestEntry(t *testing.T) {
	now := time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)
	cache := newSubtitleDownloadCache(time.Minute, 2)
	cache.now = func() time.Time { return now }

	cache.set("first", []byte("one"))
	now = now.Add(time.Second)
	cache.set("second", []byte("two"))
	now = now.Add(time.Second)
	cache.set("third", []byte("three"))

	if _, ok := cache.get("first"); ok {
		t.Fatal("oldest entry was not evicted")
	}
	if value, ok := cache.get("third"); !ok || string(value) != "three" {
		t.Fatalf("newest entry = %q, %t; want %q, true", value, ok, "three")
	}

	now = now.Add(time.Minute)
	if _, ok := cache.get("third"); ok {
		t.Fatal("expired entry was returned")
	}
}
