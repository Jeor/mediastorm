package config

import "testing"

func TestHDHomeRunURL(t *testing.T) {
	for _, tc := range []struct{ address, want string }{
		{"192.168.1.100", "http://192.168.1.100/lineup.m3u"},
		{" hdhomerun.local ", "http://hdhomerun.local/lineup.m3u"},
		{"http://192.168.1.100/lineup.m3u", "http://192.168.1.100/lineup.m3u"},
		{"http://[fd00::1]:80/", "http://[fd00::1]:80/lineup.m3u"},
	} {
		got, err := HDHomeRunURL(tc.address, "/lineup.m3u")
		if err != nil || got != tc.want {
			t.Errorf("address %q: got %q, %v", tc.address, got, err)
		}
	}
	for _, address := range []string{"", "file:///tmp/tuner", "http://user:pass@tuner", "http://tuner/other", "http://tuner?token=secret", "http://tuner/#fragment", "http://tuner:bad"} {
		if _, err := HDHomeRunURL(address, "/discover.json"); err == nil {
			t.Errorf("accepted invalid address %q", address)
		}
	}
}
