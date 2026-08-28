package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Ensure the latency JSON and page endpoints render.
func TestServeLatencyEndpoints(t *testing.T) {
	tr := NewPlaybackLatencyTracker(10)
	now := time.Now()
	tr.Record(PlaybackLatencySample{
		PrequeueID:          "pq1",
		SessionID:           "s1",
		ClientRequestedAt:   now.Add(-8 * time.Millisecond),
		PrequeueReadyAt:     now.Add(-5 * time.Millisecond),
		HLSSessionCreatedAt: now.Add(-4 * time.Millisecond),
		FirstSegmentReadyAt: now.Add(-1 * time.Millisecond),
		FirstSegmentSentAt:  now,
	})
	admin := NewPlaybackLatencyAdmin(tr)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/latency?limit=10", nil)
	rec := httptest.NewRecorder()
	admin.ServePlaybackLatencyJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("json status=%d", rec.Code)
	}
	var snap PlaybackLatencySnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if snap.Total != 1 || snap.Complete != 1 || len(snap.Samples) != 1 {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
	if snap.Samples[0].SessionID != "s1" {
		t.Fatalf("sample lost: %+v", snap.Samples)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/admin/latency", nil)
	rec2 := httptest.NewRecorder()
	admin.ServeLatencyPage(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("page status=%d", rec2.Code)
	}
	body := rec2.Body.String()
	if len(body) < 1000 {
		t.Fatalf("page body suspiciously small: %d bytes", len(body))
	}
	for _, want := range []string{
		"click → first frame", "Clear samples", "/admin/api/latency?limit=60", `colspan="10"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("latency page missing %q", want)
		}
	}
}
