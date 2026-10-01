package metadata

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"

	"novastream/models"
)

// searchActors uses the existing search-card projection, with a person media
// type and TMDB person ID. It never resolves people as movies or TV shows.
func (s *Service) searchActors(ctx context.Context, query string) ([]models.SearchResult, error) {
	if s.tmdb == nil || !s.tmdb.isConfigured() {
		return []models.SearchResult{}, nil
	}
	allowAdult := s.adultSearchAllowed()
	key := cacheKey("metadata", "person-search", "v2", query, s.tmdb.language, fmt.Sprint(allowAdult))
	var results []models.SearchResult
	if ok, _ := s.cache.get(key, &results); ok {
		return results, nil
	}
	results, err := s.tmdb.searchActors(ctx, query, 20, allowAdult)
	if err != nil {
		return nil, err
	}
	_ = s.cache.set(key, results)
	return results, nil
}

func (c *tmdbClient) searchActors(ctx context.Context, query string, limit int, includeAdult bool) ([]models.SearchResult, error) {
	params := url.Values{
		"api_key": {c.apiKey}, "query": {query},
		"include_adult": {fmt.Sprint(includeAdult)},
		"language":      {normalizeLanguage(c.language)},
	}
	var payload struct {
		Results []struct {
			ID         int64   `json:"id"`
			Name       string  `json:"name"`
			Department string  `json:"known_for_department"`
			Profile    string  `json:"profile_path"`
			Popularity float64 `json:"popularity"`
			Adult      bool    `json:"adult"`
		} `json:"results"`
	}
	if err := c.doGET(ctx, tmdbBaseURL+"/search/person?"+params.Encode(), &payload); err != nil {
		return nil, fmt.Errorf("tmdb actor search failed: %w", err)
	}
	results := make([]models.SearchResult, 0, len(payload.Results))
	for _, person := range payload.Results {
		name := strings.TrimSpace(person.Name)
		portrait := buildTMDBImage(person.Profile, tmdbPosterSize, "profile")
		if person.ID <= 0 || name == "" || portrait == nil || !strings.EqualFold(person.Department, "Acting") || (!includeAdult && person.Adult) {
			continue
		}
		results = append(results, models.SearchResult{
			Title: models.Title{
				ID: fmt.Sprintf("tmdb:person:%d", person.ID), Name: name,
				MediaType: "person", TMDBID: person.ID, Adult: person.Adult,
				Poster:     portrait,
				Popularity: person.Popularity,
			},
			Score: int(math.Round(person.Popularity)),
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}
