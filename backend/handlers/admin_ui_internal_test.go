package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"novastream/config"
	"novastream/models"
)

type fakeDatabaseMaintenance struct {
	watchHistoryCalls     int
	playbackProgressCalls int
	watchlistCalls        int
}

func (f *fakeDatabaseMaintenance) ClearWatchHistory() (int, error) {
	f.watchHistoryCalls++
	return 12, nil
}

func (f *fakeDatabaseMaintenance) ClearPlaybackProgress() (int, error) {
	f.playbackProgressCalls++
	return 7, nil
}

func (f *fakeDatabaseMaintenance) ClearWatchlists() (int, error) {
	f.watchlistCalls++
	return 3, nil
}

func TestNotificationsTemplateLoads(t *testing.T) {
	handler := NewAdminUIHandler("", "", nil, nil, nil, nil)
	if handler.notificationsTemplate == nil {
		t.Fatal("notifications template failed to load")
	}
}

func TestHomeDesignerTemplateProvidesAccessibleEditorStructure(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/home_designer.html")
	if err != nil {
		t.Fatalf("read Home Designer template: %v", err)
	}
	source := string(templateBytes)
	if strings.Count(source, "<h1>") != 1 {
		t.Fatalf("Home Designer must have exactly one h1, got %d", strings.Count(source, "<h1>"))
	}
	for _, marker := range []string{
		`aria-label="Row library"`,
		`aria-label="Composition preview"`,
		`aria-label="Row inspector"`,
		`data-home-designer-status aria-live="polite"`,
		`data-home-designer-errors aria-live="assertive"`,
		`data-home-designer-apply`,
		`data-home-designer-discard`,
		`data-home-designer-undo`,
		`data-home-designer-redo`,
		`data-home-designer-scope`,
		`data-home-designer-preview-profile`,
		`data-home-designer-preview-platform`,
		`href="{{.BasePath}}/settings?category=homeShelves"`,
		`href="{{.BasePath}}/settings?category=display"`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("Home Designer template missing accessibility/navigation marker %q", marker)
		}
	}
}

func TestAdminSettingsKeepsAdvancedControlsAndLinksToHomeDesigner(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)
	for _, marker := range []string{
		`homeShelves.shelves`,
		`display.appearance`,
		`Open Home Designer`,
		`settings-home-designer-callout`,
		`basePath + '/settings/home-designer'`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing Home Designer preservation marker %q", marker)
		}
	}
}

func TestToolsTemplateIncludesProfileScrobLinking(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatalf("read tools template: %v", err)
	}
	source := string(templateBytes)
	for _, marker := range []string{
		"profile.scrobAccountId",
		"updateProfileScrobLink",
		"/api/users/${profileId}/scrob",
		"No Scrob",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("tools template missing profile Scrob marker %q", marker)
		}
	}
}

func TestLibraryTemplateDirectsRemoteAccountsToIntegrations(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/library.html")
	if err != nil {
		t.Fatalf("read library template: %v", err)
	}
	source := string(templateBytes)
	if !strings.Contains(source, `Accounts are managed on the <a href="{{.BasePath}}/integrations">Integrations page</a>.`) {
		t.Fatal("library template does not direct remote account management to Integrations")
	}
	if strings.Contains(source, "Accounts are managed on the Tools page.") {
		t.Fatal("library template still directs remote account management to Tools")
	}
}

func TestJellyfinConnectionStatusMatchesPlexStyling(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatalf("read tools template: %v", err)
	}
	source := string(templateBytes)
	for _, marker := range []string{
		"badge.textContent = `${connectedCount} Connected`;",
		"badge.className = 'status-badge connected';",
		`account.connected ? '<span class="status-badge connected"`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("Jellyfin status missing Plex-style marker %q", marker)
		}
	}
	if strings.Contains(source, `class="status-badge success"`) || strings.Contains(source, "badge.className = 'status-badge success'") {
		t.Fatal("Jellyfin status still uses the undefined success badge variant")
	}
}

func TestNotificationsTemplateDoesNotRedeclareBasePath(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/notifications.html")
	if err != nil {
		t.Fatalf("read notifications template: %v", err)
	}
	if strings.Contains(string(templateBytes), "const basePath =") {
		t.Fatal("notifications template redeclares the base template's global basePath")
	}
}

func TestNotificationsTemplateOmitsRedundantPlayingEvent(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/notifications.html")
	if err != nil {
		t.Fatalf("read notifications template: %v", err)
	}
	source := string(templateBytes)
	if strings.Contains(source, `value="watch.playing"`) {
		t.Fatal("notifications template still exposes the redundant playing event")
	}
	if strings.Contains(source, "Now playing") {
		t.Fatal("notifications template still labels a playing notification")
	}
}

func TestNotificationsTemplateIncludesSystemOperationsSection(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/notifications.html")
	if err != nil {
		t.Fatalf("read notifications template: %v", err)
	}
	source := string(templateBytes)
	for _, marker := range []string{
		"System Operations",
		`value="system.startup"`,
		`value="system.shutdown"`,
		`id="system-settings"`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("notifications template missing system operations marker %q", marker)
		}
	}
}

