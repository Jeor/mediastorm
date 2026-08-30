# Home Designer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dedicated Home Designer settings page that composes existing home rows, previews real profile content and app-wide appearance live, and atomically applies global or profile-scoped changes.

**Architecture:** A focused `services/homedesigner` package owns the editor document, catalog, validation, revision, inheritance, and persistence orchestration. Cookie-authenticated admin/account handlers expose load, preview, and apply endpoints to a server-rendered page whose no-build ES modules maintain a local working copy and render responsive TV/mobile previews. Existing `HomeShelves`, `AppearanceSettings`, startup/display-list resolvers, and native-client delivery remain the source of truth.

**Tech Stack:** Go 1.26, `net/http`, Gorilla Mux, embedded Go templates/assets, PostgreSQL-or-JSON user settings service, browser ES modules, CSS, Node 22 built-in test runner.

**Spec:** `docs/superpowers/specs/2026-08-30-home-designer-design.md`

## Global Constraints

- Keep all work and commits in Jeor's local `mediastorm` repository; do not push to `origin` or any other remote.
- Do not add a JavaScript framework, package dependency, bundler, or generated asset step.
- Keep the existing `config.Settings.HomeShelves`, `models.UserSettings.HomeShelves`, and `AppearanceSettings` values as the only persisted configuration.
- Preserve the existing native startup/settings response contract and the advanced Home and Appearance forms.
- Use one shared row order for TV and mobile; platform selection changes rendering only.
- Treat Rows and Theme as independently inheritable profile sections.
- Require an explicit Apply; browser edits must not write before Apply succeeds.
- Keep preview responses presentation-safe: never return file paths, credentials, playback URLs, client addresses, or transport data.
- Every drag operation must have keyboard and explicit-button equivalents.

---

## File Structure

### Backend domain and persistence

- `backend/services/homedesigner/types.go` — editor request/response, scope, catalog, validation, and preview types.
- `backend/services/homedesigner/conversion.go` — lossless conversion between global `config` and profile-facing `models` shelf/appearance types.
- `backend/services/homedesigner/catalog.go` — canonical row catalog, defaults, multiplicity, field definitions, and capability evaluation.
- `backend/services/homedesigner/validation.go` — full-document row/theme validation and structured field errors.
- `backend/services/homedesigner/revision.go` — deterministic scope-section revision hashes.
- `backend/services/homedesigner/service.go` — authorized load/apply orchestration and independent Rows/Theme inheritance.
- Corresponding `*_test.go` files — focused unit tests per responsibility.
- `backend/config/settings.go` — serialized atomic mutation support for global settings.
- `backend/services/user_settings/service.go` — serialized per-profile mutation support that preserves unrelated overrides.

### HTTP and preview integration

- `backend/handlers/home_designer.go` — page, load, apply, and reset-aware API handlers.
- `backend/handlers/home_designer_preview.go` — preview request validation, display-list reuse, normalized projection, samples, and per-row isolation.
- `backend/handlers/home_designer_assets.go` — embedded allowlisted CSS/JavaScript asset serving.
- `backend/handlers/home_designer_test.go` and `home_designer_preview_test.go` — handler, permission, privacy, and resolver tests.
- `backend/handlers/admin_templates/home_designer.html` — semantic three-panel page shell.
- `backend/handlers/admin_templates/base.html` — Home Designer navigation entry and active state.
- `backend/main.go` — `/admin` and `/account` page/API/asset routes plus preview-provider wiring.

### Browser application

- `backend/handlers/admin_assets/home_designer/api.js` — base-path-aware fetch client and typed error normalization.
- `backend/handlers/admin_assets/home_designer/store.js` — working-copy reducer, undo/redo, dirty state, inheritance, and apply payload construction.
- `backend/handlers/admin_assets/home_designer/library.js` — catalog filtering and add/drag affordances.
- `backend/handlers/admin_assets/home_designer/outline.js` — synchronized order, visibility, selection, keyboard moves, and row inspector.
- `backend/handlers/admin_assets/home_designer/theme.js` — theme presets, fields, inheritance, and preview CSS variables.
- `backend/handlers/admin_assets/home_designer/preview.js` — request debounce/cancellation/cache and TV/mobile renderers.
- `backend/handlers/admin_assets/home_designer/app.js` — bootstrap, event wiring, status announcements, apply/discard, and unload protection.
- `backend/handlers/admin_assets/home_designer/home_designer.css` — three-panel workspace, mock device layouts, responsive drawers, contrast, and reduced motion.
- `backend/handlers/admin_assets/home_designer/store_test.mjs` and `preview_test.mjs` — no-dependency Node tests for pure browser state and stale-response behavior.

---

### Task 1: Add atomic mutation primitives to existing stores

**Files:**
- Modify: `backend/config/settings.go`
- Modify: `backend/config/settings_test.go`
- Modify: `backend/services/user_settings/service.go`
- Modify: `backend/services/user_settings/service_test.go`

