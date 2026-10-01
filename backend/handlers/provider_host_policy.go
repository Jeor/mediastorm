package handlers

import (
	"net"
	"net/url"
	"strings"

	"novastream/internal/requestsecurity"
)

func configuredProviderHostPolicy(configManager ConfigProvider) requestsecurity.RestrictedHostPolicy {
	allowed := make(map[string]struct{})
	addURLOrigin := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed == nil {
			return
		}
		scheme := strings.ToLower(parsed.Scheme)
		if parsed.Hostname() != "" && (scheme == "http" || scheme == "https") {
			port := parsed.Port()
			if port == "" {
				switch scheme {
				case "http":
					port = "80"
				case "https":
					port = "443"
				}
			}
			if port != "" {
				allowed[privateMediaEndpointKey(parsed.Hostname(), port)] = struct{}{}
			}
		}
	}
	addLivePlaylistOrigin := func(raw string) {
		addURLOrigin(raw)
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed == nil || parsed.Hostname() == "" || parsed.Path != "/lineup.m3u" {
			return
		}
		if scheme := strings.ToLower(parsed.Scheme); scheme != "http" && scheme != "https" {
			return
		}
		// HDHomeRun advertises the lineup on its web origin, but channel
		// streams use HTTP port 5004 on the same tuner. Trust that endpoint
		// only for explicitly configured native HDHomeRun playlists.
		allowed[privateMediaEndpointKey(parsed.Hostname(), "5004")] = struct{}{}
	}
	if configManager != nil {
		if settings, err := configManager.Load(); err == nil {
			for _, origin := range settings.Server.AllowedPrivateMediaOrigins {
				addURLOrigin(origin)
			}
			for _, engine := range settings.UsenetEngines {
				if engine.Enabled {
					addURLOrigin(engine.BaseURL)
					addURLOrigin(engine.WebDAVBaseURL)
				}
			}
			for _, indexer := range settings.Indexers {
				if indexer.Enabled {
					addURLOrigin(indexer.URL)
				}
			}
			for _, scraper := range settings.TorrentScrapers {
				if scraper.Enabled {
					addURLOrigin(scraper.URL)
				}
			}
			addLivePlaylistOrigin(settings.Live.PlaylistURL)
			addURLOrigin(settings.Live.ManifestURL)
			addURLOrigin(settings.Live.XtreamHost)
			addURLOrigin(settings.Live.StalkerPortalURL)
			for _, source := range append(settings.Live.Sources, settings.Live.PlaylistSources...) {
				if source.Enabled == nil || *source.Enabled {
					addLivePlaylistOrigin(source.PlaylistURL)
					addURLOrigin(source.ManifestURL)
					addURLOrigin(source.XtreamHost)
					addURLOrigin(source.StalkerPortalURL)
				}
			}
		}
	}
	return func(hostname, port string) bool {
		_, ok := allowed[privateMediaEndpointKey(hostname, port)]
		return ok
	}
}

func privateMediaEndpointKey(hostname, port string) string {
	hostname = strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(hostname), "[]"), "."))
	return net.JoinHostPort(hostname, strings.TrimSpace(port))
}
