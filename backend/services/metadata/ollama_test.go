package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaCompletion(t *testing.T) {
	for _, key := range []string{"", "proxy-key"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Errorf("path = %s", r.URL.Path)
			}
			wantAuth := ""
			if key != "" {
				wantAuth = "Bearer " + key
			}
			if r.Header.Get("Authorization") != wantAuth {
				t.Error("unexpected authorization")
			}
			var body openAIChatRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Model != "local:8b" || body.Stream {
				t.Errorf("request = %+v", body)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[{\"title\":\"Arrival\",\"year\":2016,\"mediaType\":\"movie\"}]"},"finish_reason":"stop"}]}`))
		}))
		client := newAIClient(AIConfig{Provider: "ollama", Model: "local:8b", BaseURL: server.URL, APIKey: key}, server.Client(), nil)
		recs, err := client.getCustomRecommendations(context.Background(), "sci-fi")
		server.Close()
		if err != nil || len(recs) != 1 || recs[0].Title != "Arrival" {
			t.Fatalf("recs=%v err=%v", recs, err)
		}
		if client.inferenceTimeout() != 180*time.Second {
			t.Fatal("wrong local budget")
		}
	}
}

func TestOllamaConfigurationAndCacheIdentity(t *testing.T) {
	local := newAIClient(AIConfig{Provider: "ollama", Model: "local"}, nil, nil)
	if !local.isConfigured() || local.resolvedBaseURL() != "http://localhost:11434/v1" {
		t.Fatal("keyless local not configured")
	}
	missing := newAIClient(AIConfig{Provider: "ollama", APIKey: "key"}, nil, nil)
	if missing.isConfigured() {
		t.Fatal("missing model accepted")
	}
	hosted := newAIClient(AIConfig{Provider: "openai", Model: "local"}, nil, nil)
	if hosted.isConfigured() || hosted.inferenceTimeout() != 90*time.Second {
		t.Fatal("hosted behavior changed")
	}
	other := newAIClient(AIConfig{Provider: "ollama", Model: "local", BaseURL: "http://other:11434"}, nil, nil)
	if local.cacheModelKey() == other.cacheModelKey() {
		t.Fatal("cache crosses servers")
	}
}

func TestOllamaBadOutputAndCancellation(t *testing.T) {
	for _, body := range []string{`{"choices":[]}`, `{"choices":[{"message":{"content":""}}]}`, `{"choices":[{"message":{"content":"not json"}}]}`, `{"choices":[{"message":{"content":"[]"},"finish_reason":"length"}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		client := newAIClient(AIConfig{Provider: "ollama", Model: "local", BaseURL: server.URL}, server.Client(), nil)
		_, err := client.getCustomRecommendations(context.Background(), "test")
		server.Close()
		if err == nil {
			t.Fatalf("accepted bad output: %s", body)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := newAIClient(AIConfig{Provider: "ollama", Model: "local"}, nil, nil)
	_, err := client.getCustomRecommendations(ctx, "test")
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v", err)
	}
}