**Interfaces:**
- Produces: `func (m *config.Manager) Mutate(func(*config.Settings) error) error`
- Produces: `func (s *user_settings.Service) Mutate(userID string, fn func(*models.UserSettings) error) error`
- Guarantees: callback read/check/write is serialized against existing `Save`/`Update` calls on the same service instance.

- [ ] **Step 1: Write failing global-mutation tests**

Add tests proving a mutation loads the current document, persists only callback changes, aborts on callback error, and serializes a concurrent `Save`:

```go
func TestManagerMutatePersistsCallbackChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mgr := NewManager(path)
	if err := mgr.Save(DefaultSettings()); err != nil { t.Fatal(err) }
	if err := mgr.Mutate(func(s *Settings) error {
		s.HomeShelves.Shelves[0].Name = "Pinned first"
		return nil
	}); err != nil { t.Fatal(err) }
	got, err := mgr.Load()
	if err != nil { t.Fatal(err) }
	if got.HomeShelves.Shelves[0].Name != "Pinned first" { t.Fatalf("name = %q", got.HomeShelves.Shelves[0].Name) }
}
```

- [ ] **Step 2: Run the config tests and verify the new API is missing**

Run: `cd backend && go test ./config -run 'TestManagerMutate' -count=1`

Expected: compile failure because `Manager.Mutate` is undefined.

- [ ] **Step 3: Implement locked load/save internals and `Manager.Mutate`**

Add `mu sync.Mutex` to `Manager`; make public `Load` and `Save` acquire it and delegate to `loadUnlocked`/`saveUnlocked`. Implement:

```go
func (m *Manager) Mutate(fn func(*Settings) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	settings, err := m.loadUnlocked()
	if err != nil { return err }
	if err := fn(&settings); err != nil { return err }
	return m.saveUnlocked(settings)
}
```

Ensure the missing-file path calls `saveUnlocked`, not public `Save`, to avoid self-deadlock.

- [ ] **Step 4: Write failing profile-mutation tests**

Cover preservation of unrelated settings and rollback on callback error:

```go
func TestMutatePreservesUnrelatedProfileSettings(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil { t.Fatal(err) }
	if err := svc.Update("p1", models.UserSettings{Playback: models.PlaybackSettings{PreferredPlayer: "vlc"}}); err != nil { t.Fatal(err) }
	if err := svc.Mutate("p1", func(s *models.UserSettings) error {
		s.HomeShelves.Shelves = []models.ShelfConfig{{ID: "watchlist", Name: "My List", Enabled: true}}
		return nil
	}); err != nil { t.Fatal(err) }
	got, _ := svc.Get("p1")
	if got.Playback.PreferredPlayer != "vlc" { t.Fatalf("preferred player = %q", got.Playback.PreferredPlayer) }
}
```

- [ ] **Step 5: Implement `Service.Mutate` under the existing mutex**

Validate `userID`, copy the current raw value, invoke the callback, reuse the same sanitization and emptiness rules as `Update`, and call `saveLocked` exactly once. Extract a private `normalizeForStorage(models.UserSettings) models.UserSettings` so `Update` and `Mutate` cannot diverge.

- [ ] **Step 6: Run focused and package tests**

Run: `cd backend && go test ./config ./services/user_settings -count=1`

Expected: PASS.

- [ ] **Step 7: Commit locally**

```bash
git add backend/config/settings.go backend/config/settings_test.go backend/services/user_settings/service.go backend/services/user_settings/service_test.go
git commit -m "feat(settings): add atomic mutation APIs"
```

### Task 2: Define the Home Designer contract, conversions, catalog, and validation

**Files:**
- Create: `backend/services/homedesigner/types.go`
- Create: `backend/services/homedesigner/conversion.go`
- Create: `backend/services/homedesigner/conversion_test.go`
- Create: `backend/services/homedesigner/catalog.go`
- Create: `backend/services/homedesigner/catalog_test.go`
- Create: `backend/services/homedesigner/validation.go`
- Create: `backend/services/homedesigner/validation_test.go`
- Create: `backend/services/homedesigner/revision.go`
- Create: `backend/services/homedesigner/revision_test.go`
- Modify: `backend/handlers/user_settings.go`
- Modify: `backend/handlers/user_settings_test.go`

**Interfaces:**
- Produces: `Scope`, `Actor`, `Document`, `RowsSection`, `ThemeSection`, `CatalogEntry`, `ApplyRequest`, `PreviewRequest`, and `PreviewResponse` JSON contracts.
- Produces: `ConfigShelvesToModels`, `ModelShelvesToConfig`, `ConfigAppearanceToModel`, and `ModelAppearanceToConfig` lossless conversions.
- Produces: `BuildCatalog(config.Settings, []models.User) []CatalogEntry`.
- Produces: `ValidateApply(ApplyRequest, []CatalogEntry) []FieldError`.
- Produces: `RevisionForGlobal(config.Settings) string` and `RevisionForProfile(*models.UserSettings) string`.

