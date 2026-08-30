import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import vm from 'node:vm';

class Element {
    constructor(tagName) {
        this.tagName = tagName;
        this.children = [];
        this.dataset = {};
        this.listeners = new Map();
        this.className = '';
        this.textContent = '';
        this.type = '';
    }

    addEventListener(type, listener) {
        this.listeners.set(type, listener);
    }

    click() {
        this.listeners.get('click')?.();
    }

    append(...children) {
        this.children.push(...children);
    }

    replaceChildren(...children) {
        this.children = children;
    }

    querySelector(selector) {
        if (selector === '[data-home-designer-status]') {
            return this.children.find((child) => Object.hasOwn(child.dataset, 'homeDesignerStatus')) || null;
        }
        return this.children.find((child) => selector === `.${child.className}`) || null;
    }
}

const settle = () => new Promise((resolve) => setImmediate(resolve));

test('Home Designer Retry replaces a blocking failure after a successful reload', async () => {
    const source = await readFile(new URL('./admin_assets/home_designer/app.js', import.meta.url), 'utf8');
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const header = new Element('header');
    header.textContent = 'Home Designer';
    const status = new Element('div');
    status.dataset.homeDesignerStatus = '';
    const loading = new Element('p');
    loading.className = 'home-designer-loading';
    loading.textContent = 'Loading Home Designer…';
    status.append(loading);
    root.append(header, status);
    const responses = [
        Promise.reject(new Error('offline')),
        Promise.resolve({ ok: true, json: async () => ({ revision: 'fresh' }) }),
    ];
    const document = {
        getElementById: () => root,
        createElement: (tagName) => new Element(tagName),
    };
    vm.runInNewContext(source, { document, fetch: () => responses.shift(), URLSearchParams, Error });

    await settle();
    await settle();
    assert.equal(root.children[0], header);
    assert.deepEqual(status.children.map((child) => child.textContent), ['Home Designer could not load. Try again.', 'Retry']);

    status.children[1].click();
    await settle();
    await settle();

    assert.equal(root.children[0], header);
    assert.deepEqual(status.children.map((child) => child.textContent), ['Home Designer is ready.']);
});
