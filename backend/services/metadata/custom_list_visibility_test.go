package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"novastream/models"
)

func TestCustomListReleaseFilteringBeforePagination(t *testing.T) {
	for _, warm := range []bool{false, true} {
		for _, kind := range []string{"movie", "series", "both", "legacy", "neither"} {
			t.Run(fmt.Sprintf("warm=%t/%s", warm, kind), func(t *testing.T) {
				cache := newFileCache(t.TempDir(), 24)
				listURL := "https://mdblist.com/lists/test/streaming/json"
				source := []mdblistItem{
					{Title: "Upcoming movie", MediaType: "movie", TMDBID: ptr(int64(1)), ReleaseYear: time.Now().Year()},
					{Title: "Streaming movie", MediaType: "movie", TMDBID: ptr(int64(2)), ReleaseYear: time.Now().Year()},
					{Title: "Upcoming show", MediaType: "show", TMDBID: ptr(int64(3)), ReleaseYear: time.Now().Year()},
					{Title: "Streaming show", MediaType: "show", TMDBID: ptr(int64(4)), ReleaseYear: time.Now().Year()},
				}
				httpc := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					body := `{}`
					switch {
					case req.URL.Host == "mdblist.com":
						if warm {
							return nil, fmt.Errorf("warm list should reuse cached items")
						}
						data, _ := json.Marshal(source)
						body = string(data)
					case strings.HasSuffix(req.URL.Path, "/release_dates"):
						date := "2099-01-01T00:00:00Z"
						if strings.Contains(req.URL.Path, "/2/") {
							date = time.Now().AddDate(0, 0, -1).UTC().Format(time.RFC3339)
						}
						body = fmt.Sprintf(`{"results":[{"iso_3166_1":"US","release_dates":[{"type":4,"release_date":%q}]}]}`, date)
					case req.URL.Path == "/3/tv/3":
						body = `{"id":3,"name":"Upcoming show","first_air_date":"2099-01-01"}`
					case req.URL.Path == "/3/tv/4":
						body = fmt.Sprintf(`{"id":4,"name":"Streaming show","first_air_date":%q}`, time.Now().AddDate(0, 0, -1).Format("2006-01-02"))
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})}
				svc := &Service{client: newTVDBClient("", "eng", httpc, 24), cache: cache, tmdb: newTMDBClient("test", "eng", httpc, cache)}
				if warm {
					var rows []models.TrendingItem
					for _, item := range source {
						rows = append(rows, buildLiteCustomListItem(item))
					}
					if err := cache.set(cacheKey("mdblist", "custom", "v8", "fast", listURL, "eng"), rows); err != nil {
						t.Fatal(err)
					}
				}
				opts := CustomListOptions{Lite: true, DeferArtwork: true, Limit: 1, Offset: 1, HideUnreleased: kind == "legacy", HideUnreleasedMovies: kind == "movie" || kind == "both", HideUnreleasedShows: kind == "series" || kind == "both"}
				wantID, wantTotal := int64(3), 3
				if kind == "series" {
					wantID = 2
				}
				if kind == "neither" {
					wantID, wantTotal = 2, 4
				}
				if kind == "both" || kind == "legacy" {
					wantID, wantTotal = 4, 2
				}
				rows, total, unfiltered, err := svc.GetCustomList(context.Background(), listURL, opts)
				if err != nil || len(rows) != 1 || total != wantTotal || unfiltered != 4 {
					t.Fatalf("rows=%+v total=%d unfiltered=%d err=%v", rows, total, unfiltered, err)
				}
				if rows[0].Title.TMDBID != wantID {
					t.Fatalf("page has ID %d, want %d", rows[0].Title.TMDBID, wantID)
				}
			})
		}
	}
}