- [ ] **Step 1: Add contract types with stable JSON field names**

Use concrete section types rather than generic JSON so handlers and browser code share an explicit shape:

```go
type Scope struct { Kind string `json:"kind"`; ProfileID string `json:"profileId,omitempty"` }
type Actor struct { IsAdmin bool; AccountID string }
type RowsSection struct { Inherited bool `json:"inherited"`; Effective models.HomeShelvesSettings `json:"effective"`; Override *models.HomeShelvesSettings `json:"override,omitempty"` }
type ThemeSection struct { Inherited bool `json:"inherited"`; Effective models.AppearanceSettings `json:"effective"`; Override *models.AppearanceSettings `json:"override,omitempty"` }
type SectionMutation[T any] struct { Mode string `json:"mode"`; Value *T `json:"value,omitempty"` }
type ApplyRequest struct { Scope Scope `json:"scope"`; ExpectedRevision string `json:"expectedRevision"`; Rows *SectionMutation[models.HomeShelvesSettings] `json:"rows,omitempty"`; Theme *SectionMutation[models.AppearanceSettings] `json:"theme,omitempty"` }
```

Define mode constants `custom` and `inherit`; global requests accept only `custom`.

- [ ] **Step 2: Write round-trip conversion tests for every shelf field**

Build a `config.ShelfConfig` containing Stremio, TMDB, streaming-service, collection-hub, Trakt, Simkl, Letterboxd, calendar, limits, and display fields. Assert `ModelShelvesToConfig(ConfigShelvesToModels(input))` is deeply equal to the input.

- [ ] **Step 3: Implement lossless conversions and route the legacy helper through them**

Move all field mapping into `services/homedesigner/conversion.go`. Change `handlers.convertShelves` to return `homedesigner.ConfigShelvesToModels(configShelves)`. This fixes the current omission of Stremio fields from the legacy conversion path and prevents Home Designer from creating another mapper.

- [ ] **Step 4: Write catalog tests**

Assert the catalog contains unique built-ins plus repeatable `genre`, `decade`, `streaming-service`, `mdblist`, `stremio`, `tmdb`, `trakt`, `simkl`, `letterboxd`, `collection-hub`, and `library` templates. Assert missing Trakt/TMDB/library capabilities produce `available:false`, a reason, and a settings link.

- [ ] **Step 5: Implement the canonical catalog**

Represent configurable fields explicitly:

```go
type CatalogField struct { Path, Type, Label string; Required bool; Options []Option }
type CatalogEntry struct { Type, Name, Description, Category string; Multiple, Available bool; UnavailableReason, SetupPath string; Default models.ShelfConfig; Fields []CatalogField; PreviewKind string }
```

Generate profile-aware account options without exposing credentials. Keep stable types separate from generated instance IDs.

- [ ] **Step 6: Write validation tests**

Cover duplicate unique rows, repeated genre rows, unknown types, blank names, missing required integration fields, invalid limits, non-contiguous order normalization, color format, font scale, button style/radius, and invalid global inheritance.

- [ ] **Step 7: Implement normalization and structured validation**

Return `FieldError{Section, RowID, Path, Message}` values. Normalize order to `0..n-1`, trim identifiers and names, clamp only fields whose existing settings code already clamps, and reject all other invalid values instead of guessing.

- [ ] **Step 8: Implement deterministic revisions and tests**

JSON-marshal only the persisted Rows and Theme fields for the selected scope, hash with SHA-256, and hex-encode. Verify unrelated playback changes do not alter the Home Designer revision while row or appearance changes do.

- [ ] **Step 9: Run focused tests**

Run: `cd backend && go test ./services/homedesigner ./handlers -run 'ShelfConversion|HomeDesignerCatalog|ValidateHomeDesigner|HomeDesignerRevision' -count=1`

Expected: PASS.

- [ ] **Step 10: Commit locally**

```bash
git add backend/services/homedesigner backend/handlers/user_settings.go backend/handlers/user_settings_test.go
git commit -m "feat(home-designer): define editor contract and catalog"
```

### Task 3: Implement authorized load, inheritance, revisions, and atomic apply

**Files:**
- Create: `backend/services/homedesigner/service.go`
- Create: `backend/services/homedesigner/service_test.go`

**Interfaces:**
- Consumes: Task 1 atomic `Mutate` APIs and Task 2 contract/conversion/validation/revision functions.
- Produces: `New(*config.Manager, *user_settings.Service, UserDirectory) *Service`.
- Produces: `Load(ctx context.Context, actor Actor, scope Scope) (Document, error)`.
- Produces: `Apply(ctx context.Context, actor Actor, request ApplyRequest) (Document, error)`.
- Produces sentinel errors: `ErrForbidden`, `ErrProfileNotFound`, `ErrRevisionConflict`, and `ValidationError`.

- [ ] **Step 1: Write authorization and load tests**

