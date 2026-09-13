package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"novastream/models"
	"novastream/services/remoteaccess"
)

type identityHost struct{ id string }

func (h identityHost) Ensure(context.Context) (string, error) {
	panic("identity probe must not start a host")
}
func (h identityHost) Stop(context.Context) error { return nil }
func (h identityHost) Status(context.Context) models.RemoteAccessStatus {
	return models.RemoteAccessStatus{}
}
func (h identityHost) PublicIdentity() (string, error) {
	if h.id == "" {
		return "", errors.New("unavailable")
	}
	return h.id, nil
}

func TestRemoteAccessIdentityProbe(t *testing.T) {
	for _, id := range []string{strings.Repeat("ab", 32), ""} {
		h := NewRemoteAccessHandler(remoteaccess.NewService(nil, identityHost{id: id}))
		response := httptest.NewRecorder()
		h.Identity(response, httptest.NewRequest(http.MethodGet, "/api/remote-access/identity", nil))
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("identity may be cached")
		}
		if id == "" {
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d", response.Code)
			}
		} else {
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || len(body) != 1 || body["serverId"] != id {
				t.Fatalf("unexpected identity response: %v", body)
			}
		}
	}
}