func TestNotificationListDisablesCaching(t *testing.T) {
	handler := &AdminUIHandler{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/notifications?profileId=profile", nil)

	handler.ListNotificationChannels(recorder, request)

	if got := recorder.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q, want no-store, max-age=0", got)
	}
}

func TestAdminSettingsSaveCommitsPendingTextArrayInputs(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`data-text-array-kind="tags"`,
		`data-text-array-kind="weighted-tags"`,
		"function commitPendingTextArrayInputs()",
		"if (committedPendingTextArrays) renderSettings();",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing pending text-array marker %q", marker)
		}
	}

	for _, saveFunction := range []string{"saveSection", "saveAllSettings"} {
		start := strings.Index(source, "async function "+saveFunction+"(")
		if start < 0 {
			t.Fatalf("settings template missing %s", saveFunction)
		}
		body := source[start:]
		commit := strings.Index(body, "commitPendingTextArrayInputs();")
		serialize := strings.Index(body, "JSON.stringify(")
		if commit < 0 || serialize < 0 || commit > serialize {
			t.Fatalf("%s must commit pending text-array inputs before serializing settings", saveFunction)
		}
	}
}

func TestAdminSettingsSensitiveFieldsAllowOnlyOneReveal(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		"const lockedSensitiveFields = new Set();",
		`onfocus="revealSensitiveField(this,`,
		`onblur="lockSensitiveField(this,`,
		"if (lockedSensitiveFields.has(sensitiveFieldPath(basePath, fieldKey))) return;",
		"if (input.value !== '') return;",
		"input.type = 'text';",
		"input.type = 'password';",
		"lockedSensitiveFields.add(sensitiveFieldPath(basePath, fieldKey));",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing one-time sensitive-field reveal marker %q", marker)
		}
	}
}

func TestAdminSettingsMediaLibraryOptionsDisambiguateServersAndPreserveMissingSelections(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		"library.sourceServerName || library.serverName",
		"function getMediaLibraryOptionsHTML(selectedValue)",
		"selectedValue && !mediaLibrariesData.some",
		"Missing library · ${libraryId}",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing media-library option behavior %q", marker)
		}
	}
}

func TestAdminSettingsInheritedTermListsShowEffectiveValuesAndReplacementSemantics(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`if (found && val !== undefined && val !== null)`,
		`<strong>Inherited list:</strong> These are the current `,
		`creates a complete ' + scopeLabel + ' override starting with this list.`,
		`This complete list replaces the ' + parentLabel + ' list.`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing inherited term-list behavior %q", marker)
		}
	}
}

func TestAdminSettingsCustomShelfActionsAlignWithInputs(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		".add-custom-list-form .form-group{flex:1;margin-bottom:0;}",
		".tmdb-source-actions button,.add-custom-list-submit{height:38px;display:inline-flex;align-items:center;justify-content:center;}",
		"new URLSearchParams(window.location.search).get('layoutDebug') === '1'",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing custom shelf alignment marker %q", marker)
		}
	}
}

func TestAdminSettingsAddListIncludesSharedActivityShelves(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`<option value="popular-on-server">Popular on This Server</option>`,
		`<option value="recently-watched">Recently Watched</option>`,
		`'popular-on-server': 'Popular on This Server'`,
		`'recently-watched': 'Recently Watched'`,
		`existingShelf.enabled = true`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing shared activity shelf add-list marker %q", marker)
		}
	}
}

func TestAdminSettingsUsesCategoryAndDetailProgressiveDisclosure(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`id="settingsCategoryNav"`,
		`id="settingsBasicBtn" class="settings-level-btn" type="button" disabled aria-disabled="true"`,
		`id="settingsAdvancedBtn"`,
		`autocomplete="off" autocapitalize="none" spellcheck="false"`,
		`const basicSettingsReady = false;`,
		`let settingsLevel = 'advanced';`,
		`settingsLevel = (basicSettingsReady && level === 'basic') ? 'basic' : 'advanced';`,
		`.page-header-controls .form-select {`,
		`height: 40px;`,
		`function setSettingsLevel(level)`,
		`const advancedSections = new Set`,
		`const friendlySettingsCopy = [`,
		`'Streaming Method'`,
		`'Adapt to Each Device'`,
		`const settingsOverviewGroups = [`,
		`{ id: 'sources', label: 'Sources & Providers' }`,
		`{ id: 'search', label: 'Search & Results' }`,
		`{ id: 'server', label: 'Server & Network' }`,
		`function toggleSettingsSection(header)`,
		`document.querySelectorAll('#settingsContainer .section.open')`,
		`function handleSettingsSectionKeydown(event, header)`,
		`const firstMatch = filteredSections.values().next().value;`,
		`propagateBtnLabel.textContent = 'Review Customizations'`,
		`settingsLevel === 'basic' && !searchTerm && advancedSections.has(key)`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing progressive-disclosure marker %q", marker)
		}
	}
}

func TestAdminSettingsMobileScopeControlsShowClearState(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`class="settings-scope-control-label">Person</span>`,
		`class="settings-scope-control-label">Device</span>`,
		`<option value="">Choose a person</option>`,
		`<option value="">Choose a device</option>`,
		`clientSelector.innerHTML = '<option value="">Choose a person first</option>';`,
		`clientSelector.innerHTML = '<option value="">No devices</option>';`,
		`.settings-scope-select-wrap.active {`,
		`grid-template-columns: minmax(4.5rem, auto) minmax(0, 1fr) 18px;`,
		`.settings-scope-select-wrap::after {`,
		`.settings-view-compact { align-self: flex-start; justify-content: flex-start;`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing clear mobile-scope marker %q", marker)
		}
	}
}

