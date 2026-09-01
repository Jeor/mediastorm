# Home Designer Direct Manipulation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Home Designer's separate ordering panel with an explicit edit mode where users insert, reorder, select, and customize labeled home rows directly in the schematic preview across wide, standard, and compact desktop layouts.

**Architecture:** Keep the existing server-rendered page, browser module store, and backend APIs. Add a pure workspace state machine for edit/drawer responsiveness and a focused vertical canvas controller that decorates the existing preview rows; extract the inspector from `outline.js`, then retire the old outline drag controller so the store remains the only source of truth.

**Tech Stack:** Go templates and handler tests, browser-native ES modules and HTML Drag and Drop, Node.js built-in test runner, CSS, GitHub Actions/Buildx.

**Spec:** `docs/superpowers/specs/2026-09-01-home-designer-direct-manipulation-design.md`

## Global Constraints

- Existing rows move vertically only; there is no horizontal positioning, resizing, freeform coordinate, or grid packing behavior.
- Do not add GridStack, a frontend framework, a JavaScript bundler, or a new persistence format.
- The existing Home Designer store remains the sole source of truth for rows, selection, theme, dirty state, undo/redo, inheritance, validation, and revision.
- Persist only existing normalized Rows and Theme values plus inheritance/reset intent; never persist editor mode, drawer state, selection, preview scroll, or drag state.
- The page opens in preview mode and exposes mutation controls only after **Edit Home**.
- Apply saves atomically; Cancel restores the last saved document and exits edit mode.
- The preview stays schematic and retains human-readable row labels plus abstract Movie, Series, Live, or Content tiles.
- Support wide desktop at 1440 px and above, standard desktop at 1100–1439 px, and compact desktop at 900–1099 px based on actual editor workspace width after the admin shell.
- Every pointer drag operation must have explicit Add, Move Up, Move Down, and Remove button equivalents.
- Keep all work on `build/home-designer-image` in `https://github.com/Jeor/mediastorm.git`; do not merge or push to `godver3/mediastorm` or a default branch.

## File Structure

- Create `backend/handlers/admin_assets/home_designer/workspace.js`: pure edit-mode, tool, width-band, and drag-state reducer.
- Create `backend/handlers/admin_assets/home_designer/workspace_test.mjs`: reducer transition and responsive-band tests.
- Create `backend/handlers/admin_assets/home_designer/canvas.js`: pure insertion helpers plus DOM enhancement for vertical canvas drag, row actions, focus, announcements, and edge autoscroll.
- Create `backend/handlers/admin_assets/home_designer/canvas_test.mjs`: insertion, movement, removal focus, drag recognition, edge scroll, and canvas affordance tests.
- Create `backend/handlers/admin_assets/home_designer/inspector.js`: existing schema-driven row and catalog inspector code extracted without changing its public behavior.
- Create `backend/handlers/admin_assets/home_designer/inspector_test.mjs`: inspector rendering, collection cap, form styling, and server-error accessibility tests moved from the old outline suite.
- Modify `backend/handlers/admin_assets/home_designer/store.js`: deterministic selection after removal while retaining one undoable row mutation.
- Modify `backend/handlers/admin_assets/home_designer/store_test.mjs`: selection, indexed insertion, removal, undo, redo, Apply, and Cancel regressions.
- Modify `backend/handlers/admin_assets/home_designer/library.js`: selected-row insertion defaults and drag lifecycle callbacks.
- Modify `backend/handlers/admin_assets/home_designer/library_test.mjs`: position-aware Add and drag callback tests.
- Modify `backend/handlers/admin_assets/home_designer/preview.js`: stable row markup and edit-state hooks consumed by the canvas controller; retain schematic rendering.
- Modify `backend/handlers/admin_assets/home_designer/preview_test.mjs`: stable row identifiers, labels, tile density, and preview-mode/edit-mode hook tests.
- Modify `backend/handlers/admin_assets/home_designer/app.js`: coordinate workspace state, direct canvas rendering, drawers, edit/apply/cancel lifecycle, validation focus, and responsive observation.
- Modify `backend/handlers/admin_assets/home_designer/theme.js`: render inside the context drawer without changing theme draft semantics.
- Modify `backend/handlers/admin_templates/home_designer.html`: semantic tool rail, mode actions, drawer slots, and central canvas landmarks.
- Modify `backend/handlers/admin_assets/home_designer/home_designer.css`: preview/edit states and adaptive wide, standard, and compact layouts.
- Modify `backend/handlers/admin_ui_internal_test.go`: require the new accessible editor structure and remove obsolete outline markers.
- Delete `backend/handlers/admin_assets/home_designer/outline.js`: remove the duplicate ordering surface after inspector and canvas migration.
- Delete `backend/handlers/admin_assets/home_designer/outline_test.mjs`: its retained tests move to canvas and inspector suites.

---

### Task 1: Add the Workspace State Machine and Explicit Edit Lifecycle

**Files:**
- Create: `backend/handlers/admin_assets/home_designer/workspace.js`
- Create: `backend/handlers/admin_assets/home_designer/workspace_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/app.js`
- Modify: `backend/handlers/admin_templates/home_designer.html`
- Modify: `backend/handlers/admin_ui_internal_test.go`

**Interfaces:**
- Consumes: numeric editor workspace width from `ResizeObserver` and UI events from `app.js`.
- Produces: `bandForWidth(width): 'compact' | 'standard' | 'wide'`, `createWorkspaceState(width)`, and `reduceWorkspace(state, action)` returning `{ mode, band, libraryOpen, contextTool, dragging }`.

