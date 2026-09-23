package handlers

import (
	"testing"

	"novastream/config"
)

func TestPublicMetaDBKeyIsRedactedAndPreservedOnSettingsSave(t *testing.T) {
	stored := config.Settings{PublicMetaDB: config.PublicMetaDBSettings{Accounts: []config.PublicMetaDBAccount{{ID: "one", APIKey: "secret"}}}}
	incoming := stored
	incoming.PublicMetaDB.Accounts = append([]config.PublicMetaDBAccount(nil), stored.PublicMetaDB.Accounts...)
	redactSettings(&incoming)
	if incoming.PublicMetaDB.Accounts[0].APIKey != redactedPlaceholder {
		t.Fatalf("PublicMetaDB key was not redacted")
	}
	preserveRedactedFields(&incoming, &stored)
	if incoming.PublicMetaDB.Accounts[0].APIKey != "secret" {
		t.Fatalf("settings save lost PublicMetaDB key")
	}
}
