package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaValidation(t *testing.T) {
	for _, tc := range []struct {
		name, model, key, body, want string
		status                       int
	}{
		{"keyless", "test:latest", "", `{"data":[{"id":"test:latest"}]}`, "", 200},
		{"latest alias", "test", "proxy-token", `{"data":[{"id":"test:latest"}]}`, "", 200},
		{"missing model", "other", "", `{"data":[{"id":"test:latest"}]}`, "not installed", 200},
		{"bad json", "test", "", `oops`, "invalid model list", 200},
		{"proxy auth", "test", "", `{}`, "authentication failed", 401},
		{"wrong endpoint", "test", "", `{}`, "lookup failed", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/proxy/v1/models" {
					t.Errorf("path = %s", r.URL.Path)
				}
				wantAuth := ""
				if tc.key != "" {
					wantAuth = "Bearer " + tc.key
				}
				if r.Header.Get("Authorization") != wantAuth {
					t.Error("unexpected auth header")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			err := validateOllama(context.Background(), server.Client(), server.URL+"/proxy", tc.model, tc.key)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestMetadataKeylessOllamaIgnoresLegacyGemini(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"test:latest"}]}`))
	}))
	defer server.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/test/metadata", strings.NewReader(`{"aiProvider":"ollama","aiModel":"test","aiBaseUrl":"`+server.URL+`","geminiApiKey":"old-gemini"}`))
	recorder := httptest.NewRecorder()
	(&AdminUIHandler{}).TestMetadata(recorder, req)
	if !strings.Contains(recorder.Body.String(), `"provider":"Ollama"`) || !strings.Contains(recorder.Body.String(), `"success":true`) {
		t.Fatal(recorder.Body.String())
	}
}

func TestOllamaValidationErrors(t *testing.T) {
	for _, tc := range []struct{ base, model, want string }{
		{"bad://host", "test", "HTTP(S)"}, {"http://localhost", "", "model name"},
	} {
		err := validateOllama(context.Background(), http.DefaultClient, tc.base, tc.model, "")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("error = %v", err)
		}
	}
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	if err := validateOllama(context.Background(), http.DefaultClient, server.URL, "test", ""); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatalf("offline error = %v", err)
	}
	if _, _, err := buildAIValidationRequest("openai", "key", "http://[invalid"); err == nil {
		t.Fatal("invalid URL accepted")
	}
}