- [ ] **Step 1: Write failing reducer tests**

```js
import assert from 'node:assert/strict';
import test from 'node:test';
import { bandForWidth, createWorkspaceState, reduceWorkspace } from './workspace.js';

test('workspace bands use actual available width boundaries', () => {
    assert.equal(bandForWidth(899), 'compact');
    assert.equal(bandForWidth(1099), 'compact');
    assert.equal(bandForWidth(1100), 'standard');
    assert.equal(bandForWidth(1439), 'standard');
    assert.equal(bandForWidth(1440), 'wide');
});

test('preview mode ignores mutation tools until edit starts', () => {
    const preview = createWorkspaceState(1280);
    assert.equal(reduceWorkspace(preview, { type: 'tool/library' }).libraryOpen, false);
    const editing = reduceWorkspace(preview, { type: 'edit/start' });
    assert.equal(editing.mode, 'edit');
    assert.equal(reduceWorkspace(editing, { type: 'tool/library' }).libraryOpen, true);
});

test('standard uses one drawer while wide allows library plus inspector', () => {
    let standard = reduceWorkspace(createWorkspaceState(1200), { type: 'edit/start' });
    standard = reduceWorkspace(standard, { type: 'tool/library' });
    standard = reduceWorkspace(standard, { type: 'tool/inspector' });
    assert.deepEqual([standard.libraryOpen, standard.contextTool], [false, 'inspector']);

    let wide = reduceWorkspace(createWorkspaceState(1600), { type: 'edit/start' });
    wide = reduceWorkspace(wide, { type: 'tool/library' });
    wide = reduceWorkspace(wide, { type: 'tool/inspector' });
    assert.deepEqual([wide.libraryOpen, wide.contextTool], [true, 'inspector']);
});

test('wide drawers can be unpinned independently', () => {
    let state = reduceWorkspace(createWorkspaceState(1600), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    assert.equal(state.libraryOpen, false);
    state = reduceWorkspace(state, { type: 'tool/inspector' });
    state = reduceWorkspace(state, { type: 'tool/inspector' });
    assert.equal(state.contextTool, null);
});

test('automatic inspector opening is idempotent', () => {
    let state = reduceWorkspace(createWorkspaceState(1200), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/inspector', open: true });
    state = reduceWorkspace(state, { type: 'tool/inspector', open: true });
    assert.equal(state.contextTool, 'inspector');
});

test('compact library collapses when dragging begins', () => {
    let state = reduceWorkspace(createWorkspaceState(1000), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    state = reduceWorkspace(state, { type: 'drag/start' });
    assert.deepEqual([state.libraryOpen, state.dragging], [false, true]);
});
```

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `node --test backend/handlers/admin_assets/home_designer/workspace_test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `workspace.js`.

- [ ] **Step 3: Implement the pure workspace reducer**

```js
export const bandForWidth = (width) => Number(width) >= 1440 ? 'wide' : Number(width) >= 1100 ? 'standard' : 'compact';

export const createWorkspaceState = (width = 0) => ({
    mode: 'preview', band: bandForWidth(width), libraryOpen: false,
    contextTool: null, dragging: false,
});

