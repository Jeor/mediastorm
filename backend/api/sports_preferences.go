package api

import (
	"github.com/gorilla/mux"
	"net/http"
	"novastream/handlers"
)

// Must be registered on the authenticated, ownership-checked profile router.
func registerSportsPreferenceRoutes(profileProtected *mux.Router, h *handlers.UserSettingsHandler) {
	profileProtected.HandleFunc("/{userID}/sports/preferences", h.GetSportsPreferences).Methods(http.MethodGet)
	profileProtected.HandleFunc("/{userID}/sports/preferences", h.PutSportsPreferences).Methods(http.MethodPut)
	profileProtected.HandleFunc("/{userID}/sports/preferences", h.Options).Methods(http.MethodOptions)
}
