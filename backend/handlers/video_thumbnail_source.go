package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"novastream/internal/torboxrate"
)

const thumbnailSourcePrewarmTimeout = 15 * time.Second
const thumbnailSourcePrewarmTTL = 5 * time.Minute

const (
	thumbnailSharedSourceCacheDir     = "shared-source-cache"
	thumbnailSharedSourceCacheMaxAge  = 6 * time.Hour
	thumbnailSharedSourceCacheMaxSize = int64(2 * 1024 * 1024 * 1024)
)

type thumbnailSourceCacheEntry struct {
	name    string
	path    string
	modTime time.Time
	size    int64
}

// thumbnailSourceBridge keeps one upstream HTTP transport alive for all frame
// extraction processes. Stable FFmpeg releases do not yet have the shared:
// protocol, so this removes repeated DNS/TLS/redirect setup without tying the
// backend to libavformat's C ABI.
type thumbnailSourceBridge struct {
	mu       sync.RWMutex
	sessions map[string]thumbnailSourceSession
	client   *http.Client
	once     sync.Once
	baseURL  string
	secret   string
	err      error
}

type thumbnailSourceSession struct {
	sourceURL  string
	authHeader string
}

func newThumbnailSourceBridge() *thumbnailSourceBridge {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 16
	transport.IdleConnTimeout = 2 * time.Minute
	return &thumbnailSourceBridge{
		sessions: make(map[string]thumbnailSourceSession),
		client: &http.Client{
			Transport:     transport,
			CheckRedirect: thumbnailRedirectPolicy,
		},
	}
}

func thumbnailRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if len(via) == 0 || req == nil || req.URL == nil || via[0] == nil || via[0].URL == nil {
		return nil
	}
	// WebDAV servers commonly redirect within their own origin and still require
	// Basic auth on the redirected request. Preserve that behavior without
	// forwarding credentials to a CDN or any other cross-origin destination.
	if strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme) && strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		if authorization := via[0].Header.Get("Authorization"); authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
	}
	return nil
}

func (b *thumbnailSourceBridge) register(key, sourceURL, authHeader string) (string, error) {
	if b == nil {
		return "", fmt.Errorf("thumbnail source bridge unavailable")
	}
	b.once.Do(b.start)
	if b.err != nil {
		return "", b.err
	}
	b.mu.Lock()
	b.sessions[key] = thumbnailSourceSession{sourceURL: sourceURL, authHeader: authHeader}
	b.mu.Unlock()
	return b.baseURL + "/" + key, nil
}

func (b *thumbnailSourceBridge) start() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.err = fmt.Errorf("listen for thumbnail source bridge: %w", err)
		return
	}
	secretBytes := make([]byte, 18)
	if _, err := rand.Read(secretBytes); err != nil {
		_ = listener.Close()
		b.err = fmt.Errorf("create thumbnail source bridge secret: %w", err)
		return
	}
	b.secret = hex.EncodeToString(secretBytes)
	b.baseURL = "http://" + listener.Addr().String() + "/thumbnail-source/" + b.secret
	server := &http.Server{
		Handler:           http.HandlerFunc(b.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[thumbnails] source bridge stopped: %v", err)
		}
	}()
}

