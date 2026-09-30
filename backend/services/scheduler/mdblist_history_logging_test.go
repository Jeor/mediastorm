package scheduler

import (
	"bytes"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"novastream/config"
)

func TestMDBListHistoryLogsConfigurationFiltersAndParsingWithoutCredentials(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(strconv.FormatBool(full), func(t *testing.T) {
			manager := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
			settings := config.DefaultSettings()
			secret := "mdblist-secret-must-not-be-logged"
			settings.MDBList.Accounts = []config.MDBListAccount{{ID: "account-1", Name: "MDBList account", APIKey: secret}}
			if err := manager.Save(settings); err != nil {
				t.Fatal(err)
			}
			svc := &Service{configManager: manager, historyService: auditHistoryService(t, 0, true)}
			lastRun := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
			task := config.ScheduledTask{
				ID: "task-1", Type: config.ScheduledTaskTypeMDBListHistorySync, LastRunAt: &lastRun,
				Config: map[string]string{"profileId": "profile-1", "mdblistAccountId": "account-1", "syncDirection": "mdblist_to_local", "dryRun": "true"},
			}
			if full {
				var err error
				task, err = prepareFullSync(task)
				if err != nil {
					t.Fatal(err)
				}
			}
			originalTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Query().Get("offset") == "0" {
					return jsonResponse(200, `{"movies":[
					 {"last_watched_at":"2026-01-01T00:00:00Z","movie":{"title":"Movie","ids":{"tmdb":42}}},
					 {"last_watched_at":null,"movie":{"ids":{"tmdb":43}}},
					 {"last_watched_at":"2026-01-01T00:00:00Z","movie":{"ids":{}}},5],
					 "shows":[{}],"seasons":[{}],"pagination":{"limit":500,"has_more":true,"total_movies":4,"total_episodes":3}}`), nil
				}
				return jsonResponse(200, `{"episodes":[
				 {"last_watched_at":"2026-01-01T00:00:00Z","episode":{"season":1,"number":1,"show":{"title":"Show","ids":{"tmdb":7}}}},
				 {"last_watched_at":null,"episode":{"season":1,"number":2}},
				 {"last_watched_at":"2026-01-01T00:00:00Z","episode":{"season":-1,"number":2}}],
				 "pagination":{"has_more":false,"total_movies":4,"total_episodes":3}}`), nil
			})
			var output bytes.Buffer
			originalWriter := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(originalWriter) })
			result, err := svc.executeMDBListHistorySync(task)
			if err != nil || result.Count != 2 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			got := output.String()
			mode, since := "incremental", `since="2026-09-30T07:55:00Z"`
			if full {
				mode, since = "full", `since=""`
			}
			for _, want := range []string{
				`task="task-1"`, `account="account-1"`, `accountName="MDBList account"`, `profile="profile-1"`,
				`direction="mdblist_to_local"`, "dryRun=true", "mode=" + mode, since,
				"page=1 offset=0", "page=2 offset=500", "apiTotalMovies=4 apiTotalEpisodes=3", "shows=1 seasons=1",
				"movies=1 episodes=1 skippedMalformed=1 skippedUnwatched=2 skippedIdentity=1 skippedCoordinates=1 invalidTimestamps=0",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("missing diagnostic %q in %s", want, got)
				}
			}
			if strings.Contains(got, secret) || strings.Contains(got, "apikey=") || strings.Contains(got, "https://api.mdblist.com") {
				t.Fatal("diagnostics exposed credentials or request URL")
			}
		})
	}
}
