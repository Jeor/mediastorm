import assert from 'node:assert/strict';
import test from 'node:test';
import { defaultInsertionIndex, expandStreamingService, renderLibrary } from './library.js';
import { createStore } from './store.js';

const streamingEntry = () => ({
    type: 'streaming-service', catalogOnly: true,
    fields: [
        { path: 'service', options: [{ value: 'netflix', label: 'Netflix' }] },
        { path: 'media', options: [{ value: 'movies', label: 'Movies' }, { value: 'shows', label: 'TV shows' }, { value: 'both', label: 'Movies and TV shows' }] },
    ],
});

class Element {
    constructor(tagName) {
        this.tagName = tagName;
        this.children = [];
        this.dataset = {};
        this.attributes = {};
        this.listeners = new Map();
        this.className = '';
    }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    addEventListener(type, listener) { this.listeners.set(type, listener); }
    setAttribute(name, value) { this.attributes[name] = String(value); }
}

const find = (element, predicate) => {
    if (predicate(element)) return element;
    for (const child of element.children || []) {
        const match = find(child, predicate);
        if (match) return match;
    }
    return undefined;
};

test('accessible Add defaults after selection or to the end', () => {
    const rows = [{ id: 'one' }, { id: 'two' }, { id: 'three' }];
    assert.equal(defaultInsertionIndex(rows, 'two'), 2);
    assert.equal(defaultInsertionIndex(rows, null), 3);
    assert.equal(defaultInsertionIndex(rows, 'missing'), 3);
});

test('row library search uses the admin form control treatment', () => {
    // Break caught: the editor shipping a raw browser search box inside the styled admin shell.
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        const host = new Element('aside');
        renderLibrary(host, { state: { catalog: [], rows: [] } });
        const search = find(host, (element) => element.placeholder === 'Search rows');
        assert.ok(search);
        assert.equal(search.className, 'form-input home-designer-library-search');
    } finally {
        globalThis.document = previousDocument;
    }
});

test('library reports drag lifecycle without mutating rows', () => {
    // Break caught: dragging a catalog entry fails to close/reopen workspace drawers or mutates rows before drop.
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
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
    } finally {
        globalThis.document = previousDocument;
    }
});

test('accessible Add reports its position without inserting independently', () => {
    // Break caught: the button bypassing the application insertion path and losing its selection-relative index.
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        const host = new Element('aside');
        const added = [];
        const dispatched = [];
        const state = {
            selectionId: 'one',
            rows: [{ id: 'one' }, { id: 'two' }],
            catalog: [{
                type: 'genre', name: 'Genre', available: true,
                default: { id: 'genre', name: 'Genre', type: 'genre', enabled: true },
            }],
        };
        renderLibrary(host, {
            state,
            dispatch: (action) => dispatched.push(action),
            onAdd: (token, index) => added.push([token, index]),
        });
        find(host, (element) => element.textContent === 'Add').listeners.get('click')();
        assert.deepEqual(added, [['genre', 1]]);
        assert.deepEqual(dispatched, []);
        assert.deepEqual(state.rows, [{ id: 'one' }, { id: 'two' }]);
    } finally {
        globalThis.document = previousDocument;
    }
});

test('streaming expansion preserves the selected media choice with canonical apply-ready fields', () => {
    // Break caught: streamed rows using the wrong list property or silently ignoring a movies/shows/both selection.
    const entry = streamingEntry();
    const movies = expandStreamingService(entry, [], { service: 'netflix', media: 'movies' });
    const shows = expandStreamingService(entry, [], { service: 'netflix', media: 'shows' });
    const both = expandStreamingService(entry, [], { service: 'netflix', media: 'both' });
    assert.deepEqual(movies.map((row) => [row.id, row.listUrl]), [['streaming-service-movies', 'https://mdblist.com/lists/snoak/netflix-top-10-movies/json']]);
    assert.deepEqual(shows.map((row) => [row.id, row.listUrl]), [['streaming-service-shows', 'https://mdblist.com/lists/snoak/netflix-top-10-shows/json']]);
    assert.deepEqual(both.map((row) => row.id), ['streaming-service-movies', 'streaming-service-shows']);
    assert.ok(both.every((row) => row.type === 'mdblist' && typeof row.listUrl === 'string' && !Object.hasOwn(row, 'listURL')));
});

test('streaming expansion generates a distinct stable instance when prior rows collide', () => {
    // Break caught: a second configured streaming provider reusing the first provider's row IDs.
    const rows = [{ id: 'streaming-service-movies' }, { id: 'streaming-service-shows' }];
    const next = expandStreamingService(streamingEntry(), rows, { service: 'netflix', media: 'both' });
    assert.deepEqual(next.map((row) => row.id), ['streaming-service-2-movies', 'streaming-service-2-shows']);
});

test('every streaming media expansion is apply-valid against the MDBList catalog contract', () => {
    // Break caught: an apparently configured streaming choice producing a row that the local Apply gate rejects.
    for (const media of ['movies', 'shows', 'both']) {
        const rows = expandStreamingService(streamingEntry(), [], { service: 'netflix', media });
        const store = createStore({
            scope: { kind: 'global' }, revision: 'revision',
            rows: { inherited: false, effective: { shelves: [] } }, theme: { inherited: false, effective: {} },
            catalog: [{ type: 'mdblist', available: true, fields: [{ path: 'listUrl', label: 'MDBList URL', required: true, type: 'url' }] }],
        });
        rows.forEach((row) => store.dispatch({ type: 'rows/add', row }));
        assert.equal(store.isApplyValid(), true, media);
    }
});
