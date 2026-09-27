package aiprovider

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const OllamaTimeout = 180 * time.Second

// OllamaBaseURL accepts an origin or a proxy prefix, with an optional /v1 suffix.
func OllamaBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		raw = "http://localhost:11434"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("Ollama server URL must be an HTTP(S) address without credentials, query or fragment")
	}
	if !strings.HasSuffix(u.Path, "/v1") {
		u.Path = strings.TrimRight(u.Path, "/") + "/v1"
		u.RawPath = ""
	}
	return u.String(), nil
}
