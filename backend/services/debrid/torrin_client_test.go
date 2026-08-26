package debrid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTorrinTestClient(server *httptest.Server) *TorrinClient {
	client := NewTorrinClient("test-key")
	client.baseURL = server.URL
	client.httpClient = server.Client()
	return client
}

func requireTorrinAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := r.Header.Get("User-Agent"); got != "mediastorm/1.0" {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestTorrinIsRegistered(t *testing.T) {
	provider, ok := GetProvider("torrin", "key")
	if !ok {
		t.Fatal("torrin provider is not registered")
	}
	if provider.Name() != "torrin" {
		t.Fatalf("provider name = %q, want torrin", provider.Name())
	}
}

func TestTorrinAddMagnetChecksCacheBeforeCreatingJob(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireTorrinAuth(t, r)
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/availability/" + hash:
			_, _ = io.WriteString(w, `{"available":true}`)
		case "/api/jobs":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if got := payload["magnet"]; !strings.Contains(got, hash) {
				t.Fatalf("magnet = %q", got)
			}
			_, _ = io.WriteString(w, `{"id":"job-1","info_hash":"`+hash+`","status":"complete"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := newTorrinTestClient(server).AddMagnet(context.Background(), "magnet:?xt=urn:btih:"+hash)
	if err != nil {
		t.Fatalf("AddMagnet returned error: %v", err)
	}
	if result.ID != "job-1" || !result.CacheStatusKnown || !result.Cached {
		t.Fatalf("result = %#v", result)
	}
	want := []string{"GET /api/availability/" + hash, "POST /api/jobs"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests = %#v, want %#v", requests, want)
	}
}

func TestTorrinAddMagnetDoesNotStartUncachedJob(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/availability/"+hash {
			t.Fatalf("unexpected cold-job request %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"available":false}`)
	}))
	defer server.Close()

	_, err := newTorrinTestClient(server).AddMagnet(context.Background(), "magnet:?xt=urn:btih:"+hash)
	if err == nil || !strings.Contains(err.Error(), "not cached") {
		t.Fatalf("error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestTorrinAddTorrentFileUsesCachedMagnetJobInsteadOfUpload(t *testing.T) {
	info := []byte("d4:name9:movie.mkvee")
	metainfo := append([]byte("d4:info"), info...)
	metainfo = append(metainfo, 'e')
	hash, err := torrentV1InfoHash(metainfo)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/availability/" + hash:
			_, _ = io.WriteString(w, `{"available":true}`)
		case "/api/jobs":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if got := payload["magnet"]; got != "magnet:?xt=urn:btih:"+hash {
				t.Fatalf("magnet = %q", got)
			}
			_, _ = io.WriteString(w, `{"id":"job-upload","status":"complete"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := newTorrinTestClient(server).AddTorrentFile(context.Background(), metainfo, "movie.torrent")
	if err != nil {
		t.Fatalf("AddTorrentFile returned error: %v", err)
	}
	if result.ID != "job-upload" || !result.Cached {
		t.Fatalf("result = %#v", result)
	}
}

func TestTorrinGetTorrentInfoMapsPackFilesAndSignedLinks(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/jobs/job-pack" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "job-pack",
			"info_hash":  "abc",
			"name":       "Show S01",
			"status":     "complete",
			"created_at": now,
			"updated_at": now,
			"files": []map[string]any{
				{"index": 0, "name": "Show.S01E01.mkv", "size": 100},
				{"index": 1, "name": "Show.S01E02.mkv", "size": 200},
			},
			"stream_urls": []map[string]any{
				{"file_name": "Show.S01E01.mkv", "size": 100, "signed_url": "https://stream.torrin.app/one"},
				{"file_name": "Show.S01E02.mkv", "size": 200, "signed_url": "https://stream.torrin.app/two"},
			},
		})
	}))
	defer server.Close()

	info, err := newTorrinTestClient(server).GetTorrentInfo(context.Background(), "job-pack")
	if err != nil {
		t.Fatalf("GetTorrentInfo returned error: %v", err)
	}
	if info.Status != "downloaded" || info.Bytes != 300 || len(info.Files) != 2 || len(info.Links) != 2 {
		t.Fatalf("info = %#v", info)
	}
	if info.Files[0].ID != 1 || info.Files[1].ID != 2 || info.Files[0].Selected != 1 || info.Files[1].Selected != 1 {
		t.Fatalf("files = %#v", info.Files)
	}
	link, filename, index, matched := resolveRestrictedLink(info, "2")
	if !matched || index != 1 || filename != "/Show.S01E02.mkv" || link != "https://stream.torrin.app/two" {
		t.Fatalf("resolved = %q %q %d %t", link, filename, index, matched)
	}
}

func TestTorrinBulkAvailabilityChunksAndDeduplicates(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Hashes []string `json:"hashes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Hashes) > torrinAvailabilityBatchSize {
			t.Fatalf("batch size = %d", len(request.Hashes))
		}
		response := make(map[string]map[string]bool, len(request.Hashes))
		for _, hash := range request.Hashes {
			response[hash] = map[string]bool{"available": strings.HasSuffix(hash, "0")}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	hashes := make([]string, 0, 102)
	for i := range 101 {
		hashes = append(hashes, fmt.Sprintf("%040x", i))
	}
	hashes = append(hashes, hashes[0])
	result, err := newTorrinTestClient(server).CheckInstantAvailabilityBulk(context.Background(), hashes)
	if err != nil {
		t.Fatalf("CheckInstantAvailabilityBulk returned error: %v", err)
	}
	if len(result) != 101 || calls.Load() != 2 {
		t.Fatalf("results/calls = %d/%d", len(result), calls.Load())
	}
	if !result[hashes[0]] || result[hashes[1]] {
		t.Fatalf("availability = %#v", result)
	}
}

func TestTorrinAccountInfoUsesRDCompatibilityEndpoint(t *testing.T) {
	expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/1.0/user" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"username":   "viewer",
			"email":      "viewer@example.com",
			"type":       "premium",
			"premium":    172800,
			"expiration": expires.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	info, err := newTorrinTestClient(server).GetAccountInfo(context.Background())
	if err != nil {
		t.Fatalf("GetAccountInfo returned error: %v", err)
	}
	if info.Username != "viewer" || info.Email != "viewer@example.com" || !info.PremiumActive || info.ExpiresAt == nil {
		t.Fatalf("account info = %#v", info)
	}
}

func TestTorrinUnrestrictPassesThroughSignedHTTPSURL(t *testing.T) {
	result, err := NewTorrinClient("key").UnrestrictLink(context.Background(), "https://stream.torrin.app/hash/file/Movie.mkv?sig=secret")
	if err != nil {
		t.Fatalf("UnrestrictLink returned error: %v", err)
	}
	if result.DownloadURL == "" || result.Filename != "Movie.mkv" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := NewTorrinClient("key").UnrestrictLink(context.Background(), "torrin://internal"); err == nil {
		t.Fatal("expected non-HTTP URL to be rejected")
	}
}