func TestAdminSettingsRendersUsenetIndexerTableWithoutChangingContracts(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`function renderIndexerTableSection(sectionDef, items, basePath)`,
		`<th scope="col">Name</th>`,
		`<th scope="col">Status</th>`,
		`<th scope="col">Priority</th>`,
		`<th scope="col">API Key</th>`,
		`<th scope="col">Actions</th>`,
		`data-label="Priority">' + (index + 1)`,
		`id="test-btn-indexers-' + index + '"`,
		`onclick="testProvider(\'indexers\', ' + index + ')">Test</button>`,
		`renderInput(fieldKey, fieldDef, fieldValue, basePath + '.' + index, 'indexers')`,
		`removeArrayItem('indexers', index)`,
		`addArrayItem('indexers')`,
		`saveSection(\'indexers\', event)`,
		`if (sectionKey === 'indexers') return renderIndexerTableSection(sectionDef, items, basePath);`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing indexer-table contract marker %q", marker)
		}
	}

	if strings.Contains(source, `reorderArrayItem('indexers'`) {
		t.Fatal("indexer table must not reorder rows because redacted API keys are restored by stable array index")
	}
}

func TestAdminSettingsRendersDebridAndTorrentProviderTablesWithoutChangingContracts(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`const providerTableSectionKeys = new Set(['indexers', 'debridProviders', 'torrentScrapers'])`,
		`function renderProviderTableSection(sectionKey, sectionDef, items, basePath, options)`,
		`function renderProviderEditFields(sectionKey, sectionDef, item, index, basePath)`,
		`if (sectionKey === 'debridProviders')`,
		`if (sectionKey === 'torrentScrapers')`,
		`data-label="Priority">' + (index + 1)`,
		`id="test-btn-' + sectionKey + '-' + index + '"`,
		`onclick="testProvider(\'' + sectionKey + '\', ' + index + ')">Test</button>`,
		`renderInput(fieldKey, fieldDef, fieldValue, basePath + '.' + index, sectionKey)`,
		`function refreshInlineSectionSaveState(basePath)`,
		`refreshInlineSectionSaveState(basePath || fieldKey);`,
		`removeProviderItem(\'' + sectionKey + '\', ' + index + ', event)`,
		`addProviderItem(\'' + sectionKey + '\', event)`,
		`saveSection(\'' + sectionKey + '\', event)`,
		`provider-type-badge debrid`,
		`provider-type-badge torrent`,
		`provider-type-badge direct`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing provider-table contract marker %q", marker)
		}
	}

	for _, sectionKey := range []string{"debridProviders", "torrentScrapers"} {
		if strings.Contains(source, `reorderArrayItem('`+sectionKey+`'`) {
			t.Fatalf("%s table must preserve stable credential-bearing array indices", sectionKey)
		}
	}
}

func TestAdminSettingsAccordionUsesVisibleKeyboardFocus(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`#settingsContainer .section-header:focus-visible {`,
		`outline: 2px solid var(--accent-hover);`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing accordion focus marker %q", marker)
		}
	}
}

func TestAdminMobileNavigationTrapsAndReturnsKeyboardFocus(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/base.html")
	if err != nil {
		t.Fatalf("read base template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`function mobileMenuFocusableElements()`,
		`sidebar.inert = isMobile && !isOpen;`,
		`document.body.style.overflow = shouldOpen ? 'hidden' : mobileMenuPreviousBodyOverflow;`,
		`if (event.key === 'Escape')`,
		`if (event.key !== 'Tab') return;`,
		`(event.shiftKey ? last : first).focus();`,
		`window.requestAnimationFrame(() => (returnFocus || menuButton).focus());`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("base template missing mobile-navigation accessibility marker %q", marker)
		}
	}
}

func TestSharedShellRendersOnlyRegisteredRoleLinksAndContextualLabels(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	handler := NewAdminUIHandler(settingsPath, "", nil, nil, nil, config.NewManager(settingsPath))
	if handler.statusTemplate == nil {
		t.Fatal("status template failed to load")
	}

	render := func(t *testing.T, data AdminPageData) string {
		t.Helper()
		var output bytes.Buffer
		if err := handler.statusTemplate.ExecuteTemplate(&output, "base", data); err != nil {
			t.Fatalf("render shared shell: %v", err)
		}
		return output.String()
	}

	accountHTML := render(t, AdminPageData{
		CurrentPath: "/account/status",
		BasePath:    "/account",
		Version:     "1.2.3-test",
		BuildID:     "qa-build",
	})
	for _, deadLink := range []string{`href="/account/search"`, `href="/account/prequeue"`} {
		if strings.Contains(accountHTML, deadLink) {
			t.Fatalf("regular-account shell rendered unregistered link %q", deadLink)
		}
	}
	if !strings.Contains(accountHTML, `aria-label="Account navigation"`) {
		t.Fatal("regular-account shell is missing its contextual navigation label")
	}

	adminHTML := render(t, AdminPageData{
		CurrentPath: "/admin/status",
		BasePath:    "/admin",
		IsAdmin:     true,
		Version:     "1.2.3-test",
		BuildID:     "qa-build",
	})
	for _, registeredLink := range []string{`href="/admin/search"`, `href="/admin/prequeue"`} {
		if !strings.Contains(adminHTML, registeredLink) {
			t.Fatalf("admin shell is missing registered link %q", registeredLink)
		}
	}
	if !strings.Contains(adminHTML, `aria-label="Admin navigation"`) {
		t.Fatal("admin shell is missing its contextual navigation label")
	}
	for _, removedShortcut := range []string{`href="/admin/kids-settings"`, `aria-label="Open user management"`, `href="/admin/notifications"`, `aria-label="Notifications"`} {
		if strings.Contains(adminHTML, removedShortcut) {
			t.Fatalf("admin shell still renders removed shortcut %q", removedShortcut)
		}
	}
}

