// Package mappingtest supplies offline upstream fixtures to integration tests.
package mappingtest

import (
	"context"
	_ "embed"
	"net/http"
	"novastream/internal/mediaidentity"
	"testing"
	"time"
)

// AnimeList is a subset of Anime-Lists/anime-list-master.xml, retrieved
// 2026-09-24. Keep entries unmodified so real numbering boundaries are tested.
// Source: https://github.com/Anime-Lists/anime-lists
//
//go:embed testdata/anime-list.xml
var AnimeList []byte

type snapshots struct{ xem []byte }

func (s snapshots) Get(_ context.Context, key string) (*mediaidentity.MappingSnapshot, error) {
	body := []byte(`{"result":"success","data":[]}`)
	if len(s.xem) > 0 {
		body = s.xem
	}
	if key == "anime-lists" {
		body = AnimeList
	}
	return &mediaidentity.MappingSnapshot{Key: key, Body: body, CheckedAt: time.Now()}, nil
}
func (snapshots) Put(context.Context, mediaidentity.MappingSnapshot) error { return nil }

type noNetwork struct{}

func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	panic("mapping fixtures must never use network")
}
func Install(t testing.TB) {
	t.Helper()
	InstallWithXEM(t, nil)
}

func InstallWithXEM(t testing.TB, xem []byte) {
	t.Helper()
	s := mediaidentity.NewEpisodeMappingService(snapshots{xem: xem}, &http.Client{Transport: noNetwork{}})
	prior := mediaidentity.SetEpisodeMappingService(s)
	t.Cleanup(func() { mediaidentity.SetEpisodeMappingService(prior) })
}
func Warm(ctx context.Context, title string, season, episode int) {
	mediaidentity.EnsureEpisodeMappings(ctx, title, season, episode, true)
}