Cover administrator global access, non-admin global denial, administrator profile access, owner profile access, unowned profile returning not-found, and separate Rows/Theme inherited flags from raw profile settings.

- [ ] **Step 2: Run tests and verify `Service` is missing**

Run: `cd backend && go test ./services/homedesigner -run 'TestServiceLoad' -count=1`

Expected: compile failure because `Service` is undefined.

- [ ] **Step 3: Implement `UserDirectory` and `Load`**

```go
type UserDirectory interface {
	Get(string) (models.User, bool)
	ListAll() []models.User
	ListForAccount(string) []models.User
	BelongsToAccount(profileID, accountID string) bool
}
```

For global scope, require `actor.IsAdmin`. For profile scope, allow admin or owner. Load raw profile settings with `Get`, effective settings with `GetWithDefaults`, and return explicit inheritance metadata without inferring it from effective values.

- [ ] **Step 4: Write apply tests for independent section behavior**

Cover: global Rows-only apply preserves Theme; profile Rows customization preserves playback and inherited Theme; profile Theme customization preserves inherited Rows; resetting one section preserves the other; stale revision writes nothing; invalid requests write nothing.

- [ ] **Step 5: Implement global atomic apply**

Run revision comparison and mutation inside `config.Manager.Mutate`. Convert models back to config types, normalize ordering, mutate only requested sections, and return `ErrRevisionConflict` before changing the document when hashes differ.

- [ ] **Step 6: Implement profile atomic apply**

Run revision comparison and mutation inside `user_settings.Service.Mutate`. `mode:"inherit"` clears only the selected section. `mode:"custom"` stores the complete normalized section. Preserve every non-Home-Designer profile field.

- [ ] **Step 7: Return the normalized post-write document**

Call `Load` after mutation so the response contains the new revision, effective values, explicit overrides, permissions, profiles, catalog, and presets exactly as a reload would.

- [ ] **Step 8: Run service and regression tests**

Run: `cd backend && go test ./services/homedesigner ./services/user_settings ./config -count=1`

Expected: PASS.

- [ ] **Step 9: Commit locally**

```bash
git add backend/services/homedesigner/service.go backend/services/homedesigner/service_test.go
git commit -m "feat(home-designer): load and apply scoped documents"
```

### Task 4: Add the page, API handlers, embedded assets, routes, and navigation

**Files:**
- Create: `backend/handlers/home_designer.go`
- Create: `backend/handlers/home_designer_assets.go`
- Create: `backend/handlers/home_designer_test.go`
- Create: `backend/handlers/admin_templates/home_designer.html`
- Create: `backend/handlers/admin_assets/home_designer/app.js`
- Create: `backend/handlers/admin_assets/home_designer/home_designer.css`
- Modify: `backend/handlers/admin_ui.go`
- Modify: `backend/handlers/admin_templates/base.html`
- Modify: `backend/handlers/admin_ui_internal_test.go`
- Modify: `backend/main.go`

**Interfaces:**
- Consumes: `homedesigner.Service.Load` and `Apply`.
- Produces handlers: `HomeDesignerPage`, `GetHomeDesigner`, `PutHomeDesigner`, and `HomeDesignerAsset`.
- Produces browser globals only through semantic DOM/data attributes; application behavior remains in ES modules.

- [ ] **Step 1: Write page and API handler tests**

Assert the page renders for admin and account sessions, an account with no profiles receives an actionable empty state, global load is forbidden to non-admins, owned profile load succeeds, unowned profile load is 404, validation is JSON 422, stale revision is JSON 409, and successful apply is JSON 200.

- [ ] **Step 2: Add the page template and constructor wiring**

Add `homeDesignerTemplate *template.Template` and `homeDesignerService *homedesigner.Service` to `AdminUIHandler`; initialize both in `NewAdminUIHandler`. Render `AdminPageData` with `CurrentPath: basePath + "/settings/home-designer"` and standard role/build fields.

- [ ] **Step 3: Implement strict JSON handlers**

Use `json.Decoder.DisallowUnknownFields()`, a single-document decode, and a shared JSON error shape:

```go
type homeDesignerErrorResponse struct {
	Code string `json:"code"`
	Message string `json:"message"`
	Fields []homedesigner.FieldError `json:"fields,omitempty"`
}
```

Map forbidden to 403, not-found to 404, validation to 422, revision conflict to 409, and persistence failure to 500.

- [ ] **Step 4: Embed and serve an allowlisted asset directory**

Use `//go:embed admin_assets/home_designer/*.js admin_assets/home_designer/*.css`. Reject path separators and unknown filenames; set `text/javascript; charset=utf-8` or `text/css; charset=utf-8`, `X-Content-Type-Options: nosniff`, and a short cache policy. The template loads `{{.BasePath}}/assets/home-designer/home_designer.css` and `app.js`.

- [ ] **Step 5: Register both role namespaces**

Add authenticated routes:

