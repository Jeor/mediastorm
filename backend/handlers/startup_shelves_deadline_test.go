package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"novastream/models"
	"testing"
	"time"
)

func TestStartupShelvesDeadlineDoesNotWaitForSlowProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	shelves := []models.ShelfConfig{
		{ID: "genre-16-movie", Type: "genre", Enabled: true},
		{ID: "genre-37-movie", Type: "genre", Enabled: true},
	}
	started := time.Now()
	out, errs := buildStartupHomeShelvesWithHandler(ctx, httptest.NewRequest(http.MethodGet, "/", nil), "default", shelves, 20, false, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("genreId") == "16" {
			<-release
		}
		_, _ = w.Write([]byte(`{"items":[],"total":0}`))
	})
	if time.Since(started) > time.Second {
		t.Fatal("startup waited for slow provider")
	}
	if _, ok := out["genre-37-movie"]; !ok {
		t.Fatalf("fast shelf missing: %v", out)
	}
	if errs["genre-16-movie"] == "" {
		t.Fatalf("slow shelf should fall back: %v", errs)
	}
}
