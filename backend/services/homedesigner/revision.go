package homedesigner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"novastream/config"
	"novastream/models"
)

// RevisionForGlobal hashes only persisted Home Designer fields, leaving
// unrelated global settings such as playback outside optimistic concurrency.
func RevisionForGlobal(settings config.Settings) string {
	return revisionFor(struct {
		Rows  config.HomeShelvesSettings `json:"rows"`
		Theme config.AppearanceSettings  `json:"theme"`
	}{Rows: settings.HomeShelves, Theme: settings.Display.Appearance})
}

// RevisionForProfile hashes only persisted profile Home Designer fields.
func RevisionForProfile(settings *models.UserSettings) string {
	if settings == nil {
		return revisionFor(struct {
			Rows  models.HomeShelvesSettings `json:"rows"`
			Theme models.AppearanceSettings  `json:"theme"`
		}{})
	}
	return revisionFor(struct {
		Rows  models.HomeShelvesSettings `json:"rows"`
		Theme models.AppearanceSettings  `json:"theme"`
	}{Rows: settings.HomeShelves, Theme: settings.Display.Appearance})
}

func revisionFor(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
