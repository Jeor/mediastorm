package hlsrelay

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var uriAttribute = regexp.MustCompile(`\bURI="([^"]*)"`)
var methodAttribute = regexp.MustCompile(`(?:^|,)METHOD=([^,]+)`)

func (p *Relay) rewritePlaylist(body string, base *url.URL) (string, error) {
	lines := strings.Split(body, "\n")
	encrypted := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#EXT-X-KEY:") {
			method := methodAttribute.FindStringSubmatch(strings.TrimPrefix(trimmed, "#EXT-X-KEY:"))
			encrypted = len(method) != 2 || method[1] != "NONE"
		}
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			local, err := p.resolveResource(base, trimmed, !encrypted)
			if err != nil {
				return "", err
			}
			lines[i] = local
			continue
		}
		var rewriteErr error
		// Keys and initialization sections are opaque. Only media parts may
		// contain the camouflage, and AES-encrypted media must stay byte-exact.
		normalize := strings.HasPrefix(trimmed, "#EXT-X-PART:") && !encrypted
		lines[i] = uriAttribute.ReplaceAllStringFunc(line, func(attribute string) string {
			uri := uriAttribute.FindStringSubmatch(attribute)[1]
			local, err := p.resolveResource(base, uri, normalize)
			if err != nil {
				rewriteErr = err
				return attribute
			}
			return `URI="` + local + `"`
		})
		if rewriteErr != nil {
			return "", rewriteErr
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (p *Relay) resolveResource(base *url.URL, ref string, normalize bool) (string, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid HLS resource URI")
	}
	u = base.ResolveReference(u)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported HLS resource scheme")
	}
	return p.register(resource{url: u.String(), normalize: normalize})
}