func TestSharedShellUsesOneConsistentNavigationIconSystem(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/base.html")
	if err != nil {
		t.Fatalf("read base template: %v", err)
	}
	source := string(templateBytes)

	for _, tag := range sidebarNavigationIconTags(source) {
		if tag != "svg" {
			t.Fatalf("shared shell uses %q for a sidebar navigation icon, want svg", tag)
		}
	}
	if tags := sidebarNavigationIconTags(`<span aria-hidden="true" class="sidebar-nav-icon">⌂</span><span class="utility sidebar-nav-icon">⌂</span>`); len(tags) != 2 || tags[0] != "span" || tags[1] != "span" {
		t.Fatalf("sidebar icon matcher missed attribute/class variants: %#v", tags)
	}
	for _, marker := range []string{
		`.sidebar-nav-icon {`,
		`width: 20px;`,
		`height: 20px;`,
		`stroke: currentColor;`,
		`stroke-width: 1.8;`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("shared shell missing consistent navigation icon marker %q", marker)
		}
	}
}

var htmlStartTagPattern = regexp.MustCompile(`(?is)<([a-z][a-z0-9:-]*)\b([^>]*)>`)
var classAttributePattern = regexp.MustCompile(`(?is)\bclass\s*=\s*(?:"([^"]*)"|'([^']*)')`)

func sidebarNavigationIconTags(source string) []string {
	tags := make([]string, 0)
	for _, match := range htmlStartTagPattern.FindAllStringSubmatch(source, -1) {
		classMatch := classAttributePattern.FindStringSubmatch(match[2])
		if len(classMatch) == 0 {
			continue
		}
		classValue := classMatch[1]
		if classValue == "" {
			classValue = classMatch[2]
		}
		for _, className := range strings.Fields(classValue) {
			if className == "sidebar-nav-icon" {
				tags = append(tags, strings.ToLower(match[1]))
				break
			}
		}
	}
	return tags
}

func TestSharedShellUsesTruthfulApplicationInformation(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/base.html")
	if err != nil {
		t.Fatalf("read base template: %v", err)
	}
	source := string(templateBytes)

	for _, unsupportedClaim := range []string{"Connected", "Server OK"} {
		if strings.Contains(source, unsupportedClaim) {
			t.Fatalf("shared shell contains unsupported runtime claim %q", unsupportedClaim)
		}
	}
	for _, marker := range []string{
		`<div class="admin-sidebar-version">v{{.Version}}</div>`,
		`<footer class="admin-statusbar" aria-label="Application information">`,
		`<span class="admin-statusbar-state">Mediastorm</span>`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("shared shell is missing truthful application marker %q", marker)
		}
	}
}

func TestSharedShellSupportsPlaybackTheaterMode(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/base.html")
	if err != nil {
		t.Fatalf("read base template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`body.web-player-theater-active .admin-sidebar,`,
		`body.web-player-theater-active .admin-topbar,`,
		`body.web-player-theater-active .admin-statusbar,`,
		`body.web-player-theater-active .sidebar-scrim {`,
		`body.web-player-theater-active .admin-workspace {`,
		`width: 100%;`,
		`margin-left: 0;`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("shared shell is missing theater-mode compatibility marker %q", marker)
		}
	}
}

func TestAdminToolsProvidesFocusedTasksAndIntegrationsViews(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatalf("read tools template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`id="tasksPageHost"`,
		`id="integrationsPageHost"`,
		`href="https://trakt.tv/"`,
		`href="https://mdblist.com/"`,
		`href="https://simkl.com/"`,
		`href="https://scrob.app/"`,
		`id="taskProfileFilter"`,
		`const isTasksPage =`,
		`const isIntegrationsPage =`,
		`function applyTaskFilters()`,
		`requestedTaskProfileId`,
		`name="mediastorm-task-filter"`,
		`class="import-card task-card"`,
		`class="task-schedule-label">Frequency`,
		`class="task-schedule-label">Next run`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("tools template missing focused-view marker %q", marker)
		}
	}
}

