package plex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type plexRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn plexRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestPlexLibraryItemAcceptsLegacyAndProviderGUIDs(t *testing.T) {
	var item PlexLibraryItem
	err := json.Unmarshal([]byte(`{
		"ratingKey":"10",
		"guid":"plex://movie/abc",
		"Guid":[{"id":"imdb://tt1234567"},{"id":"tmdb://42"}]
	}`), &item)
	if err != nil {
		t.Fatalf("unmarshal Plex item: %v", err)
	}
	if item.GUID != "plex://movie/abc" {
		t.Fatalf("GUID = %q", item.GUID)
	}
	if len(item.Guid) != 2 || item.Guid[0].ID != "imdb://tt1234567" || item.Guid[1].ID != "tmdb://42" {
		t.Fatalf("provider GUIDs = %#v", item.Guid)
	}
}

func TestOpenServerPathRejectsEmptyPath(t *testing.T) {
	client := NewClient("test-client")
	server := PlexResource{
		AccessToken: "tok",
		Connections: []PlexConnection{{Protocol: "https", URI: "https://plex.example:32400"}},
	}
	_, err := client.OpenServerPath(context.Background(), server, "", http.MethodGet, "bytes=0-1")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error=%q, want mention of empty path", err)
	}
}

func TestOpenServerPathRejectsServerRoot(t *testing.T) {
	client := NewClient("test-client")
	server := PlexResource{
		AccessToken: "tok",
		Connections: []PlexConnection{{Protocol: "https", URI: "https://plex.example:32400"}},
	}
	// Whitespace-only collapses to empty after TrimSpace.
	_, err := client.OpenServerPath(context.Background(), server, "   ", http.MethodGet, "")
	if err == nil {
		t.Fatal("expected error for blank path")
	}
}

func TestGetServerMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/264995" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"264995","title":"Test","Media":[{"Part":[{"id":7,"key":"/library/parts/7/x/file.mp4","file":"/f.mp4","size":10}]}]}]}}`))
	}))
	defer server.Close()

	client := NewClient("test-client")
	resource := PlexResource{
		AccessToken: "tok",
		Connections: []PlexConnection{{Protocol: "http", URI: server.URL, Local: true}},
	}
	item, err := client.GetServerMetadata(context.Background(), resource, "264995")
	if err != nil {
		t.Fatalf("GetServerMetadata: %v", err)
	}
	if item.RatingKey != "264995" || len(item.Media) != 1 || len(item.Media[0].Part) != 1 {
		t.Fatalf("unexpected item: %#v", item)
	}
	if item.Media[0].Part[0].Key != "/library/parts/7/x/file.mp4" {
		t.Fatalf("part key=%q", item.Media[0].Part[0].Key)
	}
}

func TestReportTimeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/:/timeline" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("ratingKey"); got != "42" {
			t.Fatalf("ratingKey=%q", got)
		}
		if got := r.URL.Query().Get("state"); got != "playing" {
			t.Fatalf("state=%q", got)
		}
		if got := r.URL.Query().Get("time"); got != "12000" {
			t.Fatalf("time=%q", got)
		}
		if got := r.Header.Get("X-Plex-Session-Identifier"); got != "session-1" {
			t.Fatalf("session header=%q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("strmr-test")
	resource := PlexResource{AccessToken: "token", Connections: []PlexConnection{{Protocol: "http", URI: server.URL, Local: true}}}
	if err := client.ReportTimeline(context.Background(), resource, "42", "session-1", "playing", 12*time.Second, 2*time.Hour); err != nil {
		t.Fatalf("ReportTimeline() error = %v", err)
	}
}

func TestNormalizeServerURL(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{" https://plex.example.test:32400/ ", "https://plex.example.test:32400"},
		{"http://100.64.0.10:32400/plex/", "http://100.64.0.10:32400/plex"},
	} {
		got, err := NormalizeServerURL(tc.input)
		if err != nil {
			t.Fatalf("NormalizeServerURL(%q): %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeServerURL(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}

	for _, input := range []string{
		"plex.example.test:32400",
		"ftp://plex.example.test",
		"http://user:pass@plex.example.test",
		"https://plex.example.test?token=secret",
	} {
		if _, err := NormalizeServerURL(input); err == nil {
			t.Fatalf("NormalizeServerURL(%q) unexpectedly succeeded", input)
		}
	}
}

func TestGetServerLibrariesAtUsesSelectedAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if got := r.Header.Get("X-Plex-Token"); got != "server-token" {
			t.Fatalf("X-Plex-Token=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
			{"key":"1","title":"Movies","type":"movie"},
			{"key":"2","title":"Shows","type":"show"},
			{"key":"3","title":"Music","type":"artist"}
		]}}`))
	}))
	defer server.Close()

	client := NewClient("strmr-test")
	resource := PlexResource{
		Name:        "Remote PMS",
		AccessToken: "server-token",
		Connections: []PlexConnection{{Protocol: "http", URI: "http://192.0.2.1:32400", Local: true}},
	}
	libraries, err := client.GetServerLibrariesAt(context.Background(), resource, server.URL)
	if err != nil {
		t.Fatalf("GetServerLibrariesAt() error = %v", err)
	}
	if len(libraries) != 3 || libraries[0].Title != "Movies" || libraries[1].Title != "Shows" || libraries[2].Title != "Music" {
		t.Fatalf("libraries=%#v", libraries)
	}
}

func TestGetServerLibraryItemsHydratesEpisodeParentIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections/7/all" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if got := r.URL.Query().Get("includeGuids"); got != "1" {
			t.Fatalf("includeGuids=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("type") {
		case "4":
			_, _ = w.Write([]byte(`{"MediaContainer":{"totalSize":1,"Metadata":[{
				"ratingKey":"episode-1","grandparentRatingKey":"show-1","grandparentTitle":"World War II with Tom Hanks",
				"title":"The Beginning","type":"episode","year":2026,
				"Guid":[{"id":"tmdb://7060577"},{"id":"tvdb://11564259"}]
			}]}}`))
		case "2":
			_, _ = w.Write([]byte(`{"MediaContainer":{"totalSize":1,"Metadata":[{
				"ratingKey":"show-1","title":"World War II with Tom Hanks","type":"show","year":2026,
				"Guid":[{"id":"imdb://tt40385200"},{"id":"tmdb://316992"},{"id":"tvdb://472884"}]
			}]}}`))
		default:
			t.Fatalf("type=%q", r.URL.Query().Get("type"))
		}
	}))
	defer server.Close()

	client := NewClient("strmr-test")
	resource := PlexResource{AccessToken: "server-token", Connections: []PlexConnection{{Protocol: "http", URI: server.URL, Local: true}}}
	items, err := client.GetServerLibraryItems(resource, "7", "show")
	if err != nil {
		t.Fatalf("GetServerLibraryItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d, want 1", len(items))
	}
	if items[0].GrandparentYear != 2026 || len(items[0].GrandparentGuid) != 3 {
		t.Fatalf("parent identity not hydrated: %#v", items[0])
	}
	if items[0].GrandparentGuid[1].ID != "tmdb://316992" {
		t.Fatalf("parent GUIDs=%#v", items[0].GrandparentGuid)
	}
}

func TestGetWatchHistoryForServerUsesSelectedServerAndAddress(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = plexRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch {
		case req.URL.Host == "plex.tv" && req.URL.Path == "/api/v2/resources":
			body = `[
				{"name":"Wrong","clientIdentifier":"server-wrong","owned":true,"provides":"server","presence":true,"accessToken":"wrong-token","connections":[{"protocol":"https","uri":"https://wrong.example"}]},
				{"name":"Chosen","clientIdentifier":"server-chosen","owned":true,"provides":"server","presence":true,"accessToken":"chosen-token","connections":[{"protocol":"https","uri":"https://automatic.example"}]}
			]`
		case req.URL.Host == "selected.example" && req.URL.Path == "/status/sessions/history/all":
			if got := req.Header.Get("X-Plex-Token"); got != "chosen-token" {
				t.Fatalf("history token = %q", got)
			}
			body = `{"MediaContainer":{"Metadata":[{"ratingKey":"42","title":"Selected Movie","type":"movie"}]}}`
		case req.URL.Host == "selected.example" && req.URL.Path == "/library/metadata/42":
			body = `{"MediaContainer":{"Metadata":[{"ratingKey":"42","title":"Selected Movie","type":"movie","Guid":[{"id":"tmdb://123"}]}]}}`
		default:
			t.Fatalf("unexpected Plex request: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	defer func() { http.DefaultTransport = originalTransport }()

	client := NewClient("strmr-test")
	history, err := client.GetWatchHistoryForServer("account-token", "server-chosen", "https://selected.example", 100, 0)
	if err != nil {
		t.Fatalf("GetWatchHistoryForServer() error = %v", err)
	}
	if len(history) != 1 || history[0].Title != "Selected Movie" || history[0].ExternalIDs["tmdb"] != "123" {
		t.Fatalf("history = %#v", history)
	}
}
