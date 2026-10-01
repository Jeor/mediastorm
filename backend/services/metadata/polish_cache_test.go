package metadata

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestTMDBMovieCachePreservesLocalizedNameAndAliases(t *testing.T) {
	cache := newFileCache(t.TempDir(), 24)
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		name := "Moana"
		if strings.HasPrefix(req.URL.Query().Get("language"), "pl") {
			name = "Vaiana"
		}
		if !strings.Contains(req.URL.Query().Get("append_to_response"), "alternative_titles") {
			t.Fatal("movie details must request aliases alongside the localized title")
		}
		body := fmt.Sprintf(`{"id":277834,"title":%q,"original_title":"Moana","original_language":"en","alternative_titles":{"titles":[{"title":"Vaiana","iso_3166_1":"PL"},{"title":"Oceania","iso_3166_1":"IT"}]}}`, name)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	// Fresh clients share the persistent cache, like request-scoped metadata
	// services. The English cache must never replace the Polish display title.
	for attempt := 0; attempt < 2; attempt++ {
		for _, lang := range []string{"eng", "pol"} {
			c := newTMDBClient("test-key", lang, client, cache)
			title, err := c.movieDetails(t.Context(), 277834)
			if err != nil {
				t.Fatal(err)
			}
			want := "Moana"
			if lang == "pol" {
				want = "Vaiana"
			}
			if title.Name != want || !slices.Contains(title.AlternateTitles, "Oceania") {
				t.Fatalf("lang=%s attempt=%d: name=%q aliases=%v", lang, attempt, title.Name, title.AlternateTitles)
			}
		}
	}
	if requests != 2 {
		t.Fatalf("TMDB requests=%d, want one per language with subsequent cache hits", requests)
	}
}
