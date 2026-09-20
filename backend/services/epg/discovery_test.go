package epg

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"novastream/config"
)

func TestInferPlaylistGuide(t *testing.T) {
	got := inferPlaylistGuide("https://example.com/provider/get.php?username=user+name&password=p%26ss%2B&output=ts&type=m3u_plus#fragment")
	u, err := url.Parse(got)
	if err != nil || u.Path != "/provider/xmltv.php" || u.Query().Get("username") != "user name" || u.Query().Get("password") != "p&ss+" || len(u.Query()) != 2 || u.Fragment != "" {
		t.Fatalf("incorrect inferred guide: %q", got)
	}
	for _, input := range []string{"https://example.com/get.php?username=u", "https://example.com/other?username=u&password=p", "file:///get.php?username=u&password=p"} {
		if inferPlaylistGuide(input) != "" {
			t.Errorf("unexpected inference for %q", input)
		}
	}
}

func TestPlaylistHeaderGuides(t *testing.T) {
	base, _ := url.Parse("https://example.com/provider/list.m3u")
	guides := playlistHeaderGuides("\ufeff#EXTM3U x-tvg-url=\"guide.xml,https://other.example/guide.xml\" url-tvg='guide.xml' tvg-url=javascript:bad", base)
	if len(guides) != 2 || guides[0] != "https://example.com/provider/guide.xml" || guides[1] != "https://other.example/guide.xml" {
		t.Fatalf("unexpected guides: %v", guides)
	}
	if len(playlistHeaderGuides(`#EXTINF:-1 tvg-url="guide.xml"`, base)) != 0 {
		t.Fatal("accepted channel metadata as playlist header")
	}
}

func TestDiscoveryHonorsConfiguration(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Live.EPG = config.EPGSettings{Enabled: true}
	off := false
	settings.Live.Sources = []config.LivePlaylistSource{
		{Mode: "m3u", PlaylistURL: "https://example.com/get.php?username=u&password=p"},
		{Mode: "m3u", PlaylistURL: "https://example.com/get.php?username=u&password=p", Enabled: &off},
		{Mode: "xtream", PlaylistURL: "https://example.com/get.php?username=u&password=p"},
		{Mode: "m3u", PlaylistURL: "https://example.com/get.php?username=u&password=p", EPG: config.EPGSettings{Enabled: true, XmltvUrl: "https://example.com/explicit.xml"}},
	}
	if got := discoveryPlaylists(settings); len(got) != 1 {
		t.Fatalf("candidates = %d, want 1", len(got))
	}
	settings.Live.EPG.Enabled = false
	if len(discoveryPlaylists(settings)) != 0 {
		t.Fatal("discovered disabled EPG")
	}
	settings.Live.Sources = nil
	settings.Live.PlaylistSources = nil
	settings.Live.Mode = "m3u"
	settings.Live.PlaylistURL = "https://example.com/get.php?username=u&password=p"
	settings.Live.EPG.Enabled = true
	if len(collectXMLTVSources(settings)) != 1 {
		t.Fatal("legacy playlist was not inferred")
	}
	settings.Live.EPG.XmltvUrl = "https://example.com/explicit.xml"
	if len(discoveryPlaylists(settings)) != 0 {
		t.Fatal("did not honor explicit global guide")
	}
}

func TestRefreshDiscoversM3UGuide(t *testing.T) {
	for _, kind := range []string{"xtream", "header"} {
		t.Run(kind, func(t *testing.T) {
			guideRequests := 0
			var fail atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fail.Load() {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				switch r.URL.Path {
				case "/playlist.m3u":
					http.Redirect(w, r, "/provider/list.m3u", http.StatusFound)
				case "/provider/list.m3u":
					fmt.Fprint(w, "#EXTM3U url-tvg=\"guide.xml\"\n#EXTINF:-1,Test\nhttps://example.com/stream")
				case "/provider/xmltv.php", "/provider/guide.xml":
					guideRequests++
					if kind == "xtream" && (r.URL.Query().Get("username") != "user name" || r.URL.Query().Get("password") != "p&ss") {
						t.Error("credentials changed")
					}
					now := time.Now().UTC()
					fmt.Fprintf(w, `<tv><channel id="test"><display-name>Test</display-name></channel><programme channel="test" start="%s" stop="%s"><title>Discovered</title></programme></tv>`, now.Add(-time.Hour).Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			settings := config.DefaultSettings()
			settings.Live.EPG = config.EPGSettings{Enabled: true}
			playlist := server.URL + "/playlist.m3u"
			if kind == "xtream" {
				playlist = server.URL + "/provider/get.php?username=user+name&password=p%26ss&type=m3u_plus"
			}
			settings.Live.Sources = []config.LivePlaylistSource{{Mode: "m3u", PlaylistURL: playlist, EPG: config.EPGSettings{Enabled: true}}}
			manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			if err := manager.Save(settings); err != nil {
				t.Fatal(err)
			}
			service := NewService(t.TempDir(), manager)
			<-service.restoreDone
			if err := service.Refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
			status := service.GetStatus()
			if guideRequests != 1 || status.ProgramCount != 1 || status.SourceCount != 1 {
				t.Fatalf("requests=%d status=%+v", guideRequests, status)
			}
			fail.Store(true)
			if err := service.Refresh(context.Background()); err == nil {
				t.Fatal("expected failure")
			}
			if service.GetStatus().ProgramCount != 1 {
				t.Fatal("failure replaced cached schedule")
			}
			if strings.Contains(service.GetStatus().LastError, "password=") {
				t.Fatal("credentials leaked")
			}
		})
	}
}

func TestPlaylistDiscoveryUsesProxy(t *testing.T) {
	requests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Host != "provider.invalid" {
			t.Errorf("host = %q", r.URL.Host)
		}
		fmt.Fprint(w, `#EXTM3U x-tvg-url="guide.xml"`)
	}))
	defer proxy.Close()
	service := &Service{client: proxy.Client()}
	guides, err := service.fetchPlaylistHeaderGuides(context.Background(), config.LivePlaylistSource{PlaylistURL: "http://provider.invalid/list.m3u", ProxyURL: proxy.URL})
	if err != nil || requests != 1 || len(guides) != 1 || guides[0] != "http://provider.invalid/guide.xml" {
		t.Fatalf("guides=%v requests=%d err=%v", guides, requests, err)
	}
}
