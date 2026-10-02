package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"novastream/config"
	"novastream/models"
)

func TestMDBListHistoryRetrySameOffsetAndHonorDelay(t *testing.T) {
	calls := 0
	var delays []time.Duration
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Query().Get("offset") != "20000" {
			t.Fatal("retry advanced offset")
		}
		if calls == 3 {
			return jsonResponse(200, `{}`), nil
		}
		resp := jsonResponse(429, `{}`)
		resp.Header.Set("Retry-After", "17")
		return resp, nil
	})}
	resp, err := fetchMDBListHistoryPage(context.Background(), client, "https://api.mdblist.com/sync/watched?offset=20000&apikey=secret", "task", 20000, func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls != 3 || len(delays) != 2 || delays[0] != 17*time.Second || delays[1] != 17*time.Second {
		t.Fatalf("calls=%d delays=%v", calls, delays)
	}
}

func TestMDBListHistoryRetryBoundedAndRedacted(t *testing.T) {
	for _, scenario := range []string{"exhausted", "daily", "long", "cancelled", "transport"} {
		t.Run(scenario, func(t *testing.T) {
			calls, waits := 0, 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if scenario == "transport" {
					return nil, errors.New("connection failed")
				}
				resp := jsonResponse(429, `{}`)
				resp.Header.Set("Retry-After", "0")
				if scenario == "daily" {
					resp = jsonResponse(429, `{"error":"Daily API limit exceeded!"}`)
				}
				if scenario == "long" {
					resp.Header.Set("Retry-After", "86400")
				}
				return resp, nil
			})}
			_, err := fetchMDBListHistoryPage(context.Background(), client, "https://api.mdblist.com/sync/watched?apikey=secret", "task", 20000, func(context.Context, time.Duration) error {
				waits++
				if scenario == "cancelled" {
					return context.Canceled
				}
				return nil
			})
			if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "apikey") {
				t.Fatalf("unsafe error=%v", err)
			}
			if scenario == "exhausted" {
				if calls != 4 || waits != 3 {
					t.Fatalf("calls=%d waits=%d", calls, waits)
				}
			} else if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestMDBListRetryDelay(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		header  string
		attempt int
		want    time.Duration
	}{
		{"5", 0, 5 * time.Second}, {now.Add(time.Minute).Format(http.TimeFormat), 0, time.Minute},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0, 0}, {"invalid", 2, 120 * time.Second},
		{"9223372036854775807", 0, mdblistHistoryMaxRetryWait + time.Second},
	} {
		if got := mdblistRetryDelay(tc.header, now, tc.attempt); got != tc.want {
			t.Fatalf("%q: got=%s want=%s", tc.header, got, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitMDBListRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestMDBListHistorySavesPagesBeforeFailureAndReplaysFailedRun(t *testing.T) {
	h := auditHistoryService(t, 0, true)
	manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	task := config.ScheduledTask{ID: "task", Type: config.ScheduledTaskTypeMDBListHistorySync, Config: map[string]string{"profileId": "profile", "mdblistAccountId": "account"}}
	settings := config.DefaultSettings()
	settings.ScheduledTasks.Tasks = []config.ScheduledTask{task}
	settings.MDBList.Accounts = []config.MDBListAccount{{ID: "account", APIKey: "secret"}}
	if err := manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	svc := NewService(manager, nil, nil, nil)
	svc.SetHistoryService(h)
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	fail := true
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Has("since") {
			t.Fatal("failed run became incremental cursor")
		}
		if req.URL.Query().Get("offset") == "0" {
			return jsonResponse(200, `{"movies":[{"last_watched_at":"2000-01-01T00:00:00Z","movie":{"title":"Film","ids":{"tmdb":42}}}],"pagination":{"limit":100,"has_more":true}}`), nil
		}
		if req.URL.Query().Get("offset") != "100" {
			t.Fatal("ignored server page limit")
		}
		items, err := h.ListWatchHistory("profile")
		if err != nil || len(items) != 1 {
			t.Fatalf("previous page not persisted: items=%v err=%v", items, err)
		}
		if fail {
			resp := jsonResponse(429, `{}`)
			resp.Header.Set("Retry-After", "3600")
			return resp, nil
		}
		return jsonResponse(200, `{"movies":[{"last_watched_at":"2000-01-01T00:00:00Z","movie":{"title":"Film 2","ids":{"tmdb":43}}}]}`), nil
	})
	if err := svc.RunTaskNow("task"); err != nil {
		t.Fatal(err)
	}
	svc.wg.Wait()
	got := svc.GetTaskStatus()[0]
	if got.LastStatus != config.ScheduledTaskStatusError || got.ItemsImported != 1 || svc.IsTaskRunning("task") {
		t.Fatalf("failed status=%+v", got)
	}
	fail = false
	if err := svc.RunTaskNow("task"); err != nil {
		t.Fatal(err)
	}
	svc.wg.Wait()
	got = svc.GetTaskStatus()[0]
	items, err := h.ListWatchHistory("profile")
	if got.LastStatus != config.ScheduledTaskStatusSuccess || got.LastError != "" || len(items) != 2 || err != nil {
		t.Fatalf("replayed status=%+v items=%v err=%v", got, items, err)
	}
}

type countingMDBListMetadata struct {
	fakeSchedulerMetadataService
	calls int
	fail  bool
}

func (m *countingMDBListMetadata) SeriesDetailsLite(context.Context, models.SeriesDetailsQuery) (*models.SeriesDetails, error) {
	m.calls++
	if m.fail {
		return nil, errors.New("metadata unavailable")
	}
	return m.details, nil
}

func TestMDBListHistoryCachesSeriesMetadataAcrossPagesIncludingFailures(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			meta := &countingMDBListMetadata{fakeSchedulerMetadataService: fakeSchedulerMetadataService{details: onePieceIdentityDetails()}, fail: fail}
			svc := &Service{metadataService: meta}
			old := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = old })
			calls := 0
			http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return jsonResponse(200, fmt.Sprintf(`{"episodes":[{"last_watched_at":"2000-01-01T00:00:00Z","episode":{"season":23,"number":1173,"show":{"title":"One Piece","ids":{"tmdb":37854}}}}],"pagination":{"has_more":%v}}`, calls == 1)), nil
			})
			result, err := svc.syncMDBListHistoryToLocal(config.ScheduledTask{}, &config.MDBListAccount{APIKey: "secret"}, "profile", true)
			if err != nil || result.Count != 2 || meta.calls != 1 {
				t.Fatalf("result=%+v metadataCalls=%d err=%v", result, meta.calls, err)
			}
			want := "tmdb:tv:37854:s23e17"
			if fail {
				want = "tmdb:tv:37854:s23e1173"
			}
			if result.ToAdd[0].ID != want {
				t.Fatalf("id=%s want=%s", result.ToAdd[0].ID, want)
			}
		})
	}
}

