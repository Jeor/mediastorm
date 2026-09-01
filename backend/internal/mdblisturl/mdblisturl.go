package mdblisturl

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

var ErrUnsafeRedirect = errors.New("MDBList redirect target is not allowed")

// Valid reports whether raw is the canonical public MDBList JSON-list shape.
func Valid(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host != "mdblist.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return false
	}
	if strings.Contains(parsed.EscapedPath(), "%") {
		return false
	}
	parts := strings.Split(parsed.Path, "/")
	return len(parts) == 5 && parts[0] == "" && parts[1] == "lists" && validSegment(parts[2]) && validSegment(parts[3]) && parts[4] == "json"
}

func validSegment(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".."
}

// CheckRedirect rejects redirect targets outside the same strict allowlist.
func CheckRedirect(req *http.Request, _ []*http.Request) error {
	if req == nil || req.URL == nil || !Valid(req.URL.String()) {
		return ErrUnsafeRedirect
	}
	return nil
}
