package streamheaders

import (
	"net/http"
	"strings"
	"testing"
)

func TestAttachExtractRoundTripSanitizesHeaders(t *testing.T) {
	raw := "https://cdn.example/video.mkv?sig=secret"
	decorated := Attach(raw, map[string]string{
		"referer":       "https://source.example/watch",
		"Accept":        "video/*",
		"Authorization": "Bearer secret",
		"Cookie":        "session=secret",
		"X-Evil":        "nope",
	})
	if decorated == raw || !strings.Contains(decorated, "#") {
		t.Fatalf("Attach() = %q, want fragment metadata", decorated)
	}
	clean, headers := Extract(decorated)
	if clean != raw {
		t.Fatalf("Extract() clean URL = %q, want %q", clean, raw)
	}
	if headers["Referer"] != "https://source.example/watch" || headers["Accept"] != "video/*" {
		t.Fatalf("Extract() headers = %#v", headers)
	}
	for _, forbidden := range []string{"Authorization", "Cookie", "X-Evil"} {
		if headers[forbidden] != "" {
			t.Fatalf("forbidden header %s survived: %#v", forbidden, headers)
		}
	}
}

func TestSanitizeRejectsControlCharacters(t *testing.T) {
	if got := Sanitize(map[string]string{"Referer": "https://ok.example\r\nX-Evil: yes"}); got != nil {
		t.Fatalf("Sanitize() = %#v, want nil", got)
	}
}

func TestApplyUsesCanonicalNames(t *testing.T) {
	header := make(http.Header)
	Apply(header, map[string]string{"user-agent": "PenguPlayer", "host": "evil.example"})
	if header.Get("User-Agent") != "PenguPlayer" || header.Get("Host") != "" {
		t.Fatalf("Apply() headers = %#v", header)
	}
}
