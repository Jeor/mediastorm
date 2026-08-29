package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"novastream/models"
	"novastream/services/remoteaccess"
)

type middlewareInviteRepo struct {
	invites []models.RemoteAccessInvite
	listErr error
}

func TestClaimInviteRequiresBodyAndHeaderDeviceIdentityToMatch(t *testing.T) {
	handler := NewRemoteAccessHandler(remoteaccess.NewService(&middlewareInviteRepo{}, nil))
	for _, tc := range []struct {
		name     string
		headerID string
	}{
		{name: "missing header"},
		{name: "different header", headerID: "device-2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/remote-access/invites/claim",
				strings.NewReader(`{"token":"mshost-ABCDEF-GHJKMN-PQRSTV","peerId":"device-1"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			if tc.headerID != "" {
				req.Header.Set("X-Client-ID", tc.headerID)
			}
			response := httptest.NewRecorder()

			handler.ClaimInvite(response, req)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), remoteaccess.ErrInvalidPeerID.Error()) {
				t.Fatalf("body = %q, want invalid peer id", response.Body.String())
			}
		})
	}
}

func (r *middlewareInviteRepo) Get(context.Context, string) (*models.RemoteAccessInvite, error) {
	return nil, nil
}
func (r *middlewareInviteRepo) GetByTokenHash(context.Context, string) (*models.RemoteAccessInvite, error) {
	return nil, nil
}
func (r *middlewareInviteRepo) List(context.Context) ([]models.RemoteAccessInvite, error) {
	return r.invites, r.listErr
}
func (r *middlewareInviteRepo) Create(context.Context, *models.RemoteAccessInvite) error {
	return nil
}
func (r *middlewareInviteRepo) ClaimByTokenHash(context.Context, string, string, time.Time) (*models.RemoteAccessInvite, error) {
	return nil, nil
}
func (r *middlewareInviteRepo) Update(context.Context, *models.RemoteAccessInvite) error {
	return nil
}
func (r *middlewareInviteRepo) Delete(context.Context, string) error { return nil }
func (r *middlewareInviteRepo) Count(context.Context) (int64, error) { return 0, nil }

func TestRemoteAccessRevocationMiddlewareGatesIrohRequestsToPairedDevices(t *testing.T) {
	now := time.Now().UTC()
	revokedAt := now.Add(time.Minute)
	repo := &middlewareInviteRepo{invites: []models.RemoteAccessInvite{
		{UsedAt: &now, UsedByPeerID: "active-device"},
		{UsedAt: &now, UsedByPeerID: "revoked-device", RevokedAt: &revokedAt},
	}}
	middleware := RemoteAccessRevocationMiddleware(remoteaccess.NewService(repo, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := middleware(next)

	tests := []struct {
		name     string
		method   string
		path     string
		proxied  bool
		clientID string
		want     int
	}{
		{name: "direct request bypasses pairing gate", method: http.MethodGet, path: "/api/settings", want: http.StatusNoContent},
		{name: "health is available before pairing", method: http.MethodGet, path: "/health", proxied: true, want: http.StatusNoContent},
		{name: "claim is available before pairing", method: http.MethodPost, path: "/api/remote-access/invites/claim", proxied: true, want: http.StatusNoContent},
		{name: "similar path is not a pre-pairing bypass", method: http.MethodPost, path: "/other/remote-access/invites/claim", proxied: true, want: http.StatusForbidden},
		{name: "active pairing passes", method: http.MethodGet, path: "/api/settings", proxied: true, clientID: "active-device", want: http.StatusNoContent},
		{name: "missing device is rejected", method: http.MethodGet, path: "/api/settings", proxied: true, want: http.StatusForbidden},
		{name: "unknown device is rejected", method: http.MethodGet, path: "/api/settings", proxied: true, clientID: "unknown-device", want: http.StatusForbidden},
		{name: "revoked device is rejected", method: http.MethodGet, path: "/api/settings", proxied: true, clientID: "revoked-device", want: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.proxied {
				req.Header.Set("X-Mediastorm-Iroh-Proxy", "1")
			}
			if tc.clientID != "" {
				req.Header.Set("X-Client-ID", tc.clientID)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tc.want, response.Body.String())
			}
		})
	}
}

func TestRemoteAccessRevocationMiddlewareFailsClosedWhenPairingLookupFails(t *testing.T) {
	repo := &middlewareInviteRepo{listErr: errors.New("database unavailable")}
	handler := RemoteAccessRevocationMiddleware(remoteaccess.NewService(repo, nil))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("X-Mediastorm-Iroh-Proxy", "1")
	req.Header.Set("X-Client-ID", "active-device")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
