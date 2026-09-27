package aiprovider

import "testing"

func TestOllamaBaseURL(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "http://localhost:11434/v1"},
		{" http://server:11434/ ", "http://server:11434/v1"},
		{"https://server/proxy/v1/", "https://server/proxy/v1"},
		{"https://server/proxy", "https://server/proxy/v1"},
	} {
		got, err := OllamaBaseURL(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("%q: got %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"server:11434", "file:///tmp/test", "http://", "http://user:pass@server", "https://server?key=secret", "http://server/#fragment", "http://server:bad"} {
		if _, err := OllamaBaseURL(input); err == nil {
			t.Errorf("accepted invalid URL %q", input)
		}
	}
}