export const reduceWorkspace = (state, action) => {
    if (action.type === 'resize') {
        const band = bandForWidth(action.width);
        const exclusive = band !== 'wide' && state.libraryOpen && state.contextTool;
        return { ...state, band, libraryOpen: exclusive ? false : state.libraryOpen };
    }
    if (action.type === 'edit/start') return { ...state, mode: 'edit' };
    if (action.type === 'edit/cancel' || action.type === 'edit/applied') {
        return { ...state, mode: 'preview', libraryOpen: false, contextTool: null, dragging: false };
    }
    if (state.mode !== 'edit') return state;
    if (action.type === 'tool/library') {
        const libraryOpen = action.open === true ? true : action.open === false ? false : !state.libraryOpen;
        return {
            ...state, libraryOpen,
            contextTool: libraryOpen && state.band !== 'wide' ? null : state.contextTool,
        };
    }
    if (action.type === 'tool/inspector' || action.type === 'tool/theme') {
        const wanted = action.type === 'tool/theme' ? 'theme' : 'inspector';
        const contextTool = action.open === true ? wanted : action.open === false ? null : state.contextTool === wanted ? null : wanted;
        return {
            ...state, libraryOpen: state.band === 'wide' ? state.libraryOpen : false,
            contextTool,
        };
    }
    if (action.type === 'drag/start') return {
        ...state, dragging: true, libraryOpen: state.band === 'compact' ? false : state.libraryOpen,
    };
    if (action.type === 'drag/end') return { ...state, dragging: false };
    return state;
};
```

- [ ] **Step 4: Integrate Edit Home, Apply, and Cancel mode actions**

Add semantic buttons to the toolbar:

```html
<button type="button" class="btn btn-primary" data-home-designer-edit>Edit Home</button>
<button type="button" class="btn btn-secondary" data-home-designer-cancel hidden>Cancel</button>
<button type="button" class="btn btn-primary" data-home-designer-apply hidden>Apply</button>
```

In `app.js`, keep a `workspaceState`, dispatch reducer actions through one render function, observe `editor.getBoundingClientRect().width`, call `store.discard()` after confirmed Cancel, and dispatch `edit/applied` only after `applyDocument` succeeds. Set `editor.dataset.mode` and `editor.dataset.band`; hide Undo, Redo, Add Rows, Customize, Theme, Apply, and Cancel outside edit mode.

- [ ] **Step 5: Extend the Go template test for the lifecycle controls**

```go
for _, marker := range []string{
    `data-home-designer-edit`,
    `data-home-designer-cancel`,
    `data-home-designer-apply`,
    `data-home-designer-tool="library"`,
    `data-home-designer-tool="inspector"`,
    `data-home-designer-tool="theme"`,
} {
    if !strings.Contains(source, marker) {
        t.Fatalf("Home Designer template missing edit lifecycle marker %q", marker)
    }
}
```

- [ ] **Step 6: Run focused and regression tests**

Run: `node --test backend/handlers/admin_assets/home_designer/workspace_test.mjs backend/handlers/admin_assets/home_designer/store_test.mjs`

Expected: PASS.

Run: `go test ./backend/handlers -run 'TestHomeDesignerTemplateProvidesAccessibleEditorStructure|TestHomeDesigner'`

Expected: PASS.

- [ ] **Step 7: Commit the edit lifecycle**

```bash
git add backend/handlers/admin_assets/home_designer/workspace.js backend/handlers/admin_assets/home_designer/workspace_test.mjs backend/handlers/admin_assets/home_designer/app.js backend/handlers/admin_templates/home_designer.html backend/handlers/admin_ui_internal_test.go
git commit -m "feat(home-designer): add explicit edit lifecycle"
```

### Task 2: Extract the Inspector from the Legacy Ordering Module

**Files:**
- Create: `backend/handlers/admin_assets/home_designer/inspector.js`
- Create: `backend/handlers/admin_assets/home_designer/inspector_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/outline.js`
- Modify: `backend/handlers/admin_assets/home_designer/outline_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/app.js`

**Interfaces:**
- Consumes: the existing `state`, `dispatch`, `onSelect`, `onCatalogSubmit`, `onFieldEdit`, and `sectionValidation` options.
- Produces: `renderInspector(container, options)` from `inspector.js` with the same behavior and DOM attributes currently exported by `outline.js`.

- [ ] **Step 1: Create a failing inspector import test**

Move the four inspector-specific tests from `outline_test.mjs` into `inspector_test.mjs` and change the import to:

```js
import { renderInspector } from './inspector.js';
```

Retain the existing tests for collection rendering, the 20-item cap, admin form classes, and accessible server field errors without weakening assertions.

- [ ] **Step 2: Run the inspector test and verify failure**

Run: `node --test backend/handlers/admin_assets/home_designer/inspector_test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `inspector.js`.

- [ ] **Step 3: Move the inspector implementation without behavior changes**

Move `fieldControl`, `fieldError`, `collectionEditor`, `renderInspector`, and their private `text`, `copy`, and `button` dependencies from the current `outline.js` lines 170–339 into `inspector.js` byte-for-byte. Keep this public parameter contract unchanged:

```js
renderInspector(container, {
    state, dispatch, onSelect, onCatalogSubmit, onFieldEdit, sectionValidation,
});
```

Remove those functions from `outline.js`, leaving only ordering behavior until Task 5 retires it.

- [ ] **Step 4: Switch the application import**

Change the editor module load to import library, outline, and inspector separately:

```js
editorModules = Promise.all([import('./library.js'), import('./outline.js'), import('./inspector.js')]);
const [library, outline, inspector] = await editorModules;
```

Then call:

```js
inspector.renderInspector(findDesignerElement('[data-home-designer-inspector]'), inspectorOptions);
```

- [ ] **Step 5: Run both focused suites**

Run: `node --test backend/handlers/admin_assets/home_designer/inspector_test.mjs backend/handlers/admin_assets/home_designer/outline_test.mjs`

Expected: PASS, with ordering tests remaining in `outline_test.mjs` and inspector tests passing from the new module.

- [ ] **Step 6: Commit the extraction**

```bash
git add backend/handlers/admin_assets/home_designer/inspector.js backend/handlers/admin_assets/home_designer/inspector_test.mjs backend/handlers/admin_assets/home_designer/outline.js backend/handlers/admin_assets/home_designer/outline_test.mjs backend/handlers/admin_assets/home_designer/app.js
git commit -m "refactor(home-designer): isolate row inspector"
```

### Task 3: Make Add and Remove Selection-Aware

**Files:**
- Modify: `backend/handlers/admin_assets/home_designer/store.js`
- Modify: `backend/handlers/admin_assets/home_designer/store_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/library.js`
- Modify: `backend/handlers/admin_assets/home_designer/library_test.mjs`

**Interfaces:**
- Consumes: `state.rows`, `state.selectionId`, existing `rows/add` and `rows/remove` actions.
- Produces: `defaultInsertionIndex(rows, selectionId): number`; `rows/remove` selects the next row, previous row, or `null` within the same committed mutation.

- [ ] **Step 1: Add failing store selection tests**

```js
test('removing the selected row chooses its next neighbor then previous neighbor', async () => {
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'selection/select', id: 'watchlist' });
    store.dispatch({ type: 'rows/remove', id: 'watchlist' });
    assert.equal(store.getState().selectionId, 'trending');
    store.dispatch({ type: 'rows/remove', id: 'trending' });
    assert.equal(store.getState().selectionId, 'top-ten');
    store.dispatch({ type: 'rows/remove', id: 'top-ten' });
    assert.equal(store.getState().selectionId, null);
});

test('indexed insertion selects the inserted row and remains one undo step', async () => {
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'rows/add', row: { id: 'genres', name: 'Genres' }, index: 1 });
    assert.deepEqual(store.getState().rows.map((row) => row.id), ['top-ten', 'genres', 'watchlist', 'trending']);
    assert.equal(store.getState().selectionId, 'genres');
    store.undo();
    assert.deepEqual(store.getState().rows.map((row) => row.id), ['top-ten', 'watchlist', 'trending']);
});
```