func TestSchedulerConcurrentStartsReserveOneRunAndHideOldError(t *testing.T) {
	manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.DefaultSettings()
	settings.MDBList.Accounts = []config.MDBListAccount{{ID: "account", APIKey: "secret"}}
	settings.ScheduledTasks.Tasks = []config.ScheduledTask{{ID: "task", Type: config.ScheduledTaskTypeMDBListHistorySync, LastError: "old 429", Config: map[string]string{"profileId": "profile", "mdblistAccountId": "account"}}}
	if err := manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	svc := NewService(manager, nil, nil, nil)
	svc.SetHistoryService(auditHistoryService(t, 0, true))
	release := make(chan struct{})
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { <-release; return jsonResponse(200, `{}`), nil })
	var accepted atomic.Int32
	var callers sync.WaitGroup
	for i := 0; i < 20; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if svc.RunTaskNow("task") == nil {
				accepted.Add(1)
			}
		}()
	}
	callers.Wait()
	got := svc.GetTaskStatus()[0]
	close(release)
	svc.wg.Wait()
	if accepted.Load() != 1 || got.LastStatus != config.ScheduledTaskStatusRunning || got.LastError != "" || svc.IsTaskRunning("task") {
		t.Fatalf("accepted=%d status=%+v", accepted.Load(), got)
	}
}
