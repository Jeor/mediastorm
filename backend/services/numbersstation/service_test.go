package numbersstation

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"novastream/models"
)

type memoryRepository struct {
	progress map[string]*models.NumbersStationProgress
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{progress: make(map[string]*models.NumbersStationProgress)}
}

func (r *memoryRepository) Get(_ context.Context, accountID string) (*models.NumbersStationProgress, error) {
	progress := r.progress[accountID]
	if progress == nil {
		return nil, nil
	}
	copy := *progress
	return &copy, nil
}

func (r *memoryRepository) Advance(_ context.Context, accountID string, expectedStage, nextStage int, completed bool, now time.Time) (bool, error) {
	progress := r.progress[accountID]
	if progress != nil && (progress.Stage != expectedStage || progress.Completed) {
		return false, nil
	}
	if progress == nil && expectedStage != 0 {
		return false, nil
	}
	startedAt := now
	if progress != nil {
		startedAt = progress.StartedAt
	}
	var completedAt *time.Time
	if completed {
		completedAt = &now
	}
	r.progress[accountID] = &models.NumbersStationProgress{
		AccountID: accountID, Stage: nextStage, Completed: completed,
		StartedAt: startedAt, UpdatedAt: now, CompletedAt: completedAt,
	}
	return true, nil
}

func TestCorrectAnswerCompletesOnlyTheCurrentAccount(t *testing.T) {
	repo := newMemoryRepository()
	service := NewWithRepository(repo)
	answer := lookAndSay(stages[0].transmission.Lines[len(stages[0].transmission.Lines)-1])

	state, err := service.Submit(context.Background(), "account-a", answer)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if state.Stage != 1 || !state.Completed || state.Reward == "" || state.Transmission != nil {
		t.Fatalf("Submit() state = %+v, want completed reward", state)
	}
	other, err := service.State(context.Background(), "account-b")
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if other.Stage != 0 {
		t.Fatalf("other account stage = %d, want 0", other.Stage)
	}
}

func TestIncorrectAnswerDoesNotCreateProgress(t *testing.T) {
	repo := newMemoryRepository()
	service := NewWithRepository(repo)

	state, err := service.Submit(context.Background(), "account-a", "silence")
	if err != ErrIncorrect {
		t.Fatalf("Submit() error = %v, want ErrIncorrect", err)
	}
	if state.Stage != 0 || repo.progress["account-a"] != nil {
		t.Fatalf("incorrect answer changed progress: state=%+v stored=%+v", state, repo.progress["account-a"])
	}
}

func TestCompletedTransmissionRemainsCompleted(t *testing.T) {
	repo := newMemoryRepository()
	service := NewWithRepository(repo)
	answer := lookAndSay(stages[0].transmission.Lines[len(stages[0].transmission.Lines)-1])
	if _, err := service.Submit(context.Background(), "account-a", answer); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	state, err := service.State(context.Background(), "account-a")
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if !state.Completed || state.Stage != 1 || state.StageCount != 1 || state.Reward == "" || state.Transmission != nil {
		t.Fatalf("completed state = %+v", state)
	}
}

func lookAndSay(value string) string {
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
