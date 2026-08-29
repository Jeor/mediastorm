package handlers

import (
	"strings"
	"testing"
)

func TestPairedDevicesTemplateUsesBaseTemplateBasePath(t *testing.T) {
	content, err := adminTemplates.ReadFile("admin_templates/paired_devices.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "const basePath") {
		t.Fatal("paired devices template must use base.html's global basePath instead of redeclaring it")
	}
}
