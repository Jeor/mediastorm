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
const sourceWithModules = async () => (await readFile(new URL('./admin_assets/home_designer/app.js', import.meta.url), 'utf8'))
    .replace(/const modules = Promise\.all\(\[import\('\.\/api\.js'\), import\('\.\/store\.js'\)\]\)\s*\.then\(\(\[api, editorStore\]\) => \[api\.default \?\? api, editorStore\.default \?\? editorStore\]\);/, 'const modules = Promise.resolve([homeDesignerAPI, homeDesignerStore]);');

test('Home Designer Retry replaces a blocking failure after a successful reload', async () => {
    const source = await sourceWithModules();
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
    let loadAttempts = 0;
    const document = {
        getElementById: () => root,
        createElement: (tagName) => new Element(tagName),
    };
    vm.runInNewContext(source, {
        document, Error, Promise,
        homeDesignerAPI: {
            loadDocument: async () => {
                if (loadAttempts++ === 0) throw new Error('offline');
                return { revision: 'fresh' };
            },
            applyDocument: async () => ({}),
            APIError: class APIError extends Error {},
        },
        homeDesignerStore: { createStore: () => ({ isDirty: () => false, buildApplyRequest: () => null, replaceWithSaved: () => {}, discard: () => {} }) },
    });

    await settle();
    await settle();
    await settle();
    await settle();
    assert.equal(root.children[0], header);
    assert.equal(loadAttempts, 1);
    assert.deepEqual(status.children.map((child) => child.textContent), ['Home Designer could not load. Try again.', 'Retry']);

    status.children[1].click();
    await settle();
    await settle();
    await settle();
    await settle();

    assert.equal(root.children[0], header);
    assert.deepEqual(status.children.map((child) => child.textContent), ['Home Designer is ready.']);
});

test('Home Designer bootstrap exposes explicit apply without writing the loaded document', async () => {
    // Break caught: bootstrap sending a mutation before the editor explicitly applies a working copy.
    const source = await sourceWithModules();
    const root = new Element('section');
    root.dataset = { basePath: '/account', isAdmin: 'false', profileId: 'profile-1' };
    const status = new Element('div');
    status.dataset.homeDesignerStatus = '';
    root.append(status);
    let applyCalls = 0;
    const store = {
        isDirty: () => true,
        buildApplyRequest: () => ({ scope: { kind: 'profile', profileId: 'profile-1' }, expectedRevision: 'revision-1', theme: { mode: 'inherit' } }),
        replaceWithSaved: () => {},
        discard: () => {},
    };
    const document = {
        getElementById: () => root,
        createElement: (tagName) => new Element(tagName),
    };
    const homeDesignerAPI = {
            loadDocument: async () => ({ scope: { kind: 'profile', profileId: 'profile-1' }, revision: 'revision-1', rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }),
            applyDocument: async () => { applyCalls += 1; return { scope: { kind: 'profile', profileId: 'profile-1' }, revision: 'revision-2', rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }; },
            APIError: class APIError extends Error {},
    };
    vm.runInNewContext(source, { document, Error, Promise, homeDesignerAPI, homeDesignerStore: { createStore: () => store } });

    await settle();
    await settle();
    await settle();
    await settle();
    assert.ok(root.homeDesigner, 'bootstrap exposes the controller for the later editor controls');
    assert.equal(await root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' }), true);
    assert.equal(applyCalls, 0);
    await root.homeDesigner.apply();
    assert.equal(applyCalls, 1);
});

test('Home Designer Retry keeps the scope selected before a failed reload', async () => {
    // Break caught: Retry reverting a failed profile switch to the initial global scope.
    const source = await sourceWithModules();
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div');
    status.dataset.homeDesignerStatus = '';
    root.append(status);
    const scopes = [];
    let shouldFail = false;
    const document = { getElementById: () => root, createElement: (tagName) => new Element(tagName) };
    vm.runInNewContext(source, {
        document, Error, Promise,
        homeDesignerAPI: {
            loadDocument: async (_, scope) => {
                scopes.push(scope.kind);
                if (shouldFail) throw new Error('offline');
                return { scope, revision: 'revision', rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } };
            },
            applyDocument: async () => ({}),
            APIError: class APIError extends Error {},
        },
        homeDesignerStore: { createStore: () => ({ isDirty: () => false, buildApplyRequest: () => null, replaceWithSaved: () => {}, discard: () => {} }) },
    });
    await settle(); await settle(); await settle(); await settle();
    shouldFail = true;
    assert.equal(await root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' }), false);
    shouldFail = false;
    status.children[1].click();
    await settle(); await settle(); await settle(); await settle();
    assert.deepEqual(scopes, ['global', 'profile', 'profile']);
});

test('a stale Apply response cannot replace a newer scope store', async () => {
    // Break caught: an apply for the old scope replacing the current scope after a user switch.
    const source = await sourceWithModules();
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div');
    status.dataset.homeDesignerStatus = '';
    root.append(status);
    let resolveApply;
    const applied = new Promise((resolve) => { resolveApply = resolve; });
    const stores = [];
    const document = { getElementById: () => root, createElement: (tagName) => new Element(tagName) };
    vm.runInNewContext(source, {
        document, Error, Promise,
        homeDesignerAPI: {
            loadDocument: async (_, scope) => ({ scope, revision: `${scope.kind}-revision`, rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }),
            applyDocument: async () => applied,
            APIError: class APIError extends Error {},
        },
        homeDesignerStore: { createStore: (saved) => {
            const store = { saved: [], isDirty: () => false, buildApplyRequest: () => ({ scope: saved.scope, expectedRevision: saved.revision, theme: { mode: 'custom', value: {} } }), replaceWithSaved: (next) => store.saved.push(next), discard: () => {} };
            stores.push(store);
            return store;
        } },
    });
    await settle(); await settle(); await settle(); await settle();
    const applying = root.homeDesigner.apply();
    await settle();
    await root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' });
    resolveApply({ scope: { kind: 'global' }, revision: 'global-new', rows: { inherited: false, effective: { shelves: [] } }, theme: { inherited: false, effective: {} } });
    await applying;
    assert.equal(stores[1].saved.length, 0);
});
