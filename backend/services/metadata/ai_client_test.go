package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAIClientOpenAICompatibleProviderUsesChatCompletions(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[{\"title\":\"Arrival\",\"year\":2016,\"mediaType\":\"movie\"}]"}}]}`))
	}))
	defer server.Close()

	client := newAIClient(AIConfig{
		Provider: "openrouter",
		APIKey:   "test-key",
		Model:    "test-model",
		BaseURL:  server.URL,
	}, server.Client(), nil)

	recs, err := client.getCustomRecommendations(context.Background(), "thoughtful sci-fi")
	if err != nil {
		t.Fatalf("getCustomRecommendations error: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotModel != "test-model" {
		t.Fatalf("model = %q, want test-model", gotModel)
	}
	if len(recs) != 1 || recs[0].Title != "Arrival" {
		t.Fatalf("unexpected recommendations: %+v", recs)
	}
}

func TestAIClientAnthropicProviderUsesMessagesAPI(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotVersion string
	var gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"[{\"title\":\"Severance\",\"year\":2022,\"mediaType\":\"series\"}]"}]}`))
	}))
	defer server.Close()

	client := newAIClient(AIConfig{
		Provider: "claude",
		APIKey:   "anthropic-key",
		Model:    "claude-test",
		BaseURL:  server.URL,
	}, server.Client(), nil)

	recs, err := client.getCustomRecommendations(context.Background(), "office mystery shows")
	if err != nil {
		t.Fatalf("getCustomRecommendations error: %v", err)
	}
	if gotPath != "/messages" {
		t.Fatalf("path = %q, want /messages", gotPath)
	}
	if gotAPIKey != "anthropic-key" {
		t.Fatalf("x-api-key = %q, want anthropic-key", gotAPIKey)
	}
	if gotVersion == "" {
		t.Fatal("anthropic-version header was empty")
	}
	if gotModel != "claude-test" {
		t.Fatalf("model = %q, want claude-test", gotModel)
	}
	if len(recs) != 1 || recs[0].Title != "Severance" {
		t.Fatalf("unexpected recommendations: %+v", recs)
	}
}

func TestGetAICustomRecommendationsCoalescesInFlightCalls(t *testing.T) {
	var aiHits atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if aiHits.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			t.Error("timed out waiting to release AI handler")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[{\"title\":\"Superbad\",\"year\":2007,\"mediaType\":\"movie\"}]"}}]}`))
	}))
	defer aiServer.Close()

	svc := &Service{
		ai: newAIClient(AIConfig{
			Provider: "openai",
			APIKey:   "test-key",
			Model:    "test-model",
			BaseURL:  aiServer.URL,
		}, aiServer.Client(), nil),
		tmdb: &tmdbClient{
			apiKey:      "tmdb-key",
			language:    "en",
			httpc:       &http.Client{Transport: staticTMDBSearchTransport(t, "Superbad", 123)},
			minInterval: time.Millisecond,
		},
		cache: newFileCache(t.TempDir(), 1),
	}

	const callers = 3
	var wg sync.WaitGroup
	errs := make([]error, callers)
	counts := make([]int, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			items, err := svc.GetAICustomRecommendations(context.Background(), "funny movies")
			errs[i] = err
			counts[i] = len(items)
		}(i)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("AI provider was never called")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := aiHits.Load(); got != 1 {
		t.Fatalf("AI provider hits = %d, want 1", got)
	}
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d error: %v", i, errs[i])
		}
		if counts[i] != 1 {
			t.Fatalf("caller %d items = %d, want 1", i, counts[i])
		}
	}
}

func TestGetAICustomRecommendationsCanceledWaiterDoesNotRetrigger(t *testing.T) {
	var aiHits atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if aiHits.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			t.Error("timed out waiting to release AI handler")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[{\"title\":\"Superbad\",\"year\":2007,\"mediaType\":\"movie\"}]"}}]}`))
	}))
	defer aiServer.Close()

	svc := &Service{
		ai: newAIClient(AIConfig{
			Provider: "openai",
			APIKey:   "test-key",
			Model:    "test-model",
			BaseURL:  aiServer.URL,
		}, aiServer.Client(), nil),
		tmdb: &tmdbClient{
			apiKey:      "tmdb-key",
			language:    "en",
			httpc:       &http.Client{Transport: staticTMDBSearchTransport(t, "Superbad", 123)},
			minInterval: time.Millisecond,
		},
		cache: newFileCache(t.TempDir(), 1),
	}

	leaderErr := make(chan error, 1)
	go func() {
		_, err := svc.GetAICustomRecommendations(context.Background(), "funny movies")
		leaderErr <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("AI provider was never called")
	}

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, err := svc.GetAICustomRecommendations(waiterCtx, "funny movies")
		waiterErr <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancelWaiter()

	select {
	case err := <-waiterErr:
		if err == nil {
			t.Fatal("canceled waiter returned nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled waiter never returned")
	}

	close(release)
	select {
	case err := <-leaderErr:
		if err != nil {
			t.Fatalf("leader error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leader never returned")
	}
	if got := aiHits.Load(); got != 1 {
		t.Fatalf("AI provider hits = %d, want 1", got)
	}
}

func staticTMDBSearchTransport(t *testing.T, title string, id int) roundTripFunc {
	t.Helper()
	return func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/search/") {
			t.Errorf("unexpected TMDB path %s", r.URL.Path)
		}
		body := `{"results":[{"id":` + strconv.Itoa(id) + `,"title":"` + title + `","media_type":"movie","release_date":"2007-08-17"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}
}