func (b *thumbnailSourceBridge) serveHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "thumbnail-source" || subtle.ConstantTimeCompare([]byte(parts[1]), []byte(b.secret)) != 1 || !validThumbnailKey(parts[2]) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	session, ok := b.sessions[parts[2]]
	b.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	response, err := b.doRequest(r.Context(), r.Method, session, r.Header)
	if err != nil {
		http.Error(w, "thumbnail source unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	for _, name := range []string{"Accept-Ranges", "Cache-Control", "Content-Length", "Content-Range", "Content-Type", "ETag", "Last-Modified"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

func (b *thumbnailSourceBridge) doRequest(ctx context.Context, method string, session thumbnailSourceSession, sourceHeaders http.Header) (*http.Response, error) {
	isTorboxDownload := torboxrate.IsDownloadURL(session.sourceURL)
	for attempt := 0; ; attempt++ {
		if isTorboxDownload {
			if err := torboxrate.Downloads.Wait(ctx, session.sourceURL); err != nil {
				return nil, err
			}
		}

		upstream, err := http.NewRequestWithContext(ctx, method, session.sourceURL, nil)
		if err != nil {
			return nil, err
		}
		for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
			if value := sourceHeaders.Get(name); value != "" {
				upstream.Header.Set(name, value)
			}
		}
		applyRawHTTPHeaders(upstream.Header, session.authHeader)
		response, err := b.client.Do(upstream)
		if err != nil || !isTorboxDownload || response.StatusCode != http.StatusTooManyRequests {
			return response, err
		}

		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		response.Body.Close()
		delay := torboxrate.Downloads.Record(session.sourceURL, response.Header.Get("Retry-After"), body)
		if attempt >= 1 {
			response.Body = io.NopCloser(bytes.NewReader(body))
			response.ContentLength = int64(len(body))
			return response, nil
		}
		log.Printf("[thumbnails] TorBox CDN rate limited source bridge; retrying after %s", delay.Round(time.Millisecond))
	}
}

func applyRawHTTPHeaders(headers http.Header, raw string) {
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(name) != "" {
			headers.Set(strings.TrimSpace(name), strings.TrimSpace(value))
		}
	}
}

func (b *thumbnailSourceBridge) prewarm(ctx context.Context, key, sourceURL, authHeader string) error {
	if _, err := b.register(key, sourceURL, authHeader); err != nil {
		return err
	}
	resp, err := b.doRequest(ctx, http.MethodHead, thumbnailSourceSession{sourceURL: sourceURL, authHeader: authHeader}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func detectFFmpegSharedProtocol(ctx context.Context, ffmpegPath string) (bool, error) {
	path := strings.TrimSpace(ffmpegPath)
	if path == "" {
		path = "ffmpeg"
	}
	output, err := exec.CommandContext(ctx, path, "-hide_banner", "-protocols").CombinedOutput()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) == "shared" {
			return true, nil
		}
	}
	return false, nil
}

func detectFFmpegHTTPInputOptions(ctx context.Context, ffmpegPath string) ([]string, error) {
	path := strings.TrimSpace(ffmpegPath)
	if path == "" {
		path = "ffmpeg"
	}
	output, err := exec.CommandContext(ctx, path, "-hide_banner", "-h", "protocol=http").CombinedOutput()
	if err != nil {
		return nil, err
	}
	available := string(output)
	options := make([]string, 0, 8)
	if strings.Contains(available, "-multiple_requests") {
		options = append(options, "-multiple_requests", "1")
	}
	if strings.Contains(available, "-short_seek_size") {
		options = append(options, "-short_seek_size", "1048576")
	}
	if strings.Contains(available, "-initial_request_size") {
		options = append(options, "-initial_request_size", "2097152")
	}
	if strings.Contains(available, "-request_size") {
		options = append(options, "-request_size", "33554432")
	}
	return options, nil
}

func (m *ThumbnailManager) sharedProtocolAvailable() bool {
	m.protocolOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.sharedProtocol, m.protocolErr = detectFFmpegSharedProtocol(ctx, m.ffmpegPath)
		if m.protocolErr != nil {
			log.Printf("[thumbnails] unable to inspect ffmpeg protocols: %v", m.protocolErr)
		}
		if m.sharedProtocol {
			log.Printf("[thumbnails] native ffmpeg shared source cache enabled")
		}
	})
	return m.sharedProtocol
}

func (m *ThumbnailManager) ffmpegHTTPOptions() []string {
	m.httpOptionsOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		options, err := detectFFmpegHTTPInputOptions(ctx, m.ffmpegPath)
		if err != nil {
			log.Printf("[thumbnails] unable to inspect ffmpeg HTTP options: %v", err)
			return
		}
		m.httpInputOptions = options
	})
	return append([]string(nil), m.httpInputOptions...)
}

