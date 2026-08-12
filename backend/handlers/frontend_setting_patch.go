package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"novastream/config"
)

type frontendSettingPatch struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
	Reset bool            `json:"reset,omitempty"`
}

func decodeFrontendSettingPatch(decoder *json.Decoder, manager *config.Manager) (frontendSettingPatch, error) {
	var patch frontendSettingPatch
	if err := decoder.Decode(&patch); err != nil {
		return patch, errors.New("invalid request body")
	}
	patch.Path = strings.TrimSpace(patch.Path)
	if patch.Path == "" {
		return patch, errors.New("setting path is required")
	}
	if manager == nil {
		return patch, errors.New("settings configuration is unavailable")
	}
	settings, err := manager.Load()
	if err != nil {
		return patch, fmt.Errorf("load settings configuration: %w", err)
	}
	allowed := false
	for _, path := range filterUserEditableSettings(settings.UI.UserEditableSettings) {
		if path == patch.Path {
			allowed = true
			break
		}
	}
	if !allowed {
		return patch, fmt.Errorf("setting %q is not available for frontend editing", patch.Path)
	}
	if !patch.Reset {
		if len(bytes.TrimSpace(patch.Value)) == 0 || bytes.Equal(bytes.TrimSpace(patch.Value), []byte("null")) {
			return patch, errors.New("setting value is required")
		}
		if err := validateUserEditableSettingValue(patch.Path, patch.Value); err != nil {
			return patch, err
		}
	}
	return patch, nil
}

func patchJSONObject(document []byte, path string, value json.RawMessage, reset bool) ([]byte, error) {
	root := make(map[string]interface{})
	if len(bytes.TrimSpace(document)) > 0 && !bytes.Equal(bytes.TrimSpace(document), []byte("null")) {
		if err := json.Unmarshal(document, &root); err != nil {
			return nil, err
		}
	}
	parts := strings.Split(path, ".")
	current := root
	parents := make([]map[string]interface{}, 0, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		parents = append(parents, current)
		next, _ := current[part].(map[string]interface{})
		if next == nil {
			if reset {
				return json.Marshal(root)
			}
			next = make(map[string]interface{})
			current[part] = next
		}
		current = next
	}
	leaf := parts[len(parts)-1]
	if reset {
		delete(current, leaf)
		for i := len(parents) - 1; i >= 0; i-- {
			childKey := parts[i]
			child, _ := parents[i][childKey].(map[string]interface{})
			if len(child) != 0 {
				break
			}
			delete(parents[i], childKey)
		}
	} else {
		var decoded interface{}
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, err
		}
		current[leaf] = decoded
	}
	return json.Marshal(root)
}

func clientSettingPath(globalPath string) (string, bool) {
	section, field, ok := strings.Cut(globalPath, ".")
	if !ok || field == "" {
		return "", false
	}
	switch section {
	case "filtering", "playback", "display", "network":
		return field, true
	case "ranking":
		if field == "newestReleaseFirst" {
			return field, true
		}
	}
	return "", false
}