func TestAdminMaintenanceLinksAllSubpages(t *testing.T) {
	toolsBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatalf("read tools template: %v", err)
	}
	toolsSource := string(toolsBytes)

	maintenancePages := map[string]string{
		"hidden items":  "tools/hidden-items",
		"bad streams":   "tools/bad-streams",
		"resolved NZBs": "tools/resolved-nzbs",
		"share links":   "tools/share-links",
		"prequeues":     "prequeue",
	}
	for name, path := range maintenancePages {
		link := `href="{{.BasePath}}/` + path + `"`
		if !strings.Contains(toolsSource, link) {
			t.Errorf("maintenance page missing link to %s (%s)", name, path)
		}
	}

	for _, templateName := range []string{
		"hidden_items.html",
		"bad_streams.html",
		"resolved_nzbs.html",
		"share_links.html",
		"prequeue.html",
	} {
		templateBytes, readErr := adminTemplates.ReadFile("admin_templates/" + templateName)
		if readErr != nil {
			t.Errorf("read %s: %v", templateName, readErr)
			continue
		}
		if !strings.Contains(string(templateBytes), `href="{{.BasePath}}/tools"`) {
			t.Errorf("%s missing link back to maintenance", templateName)
		}
	}

	if strings.Contains(toolsSource, `id="prequeueManagementSection" style="display: none;"`) ||
		strings.Contains(toolsSource, "function updatePrequeueManagementSection()") {
		t.Fatal("prequeue management link remains conditional on an enabled prewarm automation")
	}
}

func TestDatabaseSnapshotUploadKeepsShareLinkVisible(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatalf("read tools template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`const status = document.getElementById('troubleshooting-upload-status');`,
		`status.textContent = 'Uploading de-identified database snapshot...'`,
		`status.innerHTML = ` + "`Shared database snapshot: <a",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("database snapshot upload missing persistent status marker %q", marker)
		}
	}
}

