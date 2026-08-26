package torboxrate

import (
	"context"
	"testing"
	"time"
)

func TestIsDownloadURL(t *testing.T) {
	for _, test := range []struct {
		url  string
		want bool
	}{
		{url: "https://store-073.wnam.tb-cdn.io/file", want: true},
		{url: "https://tb-cdn.io/file", want: true},
		{url: "https://api.torbox.app/v1/api/torrents/mylist", want: false},
		{url: "https://tb-cdn.io.example.com/file", want: false},
	} {
		if got := IsDownloadURL(test.url); got != test.want {
			t.Errorf("IsDownloadURL(%q) = %t, want %t", test.url, got, test.want)
		}
	}
}

func TestRetryDelayPrefersHeaderAndParsesTorBoxBody(t *testing.T) {
	now := time.Now()
	if got, ok := retryDelay("7", []byte("retry in 2s"), now); !ok || got != 7*time.Second {
		t.Fatalf("header retry delay = %s, %t; want 7s, true", got, ok)
	}
	if got, ok := retryDelay("", []byte("Too many requests, retry in 4s"), now); !ok || got != 4*time.Second {
		t.Fatalf("body retry delay = %s, %t; want 4s, true", got, ok)
	}
	if got, ok := retryDelay("", []byte("retry in 1500ms"), now); !ok || got != 1500*time.Millisecond {
		t.Fatalf("millisecond retry delay = %s, %t; want 1.5s, true", got, ok)
	}
}

func TestCooldownWaitHonorsContext(t *testing.T) {
	cooldown := &Cooldown{}
	cooldown.Record("https://store-073.wnam.tb-cdn.io/file", "10", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cooldown.Wait(ctx, "https://store-073.wnam.tb-cdn.io/file"); err != context.Canceled {
		t.Fatalf("Wait() error = %v, want context.Canceled", err)
	}
}
