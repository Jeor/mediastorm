package remoteaccess

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPublicIdentityStableAcrossAddressChanges(t *testing.T) {
	id := strings.Repeat("ab", 32)
	for _, address := range []string{"old", "new"} {
		invite := "mshost-iroh-" + base64.RawURLEncoding.EncodeToString([]byte(`{"id":"`+id+`","addrs":["`+address+`"]}`))
		host := &IrohHostManager{invite: invite}
		got, err := NewService(nil, host).PublicIdentity()
		if err != nil || got != id {
			t.Fatalf("identity = %q, %v", got, err)
		}
	}
}

func TestPublicIdentityRejectsUnavailableOrMalformedHost(t *testing.T) {
	for _, invite := range []string{"", "secret", "mshost-iroh-invalid", "mshost-iroh-" + base64.RawURLEncoding.EncodeToString([]byte(`{"id":"short"}`))} {
		if _, err := publicIdentityFromInvite(invite); err == nil {
			t.Fatal("accepted invalid identity")
		}
	}
	if _, err := NewService(nil, nil).PublicIdentity(); err == nil {
		t.Fatal("accepted absent host")
	}
}
