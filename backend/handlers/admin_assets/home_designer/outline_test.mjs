import assert from 'node:assert/strict';
import test from 'node:test';
import { isHomeDesignerDrop, insertionIndex, removalFocusTarget, renderInspector } from './outline.js';

class Element {
    constructor(tagName) { this.tagName = tagName; this.children = []; this.dataset = {}; this.attributes = {}; this.className = ''; this.classList = { add() {}, remove() {}, toggle() {} }; }
    append(...children) { children.forEach((child) => { child.parentElement = this; this.children.push(child); }); }
    replaceChildren(...children) { this.children = children; }
    addEventListener() {}
    setAttribute(name, value) { this.attributes[name] = String(value); }
}

const findPath = (element, path) => {
    if (element.dataset?.fieldPath === path) return element;
    return element.children.map((child) => findPath(child, path)).find(Boolean);
};

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

test('collection inspector renders an existing item without appending stray globals', () => {
    // Break caught: collection rendering throwing for an undeclared name or moving controls twice.
    const originalDocument = globalThis.document;
    globalThis.document = { createElement: () => new Element() };
    const container = new Element();
    assert.doesNotThrow(() => renderInspector(container, {
        state: {
            selectionId: 'hub', rowValidation: {}, catalog: [{ type: 'collection-hub', fields: [{ path: 'collectionItems', type: 'collection', label: 'Collections' }] }],
            rows: [{ id: 'source', name: 'Source', type: 'genre' }, { id: 'hub', name: 'Hub', type: 'collection-hub', collectionItems: [{ id: 'item', name: 'Item', sourceShelfId: 'source', enabled: true }] }],
        }, dispatch() {}, onSelect() {},
    }));
    globalThis.document = originalDocument;
    assert.equal(container.children.length, 2);
});

test('collection inspector disables Add collection at the twenty-item cap', () => {
    // Break caught: the UI offering a 21st collection item after the local Apply gate has rejected the hub.
    const originalDocument = globalThis.document;
    globalThis.document = { createElement: () => new Element() };
    const items = Array.from({ length: 20 }, (_, index) => ({ id: `item-${index}`, name: `Item ${index}`, sourceShelfId: `source-${index}`, enabled: true }));
    const rows = [...items.map((_, index) => ({ id: `source-${index}`, name: `Source ${index}`, type: 'genre' })), { id: 'hub', name: 'Hub', type: 'collection-hub', collectionItems: items }];
    const container = new Element();
    renderInspector(container, { state: { selectionId: 'hub', rowValidation: {}, catalog: [{ type: 'collection-hub', fields: [{ path: 'collectionItems', type: 'collection' }] }], rows }, dispatch() {} });
    globalThis.document = originalDocument;
    assert.equal(findText(container, 'Add collection').disabled, true);
});

test('row inspector uses admin form treatments for text and select controls', () => {
    // Break caught: inspector fields rendering as unstyled native controls and visually breaking the panel grid.
    const previousDocument = globalThis.document;
    globalThis.document = { createElement: (tagName) => new Element(tagName) };
    try {
        const container = new Element('section');
        renderInspector(container, {
            state: {
                selectionId: 'genre-row', rowValidation: {},
                catalog: [{ type: 'genre', fields: [{ path: 'genre', type: 'select', options: [{ value: '28', label: 'Action' }] }] }],
                rows: [{ id: 'genre-row', name: 'Action movies', type: 'genre', genre: '28' }],
            },
            dispatch() {},
        });
        assert.equal(findPath(container, 'name').className, 'form-input');
        assert.equal(findPath(container, 'genre').className, 'form-select');
    } finally {
        globalThis.document = previousDocument;
    }
});

test('server field errors are connected to row and collection controls accessibly', () => {
    // Break caught: a 422 collection error shown only in a global alert, with no invalid control or description.
    const originalDocument = globalThis.document;
    globalThis.document = { createElement: () => new Element() };
    const container = new Element();
    renderInspector(container, {
        state: {
            selectionId: 'hub', catalog: [{ type: 'collection-hub', fields: [{ path: 'collectionItems', type: 'collection', label: 'Collections' }] }],
            rowValidation: { hub: [{ path: 'collectionItems.0.logoUrl', message: 'Use https artwork' }] },
            rows: [{ id: 'source', name: 'Source', type: 'genre' }, { id: 'hub', name: 'Hub', type: 'collection-hub', collectionItems: [{ id: 'item', name: 'Item', sourceShelfId: 'source', logoUrl: 'http://bad' }] }],
        }, dispatch() {}, onSelect() {},
    });
    globalThis.document = originalDocument;
    const control = findPath(container, 'collectionItems.0.logoUrl');
    assert.equal(control.attributes['aria-invalid'], 'true');
    assert.match(control.attributes['aria-describedby'], /collectionItems-0-logoUrl/);
    assert.ok(findText(container, 'Use https artwork'));
});
