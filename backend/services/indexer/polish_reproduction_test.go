package indexer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"novastream/config"
	"novastream/models"
)

// Regression coverage for MEDIASTORM-414. Metadata is a
// controlled fixture, not a claim about current provider results or release year.
func TestPolishReproductionHydratedAliasPriority(t *testing.T) {
	metadata := &mockMetadataWithAliases{
		results: []models.SearchResult{{Title: models.Title{
			Name: "Moana", MediaType: "movie", Language: "eng", TVDBID: 1,
		}}},
		langAliases: map[int64][]models.LanguageAlias{1: {
			{Name: "VAIANA", Language: "pol"},
			{Name: "Moana Alternate", Language: "eng"},
		}},
	}
	svc := &Service{metadata: &preferredTitleMetadata{mockMetadataWithAliases: *metadata, preferred: "VAIANA"}}
	for _, hydrated := range []bool{false, true} {
		name := "provider_language_aliases"
		opts := SearchOptions{Query: "Moana", MediaType: "movie"}
		if hydrated {
			name = "hydrated_movie_aliases"
			opts.AlternateTitles = []string{"Moana", "Moana Alternate", "VAIANA"}
		}
		t.Run(name, func(t *testing.T) {
			search, identities, _ := svc.resolveSearchTitles(t.Context(), opts, "pol", 1)
			t.Logf("pol preference, one alternate slot: queries=%v; filter identities=%v", search, identities)
			if !slices.Contains(search, "VAIANA") {
				t.Error("Polish alias is missing from outbound queries despite being available")
			}
		})
	}
}

func TestPolishPreferredTitleIsQueriedAndReleaseSurvivesFiltering(t *testing.T) {
	for _, mode := range []string{"search", "split", "scored", "scored-split"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			var queries []string
			const release = "Vaiana.2016.1080p.WEB-DL.POLISH.AAC"
			const englishRelease = "Moana.2016.1080p.WEB-DL.ENGLISH.AAC"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query().Get("q")
				mu.Lock()
				queries = append(queries, q)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w, "<rss><channel>")
				if strings.HasPrefix(q, "Vaiana") {
					fmt.Fprintf(w, `<item><title>%s</title><guid>polish</guid><link>http://example.com/polish.nzb</link></item>`, release)
				}
				if strings.HasPrefix(q, "Moana") {
					fmt.Fprintf(w, `<item><title>%s</title><guid>english</guid><link>http://example.com/english.nzb</link></item>`, englishRelease)
				}
				fmt.Fprint(w, "</channel></rss>")
			}))
			defer server.Close()
			mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			settings := config.DefaultSettings()
			settings.Metadata.Language = []string{"eng", "pol"}
			settings.Streaming.MaxAlternateTitleSearches = 1
			settings.Streaming.ServiceMode = config.StreamingServiceModeUsenet
			settings.Indexers = []config.IndexerConfig{{Name: "Fixture", URL: server.URL, Type: "newznab", Enabled: true}}
			if err := mgr.Save(settings); err != nil {
				t.Fatal(err)
			}
			svc := NewService(mgr, &preferredTitleMetadata{preferred: "Vaiana"}, nil)
			profile := &models.UserSettings{}
			profile.Metadata.PrimaryLanguage = "pol"
			svc.SetUserSettingsProvider(staticUserSettingsProvider{settings: profile})
			opts := SearchOptions{Query: "Vaiana", MediaType: "movie", Year: 2016, UserID: "polish",
				AlternateTitles: []string{"Moana", "Unrelated Alias", "Vaiana"}}
			results := runLanguageSearchMode(t, svc, opts, mode)
			seen := map[string]bool{}
			for _, result := range results {
				seen[result.Title] = true
			}
			if len(results) != 2 || !seen[release] || !seen[englishRelease] {
				t.Fatalf("expected distinct releases from both successful language queries, got %v", results)
			}
			mu.Lock()
			defer mu.Unlock()
			// Movie search retains the raw canonical query plus its year variant.
			// Only the preferred alias may use the single alternate-title slot.
			if len(queries) != 3 || !slices.Contains(queries, "Vaiana 2016") || !slices.Contains(queries, "Moana 2016") || slices.Contains(queries, "Unrelated Alias 2016") {
				t.Fatalf("expected canonical plus preferred title within budget, queries=%v", queries)
			}
		})
	}
}

