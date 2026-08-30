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

test('catalog additions keep built-ins unique and mark incomplete configured rows invalid until completed', async () => {
    // Break caught: catalog actions duplicating a built-in or letting a required configuration reach Apply.
    const { createStore } = await moduleFromFile('store.js');
    const document = {
        ...documentFixture(),
        catalog: [
            { type: 'builtin', available: true, default: { id: 'top-ten', name: 'Top Ten', type: 'builtin' }, fields: [{ path: 'name', required: true }] },
            { type: 'mdblist', multiple: true, available: true, default: { type: 'mdblist' }, fields: [{ path: 'listUrl', label: 'MDBList URL', required: true, type: 'url' }] },
        ],
    };
    const store = createStore(document);

    assert.equal(store.dispatch({ type: 'rows/add', row: { id: 'top-ten', name: 'Top Ten', type: 'builtin', enabled: true } }), false);
    store.dispatch({ type: 'rows/add', row: { id: 'mdblist', name: 'My list', type: 'mdblist', enabled: true } });
    store.dispatch({ type: 'rows/add', row: { id: 'mdblist', name: 'Another list', type: 'mdblist', enabled: true } });
    assert.deepEqual(store.getState().rows.slice(-2).map((row) => row.id), ['mdblist', 'mdblist-2']);
    assert.deepEqual(store.getRowValidation().mdblist, [{ path: 'listUrl', message: 'MDBList URL is required' }]);
    assert.equal(store.isApplyValid(), false);

    store.dispatch({ type: 'rows/field', id: 'mdblist', path: 'listUrl', value: 'https://mdblist.com/lists/example/json' });
    store.dispatch({ type: 'rows/field', id: 'mdblist-2', path: 'listUrl', value: 'https://mdblist.com/lists/another/json' });
    assert.deepEqual(store.getRowValidation(), {});
    assert.equal(store.isApplyValid(), true);
});

test('row validation is public and an ID rename preserves selection plus collection references', async () => {
    // Break caught: locally-invalid collection hubs appearing apply-ready or an ID edit closing the active inspector and orphaning its hub source.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore({
        ...documentFixture(),
        catalog: [{ type: 'collection-hub', available: true, fields: [{ path: 'collectionItems', type: 'collection' }] }],
        rows: { inherited: false, effective: { shelves: [
            { id: 'source', name: 'Source', type: 'genre', enabled: true, order: 0 },
            { id: 'hub', name: 'Hub', type: 'collection-hub', enabled: true, order: 1, collectionItems: [{ id: 'item-1', name: 'One', sourceShelfId: 'source', enabled: true, order: 0 }] },
        ] } },
    });
    store.dispatch({ type: 'selection/select', id: 'source' });
    store.dispatch({ type: 'rows/field', id: 'source', path: 'id', value: 'renamed-source' });
    assert.equal(store.getState().selectionId, 'renamed-source');
    assert.equal(store.getState().rows[1].collectionItems[0].sourceShelfId, 'renamed-source');

    store.dispatch({ type: 'rows/field', id: 'hub', path: 'collectionItems', value: [{ id: '', name: '', sourceShelfId: '', logoUrl: 'bad', heroArtUrl: 'https://ok.example/art.png', logoScale: 3, tintColor: 'blue' }] });
    const validation = store.getState().rowValidation;
    assert.equal(store.isApplyValid(), false);
    assert.ok(validation.hub.some((error) => error.path === 'collectionItems.0.id'));
    assert.ok(validation.hub.some((error) => error.path === 'collectionItems.0.sourceShelfId'));
    assert.ok(validation.hub.some((error) => error.path === 'collectionItems.0.logoUrl'));
    assert.ok(validation.hub.some((error) => error.path === 'collectionItems.0.logoScale'));
    assert.ok(validation.hub.some((error) => error.path === 'collectionItems.0.tintColor'));
});

