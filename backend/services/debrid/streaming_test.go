package debrid

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"novastream/config"
	"novastream/internal/torboxrate"
	"novastream/services/streaming"
)

type streamingMockProvider struct {
	name            string
	info            *TorrentInfo
	infoCalls       int64
	unrestrictCalls int64
}

func (m *streamingMockProvider) Name() string { return m.name }
func (m *streamingMockProvider) AddMagnet(context.Context, string) (*AddMagnetResult, error) {
	return &AddMagnetResult{ID: "torrent1"}, nil
}
func (m *streamingMockProvider) AddTorrentFile(context.Context, []byte, string) (*AddMagnetResult, error) {
	return &AddMagnetResult{ID: "torrent1"}, nil
}
func (m *streamingMockProvider) GetTorrentInfo(context.Context, string) (*TorrentInfo, error) {
	atomic.AddInt64(&m.infoCalls, 1)
	return m.info, nil
}
func (m *streamingMockProvider) SelectFiles(context.Context, string, string) error { return nil }
func (m *streamingMockProvider) DeleteTorrent(context.Context, string) error       { return nil }
func (m *streamingMockProvider) UnrestrictLink(_ context.Context, link string) (*UnrestrictResult, error) {
	atomic.AddInt64(&m.unrestrictCalls, 1)
	return &UnrestrictResult{DownloadURL: link}, nil
}
func (m *streamingMockProvider) CheckInstantAvailability(context.Context, string) (bool, error) {
	return true, nil
}

func TestStreamingProviderStreamsRelatedFileByTorrentPath(t *testing.T) {
	clpiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("clpi-data"))
	}))
	defer clpiServer.Close()

	providerName := "testprovider_related_file"
	mock := &streamingMockProvider{
		name: providerName,
		info: &TorrentInfo{
			ID: "torrent1",
			Files: []File{
				{ID: 180, Path: "Disc/BDMV/STREAM/00801.m2ts", Selected: 1},
				{ID: 337, Path: "Disc/BDMV/CLIPINF/00801.clpi", Selected: 1},
			},
			Links: []string{"unused-video-link", clpiServer.URL + "/00801.clpi"},
		},
	}

	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{Streaming: config.StreamingSettings{
		DebridProviders: []config.DebridProviderSettings{
			{Provider: providerName, APIKey: "test-key", Enabled: true},
		},
	}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	RegisterProvider(providerName, func(string) Provider { return mock })

	p := NewStreamingProvider(mgr)
	resp, err := p.StreamRelatedFile(
		context.Background(),
		"/debrid/"+providerName+"/torrent1/file/180/Disc/BDMV/STREAM/00801.m2ts",
		"Disc/BDMV/CLIPINF/00801.clpi",
	)
	if err != nil {
		t.Fatalf("StreamRelatedFile() error = %v", err)
	}
	defer resp.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read related file: %v", err)
	}
	if got, want := string(body), "clpi-data"; got != want {
		t.Fatalf("related body = %q, want %q", got, want)
	}
	if got := atomic.LoadInt64(&mock.infoCalls); got != 1 {
		t.Fatalf("GetTorrentInfo calls = %d, want 1", got)
	}
}

