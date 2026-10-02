package scheduler

import (
	"context"
	"net/http"
	"novastream/config"
	"novastream/models"
	"novastream/services/history"
	"novastream/services/simkl"
	"novastream/services/trakt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPrepareFullSyncPreservesSavedOptions(t *testing.T) {
	kinds := []config.ScheduledTaskType{config.ScheduledTaskTypePlexWatchlistSync, config.ScheduledTaskTypeTraktListSync, config.ScheduledTaskTypeTraktHistorySync, config.ScheduledTaskTypeSimklHistorySync, config.ScheduledTaskTypeScrobHistorySync, config.ScheduledTaskTypePlexHistorySync, config.ScheduledTaskTypeJellyfinFavoritesSync, config.ScheduledTaskTypeJellyfinHistorySync, config.ScheduledTaskTypeMDBListWatchlistSync, config.ScheduledTaskTypeMDBListHistorySync}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			now := time.Now()
			original := config.ScheduledTask{Type: kind, LastRunAt: &now, Config: map[string]string{"lastSimklActivityAt": "saved", "dryRun": "true", "syncDirection": "bidirectional"}}
			full, err := prepareFullSync(original)
			if err != nil {
				t.Fatal(err)
			}
			if full.LastRunAt != nil || full.Config["lastSimklActivityAt"] != "" || full.Config["fullExport"] != "true" || full.Config["fullSync"] != "true" {
				t.Fatalf("full=%+v", full)
			}
			if original.LastRunAt != &now || original.Config["lastSimklActivityAt"] != "saved" || original.Config["fullSync"] != "" {
				t.Fatalf("original mutated: %+v", original)
			}
			if full.Config["dryRun"] != "true" || full.Config["syncDirection"] != "bidirectional" {
				t.Fatalf("options changed: %+v", full.Config)
			}
		})
	}
	if _, err := prepareFullSync(config.ScheduledTask{Type: config.ScheduledTaskTypeBackup}); err == nil {
		t.Fatal("accepted non-sync task")
	}
}

func TestMDBListHistoryRejectsHTTPFailures(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	for _, status := range []int{401, 429, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(status, `{"error":"Daily API limit exceeded!"}`), nil
			})
			_, err := (&Service{}).syncMDBListHistoryToLocal(config.ScheduledTask{}, &config.MDBListAccount{APIKey: "secret"}, "profile", false)
			if err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMDBListFullSyncOmitsCursorOnEveryPage(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	now := time.Now()
	original := config.ScheduledTask{Type: config.ScheduledTaskTypeMDBListHistorySync, LastRunAt: &now}
	for _, full := range []bool{false, true} {
		t.Run(strconv.FormatBool(full), func(t *testing.T) {
			task := original
			if full {
				task, _ = prepareFullSync(task)
			}
			calls := 0
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.URL.Query().Get("since"); (got == "") != full {
					t.Fatalf("since=%q full=%v", got, full)
				}
				if got := req.URL.Query().Get("offset"); got != strconv.Itoa(calls*500) {
					t.Fatalf("offset=%s", got)
				}
				calls++
				if calls == 1 {
					return jsonResponse(200, `{"movies":[{"movie":{"title":"Old movie","ids":{"imdb":"tt123"}},"last_watched_at":"2000-01-01T00:00:00Z"}],"pagination":{"has_more":true}}`), nil
				}
				return jsonResponse(200, `{"movies":[],"pagination":{"has_more":false}}`), nil
			})
			result, err := (&Service{}).syncMDBListHistoryToLocal(task, &config.MDBListAccount{APIKey: "test"}, "profile", true)
			if err != nil || result.Count != 1 || calls != 2 {
				t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
			}
		})
	}
}

func TestTraktManualFullSyncIncludesAllYears(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	calls := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Query().Has("start_at") {
			t.Fatalf("full sync has start_at: %s", req.URL.RawQuery)
		}
		return jsonResponse(200, `[]`), nil
	})
	svc := &Service{traktClient: trakt.NewClient("test", "test"), lastFullSyncTimes: map[string]time.Time{"task": time.Now()}}
	task, _ := prepareFullSync(config.ScheduledTask{ID: "task", Type: config.ScheduledTaskTypeTraktHistorySync})
	if _, err := svc.syncTraktHistoryToLocal(task, &config.TraktAccount{}, "profile", true); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestSimklManualFullSyncBypassesActivityCursor(t *testing.T) {
	client := simkl.NewClient()
	paths := []string{}
	client.SetHTTPClientForTest(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if req.URL.Query().Has("date_from") {
			t.Fatalf("full sync has delta: %s", req.URL.RawQuery)
		}
		if req.URL.Path == "/sync/activities" {
			return jsonResponse(200, `{"all":"2026-01-01T00:00:00Z"}`), nil
		}
		return jsonResponse(200, `{}`), nil
	})})
	svc := &Service{simklClient: client}
	task, _ := prepareFullSync(config.ScheduledTask{Type: config.ScheduledTaskTypeSimklHistorySync, Config: map[string]string{"lastSimklActivityAt": "2026-01-01T00:00:00Z", "simklHistoryImportVersion": "2"}})
	if _, err := svc.syncSimklHistoryToLocal(task, &config.SimklAccount{ClientID: "test", AccessToken: "test"}, "profile", true); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatalf("expected activities and three buckets, got %v", paths)
	}
}

func TestRunTaskNowFullSyncIsOneRunOnly(t *testing.T) {
	manager := config.NewManager(t.TempDir() + "/settings.json")
	now := time.Now().UTC()
	settings := config.DefaultSettings()
	settings.MDBList.Accounts = []config.MDBListAccount{{ID: "account", APIKey: "test"}}
	settings.ScheduledTasks.Tasks = []config.ScheduledTask{{ID: "task", Type: config.ScheduledTaskTypeMDBListHistorySync, LastRunAt: &now, Config: map[string]string{"profileId": "profile", "mdblistAccountId": "account"}}}
	if err := manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	historySvc, err := history.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(manager, nil, nil, nil)
	svc.SetHistoryService(historySvc)
	svc.SetUsersService(&fakeSchedulerUsersProvider{users: map[string]models.User{"profile": {ID: "profile"}}})
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	requests := make(chan string, 2)
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests <- req.URL.Query().Get("since")
		return jsonResponse(200, `{"movies":[],"episodes":[]}`), nil
	})
	for _, full := range []bool{true, false} {
		if err := svc.RunTaskNow("task", full); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := svc.Stop(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		select {
		case since := <-requests:
			if (since == "") != full {
				t.Fatalf("full=%v since=%q", full, since)
			}
		default:
			t.Fatal("missing request")
		}
		saved, err := manager.Load()
		if err != nil {
			t.Fatal(err)
		}
		if saved.ScheduledTasks.Tasks[0].Config["fullSync"] != "" || saved.ScheduledTasks.Tasks[0].Config["fullExport"] != "" {
			t.Fatal("one-run options persisted")
		}
	}
}
