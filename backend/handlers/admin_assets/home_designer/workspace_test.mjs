import assert from 'node:assert/strict';
import test from 'node:test';
import { bandForWidth, createWorkspaceState, reduceWorkspace } from './workspace.js';

test('workspace bands use actual available width boundaries', () => {
    // Break caught: choosing a breakpoint from viewport width or shifting either available-width boundary.
    assert.equal(bandForWidth(899), 'compact');
    assert.equal(bandForWidth(1099), 'compact');
    assert.equal(bandForWidth(1100), 'standard');
    assert.equal(bandForWidth(1439), 'standard');
    assert.equal(bandForWidth(1440), 'wide');
});

test('preview mode ignores mutation tools until edit starts', () => {
    // Break caught: mutation tools changing preview-only workspace state before an explicit edit session begins.
    const preview = createWorkspaceState(1280);
    assert.equal(reduceWorkspace(preview, { type: 'tool/library' }).libraryOpen, false);
    const editing = reduceWorkspace(preview, { type: 'edit/start' });
    assert.equal(editing.mode, 'edit');
    assert.equal(reduceWorkspace(editing, { type: 'tool/library' }).libraryOpen, true);
});

test('standard uses one drawer while wide allows library plus inspector', () => {
    // Break caught: retaining the library drawer alongside an inspector at standard width.
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
    // Break caught: toggling one wide drawer accidentally retaining or closing the other.
    let state = reduceWorkspace(createWorkspaceState(1600), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    assert.equal(state.libraryOpen, false);
    state = reduceWorkspace(state, { type: 'tool/inspector' });
    state = reduceWorkspace(state, { type: 'tool/inspector' });
    assert.equal(state.contextTool, null);
});

test('automatic inspector opening is idempotent', () => {
    // Break caught: an automatic inspector-open event toggling a visible inspector closed.
    let state = reduceWorkspace(createWorkspaceState(1200), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/inspector', open: true });
    state = reduceWorkspace(state, { type: 'tool/inspector', open: true });
    assert.equal(state.contextTool, 'inspector');
});

test('compact library collapses when dragging begins', () => {
    // Break caught: compact dragging retaining the library drawer and obstructing the workspace.
    let state = reduceWorkspace(createWorkspaceState(1000), { type: 'edit/start' });
    state = reduceWorkspace(state, { type: 'tool/library' });
    state = reduceWorkspace(state, { type: 'drag/start' });
    assert.deepEqual([state.libraryOpen, state.dragging], [false, true]);
});
