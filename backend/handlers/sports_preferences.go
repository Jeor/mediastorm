package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"novastream/models"
	user_settings "novastream/services/user_settings"
)

type sportsPreferenceService interface {
	GetSportsPreferences(string) (models.SportsPreferenceSnapshot, error)
	UpdateSportsPreferences(string, uint64, models.SportsPreferences) (models.SportsPreferenceSnapshot, error)
}

func writeSportsPreferenceError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (h *UserSettingsHandler) GetSportsPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	service, ok := h.Service.(sportsPreferenceService)
	if !ok {
		writeSportsPreferenceError(w, "sports preferences unavailable", http.StatusServiceUnavailable)
		return
	}
	snapshot, err := service.GetSportsPreferences(id)
	if err != nil {
		writeSportsPreferenceError(w, "unable to load sports preferences", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(snapshot)
}
func (h *UserSettingsHandler) PutSportsPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	service, ok := h.Service.(sportsPreferenceService)
	if !ok {
		writeSportsPreferenceError(w, "sports preferences unavailable", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			writeSportsPreferenceError(w, "sports preferences too large", http.StatusRequestEntityTooLarge)
		} else {
			writeSportsPreferenceError(w, "unable to read sports preferences", http.StatusBadRequest)
		}
		return
	}
	input, err := models.ParseSportsPreferenceSnapshot(body)
	if err != nil {
		writeSportsPreferenceError(w, "invalid sports preference document", http.StatusBadRequest)
		return
	}
	snapshot, err := service.UpdateSportsPreferences(id, input.Revision, input.Preferences)
	if errors.Is(err, user_settings.ErrSportsPreferencesConflict) {
		writeSportsPreferenceError(w, "sports preferences changed; reload before saving", http.StatusConflict)
		return
	}
	if err != nil {
		writeSportsPreferenceError(w, "unable to save sports preferences", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(snapshot)
}
