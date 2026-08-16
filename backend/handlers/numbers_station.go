package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"novastream/internal/auth"
	"novastream/services/numbersstation"
)

const numbersStationMaxBodyBytes int64 = 4096

type NumbersStationHandler struct {
	service *numbersstation.Service
}

func NewNumbersStationHandler(service *numbersstation.Service) *NumbersStationHandler {
	return &NumbersStationHandler{service: service}
}

func (h *NumbersStationHandler) State(w http.ResponseWriter, r *http.Request) {
	state, err := h.service.State(r.Context(), auth.GetAccountID(r))
	if err != nil {
		log.Printf("[numbers-station] load state failed for account %q: %v", auth.GetAccountID(r), err)
		http.Error(w, "Unable to tune the receiver", http.StatusInternalServerError)
		return
	}
	writeNumbersStationJSON(w, http.StatusOK, state)
}

func (h *NumbersStationHandler) Submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, numbersStationMaxBodyBytes)
	var request struct {
		Answer string `json:"answer"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Answer) == "" {
		writeNumbersStationJSON(w, http.StatusBadRequest, map[string]string{"error": "A response is required"})
		return
	}

	state, err := h.service.Submit(r.Context(), auth.GetAccountID(r), request.Answer)
	if errors.Is(err, numbersstation.ErrIncorrect) {
		writeNumbersStationJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
			"error": "Transmission rejected",
			"state": state,
		})
		return
	}
	if err != nil {
		log.Printf("[numbers-station] submit failed for account %q: %v", auth.GetAccountID(r), err)
		http.Error(w, "Unable to verify the transmission", http.StatusInternalServerError)
		return
	}
	writeNumbersStationJSON(w, http.StatusOK, state)
}

func writeNumbersStationJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