test('collection hubs reject duplicate or forbidden local sources before Apply', async () => {
    // Break caught: collection controls accepting duplicate sources or the reserved streaming-services shelf until the server rejects Apply.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore({
        ...documentFixture(),
        catalog: [{ type: 'collection-hub', available: true, fields: [{ path: 'collectionItems', type: 'collection' }] }],
        rows: { inherited: false, effective: { shelves: [
            { id: 'streaming-services', name: 'Services', type: 'builtin', enabled: true, order: 0 },
            { id: 'source', name: 'Source', type: 'genre', enabled: true, order: 1 },
            { id: 'hub', name: 'Hub', type: 'collection-hub', enabled: true, order: 2, collectionItems: [
                { id: 'first', name: 'First', sourceShelfId: 'source', enabled: true, order: 0 },
                { id: 'second', name: 'Second', sourceShelfId: 'source', enabled: true, order: 1 },
                { id: 'third', name: 'Third', sourceShelfId: 'streaming-services', enabled: true, order: 2 },
            ] },
        ] } },
    });
    const errors = store.getState().rowValidation.hub;
    assert.ok(errors.some((error) => error.path === 'collectionItems.1.sourceShelfId'));
    assert.ok(errors.some((error) => error.path === 'collectionItems.2.sourceShelfId'));
    assert.equal(store.isApplyValid(), false);
});

test('collection hubs block Apply when loaded with more than twenty valid items', async () => {
    // Break caught: an over-cap hub appearing locally valid even though the persisted Home Designer contract rejects it.
    const { createStore } = await moduleFromFile('store.js');
    const sources = Array.from({ length: 21 }, (_, index) => ({ id: `source-${index}`, name: `Source ${index}`, type: 'genre', enabled: true, order: index }));
    const items = Array.from({ length: 21 }, (_, index) => ({ id: `item-${index}`, name: `Item ${index}`, sourceShelfId: `source-${index}`, enabled: true, order: index }));
    const store = createStore({
        ...documentFixture(), catalog: [{ type: 'collection-hub', available: true, fields: [{ path: 'collectionItems', type: 'collection' }] }],
        rows: { inherited: false, effective: { shelves: [...sources, { id: 'hub', name: 'Hub', type: 'collection-hub', enabled: true, order: 21, collectionItems: items }] } },
    });
    assert.equal(store.isApplyValid(), false);
    assert.ok(store.getState().rowValidation.hub.some((error) => error.path === 'collectionItems' && error.message.includes('at most 20')));
});

test('an occupied row ID rename is a true no-op that preserves selection and hub references', async () => {
    // Break caught: a colliding identity edit selecting a different row and rewriting collection references ambiguously.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore({
        ...documentFixture(), rows: { inherited: false, effective: { shelves: [
            { id: 'first', name: 'First', type: 'genre', enabled: true, order: 0 },
            { id: 'second', name: 'Second', type: 'genre', enabled: true, order: 1 },
            { id: 'hub', name: 'Hub', type: 'collection-hub', enabled: true, order: 2, collectionItems: [{ id: 'item', name: 'Item', sourceShelfId: 'second', enabled: true, order: 0 }] },
        ] } },
    });
    store.dispatch({ type: 'selection/select', id: 'second' });
    assert.equal(store.dispatch({ type: 'rows/field', id: 'second', path: 'id', value: 'first' }), false);
    assert.equal(store.getState().selectionId, 'second');
    assert.deepEqual(store.getState().rows.map((row) => row.id), ['first', 'second', 'hub']);
    assert.equal(store.getState().rows[2].collectionItems[0].sourceShelfId, 'second');
});

test('a whitespace-bearing row ID rename uses one canonical identity for row selection and collection references', async () => {
    // Break caught: trimming only selection/reference migration while saving the raw ID and orphaning the selected source.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore({
        ...documentFixture(), rows: { inherited: false, effective: { shelves: [
            { id: 'source', name: 'Source', type: 'genre', enabled: true, order: 0 },
            { id: 'hub', name: 'Hub', type: 'collection-hub', enabled: true, order: 1, collectionItems: [{ id: 'item', name: 'Item', sourceShelfId: 'source', enabled: true, order: 0 }] },
        ] } },
    });
    store.dispatch({ type: 'selection/select', id: 'source' });
    store.dispatch({ type: 'rows/field', id: 'source', path: 'id', value: ' renamed ' });
    assert.equal(store.getState().rows[0].id, 'renamed');
    assert.equal(store.getState().selectionId, 'renamed');
    assert.equal(store.getState().rows[1].collectionItems[0].sourceShelfId, 'renamed');
    assert.equal(store.isApplyValid(), true);
});

