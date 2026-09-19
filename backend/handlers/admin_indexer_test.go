package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"novastream/internal/httpheaders"
)

func TestAdminIndexerSearchRequest(t *testing.T) {
	for _, suffix := range []string{"", "/api", "/api/"} {
		t.Run(suffix, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api" {
					t.Errorf("path = %q", r.URL.Path)
				}
				if r.URL.Query().Get("apikey") != "key+&=value" {
					t.Errorf("API key was not preserved")
				}
				if r.URL.Query().Get("t") != "search" || r.URL.Query().Get("q") != "test" {
					t.Errorf("missing search parameters")
				}
				if r.Header.Get("User-Agent") != httpheaders.UserAgent || !strings.Contains(r.Header.Get("Accept"), "application/xml") {
					http.Error(w, "blocked client headers", http.StatusForbidden)
					return
				}
				w.Write([]byte(`<rss><channel></channel></rss>`))
			}))
			defer server.Close()
			payload, _ := json.Marshal(TestIndexerRequest{URL: server.URL + suffix, APIKey: "key+&=value"})
			rec := httptest.NewRecorder()
			(&AdminUIHandler{}).TestIndexer(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(payload))))
			var result struct {
				Success bool   `json:"success"`
				Error   string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !result.Success {
				t.Fatalf("test failed: %s", result.Error)
			}
		})
	}
}

func TestAdminIndexerErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"cloudflare", 403, `<!DOCTYPE html><title>Attention Required! | Cloudflare</title>`, "Cloudflare blocked the indexer request"},
		{"api key", 200, `<error code="100" description="Incorrect user credentials"/>`, "Incorrect user credentials"},
		{"other forbidden", 403, `Access denied`, "HTTP 403: Access denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer server.Close()
			payload, _ := json.Marshal(TestIndexerRequest{URL: server.URL})
			rec := httptest.NewRecorder()
			(&AdminUIHandler{}).TestIndexer(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(payload))))
			var result struct {
				Success bool   `json:"success"`
				Error   string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Success || !strings.Contains(result.Error, tc.want) || strings.Contains(result.Error, "<!DOCTYPE") {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}
