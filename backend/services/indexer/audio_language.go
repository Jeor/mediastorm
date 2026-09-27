package indexer

import (
	"strings"

	"novastream/config"
	"novastream/utils/language"
)

// Playback preferences cascade from global to profile to device, independently
// of the metadata language that selects release search titles.
func (s *Service) getEffectiveAudioLanguage(userID, clientID string, settings config.Settings) string {
	preferred := strings.TrimSpace(settings.Playback.PreferredAudioLanguage)
	if userID != "" && s.userSettings != nil {
		if profile, err := s.userSettings.Get(userID); err == nil && profile != nil && strings.TrimSpace(profile.Playback.PreferredAudioLanguage) != "" {
			preferred = strings.TrimSpace(profile.Playback.PreferredAudioLanguage)
		}
	}
	if clientID != "" && s.clientSettings != nil {
		if client, err := s.clientSettings.Get(clientID, userID); err == nil && client != nil && client.PreferredAudioLanguage != nil {
			preferred = strings.TrimSpace(*client.PreferredAudioLanguage)
		}
	}
	if normalized := language.NormalizeToCode(preferred); normalized != "" {
		return normalized
	}
	return preferred
}
