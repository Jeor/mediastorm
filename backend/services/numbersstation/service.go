package numbersstation

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"

	"novastream/internal/datastore"
	"novastream/models"
)

const answerDomain = "numbers-station-7777:v1:"

var ErrIncorrect = errors.New("transmission rejected")

type repository interface {
	Get(ctx context.Context, accountID string) (*models.NumbersStationProgress, error)
	Advance(ctx context.Context, accountID string, expectedStage, nextStage int, completed bool, now time.Time) (bool, error)
}

type Transmission struct {
	Callsign string   `json:"callsign"`
	Lines    []string `json:"lines,omitempty"`
	Prompt   string   `json:"prompt"`
}

type State struct {
	Stage        int           `json:"stage"`
	StageCount   int           `json:"stageCount"`
	Completed    bool          `json:"completed"`
	Transmission *Transmission `json:"transmission,omitempty"`
	Reward       string        `json:"reward,omitempty"`
}

type stageDefinition struct {
	transmission  Transmission
	answerDigests []string
}

var stages = []stageDefinition{
	{
		transmission: Transmission{
			Callsign: "TRANSMISSION 01",
			Lines:    []string{"1", "11", "21", "1211", "111221", "312211", "13112221", "1113213211"},
			Prompt:   "The carrier is still open.",
		},
		answerDigests: []string{"19cb6b814d04a42ee02f897d9a03d4e10705a398a4980c36756aab69bfb81a5f"},
	},
}

type Service struct {
	repo repository
	now  func() time.Time
}

func New(store *datastore.DataStore) *Service {
	return &Service{repo: store.NumbersStation(), now: time.Now}
}

func NewWithRepository(repo repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) State(ctx context.Context, accountID string) (State, error) {
	progress, err := s.repo.Get(ctx, accountID)
	if err != nil {
		return State{}, err
	}
	stage := 0
	completed := false
	if progress != nil {
		stage = progress.Stage
		completed = progress.Completed
	}
	return stateFor(stage, completed), nil
}

func (s *Service) Submit(ctx context.Context, accountID, answer string) (State, error) {
	current, err := s.State(ctx, accountID)
	if err != nil || current.Completed {
		return current, err
	}
	if !matchesStage(current.Stage, answer) {
		return current, ErrIncorrect
	}

	nextStage := current.Stage + 1
	completed := nextStage >= len(stages)
	advanced, err := s.repo.Advance(ctx, accountID, current.Stage, nextStage, completed, s.now().UTC())
	if err != nil {
		return State{}, err
	}
	if !advanced {
		return s.State(ctx, accountID)
	}
	return stateFor(nextStage, completed), nil
}

func stateFor(stage int, completed bool) State {
	state := State{Stage: stage, StageCount: len(stages), Completed: completed}
	if completed || stage >= len(stages) {
		state.Completed = true
		state.Reward = "I heard the storm."
		return state
	}
	transmission := stages[stage].transmission
	state.Transmission = &transmission
	return state
}

func matchesStage(stage int, answer string) bool {
	if stage < 0 || stage >= len(stages) {
		return false
	}
	digest := sha256.Sum256([]byte(answerDomain + normalize(answer)))
	for _, encoded := range stages[stage].answerDigests {
		expected, err := hex.DecodeString(encoded)
		if err == nil && len(expected) == len(digest) && subtle.ConstantTimeCompare(digest[:], expected) == 1 {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(value))
}
