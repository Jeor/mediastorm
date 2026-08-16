package models

import "time"

// NumbersStationProgress records an account's progress through the hidden
// Numbers Station puzzle. Stage is the next transmission to solve.
type NumbersStationProgress struct {
	AccountID   string     `json:"accountId"`
	Stage       int        `json:"stage"`
	Completed   bool       `json:"completed"`
	StartedAt   time.Time  `json:"startedAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
