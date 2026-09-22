package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIntroSegmentsProviderPrecedence(t *testing.T) {
	introCalls, skipCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/intro":
			introCalls++
			if r.URL.Query().Get("episode") == "2" {
				_, _ = w.Write([]byte(`{"intro":{"start_ms":40000,"end_ms":90000},"recap":{"start_ms":0,"end_ms":30000},"outro":{"start_ms":1100000,"end_ms":1200000}}`))
				return
			}
			_, _ = w.Write([]byte(`{"intro":{"start_ms":40000,"end_ms":90000},"recap":null,"outro":null}`))
		case "/skip":
			skipCalls++
			if r.URL.Query().Get("duration") != "1200" {
				t.Errorf("SkipDB duration = %q, want 1200", r.URL.Query().Get("duration"))
			}
			if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
				t.Error("public SkipDB read must not send a credential")
			}
			_, _ = w.Write([]byte(`{"segments":{"intro":{"start_ms":45000,"end_ms":95000,"match":"exact"},"recap":{"start_ms":0,"end_ms":35000,"match":"shifted"},"outro":{"start_ms":1100000,"end_ms":1200000,"match":"out-of-range"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldIntro, oldSkip := introDBSegmentsURL, skipDBSegmentsURL
	introDBSegmentsURL, skipDBSegmentsURL = server.URL+"/intro", server.URL+"/skip"
	defer func() { introDBSegmentsURL, skipDBSegmentsURL = oldIntro, oldSkip }()

	request := httptest.NewRequest(http.MethodGet, "/video/segments?imdbId=tt987654321&season=1&episode=1&duration=1200", nil)
	response := httptest.NewRecorder()
	(&VideoHandler{}).GetIntroSegments(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var segments introDBSegmentsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &segments); err != nil {
		t.Fatal(err)
	}
	if segments.Intro == nil || *segments.Intro.StartMS != 40000 || segments.Intro.Source != "" {
		t.Errorf("IntroDB intro should win: %+v", segments.Intro)
	}
	if segments.Recap == nil || *segments.Recap.EndMS != 35000 || segments.Recap.Source != "skipdb" {
		t.Errorf("SkipDB should fill recap: %+v", segments.Recap)
	}
	if segments.Outro != nil {
		t.Errorf("out-of-range SkipDB outro should be rejected: %+v", segments.Outro)
	}
	if introCalls != 1 || skipCalls != 1 {
		t.Errorf("provider calls = %d/%d, want 1/1", introCalls, skipCalls)
	}

	complete := httptest.NewRecorder()
	(&VideoHandler{}).GetIntroSegments(complete, httptest.NewRequest(http.MethodGet, "/video/segments?imdbId=tt987654321&season=1&episode=2&duration=1200", nil))
	if complete.Code != http.StatusOK || introCalls != 2 || skipCalls != 1 {
		t.Errorf("complete IntroDB result should skip fallback: status %d, provider calls %d/%d", complete.Code, introCalls, skipCalls)
	}
}
