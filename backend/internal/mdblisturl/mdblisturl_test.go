package mdblisturl

import (
	"net/http"
	"net/url"
	"testing"
)

func TestCanonicalAcceptsSafeDocumentedForms(t *testing.T) {
	const want = "https://mdblist.com/lists/user/list/json"
	for _, raw := range []string{
		want,
		want + "/",
		"https://mdblist.com/lists/user/list",
		"https://mdblist.com/lists/user/list/",
	} {
		got, ok := Canonical(raw)
		if !ok || got != want {
			t.Fatalf("Canonical(%q) = %q, %v; want %q, true", raw, got, ok, want)
		}
	}
}

func TestCheckRedirectRevalidatesEveryTarget(t *testing.T) {
	allowed, _ := url.Parse("https://mdblist.com/lists/user/renamed/json")
	blocked, _ := url.Parse("http://169.254.169.254/latest/meta-data")
	if err := CheckRedirect(&http.Request{URL: allowed}, nil); err != nil {
		t.Fatalf("safe redirect rejected: %v", err)
	}
	if err := CheckRedirect(&http.Request{URL: blocked}, nil); err == nil {
		t.Fatal("link-local redirect was accepted")
	}
}
