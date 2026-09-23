package publicmetadb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListsAndItemsUseBearerKeyAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer credential")
		}
		if r.URL.Query().Get("perPage") != "100" {
			t.Errorf("unexpected page size: %s", r.URL.Query().Get("perPage"))
		}
		switch r.URL.Path {
		case "/api/external/lists":
			fmt.Fprint(w, `{"items":[{"id":"native","name":"Watchlist","type":"watchlist"}],"page":1,"totalPages":1}`)
		case "/api/external/lists/native/items":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprint(w, `{"items":[{"media_type":"movie","tmdb_id":27205}],"page":1,"totalPages":2}`)
			} else {
				fmt.Fprint(w, `{"items":[{"media_type":"tv","tmdb_id":1396}],"page":2,"totalPages":2}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL + "/api"}
	lists, err := client.Lists(context.Background(), "test-key")
	if err != nil || len(lists) != 1 || lists[0].Type != "watchlist" {
		t.Fatalf("lists = %+v, err = %v", lists, err)
	}
	items, err := client.Items(context.Background(), "test-key", lists[0].ID)
	if err != nil || len(items) != 2 || items[1].TMDBID != 1396 {
		t.Fatalf("items = %+v, err = %v", items, err)
	}
}
