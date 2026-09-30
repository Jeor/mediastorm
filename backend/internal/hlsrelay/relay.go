// Package hlsrelay provides a session-local HLS input for FFmpeg. It preserves
// playlist structure and upstream request context while removing image headers
// from otherwise valid MPEG-TS segments.
package hlsrelay

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type resource struct {
	url       string
	normalize bool
}

type entry struct {
	resource
	used     time.Time
	playlist bool
}

type Relay struct {
	URL       string
	ctx       context.Context
	cancel    context.CancelFunc
	client    *http.Client
	headers   http.Header
	server    *http.Server
	base      string
	prefix    string
	mu        sync.Mutex
	resources map[string]entry
	paths     map[resource]string
	next      uint64
	closeOnce sync.Once
}

// Start binds only to loopback and exposes opaque, unguessable resource paths.
// Call Close when the FFmpeg run ends; context cancellation also closes it.
func Start(ctx context.Context, upstream string, headers http.Header, client *http.Client) (*Relay, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		listener.Close()
		return nil, err
	}
	relayCtx, cancel := context.WithCancel(ctx)
	if headers == nil {
		headers = make(http.Header)
	}
	p := &Relay{
		ctx: relayCtx, cancel: cancel, client: client, headers: headers.Clone(),
		base:      "http://" + listener.Addr().String() + "/" + hex.EncodeToString(token[:]),
		resources: make(map[string]entry), paths: make(map[resource]string),
	}
	p.prefix = "/" + hex.EncodeToString(token[:])
	p.URL, err = p.register(resource{url: upstream, normalize: true})
	if err != nil {
		listener.Close()
		cancel()
		return nil, err
	}
	p.server = &http.Server{Handler: http.HandlerFunc(p.serve), ReadHeaderTimeout: 5 * time.Second}
	go p.server.Serve(listener)
	go func() {
		<-relayCtx.Done()
		p.Close()
	}()
	return p, nil
}

func (p *Relay) Close() {
	p.closeOnce.Do(func() {
		p.cancel()
		p.server.Close()
		p.client.CloseIdleConnections()
	})
}

func (p *Relay) register(r resource) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if path, ok := p.paths[r]; ok {
		e := p.resources[path]
		e.used = now
		p.resources[path] = e
		return p.base + path, nil
	}
	// Live playlists introduce new signed URLs throughout a match. Retain a
	// generous fetch/retry window without growing the session map indefinitely.
	for path, e := range p.resources {
		if !e.playlist && now.Sub(e.used) > 5*time.Minute {
			delete(p.resources, path)
			delete(p.paths, e.resource)
		}
	}
	if len(p.resources) >= 4096 {
		return "", fmt.Errorf("too many live HLS resources")
	}
	p.next++
	path := "/" + strconv.FormatUint(p.next, 10) + ".m3u8"
	p.resources[path] = entry{resource: r, used: now}
	p.paths[r] = path
	return p.base + path, nil
}

func (p *Relay) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	prefix := p.prefix
	if !strings.HasPrefix(r.URL.Path, prefix+"/") {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, prefix)
	p.mu.Lock()
	e, ok := p.resources[path]
	if ok {
		e.used = time.Now()
		p.resources[path] = e
	}
	p.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()
	// GET also lets HEAD report the normalized segment's actual length.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.url, nil)
	if err != nil {
		http.Error(w, "invalid upstream resource", http.StatusBadGateway)
		return
	}
	req.Header = p.headers.Clone()
	if value := r.Header.Get("Range"); value != "" && value != "bytes=0-" {
		req.Header.Set("Range", value)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, "upstream HLS request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "upstream HLS resource unavailable", resp.StatusCode)
		return
	}
	reader := bufio.NewReaderSize(resp.Body, 8192)
	sample, readErr := reader.Peek(8192)
	if readErr != nil && readErr != io.EOF && readErr != bufio.ErrBufferFull {
		http.Error(w, "upstream HLS read failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(string(sample), "\ufeff")), "#EXTM3U") {
		// FFmpeg may revisit a master/alternate rendition after a long match.
		// Keep playlists registered; only rolling media/key URLs expire.
		p.mu.Lock()
		e.playlist = true
		p.resources[path] = e
		p.mu.Unlock()
		body, err := io.ReadAll(io.LimitReader(reader, 1024*1024+1))
		if err != nil || len(body) > 1024*1024 {
			http.Error(w, "invalid upstream HLS playlist", http.StatusBadGateway)
			return
		}
		// Relative resources are resolved against the final URL after redirects.
		playlist, err := p.rewritePlaylist(string(body), resp.Request.URL)
		if err != nil {
			http.Error(w, "invalid upstream HLS resources", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Content-Length", strconv.Itoa(len(playlist)))
		if r.Method != http.MethodHead {
			io.WriteString(w, playlist)
		}
		return
	}
	offset := 0
	// Byte-range resources and encrypted segments must remain byte-exact.
	if e.normalize && (r.Header.Get("Range") == "" || r.Header.Get("Range") == "bytes=0-") && resp.StatusCode == http.StatusOK {
		offset = imageTransportStreamOffset(sample)
	}
	for _, name := range []string{"Content-Type", "Content-Range", "Accept-Ranges"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	if offset > 0 {
		reader.Discard(offset)
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Accept-Ranges", "none")
	}
	if resp.ContentLength >= int64(offset) {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength-int64(offset), 10))
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		io.Copy(w, reader)
	}
}