func (m *ThumbnailManager) frameInput(key, sourceURL, authHeader string) (string, string, []string) {
	bridgeURL, bridgeErr := m.sourceBridge.register(key, sourceURL, authHeader)
	httpOptions := m.ffmpegHTTPOptions()
	if m.sharedProtocolAvailable() {
		cacheDir := m.sharedSourceCacheDir(key)
		if bridgeErr == nil {
			if err := os.MkdirAll(cacheDir, 0o755); err == nil {
				return "shared:" + bridgeURL, "", append([]string{"-cache_dir", cacheDir}, httpOptions...)
			}
		}
	}
	if bridgeErr == nil {
		return bridgeURL, "", httpOptions
	}
	log.Printf("[thumbnails] source bridge unavailable key=%s: %v", key, bridgeErr)
	return sourceURL, authHeader, nil
}

func (m *ThumbnailManager) sharedSourceCacheRoot() string {
	return filepath.Join(m.baseDir, thumbnailSharedSourceCacheDir)
}

func (m *ThumbnailManager) sharedSourceCacheDir(key string) string {
	return filepath.Join(m.sharedSourceCacheRoot(), key)
}

func (m *ThumbnailManager) removeSharedSourceCache(key string) {
	if m == nil || !validThumbnailKey(key) {
		return
	}
	if err := os.RemoveAll(m.sharedSourceCacheDir(key)); err != nil {
		log.Printf("[thumbnails] unable to remove transient source cache key=%s: %v", key, err)
	}
}

func thumbnailSourceCacheEntrySize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(current string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// pruneSharedSourceCache removes cache files orphaned by older releases or an
// interrupted thumbnail generation. Active generations are kept even when the
// remaining cache temporarily exceeds the configured ceiling.
func (m *ThumbnailManager) pruneSharedSourceCache(now time.Time) {
	if m == nil {
		return
	}
	root := m.sharedSourceCacheRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[thumbnails] unable to inspect transient source cache: %v", err)
		}
		return
	}

	m.mu.Lock()
	active := make(map[string]struct{}, len(m.inFlight))
	for key := range m.inFlight {
		active[key] = struct{}{}
	}
	m.mu.Unlock()

	candidates := make([]thumbnailSourceCacheEntry, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if _, ok := active[entry.Name()]; ok {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		// Flat files are leftovers from releases that did not isolate cache data
		// per media key. No running generation can own one after this version starts.
		if !entry.IsDir() || now.Sub(info.ModTime()) > thumbnailSharedSourceCacheMaxAge {
			if removeErr := os.RemoveAll(path); removeErr != nil {
				log.Printf("[thumbnails] unable to prune orphaned source cache %q: %v", entry.Name(), removeErr)
			}
			continue
		}
		size := thumbnailSourceCacheEntrySize(path)
		total += size
		candidates = append(candidates, thumbnailSourceCacheEntry{
			name:    entry.Name(),
			path:    path,
			modTime: info.ModTime(),
			size:    size,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.Before(candidates[j].modTime)
	})
	for _, candidate := range candidates {
		if total <= thumbnailSharedSourceCacheMaxSize {
			break
		}
		if removeErr := os.RemoveAll(candidate.path); removeErr != nil {
			log.Printf("[thumbnails] unable to enforce source cache limit for %q: %v", candidate.name, removeErr)
			continue
		}
		total -= candidate.size
	}
}

func (m *ThumbnailManager) prewarm(cleanPath, sourceURL, authHeader string) {
	if m == nil || strings.TrimSpace(sourceURL) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), thumbnailSourcePrewarmTimeout)
	defer cancel()
	key := thumbnailKey(cleanPath)
	m.prewarmMu.Lock()
	if last := m.prewarmed[key]; !last.IsZero() && time.Since(last) < thumbnailSourcePrewarmTTL {
		m.prewarmMu.Unlock()
		return
	}
	m.prewarmed[key] = time.Now()
	m.prewarmMu.Unlock()
	if err := m.sourceBridge.prewarm(ctx, key, sourceURL, authHeader); err != nil {
		m.prewarmMu.Lock()
		delete(m.prewarmed, key)
		m.prewarmMu.Unlock()
		log.Printf("[thumbnails] source prewarm failed key=%s path=%q: %v", key, cleanPath, err)
		return
	}
	log.Printf("[thumbnails] source prewarmed key=%s path=%q", key, cleanPath)
}
