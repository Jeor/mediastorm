package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"novastream/internal/aiprovider"
	"novastream/internal/apiusage"
)

// validateOllama checks model availability without loading a model or running inference.
func validateOllama(ctx context.Context, client *http.Client, baseURL, model, apiKey string) error {
	base, err := aiprovider.OllamaBaseURL(baseURL)
	if err != nil {
		return err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("Ollama requires an installed model name")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return fmt.Errorf("invalid Ollama server URL")
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := apiusage.Do(client, "Ollama", "Model validation", req)
	if err != nil {
		return fmt.Errorf("cannot reach Ollama from the backend; check server address and network: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Ollama proxy authentication failed (%s)", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama model lookup failed (%s)", resp.Status)
	}
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return fmt.Errorf("Ollama returned an invalid model list")
	}
	for _, candidate := range models.Data {
		if candidate.ID == model || (!strings.Contains(model, ":") && candidate.ID == model+":latest") {
			return nil
		}
	}
	return fmt.Errorf("Ollama model %q is not installed; pull it on the Ollama server or choose an installed model", model)
}
