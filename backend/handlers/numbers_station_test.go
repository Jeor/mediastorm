package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"novastream/internal/auth"
	"novastream/models"
	"novastream/services/numbersstation"
)

type numbersStationTestRepository struct {
	lastAccountID string
}

func (r *numbersStationTestRepository) Get(_ context.Context, accountID string) (*models.NumbersStationProgress, error) {
	r.lastAccountID = accountID
	return nil, nil
}

func (r *numbersStationTestRepository) Advance(_ context.Context, _ string, _, _ int, _ bool, _ time.Time) (bool, error) {
	return true, nil
}

func TestNumbersStationStateReturnsOnlyUnlockedTransmission(t *testing.T) {
	repo := &numbersStationTestRepository{}
	handler := NewNumbersStationHandler(numbersstation.NewWithRepository(repo))
	req := httptest.NewRequest(http.MethodGet, "/admin/api/numbers-station", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextKeyAccountID, "account-a"))
	recorder := httptest.NewRecorder()

	handler.State(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if repo.lastAccountID != "account-a" {
		t.Fatalf("repository account = %q, want account-a", repo.lastAccountID)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "TRANSMISSION 01") {
		t.Fatalf("response missing current transmission: %s", body)
	}
	for _, locked := range []string{"TRANSMISSION 02", "TRANSMISSION 03", "audioactive decay"} {
		if strings.Contains(body, locked) {
			t.Fatalf("response exposed locked clue %q: %s", locked, body)
		}
	}
}

func TestNumbersStationRejectsIncorrectTransmission(t *testing.T) {
	repo := &numbersStationTestRepository{}
	handler := NewNumbersStationHandler(numbersstation.NewWithRepository(repo))
	req := httptest.NewRequest(http.MethodPost, "/admin/api/numbers-station/answer", strings.NewReader(`{"answer":"silence"}`))
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextKeyAccountID, "account-a"))
	recorder := httptest.NewRecorder()

	handler.Submit(recorder, req)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Transmission rejected") {
		t.Fatalf("response = %s", recorder.Body.String())
	}
}
