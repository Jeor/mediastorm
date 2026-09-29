package trakt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestHistoryPaginationWithAndWithoutCountHeader(t *testing.T) {
	for _, headers := range []bool{false, true} {
		t.Run(strconv.FormatBool(headers), func(t *testing.T) {
			for _, incremental := range []bool{false, true} {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.Query().Get("page") != strconv.Itoa(calls) || r.URL.Query().Has("start_at") != incremental {
						t.Errorf("incorrect history pagination %s", r.URL.RawQuery)
					}
					if headers {
						w.Header().Set("X-Pagination-Item-Count", "101")
					}
					count := 100
					if calls == 2 {
						count = 1
					}
					if calls > 2 {
						t.Errorf("unnecessary page %d", calls)
						count = 0
					}
					items := make([]HistoryItem, count)
					_ = json.NewEncoder(w).Encode(items)
				}))
				originalURL := traktAPIBaseURL
				setBaseURL(server.URL)
				client := NewClient("client", "secret")
				var items []HistoryItem
				var err error
				if incremental {
					items, err = client.GetWatchHistorySince("token", time.Now())
				} else {
					items, err = client.GetAllWatchHistory("token")
				}
				setBaseURL(originalURL)
				server.Close()
				if err != nil || len(items) != 101 || calls != 2 {
					t.Fatalf("incremental=%v items=%d calls=%d err=%v", incremental, len(items), calls, err)
				}
			}
		})
	}
}

func TestHistoryPaginationRejectsLaterPageFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("X-Pagination-Item-Count", "2")
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()
	originalURL := traktAPIBaseURL
	setBaseURL(server.URL)
	defer setBaseURL(originalURL)
	items, err := NewClient("client", "secret").GetWatchHistorySince("token", time.Time{})
	if err == nil || items != nil {
		t.Fatalf("partial history returned as complete: items=%v err=%v", items, err)
	}
}
