package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"novastream/config"
	"novastream/models"
	"novastream/services/publicmetadb"
)

func TestPublicMetaDBListLoadsPrivateCustomListAndRejectsOtherOwner(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer key")
		}
		switch r.URL.Path {
		case "/api/external/lists":
			fmt.Fprint(w, `{"items":[{"id":"list-1","name":"Private","type":"custom"}],"totalPages":1}`)
		case "/api/external/lists/list-1/items":
			fmt.Fprint(w, `{"items":[{"media_type":"movie","tmdb_id":27205},{"media_type":"tv","tmdb_id":1396}],"totalPages":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	mgr := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	settings, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings.PublicMetaDB.Accounts = []config.PublicMetaDBAccount{{ID: "account-1", Name: "Test", APIKey: "test-key", OwnerAccountID: "owner"}}
	if err := mgr.Save(settings); err != nil {
		t.Fatal(err)
	}
	fake := &fakeMetadataService{}
	h := NewMetadataHandler(fake, mgr)
	h.PublicMetaDBClient = &publicmetadb.Client{BaseURL: upstream.URL + "/api"}
	h.SetUsersService(&fakeUsersServiceForSearch{users: map[string]models.User{
		"owned": {ID: "owned", AccountID: "owner"},
		"other": {ID: "other", AccountID: "other"},
	}})
	h.SetAccountsService(&fakeAccountsServiceForMetadata{accounts: map[string]models.Account{
		"owner": {ID: "owner"}, "other": {ID: "other"},
	}})

	for _, tc := range []struct {
		user   string
		status int
	}{{"owned", 200}, {"other", 403}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/lists/publicmetadb?userId="+tc.user+"&accountId=account-1&listId=list-1", nil)
		h.PublicMetaDBList(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s: status %d, body %s", tc.user, rec.Code, rec.Body.String())
		}
	}
	if len(fake.lastCuratedItems) != 2 || fake.lastCuratedItems[0].TMDBID != 27205 || fake.lastCuratedItems[1].MediaType != "series" {
		t.Fatalf("wrong curated items: %+v", fake.lastCuratedItems)
	}
}
