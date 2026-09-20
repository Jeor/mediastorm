package history

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"novastream/models"
)

func TestTVmazeAirtimeMatching(t *testing.T) {
	ep := models.EpisodeReference{SeasonNumber: 23, EpisodeNumber: 27, AirDate: "2026-09-20"}
	broadcast := tvmazeEpisode{Season: 2026, Number: 24, Airdate: ep.AirDate, Airtime: "23:15", Airstamp: "2026-09-20T23:15:00+09:00"}
	for _, tc := range []struct {
		name string
		rows []tvmazeEpisode
		want string
	}{
		{"different numbering unique date", []tvmazeEpisode{broadcast}, "2026-09-20T14:15:00Z"},
		{"ambiguous date", []tvmazeEpisode{broadcast, broadcast}, ""},
		{"wrong date", []tvmazeEpisode{{Airdate: "2026-09-21", Airtime: "23:15", Airstamp: broadcast.Airstamp}}, ""},
		{"unknown streaming time", []tvmazeEpisode{{Airdate: ep.AirDate, Airstamp: "2026-09-20T00:00:00Z"}}, ""},
		{"bad timestamp", []tvmazeEpisode{{Airdate: ep.AirDate, Airtime: "23:15", Airstamp: "invalid"}}, ""},
		{"batch exact episode", []tvmazeEpisode{broadcast, {Season: 23, Number: 27, Airdate: ep.AirDate, Airtime: "23:45", Airstamp: "2026-09-20T23:45:00+09:00"}}, "2026-09-20T14:45:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchTVmazeAirtime(tc.rows, ep); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

type airtimeMetadataService struct {
	*mockMetadataService
	tvdb bool
}

func (m *airtimeMetadataService) TVDBConfigured() bool { return m.tvdb }

func TestContinueWatchingAirtimeCacheAndScope(t *testing.T) {
	date := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/lookup/shows" {
			if r.URL.Query().Get("imdb") != "tt0388629" || r.Header.Get("Authorization") != "" {
				t.Errorf("unexpected lookup or credentials: %v", r.URL)
			}
			fmt.Fprint(w, `{"id":1505}`)
			return
		}
		if r.URL.Path != "/shows/1505/episodesbydate" || r.URL.Query().Get("date") != date {
			t.Errorf("unexpected episodes request: %v", r.URL)
		}
		fmt.Fprintf(w, `[{"season":2026,"number":24,"airdate":%q,"airtime":"23:15","airstamp":%q}]`, date, date+"T14:15:00Z")
	}))
	defer server.Close()
	svc := &Service{metadataService: &airtimeMetadataService{}, airtimeClient: &tvmazeAirtimeClient{baseURL: server.URL, http: server.Client()}}
	title := models.Title{ID: "tmdb:tv:37854", IMDBID: "tt0388629", TVDBID: 81797}
	base := models.EpisodeReference{AirDate: date, AirDateTimeUTC: date + "T23:59:59Z", AirTimeEstimated: true}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep := base
			svc.enrichContinueWatchingAirtime(context.Background(), title, nil, &ep)
			if ep.AirTimeEstimated || ep.AirDateTimeUTC != date+"T14:15:00Z" {
				t.Errorf("incorrect enrichment: %+v", ep)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 2 {
		t.Fatalf("parallel refreshes made %d calls, want 2", got)
	}
	for _, ep := range []models.EpisodeReference{
		{AirDate: date, AirTimeEstimated: false},
		{AirDate: "2000-01-01", AirTimeEstimated: true},
		{AirDate: "2999-01-01", AirTimeEstimated: true},
		base,
	} {
		svc.enrichContinueWatchingAirtime(context.Background(), title, nil, &ep)
	}
	if calls.Load() != 2 {
		t.Fatal("cache hits or out-of-scope episodes made additional calls")
	}
}

func TestContinueWatchingAirtimeSkipsTVDB(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	for _, tc := range []struct {
		name     string
		metadata MetadataService
		titleID  string
	}{
		{"TVDB configured even with TMDB fallback", &airtimeMetadataService{tvdb: true}, "tmdb:tv:37854"},
		{"cached TVDB after removing key", &airtimeMetadataService{}, "tvdb:series:81797"},
		{"unknown provider configuration", &mockMetadataService{}, "tmdb:tv:37854"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{metadataService: tc.metadata, airtimeClient: &tvmazeAirtimeClient{baseURL: server.URL, http: server.Client()}}
			date := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
			ep := models.EpisodeReference{AirDate: date, AirDateTimeUTC: date + "T23:59:59Z", AirTimeEstimated: true}
			svc.enrichContinueWatchingAirtime(context.Background(), models.Title{ID: tc.titleID, IMDBID: "tt0388629"}, nil, &ep)
			if calls.Load() != 0 || !ep.AirTimeEstimated || ep.AirDateTimeUTC != date+"T23:59:59Z" {
				t.Fatalf("TVDB/provider boundary violated: calls=%d episode=%+v", calls.Load(), ep)
			}
		})
	}
}

func TestTVmazeFailuresAreCached(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
			}))
			defer server.Close()
			client := &tvmazeAirtimeClient{baseURL: server.URL, http: server.Client()}
			for range 3 {
				if got := client.airtime(context.Background(), "tt0388629", models.EpisodeReference{AirDate: "2026-09-20"}); got != "" {
					t.Fatalf("failure produced timestamp %q", got)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("failure was retried %d times", calls.Load())
			}
		})
	}
}

func TestDateOnlyNextEpisodeIsEstimated(t *testing.T) {
	date := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	details := &models.SeriesDetails{Seasons: []models.SeriesSeason{{Number: 1, Episodes: []models.SeriesEpisode{
		{SeasonNumber: 1, EpisodeNumber: 1, AiredDate: "2020-01-01"},
		{SeasonNumber: 1, EpisodeNumber: 2, AiredDate: date},
	}}}}
	svc := &Service{}
	ep := svc.findNextUnwatchedEpisode(details, models.WatchHistoryItem{SeasonNumber: 1, EpisodeNumber: 1}, nil)
	if ep == nil || !ep.AirTimeEstimated || ep.AirDateTimeUTC != date+"T23:59:59Z" {
		t.Fatalf("date-only precision was lost: %+v", ep)
	}
	details.Title.AirsTime, details.Title.AirsTimezone = "23:15", "Asia/Tokyo"
	ep = svc.findNextUnwatchedEpisode(details, models.WatchHistoryItem{SeasonNumber: 1, EpisodeNumber: 1}, nil)
	if ep == nil || ep.AirTimeEstimated || ep.AirDateTimeUTC != date+"T14:15:00Z" {
		t.Fatalf("known airtime was lost: %+v", ep)
	}
}
