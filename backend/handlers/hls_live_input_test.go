package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveTranscodingDecodesImagePrefixedHLS(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.ts")
	generate := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=size=160x90:rate=10",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
		"-t", "2", "-c:v", "mpeg2video", "-c:a", "aac", "-f", "mpegts", fixture)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, output)
	}
	ts, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	prefix := make([]byte, 42)
	copy(prefix, "RIFF")
	copy(prefix[8:], "WEBPVP8L")
	wrapped := append(prefix, ts...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://sports.test/" {
			t.Error("provider Referer lost")
		}
		if r.URL.Path == "/live.m3u8" {
			http.SetCookie(w, &http.Cookie{Name: "media_session", Value: "ok", Path: "/"})
			io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\nsegment.webp\n#EXT-X-ENDLIST\n")
			return
		}
		if cookie, err := r.Cookie("media_session"); err != nil || cookie.Value != "ok" {
			t.Error("playlist-issued media cookie lost")
			http.Error(w, "missing media session", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Write(wrapped)
	}))
	defer upstream.Close()
	session := &HLSSession{
		ID: "wrapped-live", Path: upstream.URL + "/live.m3u8", LiveProvider: "stremio",
		OutputDir: filepath.Join(dir, "output"), PlaybackTarget: "native", IsLive: true,
		LastSegmentRequest: time.Now(),
		LiveTuning:         LiveTuningSettings{RequestHeaders: map[string]string{"Referer": "https://sports.test/"}},
	}
	m := &HLSManager{ffmpegPath: ffmpeg}
	if err := m.startLiveTranscoding(ctx, session, 0); err != nil {
		t.Fatalf("transcode prefixed HLS: %v", err)
	}
	// Decode the actual output, rather than only checking FFmpeg exit status
	// or existence of a playlist that might contain no playable media.
	decode := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i",
		filepath.Join(session.OutputDir, "stream.m3u8"), "-map", "0:v:0", "-map", "0:a:0",
		"-f", "null", "-", "-progress", "pipe:1")
	output, err := decode.CombinedOutput()
	if err != nil {
		t.Fatalf("decode output HLS: %v: %s", err, output)
	}
	var frames int
	for _, line := range strings.Split(string(output), "\n") {
		var n int
		if _, err := fmt.Sscanf(line, "frame=%d", &n); err == nil && n > frames {
			frames = n
		}
	}
	if frames < 20 {
		t.Fatalf("decoded %d frames, expected the full 2s fixture: %s", frames, output)
	}
}
