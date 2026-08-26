package handlers

import (
	"strings"
	"testing"
)

func TestAdminStatusAcceptsLibraryStreamServiceTypes(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	if marker := `['debrid', 'usenet', 'local', 'plex', 'jellyfin', 'stream']`; !strings.Contains(string(templateBytes), marker) {
		t.Fatalf("status template missing library stream service types %s", marker)
	}
}