- [ ] **Step 2: Add a failing library insertion-position test**

```js
import { defaultInsertionIndex } from './library.js';

test('accessible Add defaults after selection or to the end', () => {
    const rows = [{ id: 'one' }, { id: 'two' }, { id: 'three' }];
    assert.equal(defaultInsertionIndex(rows, 'two'), 2);
    assert.equal(defaultInsertionIndex(rows, null), 3);
    assert.equal(defaultInsertionIndex(rows, 'missing'), 3);
});
```

- [ ] **Step 3: Run the tests and verify failures**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/library_test.mjs`

Expected: FAIL because removal clears selection and `defaultInsertionIndex` is not exported.

- [ ] **Step 4: Implement deterministic removal selection in the store**

Before filtering the row in the `rows/remove` case, capture its index and assign selection only when the removed row is selected:

```js
const index = state.rows.shelves.indexOf(row);
const nextSelection = state.rows.shelves[index + 1]?.id || state.rows.shelves[index - 1]?.id || null;
state.rows.shelves = state.rows.shelves.filter((candidate) => candidate.id !== action.id);
if (state.selectionId === action.id) state.selectionId = nextSelection;
```

- [ ] **Step 5: Implement selected-row Add placement**

```js
export const defaultInsertionIndex = (rows = [], selectionId = null) => {
    const selected = rows.findIndex((row) => row.id === selectionId);
    return selected < 0 ? rows.length : selected + 1;
};
```

In `renderLibrary`, use `defaultInsertionIndex(state.rows, state.selectionId)` for the button Add path and preserve an explicitly supplied drop index for configured catalog entries.

- [ ] **Step 6: Run focused tests**

Run: `node --test backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/library_test.mjs`

Expected: PASS.

- [ ] **Step 7: Commit selection-aware row operations**

```bash
git add backend/handlers/admin_assets/home_designer/store.js backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/library.js backend/handlers/admin_assets/home_designer/library_test.mjs
git commit -m "feat(home-designer): make row actions position aware"
```

### Task 4: Build the Vertical Canvas Interaction Controller

**Files:**
- Create: `backend/handlers/admin_assets/home_designer/canvas.js`
- Create: `backend/handlers/admin_assets/home_designer/canvas_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/preview.js`
- Modify: `backend/handlers/admin_assets/home_designer/preview_test.mjs`

**Interfaces:**
- Consumes: preview DOM rows marked with `data-preview-row-id`, current `state.rows`, `state.selectionId`, the store `dispatch`, catalog drop tokens, and callbacks supplied by `app.js`.
- Produces: `isCanvasDrop(types)`, `insertionIndex(rows, targetID, after)`, `moveDestination(rows, sourceID, rawIndex)`, `removalFocusTarget(rows, rowID)`, `edgeScrollDelta(pointerY, rect, threshold, maximum)`, and `mountCanvasInteractions(host, options): () => void`.

- [ ] **Step 1: Write failing pure interaction tests**

```js
import assert from 'node:assert/strict';
import test from 'node:test';
import {
    edgeScrollDelta, insertionIndex, isCanvasDrop, moveDestination, removalFocusTarget,
} from './canvas.js';

const rows = [{ id: 'one' }, { id: 'two' }, { id: 'three' }];

test('canvas recognizes catalog and row drags from advertised types', () => {
    assert.equal(isCanvasDrop(['application/x-home-designer-catalog']), true);
    assert.equal(isCanvasDrop(['application/x-home-designer-row']), true);
    assert.equal(isCanvasDrop(['text/plain']), false);
});

test('drop position adjusts when a source moves downward', () => {
    assert.equal(insertionIndex(rows, 'three', true), 3);
    assert.equal(moveDestination(rows, 'one', 3), 2);
    assert.equal(moveDestination(rows, 'three', 0), 0);
});

test('removal focus chooses next, previous, then canvas', () => {
    assert.equal(removalFocusTarget(rows, 'two'), 'three');
    assert.equal(removalFocusTarget(rows, 'three'), 'two');
    assert.equal(removalFocusTarget([{ id: 'one' }], 'one'), null);
});

test('edge scrolling is directional only inside the threshold', () => {
    const rect = { top: 100, bottom: 500 };
    assert.equal(edgeScrollDelta(110, rect, 48, 18), -18);
    assert.equal(edgeScrollDelta(490, rect, 48, 18), 18);
    assert.equal(edgeScrollDelta(300, rect, 48, 18), 0);
});
```

- [ ] **Step 2: Run the canvas test and verify failure**

Run: `node --test backend/handlers/admin_assets/home_designer/canvas_test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `canvas.js`.

- [ ] **Step 3: Implement the pure canvas helpers**

