package handlers

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// Personal list shelves use the established MDBList-compatible transport. The
// list remains owned by the requesting profile; the URL contains only a list ID.
const customListShelfURLPrefix = "mediastorm:custom-list:"

// ProfileCustomLists exposes the profile's lists to the session-authenticated
// settings editor, using the same account ownership checks as profile settings.
func (h *AdminUIHandler) ProfileCustomLists(lists *CustomListsHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(mux.Vars(r)["userID"])
		if userID == "" {
			http.Error(w, "user id is required", http.StatusBadRequest)
			return
		}
		if ok, _ := h.requireProfileScope(w, r, userID); !ok {
			return
		}
		lists.ListLists(w, r)
	}
}
