package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestNumbersStationAcceptsTheNextSequenceLine(t *testing.T) {
	repo := &numbersStationTestRepository{}
	handler := NewNumbersStationHandler(numbersstation.NewWithRepository(repo))
	answer := numbersStationLookAndSay("1113213211")
	req := httptest.NewRequest(http.MethodPost, "/admin/api/numbers-station/answer", strings.NewReader(`{"answer":"`+answer+`"}`))
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextKeyAccountID, "account-a"))
	recorder := httptest.NewRecorder()

	handler.Submit(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"completed":true`) {
		t.Fatalf("response did not complete puzzle: %s", recorder.Body.String())
	}
}

func numbersStationLookAndSay(value string) string {
	var result strings.Builder
	for start := 0; start < len(value); {
		end := start + 1
		for end < len(value) && value[end] == value[start] {
			end++
		}
		result.WriteString(strconv.Itoa(end - start))
		result.WriteByte(value[start])
		start = end
	}
	return result.String()
}

func TestNumbersStationDashboardUsesCompactMobileSafeReceiver(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	content := string(templateBytes)
	for _, unwanted := range []string{"Complete the next line.", "numbers-station-hint", "NUMBERS STATION ·", "numbers-station-submit", ">TRANSMIT<", "submitNumbersStation"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("status template still contains %q", unwanted)
		}
	}
	for _, required := range []string{
		`<div class="numbers-station-frequency">77.77 MHz</div>`,
		`transform: translate(-50%, -50%)`,
		`max-height: calc(100dvh - 2rem)`,
		`lockNumbersStationScroll()`,
		`addEventListener('close', () =>`,
		`numbers-station-sender.jpg`,
		`numbers-station-sender numbers-station-sender-static`,
		`aria-label="Radio static"`,
		`SIGNAL ACQUIRED`,
		`The storm is closer than the forecast says.`,
		`numbersStationImageStatic`,
		`numbersStationSequenceFlash`,
		`--numbers-station-line-index: ${index}`,
		`inputmode="numeric"`,
		`oninput="queueNumbersStationAnswer()"`,
		`window.setTimeout(() => validateNumbersStationAnswer(answer, attempt), 320)`,
		`if (response.status !== 422 && feedback)`,
		`readNumbersStationResponse(response)`,
		`SESSION EXPIRED — RELOAD THE DASHBOARD`,
		`numbers-station-achievement-badge" type="button" onclick="openNumbersStation()"`,
		`topbar.appendChild(achievement)`,
		`numbers-station-achievement-present`,
		`numbers-station-achievement-scrolled`,
		`window.addEventListener('scroll', updateNumbersStationAchievementVisibility, { passive: true })`,
		`options.showBroadcast ? 'SIGNAL ACQUIRED' : 'NO CARRIER'`,
		`dashboard-uptime-count`,
		`class="numbers-station-static-break numbers-station-silence-heading"`,
		`— RADIO SILENCE —`,
		`Just static. It sounds like rain.`,
		`onclick="showNumbersStationPreviousMessage()">PREVIOUS MESSAGE</button>`,
		`renderNumbersStation(numbersStationCompletedState, { showBroadcast: true })`,
		`options.showReward ?`,
		`renderNumbersStation(payload, { showBroadcast: true, showReward: true })`,
		`initializeNumbersStationSignal()`,
		`markNumbersStationCompleted()`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("status template missing %q", required)
		}
	}
	for _, unwanted := range []string{"AudioContext", "numbersStationAudioToggle", "startNumbersStationStatic"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("status template still contains audio implementation %q", unwanted)
		}
	}
}

func TestAdminShellDisablesTelephoneNumberDetection(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/base.html")
	if err != nil {
		t.Fatalf("read base template: %v", err)
	}
	if !strings.Contains(string(templateBytes), `<meta name="format-detection" content="telephone=no">`) {
		t.Fatalf("admin shell does not disable telephone number detection")
	}
}

func TestNumbersStationSenderImageIsEmbedded(t *testing.T) {
	image, err := staticAssets.ReadFile("static/numbers-station-sender.jpg")
	if err != nil {
		t.Fatalf("read embedded sender image: %v", err)
	}
	if len(image) < 4 || image[0] != 0xff || image[1] != 0xd8 {
		t.Fatalf("embedded sender image is not a JPEG")
	}
}
