# Home Designer Direct-Manipulation Editor Design

**Date:** 2026-09-01
**Status:** Approved design, awaiting implementation planning

## Summary

Home Designer will be reshaped around the interaction model introduced by the modular dashboard editor: an explicit edit mode, direct manipulation, clear draft controls, keyboard alternatives, and predictable Save/Cancel behavior. The home-screen workspace is not a two-dimensional dashboard grid, however. It is a one-dimensional ordered list of rows, so the feature will reuse the dashboard editor's interaction language without reusing GridStack or its 12-column persistence model.

The center of the page becomes the primary editing surface. Users enter **Edit Home**, drag existing rows vertically, drag premade rows from a library into insertion points anywhere in the mock home screen, and select a row to customize it. The preview continues to use labeled schematic content instead of realistic posters so row hierarchy, density, and theme remain legible at useful scale.

This document revises the information architecture, preview fidelity, and editor-interaction portions of `2026-08-30-home-designer-design.md`. The earlier document remains authoritative for permissions, scope and inheritance, persistence, API contracts, validation, backward compatibility, and native-client behavior unless this document explicitly says otherwise.

## Goals

- Make the mock home screen itself the row-ordering workspace.
- Preserve the modular dashboard editor's safe and learnable edit lifecycle.
- Let users insert premade rows at the intended position rather than adding first and reordering later.
- Keep row customization close to the selected row without permanently squeezing the preview.
- Scale cleanly across compact, standard, and wide desktop admin viewports.
- Preserve existing Home Designer draft, undo/redo, validation, inheritance, and persistence behavior.

## Non-goals

- Horizontal row placement, row resizing, freeform coordinates, or automatic grid packing.
- Reusing the dashboard editor's GridStack document format.
- Recreating realistic native media artwork in the editor.
- Separate row orders for different preview platforms.
- Changing the existing Home Designer backend document or native-client configuration formats solely to support panel layout.
- Saving drawer state, selection, preview scale, or other editor-only presentation state.

## Dashboard Editor Reuse Boundary

Home Designer adopts these dashboard editor concepts:

- an explicit read-only state followed by an explicit edit mode;
- visible drag handles only while editing;
- clear Apply and Cancel actions;
- keyboard and button alternatives to pointer dragging;
- a fallback path when drag-and-drop is unavailable;
- consistent selected, dragging, valid-drop, invalid-drop, dirty, saving, and error states.

Home Designer does not adopt:

- the 12-column GridStack canvas;
- horizontal coordinates or width/height units;
- resize handles;
- automatic two-dimensional packing;
- dashboard-layout persistence or `admin-dashboard-layout.json`.

Using GridStack with every item forced to full width would add an unnecessary layout engine, retain desktop-centric assumptions, and create a second state model that must be synchronized with the existing Home Designer store. A focused vertical sortable interaction is the smaller and more faithful abstraction.

## Page Modes

### Preview mode

The page opens in preview mode. Rows cannot be dragged, inserted, removed, or edited. The user may still change non-mutating preview context such as preview profile or platform when permitted.

The primary action is **Edit Home**. Entering edit mode creates or resumes the browser working copy and reveals editing affordances.

### Edit mode

Edit mode enables:

- drag handles on existing rows;
- insertion points before, between, and after rows;
- Add Rows, Customize, and Theme tools;
- row visibility and removal controls;
- Undo and Redo;
- Apply and Cancel.

All mutations affect the existing browser draft. Nothing is persisted until Apply succeeds.

**Apply** validates and saves the entire changed draft atomically, then returns to preview mode with the normalized saved document. **Cancel** discards changes made since entering edit mode and restores the last saved document. If the user has unsaved changes, navigation, scope changes, and profile changes require confirmation.

## Workspace Structure

The editing workspace has three logical regions:

1. **Tool rail** — a narrow control rail for Add Rows, Customize, and Theme.
2. **Home canvas** — the central mock home screen and direct row-ordering surface.
3. **Context drawers** — the Row Library and Customize/Theme controls, exposed according to available width.

The previous permanent composition outline is not the primary ordering surface. Direct vertical manipulation in the home canvas replaces it. A compact ordered list may remain as an accessibility or fallback view if implementation testing shows it is useful, but it must use the same row operations and must not become a second draft model.

The top toolbar retains editing scope, preview profile, preview platform, Undo, Redo, and the mode-specific Edit Home, Apply, and Cancel actions. Controls may collapse into labeled menus at compact widths, but Apply and Cancel remain directly discoverable while editing.

