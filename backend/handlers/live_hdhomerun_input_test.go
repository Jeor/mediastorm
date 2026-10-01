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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"novastream/config"
)

func TestHDHomeRunDirectHLSAndQualityFromTransportStreamFixture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.ts")
	generate := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=size=160x90:rate=10", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
		"-t", "2", "-c:v", "mpeg2video", "-c:a", "aac", "-f", "mpegts", fixture)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v %s", err, output)
	}
	ts, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	const streamURL = "http://192.168.1.100:5004/auto/v5.1"
	settings := hdHomeRunTestSettings()
	mgr := config.NewManager(filepath.Join(dir, "settings.json"))
	if err := mgr.Save(settings); err != nil {
		t.Fatal(err)
	}
	transport := discoveryTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == settings.Live.Sources[0].PlaylistURL {
			return discoveryTestResponse(200, "#EXTM3U\n#EXTINF:-1 tvg-id=\"channel-5\",Fixture\n"+streamURL+"\n", http.Header{}), nil
		}
		if r.URL.String() != streamURL {
			t.Errorf("unchecked secondary request: %s", r.URL)
			return discoveryTestResponse(403, "blocked", http.Header{}), nil
		}
		if r.UserAgent() != liveStreamUserAgent {
			t.Error("tuner request missing player User-Agent")
		}
		return discoveryTestResponse(200, string(ts), http.Header{"Content-Type": []string{"video/mp2t"}}), nil
	})
	// All tuner traffic uses fixtures. No physical LAN device is contacted.
	h := NewLiveHandler(&http.Client{Transport: transport}, true, ffmpeg, 24, 0, 0, false, mgr, nil)
	original := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = original }()

	for _, target := range []string{"native", "web"} {
		request := httptest.NewRequest(http.MethodGet, "/live/stream?sourceId=tuner&channelId=channel-5&target="+target+"&url="+url.QueryEscape(streamURL), nil).WithContext(ctx)
		response := httptest.NewRecorder()
		h.StreamChannel(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s direct status=%d body=%s", target, response.Code, response.Body.String())
		}
		if target == "native" {
			if response.Body.String() != string(ts) {
				t.Fatal("native relay changed transport stream bytes")
			}
		} else {
			output := filepath.Join(dir, "web.mp4")
			if err := os.WriteFile(output, response.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			assertHDHomeRunFixtureDecodes(t, ctx, ffmpeg, output)
		}
	}

	m := &HLSManager{ffmpegPath: ffmpeg}
	for _, target := range []string{"native", "web", "cast"} {
		t.Run("HLS/"+target, func(t *testing.T) {
			session := &HLSSession{
				ID: "tuner-fixture-" + target, Path: streamURL, LiveProvider: "m3u", IsLive: true,
				OutputDir: filepath.Join(dir, "hls-"+target), PlaybackTarget: target, LastSegmentRequest: time.Now(),
				LiveTuning: LiveTuningSettings{HDHomeRunInput: true},
			}
			if err := m.startLiveTranscoding(ctx, session, 0); err != nil {
				t.Fatalf("HLS from tuner fixture: %v", err)
			}
			playlist := filepath.Join(session.OutputDir, "stream.m3u8")
			assertHDHomeRunFixtureDecodes(t, ctx, ffmpeg, playlist)
			if target != "native" {
				probe := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", playlist)
				output, err := probe.CombinedOutput()
				if err != nil || !strings.Contains(string(output), "h264") || !strings.Contains(string(output), "aac") {
					t.Fatalf("compatibility codecs: %v %s", err, output)
				}
			}
		})
	}

	video := &VideoHandler{ffprobePath: ffprobe, configManager: staticSecurityConfigProvider{settings},
		liveChannels: staticLiveChannelProvider{channels: []LiveChannel{{ID: "channel-5", SourceID: "tuner", URL: streamURL}}}}
	body := fmt.Sprintf(`{"url":%q,"sourceId":"tuner","channelId":"channel-5"}`, streamURL)
	response := httptest.NewRecorder()
	video.ProbeLiveQuality(response, httptest.NewRequest(http.MethodPost, "/video/live/quality", strings.NewReader(body)).WithContext(ctx))
	if response.Code != http.StatusOK {
		t.Fatalf("quality status=%d body=%s", response.Code, response.Body.String())
	}
	var quality liveQualityResult
	if err := json.Unmarshal(response.Body.Bytes(), &quality); err != nil {
		t.Fatal(err)
	}
	if quality.Width != 160 || quality.Height != 90 || len(quality.Audio) == 0 {
		t.Fatalf("fixture quality=%+v", quality)
	}
}

func assertHDHomeRunFixtureDecodes(t *testing.T, ctx context.Context, ffmpeg, path string) {
	t.Helper()
	decode := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", path,
		"-map", "0:v:0", "-map", "0:a:0", "-f", "null", "-", "-progress", "pipe:1")
	output, err := decode.CombinedOutput()
	if err != nil {
		t.Fatalf("decode %s: %v %s", path, err, output)
	}
	frames := 0
	for _, line := range strings.Split(string(output), "\n") {
		var count int
		if _, err := fmt.Sscanf(line, "frame=%d", &count); err == nil && count > frames {
			frames = count
		}
	}
	if frames == 0 {
		t.Fatalf("output decoded no video frames: %s", output)
	}
}

func TestHDHomeRunHLSProcessExitReleasesStalledInput(t *testing.T) {
	failingProcess, err := exec.LookPath("false")
	if err != nil {
		t.Skip("false executable unavailable")
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	original := http.DefaultTransport
	http.DefaultTransport = discoveryTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: reader}, nil
	})
	defer func() { http.DefaultTransport = original }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &HLSSession{
		ID: "stalled-tuner", Path: "http://192.168.1.100:5004/auto/v5.1", LiveProvider: "m3u", IsLive: true,
		OutputDir: t.TempDir(), PlaybackTarget: "cast", LastSegmentRequest: time.Now(),
		LiveTuning: LiveTuningSettings{HDHomeRunInput: true},
	}
	m := &HLSManager{ffmpegPath: failingProcess}
	done := make(chan error, 1)
	go func() { done <- m.startLiveTranscoding(ctx, session, 0) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("failed encoder reported success")
		}
	case <-time.After(5 * time.Second):
		// Unblock the mocked body and join before restoring the transport.
		cancel()
		reader.Close()
		<-done
		t.Fatal("encoder exit remained blocked on the stalled tuner input")
	}
	if _, err := writer.Write([]byte("late tuner data")); err != io.ErrClosedPipe {
		t.Fatalf("tuner connection still open after encoder exit: %v", err)
	}
}
