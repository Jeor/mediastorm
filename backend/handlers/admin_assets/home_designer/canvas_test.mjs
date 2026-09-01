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
    insertBefore(child, before) {
        child.remove(); child.parentNode = this;
        const index = before ? this.children.indexOf(before) : -1;
        if (index < 0) this.children.push(child); else this.children.splice(index, 0, child);
    }
    remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); this.parentNode = null; }
    addEventListener(type, listener) { this.listeners.set(type, listener); }
    removeEventListener(type) { this.listeners.delete(type); }
    setAttribute() {}
    focus() { this.focused = true; }
    click() { this.listeners.get('click')?.({ preventDefault() {}, stopPropagation() {} }); }
    querySelectorAll(selector) { return descendants(this).filter((element) => matches(element, selector)); }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
    closest(selector) { for (let current = this; current; current = current.parentNode) if (matches(current, selector)) return current; return null; }
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

test('a row control customizes its owning row', async () => {
    // Break caught: canvas controls opening Inspector without selecting the row they mutate.
    const { mountCanvasInteractions } = await moduleFromFile('canvas.js');
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        const preview = previewHost(['one', 'two']);
        const customized = [];
        mountCanvasInteractions(preview.host, {
            state: { rows: rows.slice(0, 2) }, editing: true, dispatch: () => {},
            onCustomize: (id) => customized.push(id),
        });
        preview.host.querySelector('[data-preview-row-id="two"]').querySelector('[data-home-designer-visibility]').click();
        assert.deepEqual(customized, ['two']);
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

test('drop indicators use model row identities instead of hero or reordered presentation siblings', async () => {
    // Break caught: TV hero markup or a promoted mobile carousel shifting a model insertion by one row.
    const { mountCanvasInteractions } = await moduleFromFile('canvas.js');
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        for (const ids of [['hero', 'one', 'two', 'three'], ['two', 'one', 'three']]) {
            const preview = previewHost([]);
            ids.forEach((id) => {
                const child = new Element(id === 'hero' ? 'div' : 'section');
                if (id !== 'hero') child.dataset.previewRowId = id;
                child.getBoundingClientRect = () => ({ top: 0, bottom: 100, height: 100 });
                preview.viewport.append(child);
            });
            preview.viewport.getBoundingClientRect = () => ({ top: 0, bottom: 500 });
            preview.viewport.scrollBy = () => {};
            let moved;
            mountCanvasInteractions(preview.host, { state: { rows }, editing: true, dispatch: (action) => { moved = action; } });
            const target = preview.host.querySelector('[data-preview-row-id="two"]');
            preview.viewport.listeners.get('dragover')({ target, clientY: 25, dataTransfer: { types: ['application/x-home-designer-row'] }, preventDefault() {} });
            const indicator = preview.viewport.children.find((child) => child.className === 'home-designer-drop-indicator');
            assert.equal(indicator.dataset.dropIndex, '1');
            assert.equal(preview.viewport.children[preview.viewport.children.indexOf(indicator) + 1], target);
            preview.viewport.listeners.get('drop')({ target, dataTransfer: { types: ['application/x-home-designer-row'], getData: (type) => type.endsWith('row') ? 'three' : '' }, preventDefault() {} });
            assert.deepEqual(moved, { type: 'rows/move', id: 'three', to: 1 });
        }
    } finally { globalThis.document = previousDocument; }
});

test('drag announcements cover start, distinct targets, drop, and Escape cancellation without flooding', async () => {
    // Break caught: mouse drag state changes being silent to assistive technology or repeated dragover events flooding the live region.
    const { mountCanvasInteractions } = await moduleFromFile('canvas.js');
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        const preview = previewHost(['one', 'two', 'three']);
        preview.viewport.children.forEach((row) => { row.getBoundingClientRect = () => ({ top: 0, bottom: 100, height: 100 }); });
        preview.viewport.getBoundingClientRect = () => ({ top: 0, bottom: 500 });
        preview.viewport.scrollBy = () => {};
        const liveRegion = { set textContent(value) { this.messages.push(value); }, get textContent() { return this.messages.at(-1) || ''; }, messages: [] };
        mountCanvasInteractions(preview.host, { state: { rows }, editing: true, dispatch: () => {}, liveRegion });
        const handle = preview.host.querySelector('[data-preview-row-id="one"]').querySelector('[data-home-designer-drag-handle]');
        handle.listeners.get('dragstart')({ dataTransfer: { setData() {}, effectAllowed: '' } });
        const target = preview.host.querySelector('[data-preview-row-id="two"]');
        const over = { target, clientY: 25, dataTransfer: { types: ['application/x-home-designer-row'] }, preventDefault() {} };
        preview.viewport.listeners.get('dragover')(over);
        preview.viewport.listeners.get('dragover')(over);
        assert.equal(liveRegion.messages.length, 2);
        preview.viewport.listeners.get('drop')({ target, dataTransfer: { types: ['application/x-home-designer-row'], getData: (type) => type.endsWith('row') ? 'one' : '' }, preventDefault() {} });
        assert.match(liveRegion.textContent, /dropped at position 1 of 3/i);

        handle.listeners.get('dragstart')({ dataTransfer: { setData() {}, effectAllowed: '' } });
        preview.viewport.listeners.get('dragover')(over);
        preview.viewport.listeners.get('keydown')({ key: 'Escape', preventDefault() {} });
        assert.match(liveRegion.textContent, /drag cancelled/i);
    } finally { globalThis.document = previousDocument; }
});
