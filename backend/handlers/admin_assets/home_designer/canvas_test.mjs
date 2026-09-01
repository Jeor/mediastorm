import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const moduleFromFile = async (name) => {
    const source = await readFile(new URL(`./${name}`, import.meta.url), 'utf8');
    return import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);
};

const rows = [{ id: 'one', name: 'One' }, { id: 'two', name: 'Two' }, { id: 'three', name: 'Three' }];

test('canvas recognizes catalog and row drags from advertised types', async () => {
    // Break caught: an unsupported payload becoming a valid canvas drop.
    const { isCanvasDrop } = await moduleFromFile('canvas.js');
    assert.equal(isCanvasDrop(['application/x-home-designer-catalog']), true);
    assert.equal(isCanvasDrop(['application/x-home-designer-row']), true);
    assert.equal(isCanvasDrop(['text/plain']), false);
});

test('drop position adjusts when a source moves downward', async () => {
    // Break caught: moving a row past itself producing an off-by-one destination.
    const { insertionIndex, moveDestination } = await moduleFromFile('canvas.js');
    assert.equal(insertionIndex(rows, 'three', true), 3);
    assert.equal(moveDestination(rows, 'one', 3), 2);
    assert.equal(moveDestination(rows, 'three', 0), 0);
});

test('edge scrolling is directional only inside the threshold', async () => {
    // Break caught: dragover scrolling the viewport while the pointer is safely in its middle.
    const { edgeScrollDelta } = await moduleFromFile('canvas.js');
    const rect = { top: 100, bottom: 500 };
    assert.equal(edgeScrollDelta(110, rect, 48, 18), -18);
    assert.equal(edgeScrollDelta(490, rect, 48, 18), 18);
    assert.equal(edgeScrollDelta(300, rect, 48, 18), 0);
});

class Element {
    constructor(tagName = 'div') {
        this.tagName = tagName.toUpperCase();
        this.children = [];
        this.dataset = {};
        this.className = '';
        this.textContent = '';
        this.listeners = new Map();
        this.style = {};
        this.parentNode = null;
        this.disabled = false;
    }
    append(...children) { children.forEach((child) => { child.parentNode = this; this.children.push(child); }); }
    remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); this.parentNode = null; }
    addEventListener(type, listener) { this.listeners.set(type, listener); }
    removeEventListener(type) { this.listeners.delete(type); }
    setAttribute() {}
    focus() { this.focused = true; }
    click() { this.listeners.get('click')?.({ preventDefault() {}, stopPropagation() {} }); }
    querySelectorAll(selector) { return descendants(this).filter((element) => matches(element, selector)); }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
}

const matches = (element, selector) => {
    const data = selector.match(/^\[data-([a-z-]+)(?:="([^"]+)")?\]$/);
    if (data) {
        const key = data[1].replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
        return Object.hasOwn(element.dataset, key) && (!data[2] || element.dataset[key] === data[2]);
    }
    return selector === '.home-preview-content' ? element.className.includes('home-preview-content') : false;
};

const descendants = (root) => {
    const found = [];
    const visit = (node) => node.children.forEach((child) => { found.push(child); visit(child); });
    visit(root);
    return found;
};

const previewHost = (ids) => {
    const host = new Element('div');
    const viewport = new Element('main');
    viewport.className = 'home-preview-content';
    ids.forEach((id) => { const row = new Element('section'); row.dataset.previewRowId = id; viewport.append(row); });
    host.append(viewport);
    return { host, viewport };
};

test('editing rows receive direct manipulation controls but preview rows do not', async () => {
    // Break caught: non-edit previews acquiring mutating controls, or an editable row losing a keyboard alternative to drag-and-drop.
    const { mountCanvasInteractions } = await moduleFromFile('canvas.js');
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        const editable = previewHost(['one', 'two']);
        mountCanvasInteractions(editable.host, { state: { rows: rows.slice(0, 2) }, editing: true, dispatch: () => {} });
        editable.host.querySelectorAll('[data-preview-row-id]').forEach((row) => {
            const labels = descendants(row).filter((element) => element.tagName === 'BUTTON').map((element) => element.textContent);
            assert.deepEqual(labels, ['Drag row', 'Hide', 'Move Up', 'Move Down', 'Remove']);
        });
        const readonly = previewHost(['one', 'two']);
        mountCanvasInteractions(readonly.host, { state: { rows: rows.slice(0, 2) }, editing: false, dispatch: () => {} });
        assert.equal(descendants(readonly.host).some((element) => element.tagName === 'BUTTON'), false);
    } finally { globalThis.document = previousDocument; }
});

test('removal focuses the store-selected row after the synchronous rerender', async () => {
    // Break caught: canvas removal guessing a neighbour independently instead of following the store's authoritative selection.
    const { mountCanvasInteractions } = await moduleFromFile('canvas.js');
    const previousDocument = globalThis.document;
    const previousRAF = globalThis.requestAnimationFrame;
    const frame = [];
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    globalThis.requestAnimationFrame = (callback) => frame.push(callback);
    try {
        const initial = previewHost(['one', 'two']);
        let state = { rows: rows.slice(0, 2), selectionId: 'two' };
        const dispatch = (action) => {
            assert.deepEqual(action, { type: 'rows/remove', id: 'two' });
            state = { rows: [rows[0]], selectionId: 'one' };
            const next = previewHost(['one']);
            initial.host.children = next.host.children;
            next.host.children.forEach((child) => { child.parentNode = initial.host; });
        };
        mountCanvasInteractions(initial.host, { state, getState: () => state, editing: true, dispatch });
        initial.host.querySelectorAll('[data-preview-row-id="two"]')[0].querySelectorAll('[data-home-designer-remove]')[0].click();
        frame.forEach((callback) => callback());
        assert.equal(initial.host.querySelector('[data-preview-row-id="one"]').focused, true);
    } finally {
        globalThis.document = previousDocument;
        globalThis.requestAnimationFrame = previousRAF;
    }
});