```js
export const isCanvasDrop = (types) => {
    const offered = new Set(Array.from(types || []));
    return offered.has('application/x-home-designer-catalog') || offered.has('application/x-home-designer-row');
};

export const insertionIndex = (rows, targetID, after) => {
    const index = rows.findIndex((row) => row.id === targetID);
    return index < 0 ? rows.length : index + (after ? 1 : 0);
};

export const moveDestination = (rows, sourceID, rawIndex) => {
    const source = rows.findIndex((row) => row.id === sourceID);
    return Math.max(0, Math.min(rawIndex - (source >= 0 && source < rawIndex ? 1 : 0), rows.length - 1));
};

export const removalFocusTarget = (rows, rowID) => {
    const index = rows.findIndex((row) => row.id === rowID);
    return rows[index + 1]?.id || rows[index - 1]?.id || null;
};

export const edgeScrollDelta = (pointerY, rect, threshold = 48, maximum = 18) =>
    pointerY < rect.top + threshold ? -maximum : pointerY > rect.bottom - threshold ? maximum : 0;
```

- [ ] **Step 4: Add stable preview row hooks**

Make schematic rendering independent of preview-content network results. Add a row-level type mapper and create a fixed structural item set:

```js
export const schematicKindForRow = (row = {}) => {
    const value = `${row.type || ''} ${row.mediaType || ''} ${row.id || ''}`.toLowerCase();
    if (value.includes('live') || value.includes('channel')) return 'live';
    if (value.includes('series') || value.includes('show') || value.includes('tv')) return 'series';
    if (value.includes('movie') || value.includes('film')) return 'movie';
    return 'content';
};

export const schematicItems = (row, count) => Array.from(
    { length: count },
    (_, index) => ({ id: `${row.id}-schematic-${index + 1}`, mediaType: schematicKindForRow(row) }),
);
```

Use five items for TV rows and four for mobile rows without waiting for preview API results. Keep `createPreviewController` available for a future realistic-content mode, but the default plan builders and `app.js` must not schedule it. Accept an `editing` option in both renderers: preview mode omits rows with `enabled === false`, while edit mode retains them with `data-row-enabled="false"`, a visible **Hidden** state, and reduced opacity so they can be selected and restored. Ensure every rendered row exposes:

```js
section.dataset.previewRowId = row.id;
section.dataset.rowEnabled = String(row.enabled !== false);
heading.dataset.homeDesignerRowSelect = row.id;
heading.textContent = text(row.name, 'Untitled row');
```

Add preview tests that call both plan builders with no result map and assert row IDs, labels, and fixed schematic item counts survive TV and mobile render paths. Assert a disabled row is absent with `{ editing: false }` and present with `{ editing: true }`. Assert schematic tiles never render poster image elements.

- [ ] **Step 5: Implement `mountCanvasInteractions`**

The function must have this public signature and return its cleanup function:

```js
const cleanup = mountCanvasInteractions(host, {
    state, editing, dispatch, liveRegion, onSelect, onCustomize,
    onCatalogDrop, onDragStart, onDragEnd,
});
cleanup();
```

For each `[data-preview-row-id]` while `editing` is true, append a labeled drag handle and Show/Hide, Move Up, Move Down, and Remove buttons. Use one `dragover`/`drop` listener on the preview content and one `.home-designer-drop-indicator`. The drop handler must read `application/x-home-designer-catalog` first, then `application/x-home-designer-row`, and execute exactly one branch:

```js
if (catalogToken) await onCatalogDrop(catalogToken, rawIndex);
else if (rowID) dispatch({ type: 'rows/move', id: rowID, to: moveDestination(state.rows, rowID, rawIndex) });
```

During dragover compute `const delta = edgeScrollDelta(event.clientY, viewport.getBoundingClientRect(), 48, 18)` and call `viewport.scrollBy({ top: delta, behavior: 'auto' })` only when `delta !== 0`. Escape and dragend remove the indicator and call `onDragEnd` without dispatching.

- [ ] **Step 6: Add a DOM-level affordance test**

Use the existing lightweight fake `Element` pattern to render two `[data-preview-row-id]` elements, mount editing interactions, and assert that each receives a drag handle plus Move Up, Move Down, and Remove buttons; mount with `editing: false` and assert no mutation controls are appended.

- [ ] **Step 7: Run canvas and preview tests**

Run: `node --test backend/handlers/admin_assets/home_designer/canvas_test.mjs backend/handlers/admin_assets/home_designer/preview_test.mjs`

Expected: PASS.

- [ ] **Step 8: Commit the canvas controller**

```bash
git add backend/handlers/admin_assets/home_designer/canvas.js backend/handlers/admin_assets/home_designer/canvas_test.mjs backend/handlers/admin_assets/home_designer/preview.js backend/handlers/admin_assets/home_designer/preview_test.mjs
git commit -m "feat(home-designer): add direct row canvas controls"
```

### Task 5: Integrate Direct Canvas Editing and Retire the Outline

**Files:**
- Modify: `backend/handlers/admin_assets/home_designer/app.js`
- Modify: `backend/handlers/admin_assets/home_designer/library.js`
- Modify: `backend/handlers/admin_assets/home_designer/library_test.mjs`
- Modify: `backend/handlers/admin_templates/home_designer.html`
- Modify: `backend/handlers/admin_ui_internal_test.go`
- Delete: `backend/handlers/admin_assets/home_designer/outline.js`
- Delete: `backend/handlers/admin_assets/home_designer/outline_test.mjs`

**Interfaces:**
- Consumes: `mountCanvasInteractions`, `renderInspector`, `renderLibrary`, `createCatalogRows`, `findCatalogEntry`, and the workspace reducer from prior tasks.
- Produces: one application-level `addCatalogAt(token, index)` path used by pointer drop, accessible Add, and configured catalog submission.

- [ ] **Step 1: Add failing library drag lifecycle assertions**

Extend the fake element to retain listeners and invoke the Add button's `dragstart` and `dragend` handlers:

