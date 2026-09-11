package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"novastream/config"
)

type seekrTransport func(*http.Request) (*http.Response, error)

func (f seekrTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSeekrQuery(t *testing.T) {
	for _, tc := range []struct {
		input url.Values
		want  string
		valid bool
	}{
		{url.Values{"mediaType": {"movie"}, "imdbId": {"tt123"}}, "imdb_id", true},
		{url.Values{"mediaType": {"episode"}, "tmdbId": {"123"}, "season": {"0"}, "episode": {"1"}}, "show_tmdb_id", true},
		{url.Values{"mediaType": {"episode"}, "imdbId": {"tt123"}}, "", false},
		{url.Values{"mediaType": {"trailer"}, "imdbId": {"tt123"}}, "", false},
	} {
		q, err := seekrQuery(tc.input, 7200)
		if (err == nil) != tc.valid {
			t.Fatalf("unexpected validity: %v", err)
		}
		if tc.valid && (q.Get(tc.want) == "" || q.Get("duration_ms") != "7200000") {
			t.Fatal(q)
		}
	}
}

func TestSeekrDownloadsTilesAndMasksKey(t *testing.T) {
	original := seekrHTTPTransport
	defer func() { seekrHTTPTransport = original }()
	img := image.NewRGBA(image.Rect(0, 0, 640, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y), 100, 255})
		}
	}
	m := NewThumbnailManager(t.TempDir(), "")
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	seekrHTTPTransport = seekrTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.Path {
		case "/sprites":
			if r.URL.Host != "api.seekr.tv" || r.Header.Get("X-API-Key") != "test-key" {
				t.Fatal("missing API authentication")
			}
			body = []byte(`{"vtt_url":"https://sprites.seekr.tv/test/thumbnails.vtt?token=signed"}`)
		case "/test/thumbnails.vtt":
			if r.URL.Host != "sprites.seekr.tv" || r.URL.Query().Get("st") != "1" || r.URL.Query().Get("token") != "signed" {
				t.Fatal("invalid signed VTT request")
			}
			if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
				t.Fatal("key leaked to VTT")
			}
			body = []byte("WEBVTT\n\n00:00:00.000 --> 00:01:00.000\nsheet.jpg?signature=test#xywh=0,0,320,180\n\n00:01:00.000 --> 00:02:00.000\nsheet2.jpg?signature=test#xywh=320,0,320,180\n")
		case "/test/sheet2.jpg":
			partial, err := m.readManifest(thumbnailKey("movie.mkv"))
			if err != nil {
				t.Fatal(err)
			}
			if partial.Status != "generating" || partial.Generated != 1 || partial.Total != 2 || !m.manifestFilesComplete(partial) {
				t.Fatalf("first sheet must be available before next download: %+v", partial)
			}
			handler := &VideoHandler{thumbnailManager: m}
			status, err := handler.thumbnailStatusResponse(httptest.NewRequest(http.MethodGet, "/", nil), partial.Key)
			if err != nil || status.Status != "generating" || len(status.Thumbnails) != 1 {
				t.Fatalf("partial previews not exposed: %+v, %v", status, err)
			}
			body = encoded.Bytes()
		case "/test/sheet.jpg":
			if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
				t.Fatal("key leaked to sheet")
			}
			body = encoded.Bytes()
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})
	if err := m.loadSeekr("movie.mkv", 120, url.Values{"mediaType": {"movie"}, "imdbId": {"tt123"}}, "test-key"); err != nil {
		t.Fatal(err)
	}
	manifest, err := m.readManifest(thumbnailKey("movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Phase != "seekr" || manifest.Generated != 2 || !m.manifestFilesComplete(manifest) {
		t.Fatalf("invalid manifest %+v", manifest)
	}
	settings := config.Settings{Playback: config.PlaybackSettings{Thumbnails: config.PlaybackThumbnailSettings{SeekrAPIKey: "test-key"}}}
	existing := settings
	redactSettings(&settings)
	if strings.Contains(settings.Playback.Thumbnails.SeekrAPIKey, "test-key") {
		t.Fatal("key not masked")
	}
	preserveRedactedFields(&settings, &existing)
	if settings.Playback.Thumbnails.SeekrAPIKey != "test-key" {
		t.Fatal("masked key not preserved")
	}
}

func TestSeekrUnavailableFallsBack(t *testing.T) {
	original := seekrHTTPTransport
	defer func() { seekrHTTPTransport = original }()
	seekrHTTPTransport = seekrTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	})
	m := NewThumbnailManager(t.TempDir(), "")
	if err := m.loadSeekr("movie", 120, url.Values{"mediaType": {"movie"}, "imdbId": {"tt123"}}, "test-key"); err == nil {
		t.Fatal("missing title must fall back")
	}
	if _, err := m.readManifest(thumbnailKey("movie")); err == nil {
		t.Fatal("failed lookup published a manifest")
	}
}

// Opt-in smoke test; credentials are supplied only through the environment.
func TestSeekrLive(t *testing.T) {
	key := os.Getenv("SEEKR_TEST_KEY")
	if key == "" {
		t.Skip("SEEKR_TEST_KEY not configured")
	}
	m := NewThumbnailManager(t.TempDir(), "")
	err := m.loadSeekr("seekr-live-test.mkv", 10140, url.Values{"mediaType": {"movie"}, "imdbId": {"tt0816692"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := m.readManifest(thumbnailKey("seekr-live-test.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Generated == 0 || !m.manifestFilesComplete(manifest) {
		t.Fatal("missing generated tiles")
	}
	t.Logf("Seekr live preview verified: %d tiles", manifest.Generated)
}

func TestSeekrOnlyMissNeverStartsLocalGeneration(t *testing.T) {
	original := seekrHTTPTransport
	defer func() { seekrHTTPTransport = original }()
	requested := make(chan struct{}, 1)
	seekrHTTPTransport = seekrTransport(func(r *http.Request) (*http.Response, error) {
		requested <- struct{}{}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	})
	settings := config.DefaultSettings()
	settings.Playback.Thumbnails.Enabled = true
	settings.Playback.Thumbnails.SeekrEnabled = true
	settings.Playback.Thumbnails.SeekrAPIKey = "test-key"
	// No source resolver and an invalid ffmpeg path: API-only must need neither.
	m := NewThumbnailManager(t.TempDir(), "/nonexistent/ffmpeg")
	h := &VideoHandler{thumbnailManager: m, configManager: staticVideoConfigProvider{settings: settings}}
	r := httptest.NewRequest(http.MethodPost, "/video/thumbnails/start?path=movie.mkv&duration=120&mediaType=movie&imdbId=tt123&seekrOnly=1", nil)
	w := httptest.NewRecorder()
	h.StartThumbnails(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status %d", w.Code)
	}
	select {
	case <-requested:
	case <-time.After(2 * time.Second):
		t.Fatal("Seekr not requested")
	}
	deadline := time.Now().Add(2 * time.Second)
	for m.isInflight(thumbnailKey("movie.mkv")) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.isInflight(thumbnailKey("movie.mkv")) {
		t.Fatal("Seekr job did not finish")
	}
	manifest, err := m.readManifest(thumbnailKey("movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "pending" || manifest.Phase != "seekr-miss" || manifest.Generated != 0 {
		t.Fatalf("unexpected local generation: %+v", manifest)
	}
	if !m.seekrRecentlyUnavailable("movie.mkv") {
		t.Fatal("miss must be remembered for fallback")
	}
}
