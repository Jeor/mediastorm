package streamheaders

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

const fragmentKey = "mediastorm-request-headers"

var allowed = map[string]struct{}{
	"Accept":          {},
	"Accept-Language": {},
	"Origin":          {},
	"Referer":         {},
	"User-Agent":      {},
}

// Sanitize keeps the small set of non-credential request headers commonly
// advertised by Stremio behaviorHints.proxyHeaders. Authorization, Cookie,
// Host, forwarding headers, and values containing control characters are
// deliberately rejected.
func Sanitize(headers map[string]string) map[string]string {
	out := make(map[string]string)
	total := 0
	for name, value := range headers {
		name = http.CanonicalHeaderKey(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if _, ok := allowed[name]; !ok || value == "" || len(value) > 2048 || strings.ContainsAny(value, "\r\n") {
			continue
		}
		total += len(name) + len(value)
		if total > 4096 {
			break
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Attach stores sanitized request headers in the URL fragment. Fragments are
// never sent to an HTTP origin, but survive MediaStorm's nested path encoding
// until its backend proxy can extract and apply them.
func Attach(rawURL string, headers map[string]string) string {
	safe := Sanitize(headers)
	if len(safe) == 0 {
		return rawURL
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL
	}
	payload, err := json.Marshal(safe)
	if err != nil {
		return rawURL
	}
	fragment := parsed.Fragment
	values, err := url.ParseQuery(fragment)
	if err != nil {
		values = make(url.Values)
	}
	values.Set(fragmentKey, base64.RawURLEncoding.EncodeToString(payload))
	parsed.Fragment = values.Encode()
	return parsed.String()
}

// Extract removes MediaStorm's private fragment metadata and returns the safe
// request headers it contained. Malformed or untrusted metadata is ignored.
func Extract(rawURL string) (string, map[string]string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL, nil
	}
	values, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		return rawURL, nil
	}
	encoded := values.Get(fragmentKey)
	if encoded == "" {
		return rawURL, nil
	}
	values.Del(fragmentKey)
	parsed.Fragment = values.Encode()
	cleanURL := parsed.String()

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(payload) > 8192 {
		return cleanURL, nil
	}
	var headers map[string]string
	if err := json.Unmarshal(payload, &headers); err != nil {
		return cleanURL, nil
	}
	return cleanURL, Sanitize(headers)
}

// Apply adds sanitized headers without replacing an explicit Range request.
func Apply(header http.Header, values map[string]string) {
	for name, value := range Sanitize(values) {
		header.Set(name, value)
	}
}

// FFmpegValue formats safe headers for ffmpeg/ffprobe's -headers option.
func FFmpegValue(values map[string]string) string {
	safe := Sanitize(values)
	var lines []string
	for _, name := range []string{"Accept", "Accept-Language", "Origin", "Referer", "User-Agent"} {
		if value := safe[name]; value != "" {
			lines = append(lines, name+": "+value+"\r\n")
		}
	}
	return strings.Join(lines, "")
}
