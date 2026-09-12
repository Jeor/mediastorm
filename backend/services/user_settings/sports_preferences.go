package user_settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"novastream/models"
)

var ErrSportsPreferencesConflict = errors.New("sports preferences changed; reload before saving")

func sportsSnapshot(raw json.RawMessage) (models.SportsPreferenceSnapshot, error) {
	if len(raw) == 0 {
		return models.SportsPreferenceSnapshot{Preferences: models.EmptySportsPreferences()}, nil
	}
	return models.ParseSportsPreferenceSnapshot(raw)
}
func (s *Service) GetSportsPreferences(userID string) (models.SportsPreferenceSnapshot, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return models.SportsPreferenceSnapshot{}, ErrUserIDRequired
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sportsSnapshot(s.settings[userID].SportsPreferences)
}

// Atomic within this service instance, matching the existing settings store.
// Multiple backend replicas require a database-native CAS implementation.
func (s *Service) UpdateSportsPreferences(userID string, expectedRevision uint64, preferences models.SportsPreferences) (models.SportsPreferenceSnapshot, error) {
	var empty models.SportsPreferenceSnapshot
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return empty, ErrUserIDRequired
	}
	raw, err := json.Marshal(preferences)
	if err != nil {
		return empty, models.ErrInvalidSportsPreferences
	}
	parsed, err := models.ParseSportsPreferences(raw)
	if err != nil {
		return empty, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.settings[userID]
	current, err := sportsSnapshot(previous.SportsPreferences)
	if err != nil {
		return empty, err
	} // Never replace an unreadable/future snapshot.
	if current.Revision != expectedRevision {
		return empty, ErrSportsPreferencesConflict
	}
	if current.Revision >= models.MaxSportsPreferenceRevision {
		return empty, fmt.Errorf("sports preference revision exhausted")
	}
	next := models.SportsPreferenceSnapshot{Revision: current.Revision + 1, Preferences: parsed}
	encoded, err := json.Marshal(next)
	if err != nil {
		return empty, err
	}
	updated := previous
	updated.SportsPreferences = encoded
	s.settings[userID] = updated
	if err := s.saveLocked(); err != nil {
		if existed {
			s.settings[userID] = previous
		} else {
			delete(s.settings, userID)
		}
		return empty, err
	}
	// Stored bytes and returned objects have no shared mutable data.
	return next, nil
}
