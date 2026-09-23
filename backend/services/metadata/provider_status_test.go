package metadata

import "testing"

func TestTVDBConfigured(t *testing.T) {
	for _, tc := range []struct {
		name string
		svc  *Service
		want bool
	}{
		{"nil service", nil, false},
		{"no client", &Service{}, false},
		{"blank key", &Service{client: &tvdbClient{apiKey: "  "}}, false},
		{"configured", &Service{client: &tvdbClient{apiKey: "test-key"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.svc.TVDBConfigured(); got != tc.want {
				t.Fatalf("TVDBConfigured = %v, want %v", got, tc.want)
			}
		})
	}
}