func TestClearDatabaseDataRequiresExactConfirmation(t *testing.T) {
	maintenance := &fakeDatabaseMaintenance{}
	handler := &AdminUIHandler{databaseMaintenance: maintenance}
	body, err := json.Marshal(clearDatabaseDataRequest{Dataset: "watch_history", Confirmation: "delete watch history"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/api/database/clear", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ClearDatabaseData(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if maintenance.watchHistoryCalls != 0 {
		t.Fatal("watch history was cleared despite mismatched confirmation")
	}
}

func TestClearDatabaseDataDispatchesSupportedDatasets(t *testing.T) {
	tests := []struct {
		dataset      string
		confirmation string
		wantDeleted  int
	}{
		{dataset: "watch_history", confirmation: "DELETE WATCH HISTORY", wantDeleted: 12},
		{dataset: "playback_progress", confirmation: "DELETE PLAYBACK PROGRESS", wantDeleted: 7},
		{dataset: "watchlists", confirmation: "DELETE WATCHLISTS", wantDeleted: 3},
	}
	for _, tt := range tests {
		t.Run(tt.dataset, func(t *testing.T) {
			maintenance := &fakeDatabaseMaintenance{}
			handler := &AdminUIHandler{databaseMaintenance: maintenance}
			body, err := json.Marshal(clearDatabaseDataRequest{Dataset: tt.dataset, Confirmation: tt.confirmation})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/api/database/clear", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ClearDatabaseData(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var response struct {
				Deleted int `json:"deleted"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Deleted != tt.wantDeleted {
				t.Fatalf("deleted = %d, want %d", response.Deleted, tt.wantDeleted)
			}
		})
	}
}

func TestDatabaseDeletionTemplateIncludesWarningsAndTypedConfirmations(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatalf("read tools template: %v", err)
	}
	source := string(templateBytes)
	for _, marker := range []string{
		`{{if .IsAdmin}}`, `id="databaseDataSection"`, `value="watch_history"`,
		`value="playback_progress"`, `value="watchlists"`, `DELETE WATCH HISTORY`,
		`DELETE PLAYBACK PROGRESS`, `DELETE WATCHLISTS`, `These actions cannot be undone`,
	} {
		if !strings.Contains(source, marker) {
			t.Errorf("tools template missing database deletion safeguard %q", marker)
		}
	}
}

func TestAdminDashboardBasicViewKeepsOnlyUserActivityCards(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := strings.ReplaceAll(string(templateBytes), "\r\n", "\n")

	for _, marker := range []string{
		"<!-- Active Streams -->\n<div class=\"card\"",
		"<!-- Usenet Activity -->\n<div class=\"card dashboard-advanced-detail\"",
		`<div class="card live-limits-card dashboard-advanced-detail"`,
		`<div class="grid grid-2 dashboard-advanced-detail"`,
		`document.querySelectorAll('.dashboard-advanced-detail')`,
		`class="settings-level-switch" aria-label="Dashboard detail level"`,
		`id="dashboardBasicBtn" class="settings-level-btn"`,
		`.dashboard-toolbar .settings-level-switch {`,
		`width: max-content`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing basic-dashboard marker %q", marker)
		}
	}
}

func TestAdminDashboardUpdateNoticeUsesCompactVersionFields(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`class="dashboard-update-versions"`,
		`id="dashboardUpdateCurrent"`,
		`id="dashboardUpdateLatest"`,
		`class="dashboard-update-instruction">Update through Docker.`,
		`current.textContent = currentLabel;`,
		`latest.textContent = latestLabel;`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing compact update marker %q", marker)
		}
	}
	if strings.Contains(source, `message.textContent = `) {
		t.Fatal("dashboard update notice still builds an unstructured sentence")
	}
}

func TestAdminDashboardStylesOnlyActiveStreamScrollbar(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`class="table-container active-streams-scroll"`,
		`.active-streams-scroll {`,
		`scrollbar-color: var(--border) transparent;`,
		`.active-streams-scroll::-webkit-scrollbar-thumb`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing scoped active-stream scrollbar marker %q", marker)
		}
	}
}

func TestAdminSettingsScopeOptionsKeepDarkThemeContrast(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`.settings-scope-select-wrap option {`,
		`background: var(--bg-secondary);`,
		`color: var(--text-primary);`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing scope-option contrast marker %q", marker)
		}
	}
}

func TestAdminSettingsShowWhenSupportsAndConditions(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)
	if count := strings.Count(source, "showWhen.operator === 'and'"); count != 4 {
		t.Fatalf("settings template AND showWhen evaluators = %d, want 4", count)
	}
}

func TestAdminDashboardWatchTimeStacksOnNarrowViewports(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := strings.ReplaceAll(string(templateBytes), "\r\n", "\n")

	if strings.Contains(source, `.dashboard-v4 .watch-time-grid {`) {
		t.Fatal("dashboard-specific watch-time grid selector overrides the narrower mobile layout")
	}
	if !strings.Contains(source, "@media (max-width: 640px) {\n        .watch-time-grid {\n            grid-template-columns: 1fr;") {
		t.Fatal("dashboard watch-time grid is missing its single-column narrow-viewport layout")
	}
}

func TestAdminDashboardActiveStreamSummaryDoesNotDependOnTransferredBytes(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`if (streams.length > 0 && totalBandwidth > 0)`,
		`} else if (streams.length > 0) {`,
		`streams.length === 1 ? '1 active stream' : streams.length + ' active streams'`,
		`subtextEl.textContent = 'No active streams';`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing active-stream summary marker %q", marker)
		}
	}
}

func TestAdminDashboardWatchTimeNormalizesRoundedMinutes(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`const totalMinutes = Math.max(1, Math.round(seconds / 60));`,
		`const hours = Math.floor(totalMinutes / 60);`,
		`const mins = totalMinutes % 60;`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing normalized watch-time marker %q", marker)
		}
	}
	if strings.Contains(source, `Math.round((seconds % 3600) / 60)`) {
		t.Fatal("watch-time formatter can still render 60 leftover minutes")
	}
}

func TestAdminAccountsSurfacesProfileTaskContext(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/accounts.html")
	if err != nil {
		t.Fatalf("read accounts template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`id="tab-tasks"`,
		`id="content-tasks"`,
		`fetch(basePath + '/api/scheduled-tasks')`,
		`function renderProfileTasksSummary()`,
		`/tasks?profileId=`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("accounts template missing task-context marker %q", marker)
		}
	}
}

func TestRegularAccountToolsExposeAutomationsAndAllIntegrations(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/tools.html")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ReplaceAll(string(templateBytes), "\r\n", "\n")
	for _, marker := range []string{
		"<!-- AUTOMATION CATEGORY -->\n<div class=\"settings-group\">",
		`id="scheduledTasksSection"`,
		`id="simklAccountsList"`,
		`id="scrobAccountsList"`,
		`id="mdblistAccountsList"`,
		`id="jellyfinAccountsSection"`,
		`[loadPlexAccounts(), loadTraktAccounts(), loadMdblistAccounts(), loadSimklAccounts(), loadScrobAccounts(), loadJellyfinAccounts()]`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("tools template missing regular-account marker %q", marker)
		}
	}
}

func TestOwnedIntegrationAccessSupportsOwnersAndLegacyProfileLinks(t *testing.T) {
	handler := &AdminUIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/account/integrations", nil)
	req = req.WithContext(context.WithValue(req.Context(), adminSessionContextKey{}, &models.Session{
		AccountID: "acct-1",
		IsMaster:  false,
	}))
	if !handler.canAccessOwnedIntegration(req, "acct-1", nil) {
		t.Fatal("regular account could not access its owned integration")
	}
	if handler.canAccessOwnedIntegration(req, "acct-2", nil) {
		t.Fatal("regular account accessed another account's integration")
	}
	if !handler.canAccessOwnedIntegration(req, "", []models.User{{AccountID: "acct-1"}}) {
		t.Fatal("regular account could not access a linked legacy integration")
	}
	if handler.canAccessOwnedIntegration(req, "", []models.User{{AccountID: "acct-2"}}) {
		t.Fatal("regular account accessed an unowned legacy integration")
	}
}

func TestAdminSettingsSharedActivityShelvesExposeAssociatedSettings(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`editSharedActivityShelf(\''+s.id+'\')`,
		`id="sharedShelfWindowDays"`,
		`id="sharedShelfMinProfiles"`,
		`id="sharedShelfPerProfileCap"`,
		`shelf.activityWindowDays`,
		`shelf.minimumProfiles`,
		`shelf.maxItemsPerProfile`,
		`Minimum Views`,
		`completed movie or episode views`,
		`saveSharedActivityShelf()`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("settings template missing shared activity shelf setting marker %q", marker)
		}
	}
}

func TestProfileActivityPrivacyCopyIncludesDashboardShelf(t *testing.T) {
	adminBytes, err := adminTemplates.ReadFile("admin_templates/accounts.html")
	if err != nil {
		t.Fatalf("read admin accounts template: %v", err)
	}
	accountBytes, err := accountTemplatesFS.ReadFile("account_templates/dashboard.html")
	if err != nil {
		t.Fatalf("read account dashboard template: %v", err)
	}

	for name, source := range map[string]string{
		"admin":   string(adminBytes),
		"account": string(accountBytes),
	} {
		for _, marker := range []string{
			"Server Activity Sharing",
			"Recently Watched, and the active Dashboard shelf",
			">Do not share</option>",
		} {
			if !strings.Contains(source, marker) {
				t.Fatalf("%s profile template missing activity privacy marker %q", name, marker)
			}
		}
	}
}

func TestAdminAccountPasswordChangeRedirectsAfterCurrentSessionRevoked(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/accounts.html")
	if err != nil {
		t.Fatalf("read admin accounts template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		"async function changePassword(e, targetAccountId)",
		"if (targetAccountId === accountId)",
		"window.location.href = serverBasePath + '/admin/login'",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("admin password change is missing revoked-session handling marker %q", marker)
		}
	}
}

func TestAdminStatusActiveStreamsPreferSeriesPosters(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)

	if strings.Contains(source, "title.poster?.url || title.backdrop?.url") {
		t.Fatal("active-stream poster lookup still falls back to landscape backdrop artwork")
	}

	loadStart := strings.Index(source, "async function loadStreamPosters(streams)")
	if loadStart < 0 {
		t.Fatal("status template missing loadStreamPosters")
	}
	loadSource := source[loadStart:]
	seriesLookup := strings.Index(loadSource, "mediaInfo.type === 'series'")
	streamArtwork := strings.Index(loadSource, "if (mediaInfo.posterUrl)")
	if seriesLookup < 0 || streamArtwork < 0 || seriesLookup > streamArtwork {
		t.Fatal("episode cards must resolve the canonical series poster before using stream artwork")
	}
}

func TestAdminStatusActiveStreamRowsKeepMediaOnOneLineAndShowService(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)

	for _, marker := range []string{
		`<th>Media</th><th>Service</th>`,
		`class="stream-table-media-subtitle"`,
		`renderStreamServiceBadge(stream, true)`,
		`function getStreamServiceType(stream)`,
		`function getStreamDebridProvider(stream)`,
		`class="stream-debrid-provider"`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing active-stream row marker %q", marker)
		}
	}
}

func TestAdminStatusActiveStreamsShowDeviceAndCompactEpisodeLabel(t *testing.T) {
	templateBytes, err := adminTemplates.ReadFile("admin_templates/status.html")
	if err != nil {
		t.Fatalf("read status template: %v", err)
	}
	source := string(templateBytes)
	for _, marker := range []string{
		`function getDeviceDisplay(stream)`,
		`class="stream-card-device"`,
		`class="stream-card-profile-name"`,
		`class="stream-table-profile"`,
		`class="stream-table-device"`,
		`const episodeCode = `,
		`[stream.year ? String(stream.year) : '', episodeCode]`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("status template missing device/episode marker %q", marker)
		}
	}
	if strings.Contains(source, `S${stream.season_number}E${stream.episode_number} - ${stream.episode_name}`) {
		t.Fatal("episode display still includes the episode name")
	}
}

func TestAddDashboardDeviceInfoPrefersNickname(t *testing.T) {
	stream := map[string]interface{}{}
	addDashboardDeviceInfo(stream, "client-1", map[string]models.Client{
		"client-1": {
			ID:         "client-1",
			Nickname:   "Living Room",
			Name:       "Admin name",
			DeviceName: "Liam's iPhone",
			DeviceType: "iPhone",
			OS:         "iOS",
		},
	})

	if got := stream["device_name"]; got != "Living Room" {
		t.Fatalf("device_name = %v, want nickname", got)
	}
	if got := stream["device_nickname"]; got != "Living Room" {
		t.Fatalf("device_nickname = %v, want nickname", got)
	}
	if got := stream["device_type"]; got != "iPhone" {
		t.Fatalf("device_type = %v, want iPhone", got)
	}
	if got := stream["client_id"]; got != "client-1" {
		t.Fatalf("client_id = %v, want client-1", got)
	}
}

func TestDashboardStreamServiceType(t *testing.T) {
	tests := []struct {
		name        string
		live        bool
		serviceType string
		paths       []string
		wanted      string
	}{
		{name: "live TV", live: true, serviceType: "debrid", paths: []string{"https://provider.test/channel.ts"}, wanted: "stream"},
		{name: "explicit debrid HTTP URL", serviceType: "debrid", paths: []string{"https://comet.example/playback/token"}, wanted: "debrid"},
		{name: "explicit usenet HTTP URL", serviceType: "usenet", paths: []string{"https://webdav.example/movie.mkv"}, wanted: "usenet"},
		{name: "explicit local source", serviceType: "local", paths: []string{"/library/movie.mkv"}, wanted: "local"},
		{name: "debrid path", paths: []string{"/debrid/realdebrid/torrent/file/0/movie.mkv"}, wanted: "debrid"},
		{name: "webdav debrid path", paths: []string{"/webdav/debrid/torbox/torrent/file/0/movie.mkv"}, wanted: "debrid"},
		{name: "original debrid path", paths: []string{"https://cdn.test/file", "/debrid/realdebrid/torrent/file/0/movie.mkv"}, wanted: "debrid"},
		{name: "legacy HTTP URL", paths: []string{"https://comet.example/playback/token"}, wanted: "usenet"},
		{name: "usenet path", paths: []string{"/nzbs/job/movie.mkv"}, wanted: "usenet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dashboardStreamServiceType(tt.live, tt.serviceType, tt.paths...); got != tt.wanted {
				t.Fatalf("dashboardStreamServiceType() = %q, want %q", got, tt.wanted)
			}
		})
	}
}

func TestDashboardDebridProvider(t *testing.T) {
	tests := []struct {
		name        string
		serviceType string
		provider    string
		paths       []string
		wanted      string
	}{
		{name: "explicit provider on signed URL", serviceType: "debrid", provider: "Real-Debrid", paths: []string{"https://comet.example/playback/token"}, wanted: "realdebrid"},
		{name: "torbox path", paths: []string{"/debrid/torbox/torrent/file/0/movie.mkv"}, wanted: "torbox"},
		{name: "real debrid webdav path", paths: []string{"/webdav/debrid/real-debrid/torrent/file/0/movie.mkv"}, wanted: "realdebrid"},
		{name: "provider in original path", serviceType: "debrid", paths: []string{"https://cdn.test/file", "/debrid/premiumize/torrent/file/0/movie.mkv"}, wanted: "premiumize"},
		{name: "signed external URL without provider", serviceType: "debrid", paths: []string{"https://comet.example/playback/token"}, wanted: ""},
		{name: "usenet ignores explicit provider", serviceType: "usenet", provider: "torbox", paths: []string{"/nzbs/job/movie.mkv"}, wanted: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dashboardDebridProvider(tt.serviceType, tt.provider, tt.paths...); got != tt.wanted {
				t.Fatalf("dashboardDebridProvider() = %q, want %q", got, tt.wanted)
			}
		})
	}
}

