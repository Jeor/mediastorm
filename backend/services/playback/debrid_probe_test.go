package playback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"novastream/config"
	"novastream/models"
)

type prequeueDebridFullProber struct {
	result *models.VideoFullResult
	calls  int
}

func (p *prequeueDebridFullProber) ProbeVideoFull(context.Context, string) (*models.VideoFullResult, error) {
	p.calls++
	return p.result, nil
}

func TestPrequeuePlaybackServiceReturnsReusableDebridProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Length", "10485760")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.NewManager(t.TempDir() + "/settings.json")
	settings := config.DefaultSettings()
	if err := cfg.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	want := &models.VideoFullResult{
		VideoCodec: "hevc",
		AudioStreams: []models.AudioStreamInfo{
			{Index: 1, Codec: "ac3", Language: "eng"},
		},
	}
	prober := &prequeueDebridFullProber{result: want}
	service := NewService(cfg, nil, nil)
	service.SetDebridFullProber(prober)

	resolution, err := service.Resolve(context.Background(), models.NZBResult{
		Title:       "Bilby.2018.2160p.UHD.BluRay.Remux-PmP.mkv",
		Link:        server.URL + "/video.mkv",
		ServiceType: models.ServiceTypeDebrid,
		Attributes: map[string]string{
			"preresolved": "true",
			"stream_url":  server.URL + "/video.mkv",
		},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Probe == nil {
		t.Fatalf("expected reusable probe, got %#v", resolution)
	}
	if resolution.Probe.VideoCodec != "hevc" || len(resolution.Probe.AudioStreams) != 1 {
		t.Fatalf("reusable probe metadata = %#v", resolution.Probe)
	}
	if prober.calls != 1 {
		t.Fatalf("ProbeVideoFull calls = %d, want 1", prober.calls)
	}
}