```go
r.HandleFunc("/admin/settings/home-designer", adminUIHandler.RequireAuth(adminUIHandler.HomeDesignerPage)).Methods(http.MethodGet)
r.HandleFunc("/admin/api/home-designer", adminUIHandler.RequireAuth(adminUIHandler.GetHomeDesigner)).Methods(http.MethodGet)
r.HandleFunc("/admin/api/home-designer", adminUIHandler.RequireAuth(adminUIHandler.PutHomeDesigner)).Methods(http.MethodPut)
r.HandleFunc("/account/settings/home-designer", adminUIHandler.RequireAuth(adminUIHandler.HomeDesignerPage)).Methods(http.MethodGet)
r.HandleFunc("/account/api/home-designer", adminUIHandler.RequireAuth(adminUIHandler.GetHomeDesigner)).Methods(http.MethodGet)
r.HandleFunc("/account/api/home-designer", adminUIHandler.RequireAuth(adminUIHandler.PutHomeDesigner)).Methods(http.MethodPut)
```

Register matching asset routes with `{asset:[A-Za-z0-9_.-]+}`.

- [ ] **Step 6: Add navigation and active-state tests**

Add a Home Designer link before Home in the Settings group. Ensure `/settings/home-designer` marks only that link active and keeps the Settings group open for both `/admin` and `/account` bases.

- [ ] **Step 7: Add a minimal bootstrap state**

`app.js` reads `document.body`/root data attributes for `basePath`, loads `GET ${basePath}/api/home-designer?scope=global` for administrators or the first owned profile for account owners, and renders a blocking Retry message on failure. Defer full controls to later tasks.

- [ ] **Step 8: Run handler/template tests**

Run: `cd backend && go test ./handlers -run 'HomeDesigner|AdminNavigation' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit locally**

```bash
git add backend/handlers/home_designer.go backend/handlers/home_designer_assets.go backend/handlers/home_designer_test.go backend/handlers/admin_templates/home_designer.html backend/handlers/admin_templates/base.html backend/handlers/admin_assets/home_designer backend/handlers/admin_ui.go backend/handlers/admin_ui_internal_test.go backend/main.go
git commit -m "feat(home-designer): add authenticated editor page"
```

### Task 5: Reuse display-list resolution for presentation-safe previews

**Files:**
- Create: `backend/handlers/home_designer_preview.go`
- Create: `backend/handlers/home_designer_preview_test.go`
- Modify: `backend/handlers/home_designer.go`
- Modify: `backend/handlers/admin_ui.go`
- Modify: `backend/handlers/startup.go`
- Modify: `backend/handlers/startup_test.go`
- Modify: `backend/main.go`

**Interfaces:**
- Consumes: unsaved `[]models.ShelfConfig`, selected profile, and existing `DisplayListHandler`/startup dependencies.
- Produces: `HomeDesignerPreviewProvider.Preview(context.Context, *http.Request, homedesigner.PreviewRequest) (homedesigner.PreviewResponse, error)`.
- Produces handler: `PreviewHomeDesigner` at POST `/admin|account/api/home-designer/preview`.

- [ ] **Step 1: Write preview authorization and privacy tests**

Cover owned/unowned profile selection, maximum requested rows/items, disabled integrations, partial row failure, empty real results receiving `sample:true`, and JSON absence of `path`, `url`, `token`, `apiKey`, `clientIp`, and provider credentials.

- [ ] **Step 2: Extract a reusable display-list query mapper**

Move the shelf-to-query logic currently split between startup and prewarm into a handler helper that maps built-ins and custom types to `DisplayListHandler` sources. Preserve startup behavior with existing tests. Required mappings include `watchlist`, `continue-watching`, `top-ten`, `trending`, `personalized`, `genre`, `decade`, MDBList, Stremio, TMDB, Trakt, Simkl, Letterboxd, popular activity, recent activity, and permanent prequeue.

- [ ] **Step 3: Implement normalized projection**

Project source items into:

```go
type PreviewItem struct { ID, Title, Subtitle, MediaType, ArtworkURL string; Progress *float64; Badges []string; Sample bool }
type PreviewRow struct { ID, Name, Layout, Status, Message string; Items []PreviewItem; Total int }
type PreviewResponse struct { Rows []PreviewRow `json:"rows"` }
```

Build values from explicit safe model fields only. Do not JSON-forward source objects.

- [ ] **Step 4: Implement isolated concurrent resolution**

Validate no more than 20 rows and 12 items per row, use `errgroup.Group` with a semaphore of 4, derive a request context timeout, preserve request order, and turn each failure into `status:"error"` without failing the entire response.

- [ ] **Step 5: Add deterministic sample items for valid empty rows**

Use a small in-code sample catalog with fictional titles and local color-gradient artwork hints. Mark every item `sample:true`. Authentication or resolver failure must remain `status:"error"` and never receive samples.

- [ ] **Step 6: Wire provider and POST routes**

Add `SetHomeDesignerPreviewProvider` to `AdminUIHandler`, inject the startup/display-list-backed provider in `main.go`, register both `/admin` and `/account` POST routes, and require profile ownership before provider invocation.

- [ ] **Step 7: Run preview, startup, and display-list tests**

Run: `cd backend && go test ./handlers -run 'HomeDesignerPreview|StartupHomeShelves|DisplayList' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit locally**

