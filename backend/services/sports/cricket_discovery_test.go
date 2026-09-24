package sports

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCricketDropdownSeparatesSeriesAndPreservesIPL(t *testing.T) {
	rows, err := parseCricketDiscovery([]byte(`{"leagues":[{"name":"Olympics 2028","id":"1481569","slug":""},{"name":"The Ashes 2027","slug":"24627"},{"name":"Indian Premier League","slug":"8048"},{"name":"Duplicate","slug":"8048"},{"name":"Bad","slug":"../all"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ProviderSeriesID != "24627" || rows[0].Season != "2027" || rows[0].Kind != "series" || rows[1].ID != "cricket-8048" || rows[1].Kind != "competition" {
		t.Fatal(rows)
	}
}

type cricketDiscoveryTransport struct {
	url  string
	base http.RoundTripper
}

func (r cricketDiscoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *req.URL
	clone.URL = &u
	if req.URL.String() != cricketDropdownURL {
		return nil, fmt.Errorf("unexpected provider URL %s", req.URL)
	}
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.url, "http://")
	return r.base.RoundTrip(clone)
}
func TestCricketDiscoveryPersistsAndKeepsLastGood(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count > 1 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"leagues":[{"name":"The Ashes 2027","slug":"24627"}]}`)
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = cricketDiscoveryTransport{server.URL, client.Transport}
	s := &Service{storageDir: t.TempDir(), client: client}
	rows, err := s.DiscoverCricketSeries(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if _, err = s.DiscoverCricketSeries(context.Background()); err != nil || count != 1 {
		t.Fatal("fresh cache refetched", err, count)
	}
	path := filepath.Join(s.storageDir, sportsCacheDir, "cricket-series.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Expire the persisted snapshot without mutating the global catalog.
	if !strings.Contains(string(data), `"complete":false`) {
		t.Fatal("claimed exhaustive dropdown")
	}
	_ = os.WriteFile(path, []byte(fmt.Sprintf(`{"updatedAt":%q,"series":[{"id":"espn:cricket:24627","providerSeriesId":"24627"}]}`, time.Now().Add(-48*time.Hour).Format(time.RFC3339))), 0600)
	rows, err = s.DiscoverCricketSeries(context.Background())
	if err == nil || len(rows) != 1 || rows[0].ProviderSeriesID != "24627" {
		t.Fatal("lost last good data", rows, err)
	}
}
