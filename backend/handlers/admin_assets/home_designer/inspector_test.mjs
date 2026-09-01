import assert from 'node:assert/strict';
import test from 'node:test';
import { renderInspector } from './inspector.js';

class Element {
    constructor(tagName) { this.tagName = tagName; this.children = []; this.dataset = {}; this.attributes = {}; this.className = ''; this.listeners = new Map(); this.classList = { add() {}, remove() {}, toggle() {} }; }
    append(...children) { children.forEach((child) => { child.parentElement = this; this.children.push(child); }); }
    replaceChildren(...children) { this.children = children; }
    addEventListener(type, listener) { this.listeners.set(type, listener); }
    setAttribute(name, value) { this.attributes[name] = String(value); }
    remove() { this.parentElement?.children.splice(this.parentElement.children.indexOf(this), 1); this.parentElement = null; }
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
