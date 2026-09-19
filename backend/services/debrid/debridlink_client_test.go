package debrid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func debridLinkTestClient(t *testing.T, handler http.HandlerFunc) *DebridLinkClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing bearer authorization")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	c := NewDebridLinkClient(" test-key ")
	c.baseURL = server.URL
	c.httpClient = server.Client()
	return c
}

func TestDebridLinkAddAndUpload(t *testing.T) {
	for _, upload := range []bool{false, true} {
		t.Run(fmt.Sprint(upload), func(t *testing.T) {
			c := debridLinkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/seedbox/add" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				if upload {
					file, header, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, _ := io.ReadAll(file)
					if string(data) != "torrent-data" || header.Filename != "upload.torrent" {
						t.Error("incorrect upload")
					}
				} else {
					var body struct {
						URL  string `json:"url"`
						Wait bool   `json:"wait"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.URL != "magnet:?xt=urn:btih:abc" || body.Wait {
						t.Errorf("incorrect add body: %+v", body)
					}
				}
				fmt.Fprint(w, `{"success":true,"value":{"id":"torrent-a"}}`)
			})
			var result *AddMagnetResult
			var err error
			if upload {
				result, err = c.AddTorrentFile(context.Background(), []byte("torrent-data"), "")
			} else {
				result, err = c.AddMagnet(context.Background(), "magnet:?xt=urn:btih:abc")
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.ID != "torrent-a" || result.CacheStatusKnown {
				t.Fatalf("unexpected result %+v", result)
			}
		})
	}
}

func TestDebridLinkTorrentReadinessAndLinkAlignment(t *testing.T) {
	for _, percent := range []int{0, 99, 100} {
		t.Run(fmt.Sprint(percent), func(t *testing.T) {
			c := debridLinkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/seedbox/list" || r.URL.Query().Get("ids") != "a" {
					t.Errorf("unexpected URL %s", r.URL)
				}
				fmt.Fprintf(w, `{"success":true,"value":[{"id":"other"},{"id":"a","name":"pack","hashString":"abc","totalSize":30,"downloadPercent":100,"status":8,"files":[{"name":"one.mkv","size":10,"downloadPercent":%d,"downloadUrl":"https://seed20.debrid.link/one"},{"name":"two.mkv","size":20,"downloadPercent":100,"downloadUrl":"https://seed20.debrid.link/two"}]}]}`, percent)
			})
			info, err := c.GetTorrentInfo(context.Background(), "a")
			if err != nil {
				t.Fatal(err)
			}
			if (info.Status == "downloaded") != (percent == 100) {
				t.Fatalf("incorrect readiness: %+v", info)
			}
			if info.Files[1].ID != 2 || info.Files[1].Selected != 1 {
				t.Fatalf("incorrect files: %+v", info.Files)
			}
			if percent < 100 && (info.Files[0].Selected != 0 || len(info.Links) != 1 || !strings.HasSuffix(info.Links[0], "/two")) {
				t.Fatalf("incorrect links: %+v", info)
			}
		})
	}
}

func TestDebridLinkAccountDeleteAndDirectURL(t *testing.T) {
	c := debridLinkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/infos":
			fmt.Fprint(w, `{"success":true,"value":{"username":"Amy","premiumLeft":172800}}`)
		case "/seedbox/a/remove":
			if r.Method != "DELETE" {
				t.Error("expected DELETE")
			}
			fmt.Fprint(w, `{"success":true,"value":["a"]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	info, err := c.GetAccountInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Username != "Amy" || !info.PremiumActive || info.DaysRemaining != 2 || info.ExpiresAt == nil {
		t.Fatalf("unexpected account %+v", info)
	}
	if err := c.DeleteTorrent(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	link := "https://seed20.debrid.link/dl/movie.mkv?token=abc"
	result, err := c.UnrestrictLink(context.Background(), link)
	if err != nil || result.DownloadURL != link {
		t.Fatalf("direct URL changed: %+v %v", result, err)
	}
	if _, err := c.CheckInstantAvailability(context.Background(), "abc"); err == nil {
		t.Fatal("cache check should report unsupported")
	}
	if p, ok := GetProvider("debridlink", "key"); !ok || p.Name() != "debridlink" {
		t.Fatal("provider not registered")
	}
}

func TestDebridLinkRejectsErrorsAndMissingData(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{401, `{}`}, {429, `{}`}, {500, `{}`}, {200, `{"success":false,"error":"badToken"}`}, {200, `invalid`}, {200, `{"success":true}`}, {200, `{"success":true,"value":null}`}, {200, `{"success":true,"value":{}}`},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.body), func(t *testing.T) {
			c := debridLinkTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) })
			if _, err := c.AddMagnet(context.Background(), "magnet:test"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	c := debridLinkTestClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"success":true,"value":[]}`) })
	if _, err := c.GetTorrentInfo(context.Background(), "missing"); err == nil {
		t.Fatal("expected missing torrent error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetAccountInfo(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestDebridLinkWaitsForInitialMetadata(t *testing.T) {
	calls := 0
	c := debridLinkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"success":true,"value":[{"id":"a","files":[]}]}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"value":[{"id":"a","files":[{"name":"movie.mkv","downloadPercent":100,"downloadUrl":"https://seed20.debrid.link/movie"}]}]}`)
	})
	info, err := c.GetTorrentInfo(context.Background(), "a")
	if err != nil || info.Status != "downloaded" || calls != 2 {
		t.Fatalf("metadata retry: info=%+v err=%v calls=%d", info, err, calls)
	}
}