test('row addition inserts at the requested index or appends independently of template order', async () => {
    // Break caught: catalog template order changing where a new row lands or leaving selection on another row.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'rows/add', index: 1, row: { id: 'catalog-row', name: 'Catalog row', enabled: true, order: 0 } });
    assert.deepEqual(store.getState().rows.map((row) => row.id), ['top-ten', 'catalog-row', 'watchlist', 'trending']);
    assert.deepEqual(store.getState().rows.map((row) => row.order), [0, 1, 2, 3]);
    assert.equal(store.getState().selectionId, 'catalog-row');

    store.dispatch({ type: 'rows/add', row: { id: 'appended', name: 'Appended', enabled: true, order: 0 } });
    assert.equal(store.getState().rows.at(-1).id, 'appended');
    assert.equal(store.getState().rows.at(-1).order, 4);
    assert.equal(store.getState().selectionId, 'appended');
});

test('stale row removal is a true no-op', async () => {
    // Break caught: an absent row ID turning inherited Rows into a dirty custom section.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'selection/select', id: 'watchlist' });
    assert.equal(store.dispatch({ type: 'rows/remove', id: 'missing' }), false);
    assert.equal(store.isDirty(), false);
    assert.equal(store.canUndo(), false);
    assert.equal(store.getState().selectionId, 'watchlist');
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

test('reset restores inherited effective values and global sections never emit inherit', async () => {
    // Break caught: reset preserving a stale local override or sending an inherit mutation the global server contract rejects.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'theme/field', path: 'accentColor', value: '#112233' });
    store.dispatch({ type: 'theme/reset' });
    store.dispatch({ type: 'theme/customize' });
    assert.deepEqual(store.buildApplyRequest(), {
        scope: { kind: 'profile', profileId: 'profile-1' },
        expectedRevision: 'revision-1',
        theme: { mode: 'custom', value: { accentColor: '#3f66ff', buttonStyle: 'soft' } },
    });

    const global = createStore({ ...documentFixture(), scope: { kind: 'global' }, rows: { inherited: false, effective: documentFixture().rows.effective }, theme: { inherited: false, effective: documentFixture().theme.effective } });
    global.dispatch({ type: 'theme/reset' });
    global.dispatch({ type: 'rows/reset' });
    assert.equal(global.buildApplyRequest(), null);
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

test('document envelope metadata stays cloned across load and saved replacement', async () => {
    // Break caught: later catalog/preview/preset controls losing server-authoritative metadata or mutating the original response.
    const { createStore } = await moduleFromFile('store.js');
    const document = {
        ...documentFixture(),
        permissions: { canEdit: true, canEditGlobal: false },
        previewProfiles: [{ id: 'profile-1', displayName: 'One' }],
        catalog: [{ type: 'genre', name: 'Genre' }],
        themePresets: [{ id: 'night', appearance: { accentColor: '#111111' } }],
    };
    const store = createStore(document);
    const initial = store.getState();
    initial.catalog[0].name = 'Mutated outside';
    assert.equal(store.getState().catalog[0].name, 'Genre');

    const saved = { ...document, revision: 'revision-2', catalog: [{ type: 'decade', name: 'Decade' }] };
    store.replaceWithSaved(saved);
    saved.catalog[0].name = 'Mutated response';
    assert.equal(store.getState().revision, 'revision-2');
    assert.equal(store.getState().catalog[0].name, 'Decade');
    assert.deepEqual(store.getState().previewProfiles, [{ id: 'profile-1', displayName: 'One' }]);
    assert.deepEqual(store.getState().themePresets, [{ id: 'night', appearance: { accentColor: '#111111' } }]);
});

test('whole-theme replacement is a single undoable semantic edit', async () => {
    // Break caught: selecting a multi-field preset requiring several undo operations to reverse.
    const { createStore } = await moduleFromFile('store.js');
    const store = createStore(documentFixture());
    store.dispatch({ type: 'theme/replace', value: { accentColor: '#112233', buttonStyle: 'filled', buttonRadius: 'pill' } });
    assert.deepEqual(store.getState().theme, { accentColor: '#112233', buttonStyle: 'filled', buttonRadius: 'pill' });
    assert.equal(store.canUndo(), true);
    store.undo();
    assert.deepEqual(store.getState().theme, { accentColor: '#3f66ff', buttonStyle: 'soft' });
    assert.equal(store.canUndo(), false);
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
