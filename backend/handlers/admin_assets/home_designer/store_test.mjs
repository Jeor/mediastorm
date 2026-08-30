import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const moduleFromFile = async (name) => {
    const source = await readFile(new URL(`./${name}`, import.meta.url), 'utf8');
    return import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);
};

const documentFixture = () => ({
    scope: { kind: 'profile', profileId: 'profile-1' },
    revision: 'revision-1',
    rows: {
        inherited: true,
        effective: {
            shelves: [
                { id: 'top-ten', name: 'Top Ten', enabled: true, order: 0 },
                { id: 'watchlist', name: 'Watchlist', enabled: true, order: 1 },
                { id: 'trending', name: 'Trending', enabled: true, order: 2 },
            ],
            itemCap: 20,
        },
    },
    theme: {
        inherited: true,
        effective: { accentColor: '#3f66ff', buttonStyle: 'soft' },
    },
});

test('row edits normalize order, selection, undo, and redo without touching the baseline', async () => {
    // Break caught: move/remove implementations that leave order gaps, stale selection, or cannot restore edits.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());

    store.dispatch({ type: 'rows/move', id: 'watchlist', to: 0 });
    assert.deepEqual(store.getState().rows.map((row) => row.order), [0, 1, 2]);
    assert.equal(store.getState().rows[0].id, 'watchlist');
    assert.equal(store.getState().selectionId, 'watchlist');

    store.dispatch({ type: 'rows/visibility', id: 'watchlist', enabled: false });
    store.dispatch({ type: 'rows/field', id: 'watchlist', path: 'name', value: 'Saved list' });
    assert.deepEqual(store.getState().rows[0], { id: 'watchlist', name: 'Saved list', enabled: false, order: 0 });
    assert.equal(store.isDirty(), true);

    store.undo();
    assert.equal(store.getState().rows[0].name, 'Watchlist');
    store.redo();
    assert.equal(store.getState().rows[0].name, 'Saved list');

    store.dispatch({ type: 'rows/remove', id: 'watchlist' });
    assert.equal(store.getState().selectionId, null);
    assert.deepEqual(store.getState().rows.map((row) => row.order), [0, 1]);
    store.undo();
    assert.equal(store.getState().rows[0].id, 'watchlist');
});

test('row additions and invalid rows are local working state only', async () => {
    // Break caught: invalid additions leaking into the baseline or row validation missing blank/duplicate identifiers.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());

    store.dispatch({ type: 'rows/add', row: { id: '', name: '', enabled: true } });
    assert.deepEqual(store.getInvalidRowIDs(), ['']);
    assert.equal(store.getState().rows.at(-1).order, 3);
    store.discard();
    assert.equal(store.getState().rows.length, 3);
    assert.equal(store.isDirty(), false);
});

test('Rows and Theme customize/reset modes are independent and apply only dirty sections', async () => {
    // Break caught: one section's inheritance action incorrectly changing the other section or emitting an incomplete apply request.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());

    store.dispatch({ type: 'rows/customize' });
    store.dispatch({ type: 'rows/field', id: 'top-ten', path: 'enabled', value: false });
    let request = store.buildApplyRequest();
    assert.deepEqual(request, {
        scope: { kind: 'profile', profileId: 'profile-1' },
        expectedRevision: 'revision-1',
        rows: {
            mode: 'custom',
            value: {
                shelves: [
                    { id: 'top-ten', name: 'Top Ten', enabled: false, order: 0 },
                    { id: 'watchlist', name: 'Watchlist', enabled: true, order: 1 },
                    { id: 'trending', name: 'Trending', enabled: true, order: 2 },
                ],
                itemCap: 20,
            },
        },
    });

    store.dispatch({ type: 'theme/customize' });
    store.dispatch({ type: 'theme/field', path: 'accentColor', value: '#112233' });
    store.dispatch({ type: 'rows/reset' });
    request = store.buildApplyRequest();
    assert.deepEqual(request, {
        scope: { kind: 'profile', profileId: 'profile-1' },
        expectedRevision: 'revision-1',
        theme: { mode: 'custom', value: { accentColor: '#112233', buttonStyle: 'soft' } },
    });

    store.dispatch({ type: 'theme/reset' });
    assert.equal(store.buildApplyRequest(), null);
});

test('editing an inherited section creates a custom override and reset discards that local value', async () => {
    // Break caught: an inherited field edit being sent as value-less inherit and silently discarded by the server.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'theme/field', path: 'accentColor', value: '#112233' });
    assert.deepEqual(store.buildApplyRequest().theme, {
        mode: 'custom',
        value: { accentColor: '#112233', buttonStyle: 'soft' },
    });
    store.dispatch({ type: 'theme/reset' });
    assert.equal(store.isDirty(), false);
    assert.equal(store.buildApplyRequest(), null);
});

test('selection-only changes do not consume undo history and saved replacement resets dirty state', async () => {
    // Break caught: selection clicks becoming undoable edits or a successful apply retaining stale history/revision.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'selection/select', id: 'trending' });
    assert.equal(store.canUndo(), false);

    store.dispatch({ type: 'theme/customize' });
    assert.equal(store.canUndo(), true);
    store.replaceWithSaved({ ...documentFixture(), revision: 'revision-2', theme: { inherited: false, override: { accentColor: '#112233', buttonStyle: 'soft' }, effective: { accentColor: '#112233', buttonStyle: 'soft' } } });
    assert.equal(store.isDirty(), false);
    assert.equal(store.canUndo(), false);
    assert.equal(store.getState().revision, 'revision-2');
});

test('API calls use the page base path, JSON headers, abort signals, and structured errors', async () => {
    // Break caught: calls escaping the mounted base path, discarding abort signals, or flattening server errors.
    const { APIError, applyDocument, loadDocument, loadPreview } = await moduleFromFile('api.js');
    const controller = new AbortController();
    const calls = [];
    const fetchImpl = async (url, options) => {
        calls.push({ url, options });
        if (url.endsWith('/preview')) {
            return new Response(JSON.stringify({ code: 'validation_error', message: 'Bad preview', fields: [{ path: 'rows' }] }), { status: 422, headers: { 'Content-Type': 'application/json' } });
        }
        return new Response(JSON.stringify({ revision: 'next' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    };

    await loadDocument('/account/', { kind: 'profile', profileId: 'profile-1' }, { signal: controller.signal, fetchImpl });
    await applyDocument('/account/', { scope: { kind: 'profile', profileId: 'profile-1' } }, { fetchImpl });
    await assert.rejects(
        () => loadPreview('/account/', { scope: { kind: 'profile', profileId: 'profile-1' } }, { fetchImpl }),
        (error) => error instanceof APIError && error.status === 422 && error.code === 'validation_error' && error.fields[0].path === 'rows',
    );

    assert.equal(calls[0].url, '/account/api/home-designer?scope=profile&profileId=profile-1');
    assert.equal(calls[0].options.signal, controller.signal);
    assert.equal(calls[0].options.headers.Accept, 'application/json');
    assert.equal(calls[1].url, '/account/api/home-designer');
    assert.equal(calls[1].options.method, 'PUT');
    assert.equal(calls[1].options.headers['Content-Type'], 'application/json');
});