```bash
git add backend/handlers/home_designer_preview.go backend/handlers/home_designer_preview_test.go backend/handlers/home_designer.go backend/handlers/admin_ui.go backend/handlers/startup.go backend/handlers/startup_test.go backend/main.go
git commit -m "feat(home-designer): add safe real-content previews"
```

### Task 6: Build the browser document store and API client test-first

**Files:**
- Create: `backend/handlers/admin_assets/home_designer/api.js`
- Create: `backend/handlers/admin_assets/home_designer/store.js`
- Create: `backend/handlers/admin_assets/home_designer/store_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/app.js`

**Interfaces:**
- Consumes: Task 2 JSON contract and Task 4/5 APIs.
- Produces: `createStore(document)`, `dispatch(action)`, `subscribe(listener)`, `canUndo`, `canRedo`, `isDirty`, `buildApplyRequest()`, and `replaceWithSaved(document)`.
- Produces: `loadDocument`, `loadPreview`, and `applyDocument` API functions with `APIError`.

- [ ] **Step 1: Write reducer tests**

Test add, remove, visibility, reorder with contiguous order values, synchronized selection, row field edits, theme edits, Rows/Theme customize/reset, undo, redo, discard, dirty state, invalid-row detection, and normalized apply payloads.

```js
test('row move is undoable and keeps contiguous order', () => {
  const store = createStore(documentFixture())
  store.dispatch({ type: 'rows/move', id: 'watchlist', to: 0 })
  assert.deepEqual(store.getState().rows.map(row => row.order), [0, 1, 2])
  store.undo()
  assert.equal(store.getState().rows[1].id, 'watchlist')
})
```

- [ ] **Step 2: Run Node tests and verify imports are missing**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs`

Expected: module-not-found failure for `store.js` or missing exports.

- [ ] **Step 3: Implement a pure immutable reducer and bounded history**

Keep the last 100 semantic edits. Exclude preview responses, loading flags, selection-only changes, and transient errors from undo history. Deep-clone server documents with `structuredClone`.

- [ ] **Step 4: Implement inheritance and apply construction**

Track independent initial/current Rows and Theme modes. Emit only dirty sections in `ApplyRequest`; emit `mode:"inherit"` without a value for resets and a complete normalized value for custom sections.

- [ ] **Step 5: Implement the API client**

Set `Accept`/`Content-Type: application/json`, propagate `AbortSignal`, parse the structured server error body, and throw `APIError{status, code, message, fields}`. Resolve paths from the page's `basePath`, never from hard-coded `/admin`.

- [ ] **Step 6: Wire bootstrap, scope switching, apply, and discard**

On scope switch with dirty state, require confirmation before replacing the store. Apply uses the current revision; success calls `replaceWithSaved`. Conflict retains the current working copy and exposes Reload latest.

- [ ] **Step 7: Run browser-state tests**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs`

Expected: PASS.

- [ ] **Step 8: Commit locally**

```bash
git add backend/handlers/admin_assets/home_designer/api.js backend/handlers/admin_assets/home_designer/store.js backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/app.js
git commit -m "feat(home-designer): add local editor state"
```

### Task 7: Implement row library, ordering, inspectors, and accessible composition

**Files:**
- Create: `backend/handlers/admin_assets/home_designer/library.js`
- Create: `backend/handlers/admin_assets/home_designer/outline.js`
- Modify: `backend/handlers/admin_assets/home_designer/app.js`
- Modify: `backend/handlers/admin_assets/home_designer/home_designer.css`
- Modify: `backend/handlers/admin_templates/home_designer.html`
- Modify: `backend/handlers/admin_assets/home_designer/store_test.mjs`

**Interfaces:**
- Consumes: catalog entries and Task 6 store actions.
- Produces: `renderLibrary`, `renderOutline`, and `renderInspector` DOM functions.
- Emits store actions only; modules do not mutate the document directly.

- [ ] **Step 1: Add failing state tests for catalog-instance behavior**

Verify unique built-ins cannot be added twice, repeatable templates get collision-safe IDs, dropping an incomplete configurable row marks Apply invalid, and completing required fields clears the validation state.

- [ ] **Step 2: Implement searchable categorized library rendering**

Render text search and All/Personal/Genres/Services/Lists filters. Disabled entries show their reason and setup link. Every available entry provides Add plus native drag metadata containing only its stable catalog type.

- [ ] **Step 3: Implement outline rendering and explicit movement controls**

Each row exposes select, visibility, Move up, Move down, and Remove controls. Use a real list with `aria-posinset`/`aria-setsize`; announce reordering through the page live region.

