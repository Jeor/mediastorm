import assert from 'node:assert/strict';
import test from 'node:test';
import { isHomeDesignerDrop, insertionIndex } from './outline.js';

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