type preferredTitleMetadata struct {
	mockMetadataWithAliases
	preferred string
	fail      bool
}

func (m preferredTitleMetadata) ResolveSearchTitle(_ context.Context, _, _ string, _ int, _, lang string) (*models.Title, error) {
	if m.fail {
		return nil, errors.New("metadata unavailable")
	}
	if lang == "eng" {
		return &models.Title{Name: "Moana"}, nil
	}
	return &models.Title{Name: m.preferred}, nil
}

func TestPreferredMetadataTitleReservesSearchBudget(t *testing.T) {
	for _, tc := range []struct {
		name, query, preferred string
		limit                  int
		anime, fail            bool
		want                   []string
	}{
		{name: "hydrated provider order", query: "Moana", preferred: "Vaiana", limit: 1, want: []string{"Vaiana"}},
		{name: "anime still reserves metadata slot", query: "Moana", preferred: "Vaiana", limit: 1, anime: true, want: []string{"Vaiana"}},
		{name: "canonical already localized", query: "Vaiana", preferred: "Vaiana", limit: 1, want: []string{"Moana"}},
		{name: "shared English name is not fallback only", query: "Other Query", preferred: "Moana", limit: 1, want: []string{"Moana"}},
		{name: "both names exceed a one-slot budget", query: "Other Query", preferred: "Vaiana", limit: 1, want: []string{"Vaiana", "Moana"}},
		{name: "unlimited preserves all aliases", query: "Moana", preferred: "Vaiana", want: []string{"Vaiana", "Unrelated Alias"}},
		{name: "metadata error preserves hydrated aliases", query: "Moana", fail: true, limit: 1, want: []string{"Unrelated Alias"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{metadata: &preferredTitleMetadata{preferred: tc.preferred, fail: tc.fail}}
			opts := SearchOptions{Query: tc.query, MediaType: "movie", IsAnime: tc.anime,
				AlternateTitles: []string{"Moana", "Unrelated Alias", "Vaiana"}}
			got, identities, _ := svc.resolveSearchTitles(t.Context(), opts, "pol", tc.limit)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("queries=%v, want %v", got, tc.want)
			}
			if !slices.Contains(identities, "Unrelated Alias") || !slices.Contains(identities, "Vaiana") {
				t.Fatalf("filter identities lost aliases: %v", identities)
			}
		})
	}
}

func TestPolishReproductionReleaseLanguageRanking(t *testing.T) {
	english := models.NZBResult{Attributes: map[string]string{"languages": "English"}}
	for _, label := range []string{"pol", "Polish", "🇵🇱", "pl"} {
		t.Run(label, func(t *testing.T) {
			polish := models.NZBResult{Attributes: map[string]string{"languages": label}}
			got := compareLanguage(polish, english, "pol")
			t.Logf("release languages=%q; pol preference comparison=%d", label, got)
			if got != -1 {
				t.Error("Polish release should rank ahead of English on the language criterion")
			}
		})
	}
}

func runLanguageSearchMode(t *testing.T, svc *Service, opts SearchOptions, mode string) []models.NZBResult {
	t.Helper()
	var results []models.NZBResult
	switch mode {
	case "search":
		var err error
		results, err = svc.Search(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
	case "split":
		deb, us := svc.SearchSplit(t.Context(), opts)
		for _, ch := range []<-chan SplitSearchResult{deb, us} {
			for batch := range ch {
				if batch.Err != nil {
					t.Fatal(batch.Err)
				}
				results = append(results, batch.Results...)
			}
		}
	case "scored":
		scored, err := svc.SearchWithScoring(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, result := range scored {
			results = append(results, result.NZBResult)
		}
	case "scored-split":
		us, deb := svc.SearchWithScoringSplit(t.Context(), opts)
		for _, ch := range []<-chan ScoredSplitSearchResult{us, deb} {
			for batch := range ch {
				if batch.Err != nil {
					t.Fatal(batch.Err)
				}
				for _, result := range batch.Scored {
					results = append(results, result.NZBResult)
				}
			}
		}
	}
	return results
}
