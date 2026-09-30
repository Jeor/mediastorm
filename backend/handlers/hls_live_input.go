package handlers

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"time"

	"novastream/internal/hlsrelay"
	"novastream/internal/netproxy"
)

// Keep Stremio HLS requests on the provider's configured network/header context
// and normalize image-camouflaged TS segments before FFmpeg probes them. Each
// FFmpeg run owns its relay, so recovery and cleanup close all upstream fetches.
func startLiveHLSInputRelay(ctx context.Context, upstream, proxyURL string, headers map[string]string) (*hlsrelay.Relay, error) {
	client, err := netproxy.NewHTTPClientWithOptions(netproxy.HTTPClientOptions{
		ResponseHeaderTimeout: 15 * time.Second,
	}, proxyURL)
	if err != nil {
		return nil, err
	}
	// FFmpeg normally carries playlist-issued cookies into segment requests.
	// Retain that behavior when the Go relay owns the upstream connection.
	client.Jar, _ = cookiejar.New(nil)
	requestHeaders := make(http.Header)
	requestHeaders.Set("User-Agent", liveStreamUserAgent)
	applyRequestHeaders(requestHeaders, headers)
	relay, err := hlsrelay.Start(ctx, upstream, requestHeaders, client)
	if err != nil {
		client.CloseIdleConnections()
	}
	return relay, err
}