```js
test('library reports drag lifecycle without mutating rows', () => {
    const host = new Element('aside');
    const events = [];
    const state = {
        rows: [],
        catalog: [{
            type: 'genre', name: 'Genre', available: true,
            default: { id: 'genre', name: 'Genre', type: 'genre', enabled: true },
        }],
    };
    renderLibrary(host, {
        state,
        onDragStart: (token) => events.push(['start', token]),
        onDragEnd: () => events.push(['end']),
    });
    const addButton = find(host, (element) => element.textContent === 'Add');
    const written = new Map();
    const dataTransfer = {
        setData: (type, value) => written.set(type, value),
        effectAllowed: '',
    };
    addButton.listeners.get('dragstart')({ dataTransfer });
    addButton.listeners.get('dragend')({});
    assert.deepEqual(events, [['start', 'genre'], ['end']]);
    assert.equal(written.get('application/x-home-designer-catalog'), 'genre');
    assert.deepEqual(state.rows, []);
});
```

- [ ] **Step 2: Run the library test and verify failure**

Run: `node --test backend/handlers/admin_assets/home_designer/library_test.mjs`

Expected: FAIL because `renderLibrary` does not call `onDragStart` or `onDragEnd`.

- [ ] **Step 3: Implement one catalog insertion path in `app.js`**

After removing the outline import, define the shared editor module order once and use it consistently:

```js
editorModules = Promise.all([import('./library.js'), import('./inspector.js'), import('./canvas.js')]);
const [library, inspector, canvas] = await editorModules;
const openWorkspaceTool = (tool) => updateWorkspace({ type: `tool/${tool}`, open: true });
```

```js
const addCatalogAt = async (token, index) => {
    const [library] = await editorModules;
    const state = store.getState();
    const entry = library.findCatalogEntry(state.catalog, token);
    if (!entry) return false;
    if (entry.catalogOnly) {
        configureCatalog(entry, index);
        openWorkspaceTool('inspector');
        return true;
    }
    const rows = library.createCatalogRows(entry, state.rows);
    rows.forEach((row, offset) => store.dispatch({ type: 'rows/add', row, index: index + offset }));
    if (rows[0]) handleAddedRow(rows[0].id, entry);
    return rows.length > 0;
};
```

Use this function for canvas drops. Keep `renderLibrary` button Add position-aware; configured catalog submission uses the saved `catalogSelection.index` and then opens Customize for the inserted row.

- [ ] **Step 4: Mount canvas interactions after every preview render**

Store the returned cleanup function in `canvasCleanup`, call it before `host.replaceChildren()` or before mounting again, and pass:

```js
canvas.mountCanvasInteractions(host, {
    state, editing: workspaceState.mode === 'edit', dispatch: store.dispatch,
    liveRegion: editor.querySelector('[data-home-designer-live]'),
    onSelect: (id) => selectRow(id, 'preview'),
    onCustomize: (id) => { selectRow(id, 'preview'); openWorkspaceTool('inspector'); },
    onCatalogDrop: addCatalogAt,
    onDragStart: () => updateWorkspace({ type: 'drag/start' }),
    onDragEnd: () => updateWorkspace({ type: 'drag/end' }),
});
```

Pass `editing: workspaceState.mode === 'edit'` to `renderTVPreview` or `renderMobilePreview` before mounting canvas interactions. Remove `previewResults`, `previewRowsSignature`, `previewObserver`, controller scheduling, visibility observation, retry wiring, and invalidation calls from the default `app.js` path. Platform, row, and theme changes should synchronously rebuild the schematic preview; no `/admin/api/home-designer/preview` request is required for editing.

- [ ] **Step 5: Make selection and validation open the correct drawer**

When a row is selected during edit mode, dispatch `{ type: 'tool/inspector', open: true }` so repeated selection never toggles the drawer closed. When `focusFirstInvalid` or a row-scoped server validation error runs, open Inspector before scheduling field focus. When a theme error runs, dispatch `{ type: 'tool/theme', open: true }` before focusing its field. Dropping a new row must select it and open Inspector automatically.

- [ ] **Step 6: Remove the composition outline**

Remove `[data-home-designer-outline]` from the template and `renderOutline` from `app.js`. Delete `outline.js` and `outline_test.mjs` only after canvas and inspector suites contain all retained helper and rendering assertions. Change the Go template test to reject reintroduction:

```go
if strings.Contains(source, `data-home-designer-outline`) {
    t.Fatal("Home Designer must order rows directly in the preview, not a duplicate outline")
}
```

- [ ] **Step 7: Run all Home Designer browser tests and focused Go tests**

Run: `node --test backend/handlers/admin_assets/home_designer/*_test.mjs`

Expected: PASS with no import of `outline.js`.

Run: `go test ./backend/handlers -run 'TestHomeDesignerTemplateProvidesAccessibleEditorStructure|TestHomeDesigner'`

Expected: PASS.

- [ ] **Step 8: Commit direct-canvas integration**

```bash
git add -A backend/handlers/admin_assets/home_designer backend/handlers/admin_templates/home_designer.html backend/handlers/admin_ui_internal_test.go
git commit -m "refactor(home-designer): order rows in the preview"
```

### Task 6: Implement Adaptive Drawers and Theme Placement

