# Home Designer Design

**Date:** 2026-08-30
**Status:** Approved design, awaiting implementation planning

## Summary

Mediastorm will add a dedicated **Home Designer** settings page: a high-fidelity, WYSIWYG-style editor for composing the native app home screen and previewing app-wide appearance changes. Users can search a catalog of premade rows, drag or add them to a mock home screen, reorder and configure them, switch between TV and mobile previews, and apply the finished configuration explicitly.

The editor is a visual layer over Mediastorm's existing `HomeShelves` and `AppearanceSettings` models. It does not introduce a second persistence format or change how native clients receive effective settings.

## Goals

- Make home-screen composition understandable without requiring users to interpret a long settings form.
- Provide a trustworthy live preview of row order, visibility, content density, theme, typography, and platform-specific layout.
- Reuse the existing shelf types, global settings, profile overrides, integrations, and native-client delivery flow.
- Support global defaults and optional profile overrides with clear inheritance behavior.
- Let users experiment safely without changing connected apps until they explicitly apply their work.
- Preserve the existing Home and Appearance forms as advanced settings backed by the same data.

## Non-goals

- Running React Native or native app components inside the admin browser.
- Pixel-perfect emulation of native animation, focus, or playback behavior.
- Server-side saved drafts, publishing history, or collaborative editing in the first version.
- Separate row ordering for TV and mobile in the first version.
- Home-only theme overrides. Appearance settings apply throughout the app for their selected global or profile scope.
- Replacing or migrating the existing settings persistence format.

## Permissions and Scope

- Administrators can edit the global default and any profile they are authorized to manage.
- Account owners can edit overrides only for profiles owned by their account.
- Ordinary profiles do not receive direct Home Designer access.
- All permissions are enforced by the backend for document load, preview resolution, apply, and reset operations.
- UI visibility and disabled controls are not treated as authorization controls.

The editor supports two independently inherited sections:

1. **Rows** — the ordered `HomeShelves` configuration.
2. **Theme** — the existing `AppearanceSettings` configuration.

A profile may customize either section while continuing to inherit the other. Customizing Rows creates an independent snapshot of the effective global row layout. Later global row changes do not merge into that profile. Resetting Rows to global removes the profile snapshot and restores live inheritance. Theme inheritance uses the existing appearance override behavior and has its own Customize and Reset actions.

## Information Architecture

Home Designer is a dedicated admin page at the base-path-aware equivalent of `/admin/settings/home-designer`. The existing Settings navigation gains a **Home Designer** destination near Home and Appearance.

The desktop workspace uses three persistent panels:

- **Left: Row library** — search, category filters, availability, and add/drag affordances.
- **Center: Live preview** — TV or mobile mock home screen using real selected-profile content where available.
- **Right: Rows/Theme panel** — ordered outline plus contextual row inspector, or theme controls.

The header contains:

- selected scope and inheritance status;
- “Preview as” profile selector when the current permissions allow it;
- TV/mobile preview selector when space requires it to move out of the preview toolbar;
- undo and redo controls;
- Discard changes;
- Apply changes.

On narrower admin viewports, side panels may collapse into drawers while preserving the same three logical areas. The center preview remains the primary surface.

The visual hierarchy approved during design is preserved in the brainstorming artifact under `.superpowers/brainstorm/`; that directory is not part of the product or committed specification.

## Row Library

The backend supplies a catalog of supported row templates. Each catalog entry includes:

- stable type identifier;
- display name and description;
- category;
- icon or presentation hint;
- whether multiple instances are allowed;
- required integrations or capabilities;
- default configuration;
- configurable field definitions;
- whether a specialized preview renderer is available.

Built-in structural rows such as Continue Watching, Top 10, and Streaming Services are unique. Parameterized rows such as genres, decades, streaming providers, external lists, and similar configurable collections may have multiple instances.

Rows whose integration is unavailable remain visible but disabled. They explain the missing requirement and link to the appropriate integration settings. Users cannot add a row that is known to be unusable.

Dropping or adding a configurable row immediately inserts and selects it, then opens its inspector. Until required configuration is complete, the preview shows a labeled placeholder and Apply remains disabled. Setup does not interrupt the direct-manipulation workflow with a modal.

## Editor Interaction Model

Opening the page loads the selected scope's effective document and creates an isolated working copy in the browser. No edit is persisted until Apply succeeds.

Supported operations include:

- add, remove, show, and hide rows;
- reorder by drag-and-drop, keyboard commands, or explicit move buttons;
- select a row from the preview or outline;
- edit the selected row in the inspector;
- switch between Rows and Theme panels;
- switch TV/mobile preview modes without changing the shared row order;
- undo and redo all working-copy mutations;
- discard all local changes;
- customize or reset Rows and Theme inheritance independently.

Selection is synchronized: selecting a preview row highlights its outline entry and reveals its inspector; selecting an outline entry scrolls the preview to that row.

The browser tracks dirty state. Navigating away or closing the page with unapplied changes triggers a warning. Reloading after departure loses the local draft by design; server-side drafts are out of scope.

## Theme Editing

The Theme panel edits the existing appearance fields and presets, including:

- font scale;
- accent, primary text, secondary text, page background, and modal background colors;
- button style and radius;
- high contrast;
- reduced overlays.

Theme edits update preview-scoped CSS variables immediately and never restyle the admin shell itself. Appearance changes apply throughout the native app for the selected global or profile scope, even though the home screen is the live preview surface.

Existing branding controls remain available through the advanced Appearance form in the first version. Home Designer may display current branding assets where they are part of the home preview, but asset upload and crop workflows are not moved into the initial editor.

## Architecture

The feature uses a dedicated server-rendered page with separate, focused CSS and browser JavaScript modules. It does not add a JavaScript bundler or frontend framework.

Browser modules have narrow responsibilities:

- **Document store** — working copy, dirty state, selection, undo/redo, normalization, and revision.
- **Row library** — catalog search, filtering, capability states, and add/drag sources.
- **Outline and inspector** — ordering, visibility, validation display, and row configuration.
- **Theme editor** — existing appearance settings and presets.
- **Preview controller** — lazy data requests, request cancellation, cache coordination, and TV/mobile selection.
- **Preview renderers** — normalized rows and cards rendered with platform-specific layout rules.
- **API client** — load, preview, apply, and reset requests with consistent error handling.

A focused Go Home Designer service sits between handlers and existing configuration/content services. It constructs editor documents, builds the row catalog, authorizes scope access, validates complete submissions, resolves presentation-safe preview content, and writes through existing stores.

The feature must not add more Home Designer behavior to the existing large `admin_templates/settings.html` script. The legacy forms and new page share backend models and stores, not duplicated client code.

## Editor Document

`HomeDesignerDocument` is an editor-facing envelope rather than a persistence model. Its response contains:

- selected global or profile scope;
- permissions for that scope;
- previewable profiles;
- revision token;
- Rows inheritance state, effective value, and explicit override when present;
- Theme inheritance state, effective value, and explicit override when present;
- row catalog and capability states;
- normalized theme presets;
- validation and platform constraints required by the editor.

Rows continue to use the existing shelf configuration structures. Global rows remain in `config.Settings.HomeShelves`; profile rows remain in `models.UserSettings.HomeShelves` through the existing user-settings service. Themes remain in the existing global and profile `AppearanceSettings` fields.

The envelope distinguishes effective values from explicit overrides so the UI never infers inheritance by comparing JSON values.

## HTTP API

Routes are base-path-aware and protected by the existing admin session middleware.

### Load

`GET /admin/api/home-designer?scope=global`
`GET /admin/api/home-designer?scope=profile&profileId=<id>`

Returns the complete editor document. A load failure is blocking; the editor does not initialize against guessed defaults.

### Preview

`POST /admin/api/home-designer/preview`

Accepts the selected preview profile, platform, and the normalized unsaved rows that currently need content. It does not persist any values. The handler verifies that the caller may preview the requested profile and rejects arbitrary or sensitive source definitions.

### Apply

`PUT /admin/api/home-designer`

Accepts:

- scope;
- expected revision;
- changed section declarations;
- complete normalized Rows and/or Theme values for those sections;
- requested inheritance/reset state.

The backend validates and atomically writes the changed sections within the selected scope. It returns the new revision and normalized saved document. No partial write is allowed.

The revision identifies the persisted scope state represented by the loaded document. Apply returns a conflict when the revision is stale. The UI offers Reload latest and preserves the unsaved working copy long enough for the user to understand the conflict; it does not overwrite or automatically merge concurrent changes.

## Preview Content and Rendering

The default preview uses real presentation-safe content for the selected profile. When a valid row has no results, it uses visibly representative sample cards so layout remains inspectable. Authentication failures, unavailable integrations, and transient resolver errors are displayed as errors rather than disguised as empty sample data.

Preview card responses contain only presentation fields needed by the renderer, such as:

- stable preview item identifier;
- title and optional subtitle;
- media type;
- poster or landscape artwork URL;
- progress;
- non-sensitive badges;
- availability state.

