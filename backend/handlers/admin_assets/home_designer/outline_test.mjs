import assert from 'node:assert/strict';
import test from 'node:test';
import { isHomeDesignerDrop, insertionIndex, removalFocusTarget, renderOutline } from './outline.js';

class Element {
    constructor(tagName) { this.tagName = tagName; this.children = []; this.dataset = {}; this.attributes = {}; this.className = ''; this.listeners = new Map(); this.classList = { add() {}, remove() {}, toggle() {} }; }
    append(...children) { children.forEach((child) => { child.parentElement = this; this.children.push(child); }); }
    replaceChildren(...children) { this.children = children; }
    addEventListener(type, listener) { this.listeners.set(type, listener); }
    setAttribute(name, value) { this.attributes[name] = String(value); }
    remove() { this.parentElement?.children.splice(this.parentElement.children.indexOf(this), 1); this.parentElement = null; }
}

const findText = (element, value) => {
    if (element.textContent === value) return element;
    for (const child of element.children) {
        const found = findText(child, value);
        if (found) return found;
    }
    return undefined;
};

test('native drag recognition uses advertised types without reading protected drag data', () => {
    // Break caught: dragover reading getData and failing to make a native catalog or row drag droppable.
    assert.equal(isHomeDesignerDrop(['application/x-home-designer-catalog']), true);
    assert.equal(isHomeDesignerDrop(['application/x-home-designer-row']), true);
    assert.equal(isHomeDesignerDrop(['text/plain']), false);
});

test('insertion index identifies the exact before or after row slot', () => {
    // Break caught: drop indicators and actions treating every row target as the same list-wide append.
    const rows = [{ id: 'one' }, { id: 'two' }, { id: 'three' }];
    assert.equal(insertionIndex(rows, 'two', false), 1);
    assert.equal(insertionIndex(rows, 'two', true), 2);
    assert.equal(insertionIndex(rows, '', false), 3);
});

test('row removal has a deterministic neighbor or empty-outline focus target', () => {
    // Break caught: removing the sole row leaving focus on a detached control.
    assert.equal(removalFocusTarget([{ id: 'only' }], 0), 'empty-outline');
    assert.equal(removalFocusTarget([{ id: 'first' }, { id: 'second' }], 0), 'second');
    assert.equal(removalFocusTarget([{ id: 'first' }, { id: 'second' }], 1), 'first');
});

test('preview outline permits selection but cannot mutate rows', () => {
    // Break caught: preview-mode outline rows exposing drag, row controls, or Alt+Arrow mutations before Edit Home.
    const previousDocument = globalThis.document;
    const previousAnimationFrame = globalThis.requestAnimationFrame;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    globalThis.requestAnimationFrame = (callback) => callback();
    try {
        const container = new Element('section');
        const dispatched = [];
        const selected = [];
        renderOutline(container, {
            editable: false,
            state: { selectionId: 'one', rows: [{ id: 'one', name: 'One', enabled: true }, { id: 'two', name: 'Two', enabled: true }] },
            dispatch: (action) => dispatched.push(action),
            onSelect: (id) => selected.push(id),
        });
        const list = container.children[1];
        const row = list.children[0];
        assert.equal(row.draggable, false);
        assert.equal(list.listeners.has('dragover'), false);
        assert.equal(list.listeners.has('drop'), false);
        assert.equal(findText(row, 'Hide'), undefined);
        assert.equal(findText(row, 'Move up'), undefined);
        assert.equal(findText(row, 'Move down'), undefined);
        assert.equal(findText(row, 'Remove'), undefined);
        row.listeners.get('keydown')({ key: 'ArrowDown', altKey: true, preventDefault() {} });
        assert.deepEqual(dispatched, []);
        row.listeners.get('click')();
        assert.deepEqual(selected, ['one']);
    } finally {
        globalThis.document = previousDocument;
        globalThis.requestAnimationFrame = previousAnimationFrame;
    }
});