**Files:**
- Modify: `backend/handlers/admin_assets/home_designer/app.js`
- Modify: `backend/handlers/admin_assets/home_designer/workspace.js`
- Modify: `backend/handlers/admin_assets/home_designer/workspace_test.mjs`
- Modify: `backend/handlers/admin_assets/home_designer/theme.js`
- Modify: `backend/handlers/admin_templates/home_designer.html`
- Modify: `backend/handlers/admin_assets/home_designer/home_designer.css`
- Modify: `backend/handlers/admin_ui_internal_test.go`

**Interfaces:**
- Consumes: `workspaceState.band`, `libraryOpen`, `contextTool`, and `dragging`.
- Produces: editor classes `is-band-wide`, `is-band-standard`, `is-band-compact`, `is-library-open`, `is-inspector-open`, `is-theme-open`, and `is-dragging`; compact-only modal drawer behavior.

- [ ] **Step 1: Extend failing reducer tests for resize normalization**

```js
test('resizing from wide to standard keeps the context drawer and closes the library', () => {
    let state = reduceWorkspace(createWorkspaceState(1600), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    state = reduceWorkspace(state, { type: 'tool/inspector' });
    state = reduceWorkspace(state, { type: 'resize', width: 1200 });
    assert.deepEqual([state.band, state.libraryOpen, state.contextTool], ['standard', false, 'inspector']);
});

test('apply and cancel close every drawer and clear drag state', () => {
    let state = reduceWorkspace(createWorkspaceState(1000), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/theme' });
    state = reduceWorkspace(state, { type: 'drag/start' });
    assert.deepEqual(reduceWorkspace(state, { type: 'edit/applied' }), createWorkspaceState(1000));
});
```

- [ ] **Step 2: Run reducer tests and verify the new assertions fail if normalization is incomplete**

Run: `node --test backend/handlers/admin_assets/home_designer/workspace_test.mjs`

Expected: FAIL until resize and exit transitions exactly match the asserted state.

- [ ] **Step 3: Restructure the semantic workspace shell**

Use one narrow tool rail and two drawer positions:

```html
<nav class="home-designer-tool-rail" aria-label="Home Designer tools">
  <button type="button" data-home-designer-tool="library" aria-controls="homeDesignerLibrary">Add Rows</button>
  <button type="button" data-home-designer-tool="inspector" aria-controls="homeDesignerInspector">Customize</button>
  <button type="button" data-home-designer-tool="theme" aria-controls="homeDesignerTheme">Theme</button>
</nav>
<aside id="homeDesignerLibrary" data-home-designer-library aria-label="Row library"></aside>
<section class="home-designer-preview" data-home-designer-canvas aria-label="Home row canvas">…</section>
<aside class="home-designer-context-drawer" aria-label="Home Designer controls">
  <section id="homeDesignerInspector" data-home-designer-inspector aria-label="Row inspector"></section>
  <section id="homeDesignerTheme" data-home-designer-theme aria-label="Theme editor"></section>
</aside>
```

- [ ] **Step 4: Replace viewport matching with workspace observation**

Remove `matchMedia('(max-width: 1100px)')`. Use `ResizeObserver` on `.home-designer-workspace`; dispatch `{ type: 'resize', width: entry.contentRect.width }`. Apply the six state classes listed in Interfaces after every workspace transition.

Only compact drawers receive `role="dialog"`, `aria-modal="true"`, backdrop, inert background, portal behavior, Escape close, and focus trap. Standard and wide drawers remain in document flow and do not mark the canvas inert.

- [ ] **Step 5: Implement the three CSS bands**

Use named grid areas so the active standard drawer occupies the same slot whether it is the library or context drawer:

```css
.home-designer-workspace {
  display:grid;
  grid-template-columns:3rem minmax(0,1fr);
  grid-template-areas:"rail canvas";
}
.home-designer-tool-rail { grid-area:rail; }
.home-designer-library { grid-area:library; }
.home-designer-preview { grid-area:canvas; min-width:0; }
.home-designer-context-drawer { grid-area:context; }
.home-designer-editor.is-band-standard.is-library-open .home-designer-workspace,
.home-designer-editor.is-band-standard.is-inspector-open .home-designer-workspace,
.home-designer-editor.is-band-standard.is-theme-open .home-designer-workspace {
  grid-template-columns:3rem 19rem minmax(40rem,1fr);
  grid-template-areas:"rail drawer canvas";
}
.home-designer-editor.is-band-standard.is-library-open .home-designer-library,
.home-designer-editor.is-band-standard.is-inspector-open .home-designer-context-drawer,
.home-designer-editor.is-band-standard.is-theme-open .home-designer-context-drawer { grid-area:drawer; }
.home-designer-editor.is-band-wide.is-library-open .home-designer-workspace {
  grid-template-columns:3rem 19rem minmax(40rem,1fr);
  grid-template-areas:"rail library canvas";
}
.home-designer-editor.is-band-wide:is(.is-inspector-open,.is-theme-open) .home-designer-workspace {
  grid-template-columns:3rem minmax(40rem,1fr) 21rem;
  grid-template-areas:"rail canvas context";
}
.home-designer-editor.is-band-wide.is-library-open:is(.is-inspector-open,.is-theme-open) .home-designer-workspace {
  grid-template-columns:3rem 19rem minmax(40rem,1fr) 21rem;
  grid-template-areas:"rail library canvas context";
}
.home-designer-editor.is-band-compact .home-designer-library,
.home-designer-editor.is-band-compact .home-designer-context-drawer {
  position:fixed; inset-block:0; width:min(22rem,calc(100vw - 3rem)); z-index:1001;
}
```