- [ ] **Step 4: Implement pointer drag-and-drop as an enhancement**

Accept library templates and existing row IDs, render a drop indicator, dispatch the same add/move actions used by buttons, and restore focus to the moved row after completion.

- [ ] **Step 5: Implement metadata-driven row inspectors**

Render text, number, select, boolean, URL, and ordered collection fields from `CatalogField`. Show field errors beside controls and a section summary; selecting an error selects its row and focuses its first invalid control.

- [ ] **Step 6: Synchronize selection between outline and preview host**

Expose `data-row-id` on both surfaces. Selection dispatch scrolls the counterpart with `{block:'nearest'}` and updates `aria-current` without adding undo history.

- [ ] **Step 7: Run Node and Go template tests**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs`

Run: `cd backend && go test ./handlers -run 'HomeDesignerPage|HomeDesignerAsset' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit locally**

```bash
git add backend/handlers/admin_assets/home_designer backend/handlers/admin_templates/home_designer.html
git commit -m "feat(home-designer): compose and configure home rows"
```

### Task 8: Implement live theme editing and TV/mobile preview rendering

**Files:**
- Create: `backend/handlers/admin_assets/home_designer/theme.js`
- Create: `backend/handlers/admin_assets/home_designer/preview.js`
- Create: `backend/handlers/admin_assets/home_designer/preview_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/app.js`
- Modify: `backend/handlers/admin_assets/home_designer/home_designer.css`
- Modify: `backend/handlers/admin_templates/home_designer.html`

**Interfaces:**
- Consumes: working Rows/Theme state and POST preview responses.
- Produces: `renderTheme`, `applyThemeVariables`, `createPreviewController`, `renderTVPreview`, and `renderMobilePreview`.

- [ ] **Step 1: Write preview-controller tests**

Use injected fake timers/fetch functions to verify 250 ms debounce, cancellation of superseded requests, stale-response suppression, cache keys based on profile/platform/normalized row, and per-row retry.

- [ ] **Step 2: Implement theme controls and scoped CSS variables**

Apply `--preview-accent`, text, secondary text, background, modal background, font scale, button radius/style, contrast, and overlay variables only on `.home-preview-device`. Preset clicks dispatch one grouped undoable theme action.

- [ ] **Step 3: Implement preview request scheduling**

Request enabled visible rows lazily, maximum 12 items, and keep results keyed by row ID plus normalized configuration. Preserve successful rows while another row reloads or fails.

- [ ] **Step 4: Implement common semantic media cards**

Render artwork with safe DOM properties, title/subtitle, badges, progress, sample label, loading skeleton, empty state, and error Retry. Never insert server strings with `innerHTML`.

- [ ] **Step 5: Implement separate TV and mobile layout renderers**

TV uses a 16:9 stage, navigation rail, hero, landscape/portrait row rules, and TV density/scale. Mobile uses a phone viewport, mobile top carousel behavior, portrait density, and bottom navigation. Both consume the same ordered row array.

- [ ] **Step 6: Honor reduced motion and high contrast independently**

The admin editor obeys `prefers-reduced-motion`; the preview displays the selected app theme's contrast/overlay behavior without weakening admin focus indicators.

- [ ] **Step 7: Run Node tests**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/preview_test.mjs`

Expected: PASS.

- [ ] **Step 8: Commit locally**

```bash
git add backend/handlers/admin_assets/home_designer backend/handlers/admin_templates/home_designer.html
git commit -m "feat(home-designer): preview themes on TV and mobile"
```

### Task 9: Complete failure states, responsive drawers, accessibility, and advanced-setting links

**Files:**
- Modify: `backend/handlers/admin_assets/home_designer/app.js`
- Modify: `backend/handlers/admin_assets/home_designer/library.js`
- Modify: `backend/handlers/admin_assets/home_designer/outline.js`
- Modify: `backend/handlers/admin_assets/home_designer/theme.js`
- Modify: `backend/handlers/admin_assets/home_designer/preview.js`
- Modify: `backend/handlers/admin_assets/home_designer/home_designer.css`
- Modify: `backend/handlers/admin_templates/home_designer.html`
- Modify: `backend/handlers/admin_templates/settings.html`
- Modify: `backend/handlers/admin_ui_internal_test.go`

**Interfaces:**
- Consumes: structured API errors and store dirty/validation state.
- Produces: complete user-facing error, focus, drawer, and navigation behavior.

- [ ] **Step 1: Add template assertions for required accessible structure**

Assert one `h1`, labeled library/preview/inspector regions, a polite status live region, an assertive error live region, Apply/Discard/Undo/Redo buttons, scope/profile/platform labels, and links to advanced Home and Appearance settings.

- [ ] **Step 2: Implement blocking load and recoverable apply states**

Initial load failure replaces the workspace with Retry. Apply network errors retain edits and focus a retryable alert. A 409 presents Reload latest without overwriting the store. A 422 maps field errors and focuses the first invalid row.

- [ ] **Step 3: Implement independent inheritance confirmations**

Rows and Theme each show Inheriting/Customized state. Customize copies effective values into the working override. Reset requires confirmation and dispatches `mode:inherit` only for that section.

- [ ] **Step 4: Implement responsive panel drawers**

At widths below 1100 px, keep the preview visible and move library/inspector into labeled modal drawers with focus trap, Escape close, focus restoration, and no background scrolling.

- [ ] **Step 5: Add dirty-navigation protection**

Register `beforeunload` only while dirty. Intercept internal scope/navigation changes with a confirmation dialog. Remove protection immediately after successful Apply or Discard.

- [ ] **Step 6: Link legacy forms to Home Designer without removing controls**

Add a prominent “Open Home Designer” callout to the Home and Appearance categories. Keep every existing advanced field and save behavior intact.

- [ ] **Step 7: Perform keyboard-only verification**

Verify Tab order, library Add, row movement, visibility, selection, inspector edits, TV/mobile switch, undo/redo, Apply, Discard, confirmations, error recovery, and drawer close without pointer input. Record discovered defects as tests before fixing them.

- [ ] **Step 8: Run browser-state and template tests**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/preview_test.mjs`

