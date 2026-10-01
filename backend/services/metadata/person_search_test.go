package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"novastream/models"
)

func TestActorSearchUsesTMDBAndCachesResults(t *testing.T) {
	var calls int
	tmdb := newTMDBClient("test-key", "fr-FR", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/3/search/person" || req.URL.Query().Get("query") != "Tom Holl" || req.URL.Query().Get("language") != "fr-FR" {
			t.Fatalf("unexpected person search: %s", req.URL.Path)
		}
		if req.URL.Query().Get("include_adult") != "false" {
			t.Fatal("expected adult filtering")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[
			{"id":1136406,"name":"Tom Holland","known_for_department":"Acting","profile_path":"/tom.jpg","popularity":30},
			{"id":2,"name":"Tom Director","known_for_department":"Directing","profile_path":"/director.jpg"},
			{"id":3,"name":"Adult actor","known_for_department":"Acting","adult":true,"profile_path":"/adult.jpg"},
			{"id":0,"name":"Invalid","known_for_department":"Acting","profile_path":"/invalid.jpg"},
			{"id":4,"name":"  ","known_for_department":"Acting","profile_path":"/blank.jpg"},
			{"id":5,"name":"Actor without photo","known_for_department":"Acting"},
			{"id":6,"name":"Actor with null photo","known_for_department":"Acting","profile_path":null},
			{"id":7,"name":"Actor with blank photo","known_for_department":"Acting","profile_path":"  "},
			{"id":8,"name":"Tom Hollander","known_for_department":"Acting","profile_path":"/hollander.jpg"}
		]}`))}, nil
	})}, nil)
	tmdb.minInterval = 0
	svc := &Service{tmdb: tmdb, cache: newFileCache(t.TempDir(), 24)}
	for range 2 {
		results, err := svc.Search(t.Context(), " Tom Holl ", "person")
		if err != nil || len(results) != 2 {
			t.Fatalf("actor results = %+v, err = %v", results, err)
		}
		actor := results[0].Title
		if actor.ID != "tmdb:person:1136406" || actor.TMDBID != 1136406 || actor.MediaType != "person" || actor.Name != "Tom Holland" || actor.Year != 0 {
			t.Fatalf("unexpected actor card: %+v", actor)
		}
		if actor.Poster == nil || actor.Poster.URL != "https://image.tmdb.org/t/p/w780/tom.jpg" {
			t.Fatalf("unexpected portrait: %+v", actor.Poster)
		}
		if results[1].Title.Name != "Tom Hollander" || results[1].Title.Poster == nil {
			t.Fatal("expected the next actor with a photo, skipping missing, null and blank profiles")
		}
	}
	if calls != 1 {
		t.Fatalf("TMDB requests = %d, want one cached search", calls)
	}
}

func TestActorSearchAdultPolicyAndUnavailableTMDB(t *testing.T) {
	svc := &Service{}
	results, err := svc.Search(context.Background(), "Tom Holland", "person")
	if err != nil || len(results) != 0 {
		t.Fatalf("unconfigured search = %+v, %v", results, err)
	}
	var policies []string
	tmdb := newTMDBClient("test-key", "en", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		policies = append(policies, req.URL.Query().Get("include_adult"))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{"id":1,"name":"Actor","known_for_department":"Acting","adult":true,"profile_path":"/actor.jpg"}]}`))}, nil
	})}, nil)
	tmdb.minInterval = 0
	svc.tmdb, svc.cache = tmdb, newFileCache(t.TempDir(), 24)
	results, err = svc.Search(t.Context(), "Actor", "person")
	if err != nil || len(results) != 0 {
		t.Fatalf("blocked results = %+v, %v", results, err)
	}
	svc.SetAllowAdultSearch(true)
	results, err = svc.Search(t.Context(), "Actor", "person")
	if err != nil || len(results) != 1 || !results[0].Title.Adult {
		t.Fatalf("allowed results = %+v, %v", results, err)
	}
	if strings.Join(policies, ",") != "false,true" {
		t.Fatalf("adult policy cache separation = %v", policies)
	}
}

func TestActorSearchIdentityIsSeparateFromMovie(t *testing.T) {
	results := mergeSearchResults([]models.SearchResult{
		{Title: models.Title{MediaType: "movie", TMDBID: 1}},
		{Title: models.Title{MediaType: "person", TMDBID: 1}},
	})
	if len(results) != 2 {
		t.Fatalf("person and movie IDs collided: %+v", results)
	}
}
