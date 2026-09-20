package epg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"novastream/config"
)

// Only infer the documented Xtream playlist endpoint, never arbitrary credential URLs.
func inferPlaylistGuide(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !strings.EqualFold(path.Base(u.Path), "get.php") {
		return ""
	}
	q := u.Query()
	if q.Get("username") == "" || q.Get("password") == "" {
		return ""
	}
	u.Path = path.Join(path.Dir(u.Path), "xmltv.php")
	u.RawPath = ""
	u.RawQuery = url.Values{"username": {q.Get("username")}, "password": {q.Get("password")}}.Encode()
	u.Fragment = ""
	return u.String()
}

func discoveryPlaylists(settings config.Settings) []config.LivePlaylistSource {
	sources := configuredLiveSources(settings)
	if len(sources) == 0 {
		sources = []config.LivePlaylistSource{{Mode: settings.Live.Mode, PlaylistURL: settings.Live.PlaylistURL, ProxyURL: settings.Live.ProxyURL, EPG: settings.Live.EPG}}
	}
	var result []config.LivePlaylistSource
	for _, source := range sources {
		mode := strings.ToLower(strings.TrimSpace(source.Mode))
		if !liveSourceEnabled(source) || (mode != "" && mode != "m3u") || (!settings.Live.EPG.Enabled && !source.EPG.Enabled) || strings.TrimSpace(source.PlaylistURL) == "" {
			continue
		}
		// Explicit guide configuration takes precedence over discovery.
		if len(appendEPGXMLTVSources(nil, source.EPG, "", "", 0)) > 0 || (settings.Live.EPG.Enabled && len(appendEPGXMLTVSources(nil, settings.Live.EPG, "", "", 0)) > 0) {
			continue
		}
		if source.ProxyURL == "" {
			source.ProxyURL = settings.Live.ProxyURL
		}
		result = append(result, source)
	}
	return result
}

var playlistGuideAttribute = regexp.MustCompile(`(?i)(?:^|\s)(?:x-tvg-url|url-tvg|tvg-url)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s]+))`)

func playlistHeaderGuides(header string, base *url.URL) []string {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(header, "\ufeff"))), "#EXTM3U") {
		return nil
	}
	var result []string
	seen := map[string]bool{}
	for _, match := range playlistGuideAttribute.FindAllStringSubmatch(header, -1) {
		value := match[1] + match[2] + match[3]
		for _, raw := range strings.Split(value, ",") {
			ref, err := url.Parse(strings.TrimSpace(raw))
			if err != nil || strings.TrimSpace(raw) == "" {
				continue
			}
			ref = base.ResolveReference(ref)
			if (ref.Scheme != "http" && ref.Scheme != "https") || ref.Host == "" || seen[ref.String()] {
				continue
			}
			seen[ref.String()] = true
			result = append(result, ref.String())
		}
	}
	return result
}

func (s *Service) discoverPlaylistHeaders(ctx context.Context, settings config.Settings) []epgXMLTVSource {
	var result []epgXMLTVSource
	for i, source := range discoveryPlaylists(settings) {
		if inferPlaylistGuide(source.PlaylistURL) != "" {
			continue
		}
		guides, err := s.fetchPlaylistHeaderGuides(ctx, source)
		if err != nil {
			// HTTP errors may contain credential-bearing URLs. Never log the raw error.
			log.Printf("[epg] playlist guide discovery failed sourceIndex=%d", i)
			continue
		}
		log.Printf("[epg] playlist guide discovery sourceIndex=%d guides=%d", i, len(guides))
		for _, guide := range guides {
			result = append(result, epgXMLTVSource{name: fmt.Sprintf("playlist guide %d", i+1), url: guide, proxyURL: source.ProxyURL, inferred: true})
		}
	}
	return result
}

func (s *Service) fetchPlaylistHeaderGuides(ctx context.Context, source config.LivePlaylistSource) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.PlaylistURL, nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := doXtreamRequestWithUserAgents(req, s.httpClient(source.ProxyURL), xtreamUserAgents)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playlist status %d", resp.StatusCode)
	}
	// Read only the header, not a potentially enormous channel playlist.
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 64*1024))
	scanner.Buffer(make([]byte, 4096), 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		return playlistHeaderGuides(line, resp.Request.URL), nil
	}
	return nil, scanner.Err()
}
