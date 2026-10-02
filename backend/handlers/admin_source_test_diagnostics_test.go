package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"novastream/services/debrid"
)

func TestSourceTestNameMatchingSuggestion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		titles  []string
		numeric int
		enabled bool
	}{
		{name: "empty"},
		{name: "readable", titles: []string{"The.Matrix.1999.1080p.mkv", "1917.2019.1080p.mkv", "1080p"}},
		{name: "numeric", titles: []string{"4673019.mp4", "986606.MP4"}, numeric: 2},
		{name: "mixed", titles: []string{"4673019.mp4", "The.Matrix.1999.1080p.mkv"}, numeric: 1},
		{name: "enabled", titles: []string{"4673019.mp4"}, numeric: 1, enabled: true},
		{name: "duplicate examples", titles: []string{"1.mp4", "1.mp4", "2.mkv", "3.webm", "4.mp4"}, numeric: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var results []debrid.ScrapeResult
			for _, title := range tc.titles {
				results = append(results, debrid.ScrapeResult{Title: title})
			}
			suggestion := sourceTestNameMatchingSuggestion(results, tc.enabled)
			if tc.numeric == 0 {
				if suggestion != nil {
					t.Fatalf("unexpected suggestion: %+v", suggestion)
				}
				return
			}
			if suggestion == nil || suggestion.NumericCount != tc.numeric || suggestion.TestedCount != len(tc.titles) || suggestion.AlreadyEnabled != tc.enabled {
				t.Fatalf("unexpected suggestion: %+v", suggestion)
			}
			if suggestion.Setting != "skipNameFiltering" || len(suggestion.Examples) > 3 {
				t.Fatalf("invalid suggestion: %+v", suggestion)
			}
			if tc.enabled && !strings.Contains(suggestion.Message, "already enabled") {
				t.Fatal(suggestion.Message)
			}
		})
	}
}

func TestScraperTestSuggestsNameMatchingFromEffectiveFilename(t *testing.T) {
	for _, sourceType := range []string{"stremio-direct", "aiostreams", "comet", "mediafusion"} {
		for _, enabled := range []bool{false, true} {
			t.Run(sourceType+map[bool]string{false: "/disabled", true: "/enabled"}[enabled], func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/manifest.json" {
						w.Write([]byte(`{"id":"fixture","name":"Fixture","version":"1.0"}`))
						return
					}
					w.Write([]byte(`{"streams":[
      {"title":"The Matrix (1999)","url":"https://stream.example/4673019.mp4","behaviorHints":{"filename":"4673019.mp4"}},
      {"title":"The.Matrix.1999.1080p.mkv","url":"https://stream.example/986606.mp4","behaviorHints":{"filename":"The.Matrix.1999.1080p.mkv"}},
      {"title":"123.mp4","externalUrl":"https://support.example/"}
     ]}`))
				}))
				defer server.Close()
				body, _ := json.Marshal(TestScraperRequest{Name: "Fixture", Type: sourceType, URL: server.URL + "/manifest.json", SkipNameFiltering: enabled})
				rec := httptest.NewRecorder()
				(&AdminUIHandler{}).TestScraper(rec, httptest.NewRequest(http.MethodPost, "/api/test/scraper", bytes.NewReader(body)))
				var result struct {
					Success    bool                          `json:"success"`
					Suggestion *sourceNameMatchingSuggestion `json:"nameMatchingSuggestion"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if !result.Success || result.Suggestion == nil || result.Suggestion.NumericCount != 1 || result.Suggestion.TestedCount != 2 || result.Suggestion.AlreadyEnabled != enabled {
					t.Fatalf("unexpected response: %s", rec.Body.String())
				}
			})
		}
	}
}

func TestSourceTestStreamResultsFilenamePrecedence(t *testing.T) {
	var streams []sourceTestStream
	if err := json.Unmarshal([]byte(`[
  {"title":"The Matrix (1999)","url":"https://stream.example/%34%36%37.mp4"},
  {"title":"The Matrix (1999)","url":"https://stream.example/play?name=123.mkv"},
  {"title":"123.mp4","infoHash":"0123456789012345678901234567890123456789"}
 ]`), &streams); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		typ     string
		numeric int
	}{{"aiostreams", 3}, {"mediafusion", 3}, {"comet", 1}, {"torrentio", 1}} {
		suggestion := sourceTestNameMatchingSuggestion(sourceTestStreamResults(streams, tc.typ), false)
		if suggestion == nil || suggestion.NumericCount != tc.numeric {
			t.Fatalf("%s: %+v", tc.typ, suggestion)
		}
	}
}

func TestOtherSourceTestsSuggestNameMatching(t *testing.T) {
	for _, sourceType := range []string{"jackett", "prowlarr", "prowlarr-discovery", "zilean", "nyaa", "internetarchive"} {
		t.Run(sourceType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/indexer" {
					w.Write([]byte(`[{"id":3,"name":"Test","protocol":"torrent","enable":true,"supportsSearch":true}]`))
					return
				}
				if r.URL.Query().Get("t") == "caps" {
					w.Write([]byte(`<caps/>`))
					return
				}
				switch sourceType {
				case "zilean":
					w.Write([]byte(`[{"raw_title":"123456.mp4","info_hash":"0123456789abcdef0123456789abcdef01234567"}]`))
				case "internetarchive":
					if r.URL.Path == "/advancedsearch.php" {
						w.Write([]byte(`{"response":{"docs":[{"identifier":"test","title":"Test"}]}}`))
					} else {
						w.Write([]byte(`{"metadata":{"title":"Test"},"files":[{"name":"123456.mp4","format":"h.264","size":"1000"}]}`))
					}
				default:
					w.Write([]byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>123456.mp4</title><link>https://torrent.example/123456</link><infoHash>0123456789abcdef0123456789abcdef01234567</infoHash><torznab:attr name="infohash" value="0123456789abcdef0123456789abcdef01234567"/></item></channel></rss>`))
				}
			}))
			defer server.Close()
			req := TestScraperRequest{Name: "Test", Type: sourceType, URL: server.URL, APIKey: "fixture", Config: map[string]string{"category": "1_2", "filter": "2"}}
			if sourceType == "prowlarr" {
				req.URL += "/3/api"
			}
			if sourceType == "prowlarr-discovery" {
				req.Type = "prowlarr"
			}
			body, _ := json.Marshal(req)
			rec := httptest.NewRecorder()
			(&AdminUIHandler{}).TestScraper(rec, httptest.NewRequest(http.MethodPost, "/api/test/scraper", bytes.NewReader(body)))
			var result struct {
				Success    bool                          `json:"success"`
				Suggestion *sourceNameMatchingSuggestion `json:"nameMatchingSuggestion"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !result.Success || result.Suggestion == nil || result.Suggestion.NumericCount != 1 {
				t.Fatalf("unexpected response: %s", rec.Body.String())
			}
		})
	}
}

func TestSourceTestSampleFailurePreservesConnectionSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			w.Write([]byte(`<caps/>`))
			return
		}
		http.Error(w, "search unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	body, _ := json.Marshal(TestScraperRequest{Name: "Test", Type: "jackett", URL: server.URL, APIKey: "fixture"})
	rec := httptest.NewRecorder()
	(&AdminUIHandler{}).TestScraper(rec, httptest.NewRequest(http.MethodPost, "/api/test/scraper", bytes.NewReader(body)))
	var result struct {
		Success    bool                          `json:"success"`
		Message    string                        `json:"message"`
		Suggestion *sourceNameMatchingSuggestion `json:"nameMatchingSuggestion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.Suggestion != nil || !strings.Contains(result.Message, "checks could not be completed") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}