## Responsive Drawer Model

The editor uses an adaptive hybrid instead of one fixed three-panel layout.

### Wide desktop: 1440 px and above

- The Row Library may be pinned on the left.
- Customize or Theme may be pinned on the right.
- Both drawers may remain visible together when the home canvas can retain its protected minimum width.
- Users may unpin either drawer to enlarge the canvas.

### Standard desktop: 1100–1439 px

- Only one context drawer is open at a time.
- Selecting a row switches the active drawer to Customize.
- Choosing Add Rows switches it to the Row Library.
- Choosing Theme switches it to Theme.
- The drawer is docked beside the canvas rather than covering it.

### Compact desktop: 900–1099 px

- A context drawer temporarily overlays the canvas.
- Beginning a library drag collapses the drawer and leaves a compact dragged-row token under pointer or keyboard control.
- The full canvas and every valid insertion point become visible before placement.
- Selecting a row can reopen Customize after the drop.

Below the supported desktop minimum, the page may use the compact behavior with horizontal admin-shell adaptation, but full phone authoring is not a first-version requirement. All breakpoints are CSS-driven and must account for actual available workspace width after the Mediastorm navigation shell, not only raw browser width.

## Row Library and Insertion

The Row Library keeps its search, filter, category, capability, and disabled-reason behavior. Each premade row is both a drag source and an accessible Add action.

While a library item is dragged:

- the canvas exposes insertion points before, between, and after existing rows;
- the nearest valid insertion point receives a strong visual highlight;
- the preview autoscrolls near its top and bottom edges;
- invalid destinations explain why placement is unavailable;
- Escape cancels the operation without changing the draft.

Dropping or adding a row inserts it at the chosen position, selects it, scrolls it into view if needed, and opens Customize automatically. A row with incomplete required configuration remains visibly incomplete and prevents Apply until corrected or removed.

The accessible Add action asks for or defaults to a position using the selected row as context: after the selected row when one exists, otherwise at the end. The user can then use Move Up and Move Down controls.

## Existing Row Reordering

Existing rows move vertically only. The row's drag handle is the pointer drag target; interacting with cards or row controls must not accidentally begin a drag.

During reordering:

- the dragged row remains identifiable by label;
- the source position leaves a lightweight placeholder;
- a single insertion indicator shows the resulting position;
- dropping produces one undoable store operation;
- cancelling restores the original order without adding history.

Keyboard users can select a row and use explicit Move Up and Move Down buttons. If dashboard-editor keyboard conventions can be reused without conflicting with browser or assistive-technology behavior, documented shortcuts may supplement these buttons; shortcuts are never the only alternative.

## Selection and Customization

Clicking or keyboard-selecting a row gives it a clear selected state and opens Customize while in edit mode. Customize edits the selected row's existing schema-driven fields, including label, content source, item limit, visibility, and row-type-specific settings.

Selection is editor state, not persisted configuration. Removing the selected row moves selection to the nearest surviving row and keeps focus predictable. Closing a drawer does not clear selection. Changing selection with invalid unsaved field input first commits valid input to the draft or keeps focus on and explains invalid input; it must not silently discard typed values.

On wide desktops the library and Customize drawers may coexist. On standard and compact desktops, opening Customize replaces or closes the library according to the responsive drawer rules.

## Theme Editing

Theme is a tool-rail destination using the same context drawer system. Theme controls continue writing to the existing appearance portion of the browser draft and update preview-scoped CSS variables live. They never restyle the admin shell.

At wide widths Theme uses the right context position. At standard and compact widths it replaces the currently open drawer. Leaving Theme does not discard theme changes; Undo, Redo, Cancel, and Apply treat theme and row mutations as one editor draft while preserving the existing independent Rows and Theme inheritance semantics.

## Schematic Preview

The preview is intentionally structural. Rows retain their human-readable labels and use abstract Movie, Series, Live, or Content tiles rather than fetched poster artwork, titles, badges, progress, or expanded media details.

This choice:

- shows several row items at once;
- makes row height and ordering easier to scan;
- reduces layout instability and network dependence;
- keeps attention on composition and theme rather than catalog artwork;
- allows TV and mobile density differences without claiming pixel-perfect native rendering.

Theme, platform, spacing, density, typography, visibility, row labels, and row-type presentation remain live. Backend preview content resolution may remain available for future modes, but the default editor does not require realistic poster requests to perform row composition.

## State and Persistence

The existing Home Designer document store remains the sole browser source of truth for:

