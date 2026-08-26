package config

import (
	"reflect"
	"testing"
)

func TestNormalizeAllowedPrivateMediaOrigins(t *testing.T) {
	settings := ServerSettings{AllowedPrivateMediaOrigins: []string{
		" http://LOCALHOST:8080/adapters/addon/file.mkv?token=secret#fragment ",
		"http://localhost:8080",
		"https://[fd00::1]:8443/media",
	}}

	if err := settings.NormalizeAllowedPrivateMediaOrigins(); err != nil {
		t.Fatalf("NormalizeAllowedPrivateMediaOrigins() error = %v", err)
	}
	want := []string{"http://localhost:8080", "https://[fd00::1]:8443"}
	if !reflect.DeepEqual(settings.AllowedPrivateMediaOrigins, want) {
		t.Fatalf("allowed origins = %#v, want %#v", settings.AllowedPrivateMediaOrigins, want)
	}
}

func TestNormalizeAllowedPrivateMediaOriginsRejectsUnsafeValues(t *testing.T) {
	for _, raw := range []string{
		"localhost:8080",
		"file:///etc/passwd",
		"http://user:password@localhost:8080",
	} {
		settings := ServerSettings{AllowedPrivateMediaOrigins: []string{raw}}
		if err := settings.NormalizeAllowedPrivateMediaOrigins(); err == nil {
			t.Errorf("NormalizeAllowedPrivateMediaOrigins(%q) error = nil, want rejection", raw)
		}
	}
}

func TestNormalizeExternalBackendURL(t *testing.T) {
	settings := ServerSettings{ExternalBackendURL: " https://Watch.Example.com/mediastorm/api/ "}
	if err := settings.NormalizeExternalBackendURL(); err != nil {
		t.Fatalf("NormalizeExternalBackendURL() error = %v", err)
	}
	if got, want := settings.ExternalBackendURL, "https://Watch.Example.com/mediastorm"; got != want {
		t.Fatalf("external backend URL = %q, want %q", got, want)
	}
}

func TestNormalizeExternalBackendURLRejectsUnsafeValues(t *testing.T) {
	for _, raw := range []string{
		"watch.example.com",
		"file:///etc/passwd",
		"https://user:password@watch.example.com",
		"https://watch.example.com/path?token=secret",
		"https://watch.example.com/path#fragment",
	} {
		settings := ServerSettings{ExternalBackendURL: raw}
		if err := settings.NormalizeExternalBackendURL(); err == nil {
			t.Errorf("NormalizeExternalBackendURL(%q) error = nil, want rejection", raw)
		}
	}
}