Hide inactive library and context drawers with `display:none`; set the drawer participating in the selected grid area to `display:block`. In preview mode hide the rail, drawers, row handles, insertion indicators, and mutation buttons. In compact drag state hide the overlay drawer while leaving a `.home-designer-drag-token` and all insertion points visible.

- [ ] **Step 6: Keep Theme in the context drawer**

Render `theme.js` into `#homeDesignerTheme`; toggle it with `contextTool === 'theme'`. Do not change `theme/field`, `theme/customize`, `theme/replace`, or `theme/reset` store actions. Preserve preview-scoped CSS variables and current validation attributes.

- [ ] **Step 7: Verify structure, reducer, and all browser modules**

Run: `node --test backend/handlers/admin_assets/home_designer/*_test.mjs`

Expected: PASS.

Run: `go test ./backend/handlers -run 'TestHomeDesignerTemplateProvidesAccessibleEditorStructure|TestHomeDesigner'`

Expected: PASS.

- [ ] **Step 8: Commit responsive drawers**

```bash
git add backend/handlers/admin_assets/home_designer/app.js backend/handlers/admin_assets/home_designer/workspace.js backend/handlers/admin_assets/home_designer/workspace_test.mjs backend/handlers/admin_assets/home_designer/theme.js backend/handlers/admin_templates/home_designer.html backend/handlers/admin_assets/home_designer/home_designer.css backend/handlers/admin_ui_internal_test.go
git commit -m "feat(home-designer): adapt editor drawers by workspace width"
```

### Task 7: Verify Accessibility, Responsiveness, and Publish the Test Image

**Files:**
- Modify only if verification finds a defect: files introduced or modified in Tasks 1–6.
- Verify: `.github/workflows/publish-home-designer-image.yml`

**Interfaces:**
- Consumes: the complete direct-manipulation editor and the existing GitHub Actions workflow.
- Produces: passing local tests, browser evidence for each width band, and `ghcr.io/jeor/mediastorm:home-designer` plus the commit-specific `home-designer-<sha>` image.

- [ ] **Step 1: Run the complete Home Designer Node suite**

Run: `node --test backend/handlers/admin_assets/home_designer/*_test.mjs`

Expected: every test passes; output contains zero failures, cancellations, or skipped tests.

- [ ] **Step 2: Run focused Go tests**

Run: `go test ./backend/handlers -run 'HomeDesigner|AdminUI'`

Expected: PASS.

If Go is unavailable on the local host, record that limitation and require the GitHub Actions Go compilation inside the Docker build to pass before reporting completion.

- [ ] **Step 3: Run whitespace and repository-scope checks**

Run: `git diff --check`

Expected: no output.

Run: `git status --short --branch`

Expected: `build/home-designer-image` with no unrelated working-tree changes.

- [ ] **Step 4: Verify preview and edit mode in a browser**

At editor workspace widths 1600, 1280, and 1000 px, verify:

1. Preview mode has no handles, insertion points, or mutation controls.
2. Edit Home reveals Add Rows, Customize, Theme, Undo, Redo, Apply, and Cancel.
3. Existing rows move only vertically and one drop creates one undo step.
4. A library row can be dropped before, between, and after rows.
5. The dropped row is selected and Customize opens automatically.
6. Add, Move Up, Move Down, Show/Hide, and Remove work without dragging.
7. Wide permits both drawers; standard permits one; compact overlay collapses during drag.
8. Theme updates the schematic preview without restyling the admin shell.
9. Cancel restores the saved document; failed Apply preserves the draft.
10. Browser console contains no errors and the document has no horizontal page overflow.

- [ ] **Step 5: Verify keyboard and zoom behavior**

At 200 percent browser zoom, verify tool controls remain reachable, drawers do not overlap Apply/Cancel, Escape cancels a drag or closes only a compact modal drawer, focus returns after Add/Remove, and live announcements identify row positions.

- [ ] **Step 6: Commit any verification fixes**

If verification changed product files, rerun Steps 1–5, then commit only those fixes:

```bash
git add backend/handlers/admin_assets/home_designer backend/handlers/admin_templates/home_designer.html backend/handlers/admin_ui_internal_test.go
git commit -m "fix(home-designer): address direct editor verification"
```

If verification required no changes, do not create an empty commit.

- [ ] **Step 7: Push only the feature branch to the Jeor fork**

Run: `git remote get-url origin`

Expected: `https://github.com/Jeor/mediastorm.git`.

Run: `git push origin build/home-designer-image`

Expected: the feature branch updates successfully; no default or upstream branch is targeted.

- [ ] **Step 8: Wait for or dispatch the image workflow**

The push should trigger `publish-home-designer-image.yml` because backend files changed. Inspect the newest run:

```bash
gh run list --workflow publish-home-designer-image.yml --branch build/home-designer-image --limit 1
gh run watch --exit-status $(gh run list --workflow publish-home-designer-image.yml --branch build/home-designer-image --limit 1 --json databaseId --jq '.[0].databaseId')
```

If no run was created, dispatch it explicitly:

```bash
gh workflow run publish-home-designer-image.yml --ref build/home-designer-image
```

Then repeat the run-list and run-watch commands. Expected: Buildx publishes linux/amd64 and linux/arm64 images and the manifest inspection step passes.

- [ ] **Step 9: Report testable image tags**

Run: `git rev-parse --short HEAD`

Report:

```text
ghcr.io/jeor/mediastorm:home-designer
ghcr.io/jeor/mediastorm:home-designer-<short-sha>
```

Also report the GitHub Actions run URL, local test results, browser widths verified, and confirmation that nothing was merged or pushed upstream.
