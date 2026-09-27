package handlers_test

import (
	"bytes"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"novastream/handlers"
	"novastream/models"
	"novastream/services/history"
	"testing"
)

type scopedHistoryService struct {
	fakeHistoryService
	scope           string
	receivedUser    string
	receivedUpdates []models.WatchHistoryUpdate
}

func (s *scopedHistoryService) BulkUpdateScopedWatchHistory(user string, updates []models.WatchHistoryUpdate, scope string) ([]models.WatchHistoryItem, error) {
	s.scope, s.receivedUser, s.receivedUpdates = scope, user, updates
	return nil, s.err
}
func TestBulkHistoryHandlerScope(t *testing.T) {
	for _, scope := range []string{"", "show", "season", "invalid"} {
		t.Run(scope, func(t *testing.T) {
			svc := &scopedHistoryService{}
			h := handlers.NewHistoryHandler(svc, fakeUserService{}, false)
			req := httptest.NewRequest(http.MethodPost, "/history/watched/bulk?scope="+scope, bytes.NewBufferString(`[{"mediaType":"episode","itemId":"tmdb:tv:1667:s01e01","watched":true,"seasonNumber":1,"episodeNumber":1}]`))
			req = mux.SetURLVars(req, map[string]string{"userID": "user"})
			rec := httptest.NewRecorder()
			h.BulkUpdateWatchHistory(rec, req)
			want := http.StatusOK
			if scope == "invalid" {
				want = http.StatusBadRequest
			}
			if rec.Code != want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
			}
			if scope == "show" || scope == "season" {
				if svc.scope != scope || svc.receivedUser != "user" || len(svc.receivedUpdates) != 1 {
					t.Fatalf("scope=%q user=%q updates=%d", svc.scope, svc.receivedUser, len(svc.receivedUpdates))
				}
			}
		})
	}
}
func TestBulkHistoryHandlerInvalidScopeShape(t *testing.T) {
	svc := &scopedHistoryService{fakeHistoryService: fakeHistoryService{err: history.ErrInvalidBulkScope}}
	h := handlers.NewHistoryHandler(svc, fakeUserService{}, false)
	req := httptest.NewRequest(http.MethodPost, "/history/watched/bulk?scope=show", bytes.NewBufferString(`[{"mediaType":"movie","itemId":"tmdb:movie:1","watched":true}]`))
	req = mux.SetURLVars(req, map[string]string{"userID": "user"})
	rec := httptest.NewRecorder()
	h.BulkUpdateWatchHistory(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}
