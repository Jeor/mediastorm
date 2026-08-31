import assert from 'node:assert/strict';
import test from 'node:test';
import { renderTheme } from './theme.js';

class Element {
    constructor(tagName) {
        this.tagName = tagName;
        this.children = [];
        this.dataset = {};
        this.attributes = new Map();
        this.listeners = new Map();
        this.parentElement = null;
        this.className = '';
        this.textContent = '';
        this.focusCount = 0;
    }

    append(...children) { children.forEach((child) => { child.parentElement = this; this.children.push(child); }); }

    replaceChildren(...children) { this.children = []; this.append(...children); }

    addEventListener(type, listener) { this.listeners.set(type, listener); }

    click() { this.listeners.get('click')?.(); }

    dispatchEvent(event) { this.listeners.get(event.type)?.(event); }

    setAttribute(name, value) { this.attributes.set(name, String(value)); }

    getAttribute(name) { return this.attributes.get(name) ?? null; }

    removeAttribute(name) { this.attributes.delete(name); }

    remove() { this.parentElement?.children.splice(this.parentElement.children.indexOf(this), 1); }

    focus() { this.focusCount += 1; }

    matches(selector) {
        const data = selector.match(/^\[data-([a-z0-9-]+)\]$/);
        if (data) return Object.hasOwn(this.dataset, data[1].replace(/-([a-z])/g, (_, letter) => letter.toUpperCase()));
        return false;
    }

    querySelector(selector) {
        for (const child of this.children) {
            if (child.matches(selector)) return child;
            const nested = child.querySelector(selector);
            if (nested) return nested;
        }
        return null;
    }
}

test('Theme mode and value 422 errors render at an accessible action target and clear on theme mutations', () => {
    const previousDocument = globalThis.document;
    const document = { activeElement: null, createElement: (tagName) => new Element(tagName) };
    globalThis.document = document;
    try {
        const host = new Element('div');
        const edits = [];
        const dispatched = [];
        const state = { scope: { kind: 'profile', profileId: 'profile-1' }, revision: 'one', themeMode: 'inherit', theme: {}, themePresets: [{ id: 'preset', name: 'Preset', appearance: { accentColor: '#112233' } }] };
        renderTheme(host, {
            state,
            dispatch: (action) => dispatched.push(action),
            onFieldEdit: (path) => edits.push(path),
            errors: [{ path: 'mode', message: 'Theme mode is invalid' }, { path: 'value', message: 'Theme value is invalid' }],
        });

        const validation = host.querySelector('[data-home-designer-theme-validation]');
        assert.ok(validation);
        assert.equal(validation.getAttribute('aria-invalid'), 'true');
        assert.ok(validation.getAttribute('aria-describedby'));
        assert.match(validation.children.at(-1).textContent, /Theme mode is invalid/);
        assert.match(validation.children.at(-1).textContent, /Theme value is invalid/);

        host.querySelector('[data-theme-customize]').click();
        assert.deepEqual(edits, ['mode', 'value']);
        assert.deepEqual(dispatched, [{ type: 'theme/customize' }]);
        host.querySelector('[data-theme-reset]').click();
        assert.deepEqual(edits, ['mode', 'value', 'mode', 'value']);
        assert.deepEqual(dispatched, [{ type: 'theme/customize' }, { type: 'theme/reset' }]);
        const accent = host.querySelector('[data-theme-path]');
        accent.value = '#334455';
        accent.dispatchEvent({ type: 'input' });
        assert.deepEqual(edits.slice(-3), ['accentColor', 'mode', 'value']);
    } finally {
        globalThis.document = previousDocument;
    }
});