- draft rows and order;
- selected row identifier;
- theme draft;
- dirty state;
- undo and redo history;
- inheritance changes;
- validation state;
- current revision.

Library insertion, direct canvas reorder, button-based movement, row customization, visibility changes, removal, and theme changes all dispatch store operations. DOM order is rendered from store state and is never treated as authoritative.

Apply persists only the existing normalized Rows and Theme values and inheritance/reset intent. Drawer state, pinned state, current tool, preview scroll position, drag state, and selection are not included in the API payload.

## Validation and Failure Behavior

- Apply runs existing client guidance and authoritative backend validation.
- The first invalid row is selected and Customize opens to its relevant field.
- A failed save leaves edit mode and the complete draft intact for retry.
- A revision conflict refuses the write and preserves the working copy while offering Reload latest.
- A preview-rendering problem affects only its row and never blocks editing unrelated rows.
- If pointer drag initialization fails, Move Up, Move Down, and accessible Add remain fully functional.
- A dropped operation is committed only after the store accepts it; rejected operations restore the prior visual order.

## Accessibility

- Drag handles, insertion points, selection, position, visibility, invalid state, and available actions have accessible names.
- Every drag operation has an explicit button and keyboard alternative.
- Live regions announce picked-up rows, target positions, completed moves, cancelled moves, validation failures, saves, and errors without excessive repetition.
- Focus returns to the inserted row after Add, to the nearest row after Remove, and to the relevant field after validation failure.
- Drawer open/close behavior manages focus and Escape consistently.
- Motion respects reduced-motion preferences; theme preview choices do not reduce editor usability.
- Touch targets and controls remain usable even though phone-sized authoring is not a first-version goal.

## Implementation Shape

The refactor should preserve the existing server-rendered, no-bundler architecture and deepen the current browser modules rather than add a framework.

- `store.js` remains authoritative for pure draft transitions and history.
- `library.js` remains responsible for catalog discovery and becomes a source for position-aware insertion.
- `outline.js` should yield its primary-ordering responsibility to a focused vertical canvas controller or be refactored into that controller; pure move operations should remain independently testable.
- `preview.js` renders schematic rows and exposes stable row containers and insertion targets without owning persisted order.
- `theme.js` continues to own theme controls and preview variable updates.
- `app.js` coordinates page mode, responsive drawers, selection, and API lifecycle.
- `home_designer.html` supplies semantic toolbar, tool rail, drawer, and canvas landmarks.
- `home_designer.css` implements the protected canvas width and adaptive wide, standard, and compact states.

The implementation plan must identify which existing outline behavior is removed, retained as fallback, or migrated so there are not two competing drag controllers.

## Testing Strategy

### Pure browser-state tests

- insert before, between, after, after selection, and at end;
- move up/down and direct reorder produce identical row arrays;
- one undo entry per completed insertion or move;
- cancelled and rejected drags do not change history;
- inserted row becomes selected;
- removing the selected row chooses the expected neighbor;
- Apply, Cancel, dirty state, inheritance, and validation remain correct.

### DOM interaction tests

- preview mode exposes no mutation affordances;
- Edit Home reveals the correct tools;
- drag handles do not steal card/control clicks;
- valid insertion points and autoscroll activate during drag;
- dropping opens Customize;
- keyboard and button alternatives complete the same operations;
- drawer focus and Escape behavior are correct;
- invalid Apply selects the row and field.

### Responsive browser verification

- 1440 px and wider: two pinned drawers without canvas overflow;
- 1100–1439 px: one docked drawer and protected canvas width;
- 900–1099 px: overlay drawer collapses when dragging starts;
- admin navigation width changes do not cause page overflow or overlapping controls;
- TV and mobile mockups remain readable at every supported editor width;
- zoom at 200 percent retains an operable non-overlapping workflow.

### Regression verification

- existing scope/profile/platform selection;
- row library filters and capability states;
- row inspector validation;
- theme live preview;
- undo/redo and dirty-navigation warning;
- atomic Apply, failed save, and revision conflict behavior;
- base-path-aware admin and account routes;
- existing advanced Home and Appearance settings compatibility.

## Acceptance Criteria

The design is complete when a user can enter Edit Home, insert a premade row at any visible position, immediately customize it, reorder every row vertically from the preview, adjust the theme with live feedback, undo or cancel safely, and atomically apply the result. The workflow must remain usable without pointer drag and must adapt across the three approved desktop width bands without compressing or jumbling the preview.
