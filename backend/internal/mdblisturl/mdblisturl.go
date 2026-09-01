package mdblisturl

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

var ErrUnsafeRedirect = errors.New("MDBList redirect target is not allowed")

// Canonical validates a public MDBList path and returns its JSON endpoint.
// The documented list-page form and one optional trailing slash are accepted
// for compatibility, but callers always receive the exact fetchable /json URL.
func Canonical(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host != "mdblist.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", false
	}
	if strings.Contains(parsed.EscapedPath(), "%") {
		return "", false
	}
	parts := strings.Split(strings.TrimSuffix(parsed.Path, "/"), "/")
	if (len(parts) != 4 && len(parts) != 5) || parts[0] != "" || parts[1] != "lists" || !validSegment(parts[2]) || !validSegment(parts[3]) || (len(parts) == 5 && parts[4] != "json") {
		return "", false
	}
	return "https://mdblist.com/lists/" + parts[2] + "/" + parts[3] + "/json", true
}

// Valid reports whether raw is a safe supported public MDBList URL form.
func Valid(raw string) bool {
	_, ok := Canonical(raw)
	return ok
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