func TestUsenetEngineStatusProbeJobIDUsesGUIDForNZBDav(t *testing.T) {
	for _, engineType := range []string{"nzbdav", "nzbdavex"} {
		t.Run(engineType, func(t *testing.T) {
			got := usenetEngineStatusProbeJobID(config.UsenetEngineSettings{Type: engineType})
			if got != "00000000-0000-4000-8000-000000000000" {
				t.Fatalf("probe job id = %q, want GUID-shaped placeholder", got)
			}
		})
	}

	got := usenetEngineStatusProbeJobID(config.UsenetEngineSettings{Type: "altmount"})
	if !strings.HasPrefix(got, "strmr-connection-test-") {
		t.Fatalf("altmount probe job id = %q, want legacy prefix", got)
	}
}

func TestExplainUsenetEngineRemoteConfigMismatchDetectsDecypharrCustomFolder(t *testing.T) {
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Fatalf("method = %s, want PROPFIND", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/webdav/mediastorm/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/></d:resourcetype>
        <d:displayname>mediastorm</d:displayname>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
	}))
	defer webdav.Close()

	message, err := explainUsenetEngineRemoteConfigMismatch(context.Background(), config.UsenetEngineSettings{
		Type:          "decypharr",
		WebDAVBaseURL: webdav.URL,
		Category:      "mediastorm",
	})
	if err != nil {
		t.Fatalf("explainUsenetEngineRemoteConfigMismatch: %v", err)
	}
	if !strings.Contains(message, "custom folder") || !strings.Contains(message, "Category will still be sent") {
		t.Fatalf("message = %q", message)
	}
}