They exclude file paths, credentials, playback URLs, private provider details, client addresses, and transport information.

Each shelf type maps to a preview resolver that reuses an existing content service when possible. Requests return only enough items to fill the visible mockup. Resolution is lazy, concurrency-limited, and briefly cached by profile plus normalized row configuration. Rapid changes are debounced and stale browser requests are cancelled.

One row's failure never prevents other rows from loading or the editor from operating. Unsupported future row types remain configurable and reorderable but use a clearly labeled generic preview until a renderer is implemented.

TV and mobile preview renderers share normalized rows and cards but have separate rules for viewport, hero treatment, navigation, spacing, card aspect ratios, and density. Row order remains shared. The preview contract covers controlled structural and appearance settings; it does not promise native animation or focus emulation.

## Validation

The browser provides immediate guidance, but the backend is authoritative. Apply validates at least:

- authorized scope and profile ownership;
- known row types and stable identifiers;
- unique-row multiplicity;
- required integrations and capabilities;
- required row fields;
- supported shelf limits and sort/filter values;
- valid colors and appearance ranges;
- valid inheritance transitions;
- current revision.

Validation errors identify the section, row, and field where possible. The UI displays inline errors, provides an Apply summary, selects the first invalid row, and moves focus to the relevant control.

## Error Handling

- Initial load errors produce a blocking retry state.
- Preview errors remain local to the affected row and expose Retry.
- Network failure during Apply preserves the working copy for retry.
- Revision conflict refuses the write and offers Reload latest.
- Server validation failure changes no persisted value.
- Persistence failure changes neither Rows nor Theme.
- Reset actions require confirmation because they remove explicit profile customization.
- Unexpected renderer input produces a generic safe row rather than executing catalog-provided markup.

## Accessibility

- Every drag-and-drop operation has keyboard and explicit button equivalents.
- Row selection, ordering, visibility, and validation status are exposed with semantic controls and accessible names.
- Focus is restored predictably after add, remove, reorder, reset, and failed Apply operations.
- Status changes such as preview loading, validation failure, successful Apply, and inheritance changes use appropriate live-region announcements.
- The editor honors its own high-contrast and reduced-motion administrative experience independently from the theme being previewed.

## Backward Compatibility

- Existing persisted `HomeShelves` and `AppearanceSettings` values remain valid.
- Existing global and profile settings APIs continue to function.
- Native clients continue receiving their effective configuration through the current startup/settings contract.
- The advanced Home and Appearance forms remain operational and edit the same source-of-truth data.
- Applying through either interface increments or otherwise changes the revision observed by Home Designer, preventing stale editor overwrites.

## Testing Strategy

### Go unit tests

- catalog construction and capability states;
- editor document construction and normalization;
- effective versus explicit inheritance;
- unique and repeatable row validation;
- integration requirements and field constraints;
- permission and ownership decisions;
- revision creation and conflict detection;
- presentation-safe preview projection;
- preview resolver empty, success, and failure behavior.

### Handler and persistence tests

- global administrator access;
- owned and unowned profile access;
- malformed and unauthorized preview requests;
- global and profile Apply operations;
- independent Rows and Theme customization/reset;
- atomic failure behavior;
- conflict responses;
- preview privacy guarantees;
- base-path-aware routing;
- continued behavior of existing settings and startup endpoints.

### Browser-state tests

Pure state transitions live outside DOM code and are tested with Node's built-in test runner, without a bundler:

- add, remove, reorder, visibility, and selection synchronization;
- configurable-row incomplete and valid states;
- undo and redo;
- dirty tracking and discard;
- independent Rows and Theme inheritance;
- apply payload normalization;
- stale response suppression.

### Browser verification

- mouse and keyboard row composition;
- responsive three-panel and drawer behavior;
- preview/outline selection synchronization;
- TV/mobile switching;
- real, sample, empty, and failed row states;
- live theme updates without admin-shell changes;
- navigation warning for dirty state;
- validation, network failure, retry, and revision conflict flows;
- accessibility names, focus behavior, and live announcements.

## Delivery Notes

The initial release should expose Home Designer as the primary visual workflow while retaining the existing forms as advanced settings. No automatic migration is needed because the editor consumes existing values. If a row is valid for native clients but lacks a specialized browser renderer, the generic preview behavior allows delivery without blocking configuration access.

The implementation plan should keep the service, handlers, state store, catalog, and renderers in focused modules and should introduce the feature incrementally behind tests rather than extending the existing monolithic settings template.