func TestStreamingProviderEvictsCachedURLAndRetriesOnRequestFailure(t *testing.T) {
	var freshHits int64
	freshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&freshHits, 1)
		if got := r.Header.Get("Range"); got != "bytes=0-4" {
			t.Errorf("Range = %q, want bytes=0-4", got)
		}
		w.Header().Set("Content-Range", "bytes 0-4/5")
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("hello"))
	}))
	defer freshServer.Close()

	deadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("dead cached URL should not receive a request after Close")
	}))
	deadURL := deadServer.URL + "/dead"
	deadServer.Close()

	providerName := "testprovider_stream_retry"
	mock := &streamingMockProvider{
		name: providerName,
		info: &TorrentInfo{
			ID:     "torrent1",
			Status: "downloaded",
			Files: []File{
				{ID: 0, Path: "Movie.mkv", Bytes: 5, Selected: 1},
			},
			Links: []string{freshServer.URL + "/fresh"},
		},
	}

	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{
		Streaming: config.StreamingSettings{
			DebridProviders: []config.DebridProviderSettings{
				{Provider: providerName, APIKey: "test-key", Enabled: true},
			},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	RegisterProvider(providerName, func(string) Provider { return mock })

	p := NewStreamingProvider(mgr)
	p.setCachedURL(cacheKeyFor("torrent1", "0"), deadURL, "Movie.mkv", 0, 0)

	resp, err := p.Stream(context.Background(), streaming.Request{
		Path:        "/debrid/" + providerName + "/torrent1/file/0/Movie.mkv",
		Method:      http.MethodGet,
		RangeHeader: "bytes=0-4",
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer resp.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", string(body))
	}
	if got := atomic.LoadInt64(&mock.unrestrictCalls); got != 1 {
		t.Fatalf("UnrestrictLink calls = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&freshHits); got != 1 {
		t.Fatalf("fresh server hits = %d, want 1", got)
	}
	if cachedURL, _, _, _, found := p.getCachedURL(cacheKeyFor("torrent1", "0")); !found || cachedURL != freshServer.URL+"/fresh" {
		t.Fatalf("cached URL = %q found=%t, want fresh URL", cachedURL, found)
	}
}

func TestStreamingProviderReturnsSourceErrorAfterCachedURLRetryFails(t *testing.T) {
	deadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := deadServer.URL + "/dead"
	deadServer.Close()

	providerName := "testprovider_stream_retry_failure"
	mock := &streamingMockProvider{
		name: providerName,
		info: &TorrentInfo{
			ID:     "torrent1",
			Status: "downloaded",
			Files: []File{
				{ID: 0, Path: "Movie.mkv", Bytes: 5, Selected: 1},
			},
			Links: []string{deadURL},
		},
	}

	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := mgr.Save(config.Settings{
		Streaming: config.StreamingSettings{
			DebridProviders: []config.DebridProviderSettings{
				{Provider: providerName, APIKey: "test-key", Enabled: true},
			},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	RegisterProvider(providerName, func(string) Provider { return mock })

	p := NewStreamingProvider(mgr)
	p.setCachedURL(cacheKeyFor("torrent1", "0"), deadURL, "Movie.mkv", 0, 0)

	_, err := p.Stream(context.Background(), streaming.Request{
		Path:        "/debrid/" + providerName + "/torrent1/file/0/Movie.mkv",
		Method:      http.MethodGet,
		RangeHeader: "bytes=0-4",
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want SourceError")
	}
	var sourceErr *SourceError
	if !errors.As(err, &sourceErr) {
		t.Fatalf("error type = %T, want SourceError; err=%v", err, err)
	}
	if !IsProviderUnavailableError(err) {
		t.Fatalf("IsProviderUnavailableError(%v) = false, want true", err)
	}
	if got := atomic.LoadInt64(&mock.unrestrictCalls); got != 1 {
		t.Fatalf("UnrestrictLink calls = %d, want 1", got)
	}
}

func TestStreamingProviderWaitsAndRetriesSameTorBoxURLOn429(t *testing.T) {
	originalCooldown := torboxrate.Downloads
	torboxrate.Downloads = &torboxrate.Cooldown{}
	defer func() { torboxrate.Downloads = originalCooldown }()

	var hits int64
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hit := atomic.AddInt64(&hits, 1)
		if got := request.Header.Get("Range"); got != "bytes=10-19" {
			t.Errorf("Range = %q, want bytes=10-19", got)
		}
		if hit == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Status:     "429 Too Many Requests",
				Header:     http.Header{"Retry-After": []string{"0"}},
				Body:       io.NopCloser(strings.NewReader("Too many requests, retry in 0s")),
			}, nil
		}
		return &http.Response{
			StatusCode:    http.StatusPartialContent,
			Status:        "206 Partial Content",
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader("hello")),
			ContentLength: 5,
		}, nil
	})}

	provider := &streamingMockProvider{name: "torbox"}
	p := &StreamingProvider{
		urlCache:   make(map[string]cachedURL),
		cacheTTL:   time.Minute,
		httpClient: client,
	}
	const downloadURL = "https://store-073.wnam.tb-cdn.io/movie.mkv"
	p.setCachedURL(cacheKeyFor("torrent1", "0"), downloadURL, "Movie.mkv", 0, 0)

	resp, err := p.streamWithProvider(context.Background(), streaming.Request{
		Method:      http.MethodGet,
		RangeHeader: "bytes=10-19",
	}, provider, "torrent1", "0", true)
	if err != nil {
		t.Fatalf("streamWithProvider() error = %v", err)
	}
	defer resp.Close()
	if body, readErr := io.ReadAll(resp.Body); readErr != nil || string(body) != "hello" {
		t.Fatalf("response body = %q, err=%v; want hello", body, readErr)
	}
	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Fatalf("upstream hits = %d, want 2", got)
	}
	if got := atomic.LoadInt64(&provider.unrestrictCalls); got != 0 {
		t.Fatalf("UnrestrictLink calls = %d, want 0", got)
	}
	if cached, _, _, _, found := p.getCachedURL(cacheKeyFor("torrent1", "0")); !found || cached != downloadURL {
		t.Fatalf("cached URL = %q found=%t, want original TorBox URL", cached, found)
	}
}