Run: `cd backend && go test ./handlers -run 'HomeDesigner|SettingsTemplate' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit locally**

```bash
git add backend/handlers/admin_assets/home_designer backend/handlers/admin_templates/home_designer.html backend/handlers/admin_templates/settings.html backend/handlers/admin_ui_internal_test.go
git commit -m "fix(home-designer): harden responsive and accessible flows"
```

### Task 10: Run full regression verification and document the local result

**Files:**
- Modify if needed: files already touched by Tasks 1-9, only for defects reproduced by verification.
- Do not modify: native-client response models except to preserve existing behavior with tests.

**Interfaces:**
- Consumes: complete implementation.
- Produces: verified local commit series with no remote side effects.

- [ ] **Step 1: Run formatting**

Run: `cd backend && gofmt -w config/settings.go config/settings_test.go services/user_settings/service.go services/user_settings/service_test.go services/homedesigner/*.go handlers/home_designer*.go handlers/user_settings.go handlers/user_settings_test.go handlers/startup.go handlers/startup_test.go main.go`

- [ ] **Step 2: Run all browser module tests**

Run: `node --test backend/handlers/admin_assets/home_designer/*.mjs`

Expected: PASS with zero failed tests.

- [ ] **Step 3: Run focused Go tests with race detection**

Run: `cd backend && go test -race ./config ./services/user_settings ./services/homedesigner ./handlers -run 'HomeDesigner|ManagerMutate|MutatePreserves|ShelfConversion|StartupHomeShelves|SettingsTemplate' -count=1`

Expected: PASS with no race reports.

- [ ] **Step 4: Run the full backend test suite**

Run: `cd backend && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 5: Build the backend**

Run: `cd backend && go build -o /tmp/mediastorm-home-designer-check main.go`

Expected: exit status 0 and binary at `/tmp/mediastorm-home-designer-check`.

- [ ] **Step 6: Perform authenticated browser verification**

Run the local server against temporary settings/data, open both `/admin/settings/home-designer` and `/account/settings/home-designer`, and verify global permissions, owned-profile permissions, real/sample/error rows, row composition, Rows/Theme inheritance, TV/mobile preview, Apply/Discard, reload persistence, revision conflict, keyboard controls, responsive drawers, and existing advanced settings links.

- [ ] **Step 7: Confirm backward compatibility explicitly**

Capture before/after JSON from the existing startup/settings endpoints for an unchanged fixture and assert structural equality. Then apply a Home Designer row/theme change and assert only existing `homeShelves` and `display.appearance` fields change.

- [ ] **Step 8: Inspect the final local diff and remote state**

Run: `git status --short && git log --oneline --decorate -12 && git remote -v`

Confirm all feature commits are local and no push command has been executed.

- [ ] **Step 9: Commit verification fixes locally if any were required**

```bash
git add backend
git commit -m "test(home-designer): complete integration verification"
```

Skip this commit when verification required no file changes.

---

## Completion Criteria

- Home Designer is reachable from authenticated admin and account navigation.
- Administrators can edit global defaults; account owners can edit only owned profiles.
- Rows and Theme inherit/customize/reset independently.
- The library exposes unique and repeatable row templates with integration-aware availability.
- Add, reorder, configure, hide, remove, undo, redo, discard, and explicit Apply work by pointer and keyboard.
- TV and mobile previews use one row order, real safe profile content when available, deterministic samples for valid empty rows, and isolated row errors.
- Theme updates are live and scoped to the mock device.
- Revision conflicts, validation errors, network failures, and dirty navigation are recoverable without silent data loss.
- Existing settings forms and native startup/settings contracts continue to pass regression tests.
- All implementation commits remain local to Jeor's repository and nothing is pushed upstream.