func TestInferAdminWebDAVPathPrefixFromRootFolder(t *testing.T) {
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Fatalf("method = %s, want PROPFIND", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/webdav/":
			w.WriteHeader(http.StatusMultiStatus)
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/webdav/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/webdav/mediastorm/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><d:displayname>mediastorm</d:displayname></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
</d:multistatus>`))
		case "/webdav/mediastorm/strmr-connection-test-1":
			w.WriteHeader(http.StatusMultiStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	defer webdav.Close()

	prefix, mappedURL, ok := inferAdminWebDAVPathPrefix(context.Background(), config.UsenetEngineSettings{
		Type:          "decypharr",
		WebDAVBaseURL: webdav.URL + "/webdav",
	}, "/mnt/debrid/decypharr_downloads/mediastorm/strmr-connection-test-1")
	if !ok {
		t.Fatal("expected prefix inference to succeed")
	}
	if prefix != "/mnt/debrid/decypharr_downloads" {
		t.Fatalf("prefix = %q, want /mnt/debrid/decypharr_downloads", prefix)
	}
	wantURL := webdav.URL + "/webdav/mediastorm/strmr-connection-test-1"
	if mappedURL != wantURL {
		t.Fatalf("mappedURL = %q, want %q", mappedURL, wantURL)
	}
}
