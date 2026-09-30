package hlsrelay

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func tsFixture() []byte {
	data := make([]byte, 188*10)
	for i := 0; i < len(data); i += 188 {
		data[i] = 0x47
	}
	return data
}

func wrappedTS(data []byte) []byte {
	prefix := make([]byte, 42)
	copy(prefix, "RIFF")
	copy(prefix[8:], "WEBPVP8L")
	return append(prefix, data...)
}

func startTestRelay(t *testing.T, upstream string, headers http.Header) *Relay {
	t.Helper()
	p, err := Start(context.Background(), upstream, headers, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func readURL(t *testing.T, address string) ([]byte, http.Header) {
	t.Helper()
	resp, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET failed: status=%d error=%v body=%q", resp.StatusCode, err, data)
	}
	return data, resp.Header
}

func TestImageTransportStreamOffset(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
		want int
	}{
		{"wrapped TS", wrappedTS(tsFixture()), 42},
		{"plain TS", tsFixture(), 0},
		{"real WebP", wrappedTS([]byte("image pixels")), 0},
		{"truncated packet signature", wrappedTS(tsFixture()[:188*3]), 0},
		{"random prefix before TS", append(bytes.Repeat([]byte{'x'}, 42), tsFixture()...), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := imageTransportStreamOffset(test.data); got != test.want {
				t.Fatalf("offset=%d, want %d", got, test.want)
			}
		})
	}
}

func TestRelayRedirectHeadersAndWrappedSegments(t *testing.T) {
	ts := tsFixture()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://embed.test/" || r.Header.Get("User-Agent") != "provider-player" {
			t.Errorf("lost provider headers: %v", r.Header)
		}
		switch r.URL.Path {
		case "/entry":
			http.Redirect(w, r, "/cdn/master.m3u8", http.StatusFound)
		case "/cdn/master.m3u8":
			io.WriteString(w, "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"audio.m3u8?token=audio\"\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nvideo.m3u8?token=video\n")
		case "/cdn/video.m3u8", "/cdn/audio.m3u8":
			if r.URL.Query().Get("token") == "" {
				t.Error("lost signed query")
			}
			io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nsegment.webp?token=segment\n#EXT-X-ENDLIST\n")
		case "/cdn/segment.webp":
			if r.URL.Query().Get("token") != "segment" || r.Header.Get("Range") != "" {
				t.Errorf("bad segment request: %v", r)
			}
			w.Header().Set("Content-Type", "image/webp")
			w.Write(wrappedTS(ts))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	p := startTestRelay(t, upstream.URL+"/entry", http.Header{"Referer": {"https://embed.test/"}, "User-Agent": {"provider-player"}})
	master, _ := readURL(t, p.URL)
	if strings.Contains(string(master), upstream.URL) || strings.Contains(string(master), "token=") {
		t.Fatalf("upstream URI leaked from playlist: %s", master)
	}
	for _, ref := range []string{uriAttribute.FindStringSubmatch(string(master))[1], strings.Split(string(master), "\n")[3]} {
		media, _ := readURL(t, ref)
		segment := strings.Split(string(media), "\n")[3]
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req, _ := http.NewRequest(method, segment, nil)
			req.Header.Set("Range", "bytes=0-") // FFmpeg's whole-resource fetch.
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.ContentLength != int64(len(ts)) || resp.Header.Get("Content-Type") != "video/mp2t" {
				t.Fatalf("normalized response: length=%d headers=%v err=%v", resp.ContentLength, resp.Header, err)
			}
			if method == http.MethodGet && !bytes.Equal(data, ts) {
				t.Error("MPEG-TS payload changed")
			}
		}
	}
}

func TestRelayPreservesEncryptedMediaKeysAndInit(t *testing.T) {
	wrapped := wrappedTS(tsFixture())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media.m3u8" {
			io.WriteString(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n#EXT-X-MAP:URI=\"init\"\n#EXTINF:4,\nencrypted.ts\n#EXT-X-KEY:METHOD=NONE\n#EXTINF:4,\nplain.ts\n")
			return
		}
		w.Write(wrapped)
	}))
	defer upstream.Close()
	p := startTestRelay(t, upstream.URL+"/media.m3u8", nil)
	playlist, _ := readURL(t, p.URL)
	lines := strings.Split(string(playlist), "\n")
	for _, address := range []string{uriAttribute.FindStringSubmatch(lines[1])[1], uriAttribute.FindStringSubmatch(lines[2])[1], lines[4]} {
		data, _ := readURL(t, address)
		if !bytes.Equal(data, wrapped) {
			t.Error("encrypted media/key/init was modified")
		}
	}
	plain, _ := readURL(t, lines[7])
	if !bytes.Equal(plain, tsFixture()) {
		t.Error("METHOD=NONE did not restore segment normalization")
	}
}

func TestRelayPreservesByteRangesAndOrdinaryMedia(t *testing.T) {
	ts := tsFixture()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=42-229" {
			w.Header().Set("Content-Range", "bytes 42-229/1922")
			w.WriteHeader(http.StatusPartialContent)
			w.Write(ts[:188])
			return
		}
		w.Write(ts)
	}))
	defer upstream.Close()
	p := startTestRelay(t, upstream.URL+"/segment.ts", nil)
	data, _ := readURL(t, p.URL)
	if !bytes.Equal(data, ts) {
		t.Fatal("plain TS changed")
	}
	req, _ := http.NewRequest(http.MethodGet, p.URL, nil)
	req.Header.Set("Range", "bytes=42-229")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != 206 || resp.Header.Get("Content-Range") != "bytes 42-229/1922" || !bytes.Equal(data, ts[:188]) {
		t.Fatalf("byte range changed: %d %v", resp.StatusCode, resp.Header)
	}
}

func TestRelayCancellationStopsInflightFetch(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, upstream.URL, nil, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	go func() {
		resp, err := http.Get(p.URL)
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation left upstream fetch running")
	}
}

func TestRelayRejectsUnknownResourceAndUnsafeScheme(t *testing.T) {
	p := startTestRelay(t, "https://unused.test/master.m3u8", nil)
	resp, err := http.Get(p.base + "/999.m3u8?url=http://attacker.test/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown resource accepted: %d", resp.StatusCode)
	}
	base, _ := url.Parse("https://upstream.test/master.m3u8")
	if _, err := p.rewritePlaylist("#EXTM3U\nfile:///etc/passwd\n", base); err == nil {
		t.Fatal("file URI accepted")
	}
}

func TestRelayRetainsPlaylistsWhileExpiringOldSegments(t *testing.T) {
	p := startTestRelay(t, "https://upstream.test/master.m3u8", nil)
	masterPath := "/1.m3u8"
	p.resources[masterPath] = entry{resource: resource{url: "https://upstream.test/master.m3u8", normalize: true}, used: time.Now().Add(-6 * time.Minute), playlist: true}
	segment := resource{url: "https://upstream.test/old.ts", normalize: true}
	address, err := p.register(segment)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(address, p.base)
	e := p.resources[path]
	e.used = time.Now().Add(-6 * time.Minute)
	p.resources[path] = e
	if _, err := p.register(resource{url: "https://upstream.test/new.ts", normalize: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.resources[path]; ok {
		t.Error("expired segment retained")
	}
	if _, ok := p.resources[masterPath]; !ok {
		t.Error("master playlist expired during live playback")
	}
}
