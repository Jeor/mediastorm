package remoteaccess

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// PublicIdentity reads the running host identity without starting a process or exposing an invite.
func (m *IrohHostManager) PublicIdentity() (string, error) {
	m.mu.RLock()
	invite := m.invite
	m.mu.RUnlock()
	return publicIdentityFromInvite(invite)
}

func publicIdentityFromInvite(invite string) (string, error) {
	encoded, ok := strings.CutPrefix(invite, "mshost-iroh-")
	if !ok {
		return "", errors.New("host identity unavailable")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("host identity unavailable")
	}
	var addr struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(payload, &addr) != nil {
		return "", errors.New("host identity unavailable")
	}
	id, err := hex.DecodeString(addr.ID)
	if err != nil || len(id) != 32 {
		return "", errors.New("host identity unavailable")
	}
	return strings.ToLower(addr.ID), nil
}

func (s *Service) PublicIdentity() (string, error) {
	host, ok := s.host.(interface{ PublicIdentity() (string, error) })
	if !ok {
		return "", errors.New("host identity unavailable")
	}
	return host.PublicIdentity()
}
