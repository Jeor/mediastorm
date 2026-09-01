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
        this.parentElement = null;
        this.attributes = new Map();
        this.inert = false;
        this.focusCount = 0;
        this.classList = {
            add: (...names) => { this.className = [...new Set(this.className.split(/\s+/).filter(Boolean).concat(names))].join(' '); },
            remove: (...names) => { this.className = this.className.split(/\s+/).filter((name) => name && !names.includes(name)).join(' '); },
            contains: (name) => this.className.split(/\s+/).includes(name),
        };
    }

    addEventListener(type, listener) {
        this.listeners.set(type, listener);
    }

    removeEventListener(type, listener) {
        if (this.listeners.get(type) === listener) this.listeners.delete(type);
    }

    click() {
        this.listeners.get('click')?.();
    }

    dispatchEvent(event) {
        event.target ||= this;
        this.listeners.get(event.type)?.(event);
        if (event.bubbles !== false) this.parentElement?.dispatchEvent(event);
        return !event.defaultPrevented;
    }

    append(...children) {
        children.forEach((child) => {
            child.parentElement?.removeChild(child);
            child.parentElement = this;
            this.children.push(child);
        });
    }

    replaceChildren(...children) {
        this.children.forEach((child) => { child.parentElement = null; });
        children.forEach((child) => {
            child.parentElement?.removeChild(child);
            child.parentElement = this;
        });
        this.children = children;
    }

    removeChild(child) {
        const index = this.children.indexOf(child);
        if (index >= 0) this.children.splice(index, 1);
        child.parentElement = null;
        return child;
    }

    insertBefore(child, before) {
        child.parentElement?.removeChild(child);
        child.parentElement = this;
        const index = before ? this.children.indexOf(before) : -1;
        if (index >= 0) this.children.splice(index, 0, child);
        else this.children.push(child);
        return child;
    }

    get nextSibling() {
        if (!this.parentElement) return null;
        return this.parentElement.children[this.parentElement.children.indexOf(this) + 1] || null;
    }

    get parentNode() { return this.parentElement; }

    setAttribute(name, value) { this.attributes.set(name, String(value)); }

    getAttribute(name) { return this.attributes.get(name) ?? null; }

    removeAttribute(name) { this.attributes.delete(name); }

    hasAttribute(name) { return this.attributes.has(name); }

    contains(target) {
        for (let current = target; current; current = current.parentElement) if (current === this) return true;
        return false;
    }

    focus() { this.focusCount += 1; }

    getBoundingClientRect() { return { width: 0 }; }

    toggleAttribute(name, force) {
        if (force) this.setAttribute(name, '');
        else this.removeAttribute(name);
    }

    matches(selector) {
        if (selector.startsWith('.')) return this.className.split(/\s+/).includes(selector.slice(1));
        if (selector === 'a[href]') return this.tagName.toLowerCase() === 'a' && this.hasAttribute('href');
        const dataMatch = selector.match(/^\[data-([a-z0-9-]+)\]$/);
        if (dataMatch) {
            const key = dataMatch[1].replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
            return Object.hasOwn(this.dataset, key);
        }
        return selector === this.tagName.toLowerCase();
    }

    closest(selector) {
        for (let current = this; current; current = current.parentElement) if (current.matches(selector)) return current;
        return null;
    }

    querySelector(selector) {
        for (const candidate of this.querySelectorAll(selector)) {
            return candidate;
        }
        return null;
    }

    querySelectorAll(selector) {
        const selectors = selector.split(',').map((item) => item.trim().replace(/:not\([^)]*\)/g, ''));
        const found = [];
        const visit = (node) => {
            node.children.forEach((child) => {
                if (selectors.some((item) => child.matches(item))) found.push(child);
                visit(child);
            });
        };
        visit(this);
        return found;
    }
}

const settle = () => new Promise((resolve) => setImmediate(resolve));
const sourceWithModules = async () => (await readFile(new URL('./admin_assets/home_designer/app.js', import.meta.url), 'utf8'))
    .replace(/const modules = Promise\.all\(\[import\('\.\/api\.js'\), import\('\.\/store\.js'\)\]\)\s*\.then\(\(\[api, editorStore\]\) => \[api\.default \?\? api, editorStore\.default \?\? editorStore\]\);/, 'const modules = Promise.resolve([homeDesignerAPI, homeDesignerStore]);')
    .replace("const workspaceModule = import('./workspace.js');", "const workspaceModule = Promise.resolve({ createWorkspaceState: () => ({ mode: 'preview', band: 'compact', libraryOpen: false, contextTool: null, dragging: false }), reduceWorkspace: (state) => state });")
    .replace(/if \(!editorModules\) editorModules = Promise\.all\(\[import\('\.\/library\.js'\), import\('\.\/outline\.js'\)\]\);/, 'if (!editorModules) editorModules = Promise.resolve([homeDesignerLibrary, homeDesignerOutline]);')
    .replace(/if \(!previewModules\) previewModules = Promise\.all\(\[import\('\.\/theme\.js'\), import\('\.\/preview\.js'\)\]\);/g, 'if (!previewModules) previewModules = Promise.resolve([homeDesignerTheme, homeDesignerPreview]);');

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
        document, Error, Promise, AbortController,
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
    vm.runInNewContext(source, { document, Error, Promise, AbortController, homeDesignerAPI, homeDesignerStore: { createStore: () => store } });

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
        document, Error, Promise, AbortController,
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
    assert.deepEqual(scopes, ['global', 'profile', 'global']);
});

test('Home Designer source keeps native undo and dirty drafts separate from document history', async () => {
    // Break caught: Ctrl/Cmd+Z stealing an unfinished field edit before change/blur or navigation silently losing that draft.
    const source = await sourceWithModules();
    assert.match(source, /const isEditableTarget = \(target\)/);
    assert.match(source, /if \(isEditableTarget\(event\.target\)\) return;/);
    assert.match(source, /homeDesignerDraft/);
    assert.match(source, /pendingDrafts\.size/);
});

test('an unfinished field draft protects navigation while native input undo remains untouched', async () => {
    const source = await sourceWithModules();
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div'); status.dataset.homeDesignerStatus = ''; root.append(status);
    let undoCalls = 0;
    let confirmCalls = 0;
    let loadCalls = 0;
    const store = { isDirty: () => false, buildApplyRequest: () => null, replaceWithSaved() {}, discard() {}, undo: () => { undoCalls += 1; } };
    vm.runInNewContext(source, {
        document: { getElementById: () => root, createElement: (tagName) => new Element(tagName) }, Error, Promise, AbortController,
        confirm: () => { confirmCalls += 1; return false; },
        homeDesignerAPI: { loadDocument: async (_, scope) => { loadCalls += 1; return { scope, revision: 'one', rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }; }, applyDocument: async () => ({}), APIError: class APIError extends Error {} },
        homeDesignerStore: { createStore: () => store },
    });
    await settle(); await settle();
    const input = { tagName: 'INPUT', type: 'text', value: 'draft', dataset: { fieldPath: 'name' } };
    root.listeners.get('input')({ target: input });
    let prevented = false;
    root.listeners.get('keydown')({ target: input, key: 'z', ctrlKey: true, metaKey: false, altKey: false, preventDefault: () => { prevented = true; } });
    assert.equal(undoCalls, 0);
    assert.equal(prevented, false);
    assert.equal(await root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' }), false);
    assert.equal(confirmCalls, 1);
    assert.equal(loadCalls, 1);
});

test('Home Designer source makes responsive drawers modal and cleans them up on resize', async () => {
    // Break caught: a dialog-looking drawer allowing background interaction or retaining aria-modal after leaving its breakpoint.
    const source = await sourceWithModules();
    assert.match(source, /setBackgroundInert\(true\)/);
    assert.match(source, /setBackgroundInert\(false\)/);
    assert.match(source, /matchMedia\?\.\('\(max-width: 1099\.98px\)'\)/);
    assert.match(source, /drawerMedia\.addEventListener\('change'/);
});

test('responsive drawer blocks the shared shell and restores it when its breakpoint closes', async () => {
    // Break caught: a high-z sidebar/topbar still receiving pointer or focus while a Home Designer drawer is open.
    const source = await sourceWithModules();
    const body = new Element('body');
    const sidebar = new Element('aside');
    const topbar = new Element('header');
    const root = new Element('section');
    root.dataset = { basePath: '/account', isAdmin: 'false', profileId: '' };
    const status = new Element('div'); status.dataset.homeDesignerStatus = '';
    const backdrop = new Element('div'); backdrop.dataset.homeDesignerDrawerBackdrop = ''; backdrop.hidden = true;
    const editor = new Element('section'); editor.dataset.homeDesignerEditor = '';
    const workspace = new Element('section');
    const library = new Element('aside'); library.dataset.homeDesignerLibrary = '';
    const close = new Element('button'); close.className = 'home-designer-drawer-close'; library.append(close);
    workspace.append(library); editor.append(workspace); root.append(status, backdrop, editor); body.append(sidebar, topbar, root);
    const documentListeners = new Map();
    const media = {
        matches: true,
        addEventListener: (_type, listener) => { media.listener = listener; },
    };
    const document = {
        body,
        activeElement: null,
        getElementById: () => root,
        createElement: (tagName) => new Element(tagName),
        addEventListener: (type, listener) => documentListeners.set(type, listener),
        removeEventListener: (type) => documentListeners.delete(type),
    };
    vm.runInNewContext(source, {
        document, Error, Promise, AbortController,
        matchMedia: () => media,
        requestAnimationFrame: (callback) => callback(),
        homeDesignerAPI: { loadDocument: async () => ({}), applyDocument: async () => ({}), APIError: class APIError extends Error {} },
        homeDesignerStore: { createStore: () => ({}) },
    });
    const opener = { closest: () => opener, hasAttribute: (name) => name === 'data-home-designer-open-library', focus: () => {} };
    root.listeners.get('click')({ target: opener });

    assert.equal(sidebar.inert, true);
    assert.equal(topbar.inert, true);
    assert.equal(library.parentElement, body, 'the active dialog leaves the main-content stack');
    assert.equal(backdrop.parentElement, body, 'the backdrop leaves the main-content stack');
    assert.equal(backdrop.hidden, false);
    assert.equal(library.getAttribute('role'), 'dialog');
    documentListeners.get('focusin')({ target: sidebar });
    assert.equal(close.focusCount, 2, 'opening and an attempted shared-shell focus return focus to the drawer');
    assert.equal(documentListeners.has('keydown'), true, 'the portaled dialog retains keyboard handling');
    const styles = await readFile(new URL('./admin_assets/home_designer/home_designer.css', import.meta.url), 'utf8');
    assert.match(styles, /z-index:\s*10001/);
    assert.match(styles, /z-index:\s*10000/);

    let escapePrevented = false;
    documentListeners.get('keydown')({ target: close, key: 'Escape', preventDefault: () => { escapePrevented = true; } });
    assert.equal(escapePrevented, true);
    assert.equal(library.parentElement, workspace, 'Escape restores the dialog position');
    assert.equal(backdrop.parentElement, root, 'Escape restores the backdrop position');
    root.listeners.get('click')({ target: opener });
    assert.equal(library.parentElement, body);
    assert.equal(backdrop.parentElement, body);

    media.matches = false;
    media.listener();
    assert.equal(sidebar.inert, false);
    assert.equal(topbar.inert, false);
    assert.equal(backdrop.hidden, true);
    assert.equal(library.parentElement, workspace, 'the dialog returns to its original workspace position');
    assert.equal(backdrop.parentElement, root, 'the backdrop returns to its original page position');
    assert.equal(library.getAttribute('role'), null);
    assert.equal(body.classList.contains('home-designer-drawer-open'), false);
    assert.equal(documentListeners.has('focusin'), false);
    assert.equal(documentListeners.has('keydown'), false);
});

test('a portaled drawer remains addressable by editor rendering and focus work', async () => {
    const source = await sourceWithModules();
    assert.match(source, /const findDesignerElement = \(selector\) =>/);
    assert.match(source, /library\.renderLibrary\(findDesignerElement\('\[data-home-designer-library\]'\)/);
    assert.match(source, /outline\.renderInspector\(findDesignerElement\('\[data-home-designer-inspector\]'\)/);
    assert.match(source, /const target = findDesignerElement\(`\[data-home-designer-\$\{kind\}\]`\)/);
});

test('a body-portaled inspector retains draft and internal-link delegates until it closes', async () => {
    // Break caught: input and setup-link events no longer bubbling through the Home Designer root after portaling.
    const source = await sourceWithModules();
    const body = new Element('body');
    const root = new Element('section'); root.dataset = { basePath: '/account', isAdmin: 'false', profileId: '' };
    const status = new Element('div'); status.dataset.homeDesignerStatus = '';
    const backdrop = new Element('div'); backdrop.dataset.homeDesignerDrawerBackdrop = ''; backdrop.hidden = true;
    const editor = new Element('section'); editor.dataset.homeDesignerEditor = '';
    const workspace = new Element('section');
    const inspector = new Element('aside'); inspector.dataset.homeDesignerInspector = '';
    const close = new Element('button'); close.className = 'home-designer-drawer-close';
    const input = new Element('INPUT'); input.type = 'text'; input.value = 'unfinished'; input.dataset.fieldPath = 'title';
    const setupLink = new Element('a'); setupLink.setAttribute('href', '/account/setup');
    inspector.append(close, input, setupLink); workspace.append(inspector); editor.append(workspace); root.append(status, backdrop, editor); body.append(root);
    const documentListeners = new Map();
    const windowListeners = new Map();
    const media = { matches: true, addEventListener: (_type, listener) => { media.listener = listener; } };
    let confirmations = 0;
    const document = {
        body,
        activeElement: null,
        getElementById: () => root,
        createElement: (tagName) => new Element(tagName),
        addEventListener: (type, listener) => documentListeners.set(type, listener),
        removeEventListener: (type) => documentListeners.delete(type),
    };
    vm.runInNewContext(source, {
        document, Error, Promise, AbortController,
        matchMedia: () => media,
        requestAnimationFrame: (callback) => callback(),
        addEventListener: (type, listener) => windowListeners.set(type, listener),
        removeEventListener: (type) => windowListeners.delete(type),
        confirm: () => { confirmations += 1; return false; },
        homeDesignerAPI: { loadDocument: async () => ({}), applyDocument: async () => ({}), APIError: class APIError extends Error {} },
        homeDesignerStore: { createStore: () => ({}) },
    });
    const opener = { closest: () => opener, hasAttribute: (name) => name === 'data-home-designer-open-inspector', focus: () => {} };
    root.listeners.get('click')({ target: opener });
    assert.equal(inspector.parentElement, body);

    input.dispatchEvent({ type: 'input', bubbles: true });
    assert.equal(windowListeners.has('beforeunload'), true, 'an unfinished portaled inspector edit protects unload');
    const linkEvent = { type: 'click', bubbles: true, defaultPrevented: false, preventDefault() { this.defaultPrevented = true; } };
    setupLink.dispatchEvent(linkEvent);
    assert.equal(confirmations, 1);
    assert.equal(linkEvent.defaultPrevented, true, 'a portaled setup link remains subject to the dirty-navigation guard');
    input.dispatchEvent({ type: 'change', bubbles: true });
    assert.equal(windowListeners.has('beforeunload'), false, 'the portaled change delegate clears the pending draft');

    media.matches = false;
    media.listener();
    assert.equal(inspector.listeners.has('input'), false);
    assert.equal(inspector.listeners.has('change'), false);
    assert.equal(inspector.listeners.has('click'), false);
});

test('authoritative document replacement clears stale 422 feedback before rendering it', async () => {
    // Break caught: a validation alert from the previous scope/revision remaining attached to a freshly loaded document.
    const source = await sourceWithModules();
    assert.match(source, /const clearValidationState = \(\) => \{[\s\S]*applyValidation = \[\];[\s\S]*clearErrors\(\);/);
    assert.match(source, /const replaceWithLoadedDocument = async \(saved\) => \{[\s\S]*clearValidationState\(\);[\s\S]*clearDrafts\(\);[\s\S]*store = createStore\(saved\);[\s\S]*connectEditor\(\);/);
    assert.match(source, /bootstrap[\s\S]*await replaceWithLoadedDocument\(result\.saved\)/);
    assert.match(source, /switchScope[\s\S]*await replaceWithLoadedDocument\(result\.saved\)/);
    assert.match(source, /Reload latest[\s\S]*await replaceWithLoadedDocument\(result\.saved\)/);
    assert.match(source, /clearValidationState\(\);[\s\S]*applyingStore\.replaceWithSaved\(saved\)/);
});

test('a stale 422 alert clears after a successful scope load or Reload latest', async () => {
    const source = await sourceWithModules();
    class APIError extends Error {
        constructor(message, { code = '', status = 0, fields = [] } = {}) {
            super(message);
            this.code = code;
            this.status = status;
            this.fields = fields;
        }
    }
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div'); status.dataset.homeDesignerStatus = '';
    const errors = new Element('div'); errors.dataset.homeDesignerErrors = '';
    root.append(status, errors);
    let response = 'validation';
    const document = { getElementById: () => root, createElement: (tagName) => new Element(tagName) };
    vm.runInNewContext(source, {
        document, Error, Promise, AbortController,
        requestAnimationFrame: (callback) => callback(),
        homeDesignerAPI: {
            loadDocument: async (_, scope) => ({ scope, revision: `${scope.kind}-revision`, rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }),
            applyDocument: async () => {
                if (response === 'validation') throw new APIError('Invalid theme', { status: 422, fields: [{ section: 'theme', path: 'fontScale', message: 'Too large' }] });
                if (response === 'conflict') throw new APIError('Conflict', { code: 'revision_conflict' });
                return {};
            },
            APIError,
        },
        homeDesignerStore: { createStore: (saved) => ({ isDirty: () => true, buildApplyRequest: () => ({ scope: saved.scope, expectedRevision: saved.revision, theme: { mode: 'custom', value: {} } }), replaceWithSaved: () => {}, discard: () => {} }) },
    });
    await settle(); await settle(); await settle(); await settle();
    await root.homeDesigner.apply();
    assert.equal(errors.children.length, 1);

    await root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' });
    assert.equal(errors.children.length, 0, 'a scope replacement removes the prior 422 alert');

    response = 'validation';
    await root.homeDesigner.apply();
    response = 'conflict';
    await root.homeDesigner.apply();
    assert.equal(errors.children.length, 1);
    errors.children[0].children[1].click();
    await settle(); await settle(); await settle(); await settle();
    assert.equal(errors.children.length, 0, 'Reload latest removes validation feedback from the old revision');
});

test('a Theme mode or value 422 focuses the Theme action validation target', async () => {
    const source = await sourceWithModules();
    class APIError extends Error { constructor() { super('Invalid theme'); this.status = 422; this.fields = [{ section: 'theme', path: 'mode', message: 'Mode is invalid' }]; } }
    const root = new Element('section'); root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div'); status.dataset.homeDesignerStatus = '';
    const errors = new Element('div'); errors.dataset.homeDesignerErrors = '';
    const editor = new Element('section'); editor.dataset.homeDesignerEditor = '';
    const themeValidation = new Element('div'); themeValidation.dataset.homeDesignerThemeValidation = ''; editor.append(themeValidation);
    root.append(status, errors, editor);
    const state = { scope: { kind: 'global' }, revision: 'one', rows: [], theme: {}, themeMode: 'inherit', rowsMode: 'inherit', previewProfiles: [] };
    const store = { getState: () => state, subscribe: () => () => {}, isDirty: () => true, isApplyValid: () => true, buildApplyRequest: () => ({ scope: state.scope, expectedRevision: state.revision, theme: { mode: 'inherit' } }), canUndo: () => false, canRedo: () => false };
    vm.runInNewContext(source, {
        document: { getElementById: () => root, createElement: (tagName) => new Element(tagName) }, Error, Promise, AbortController,
        requestAnimationFrame: (callback) => callback(), CSS: { escape: (value) => value },
        homeDesignerAPI: { loadDocument: async () => ({ scope: state.scope, revision: 'one', rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }), applyDocument: async () => { throw new APIError(); }, APIError },
        homeDesignerStore: { createStore: () => store },
        homeDesignerLibrary: { renderLibrary: () => {} },
        homeDesignerOutline: { renderOutline: () => {}, renderInspector: () => {} },
        homeDesignerTheme: { renderTheme: () => {}, applyThemeVariables: () => {} },
        homeDesignerPreview: { createPreviewController: () => ({ invalidate: () => {} }), renderTVPreview: () => {}, renderMobilePreview: () => {} },
    });
    await settle(); await settle(); await settle(); await settle();
    await root.homeDesigner.apply();
    assert.equal(themeValidation.focusCount, 1);
});

test('Home Designer source routes every 422 field identity to row, collection, theme, or section feedback', async () => {
    // Break caught: server validation disappearing unless it happened to be a row error with rowId.
    const source = await sourceWithModules();
    assert.match(source, /serverValidationByField/);
    assert.match(source, /error\.itemId/);
    assert.match(source, /sectionValidation/);
    assert.match(source, /onFieldEdit/);
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
        document, Error, Promise, AbortController,
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

test('a stale rejected load cannot replace a newer scope status', async () => {
    // Break caught: a cancelled profile load showing its failure after global has become ready.
    const source = await sourceWithModules();
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div');
    status.dataset.homeDesignerStatus = '';
    root.append(status);
    let rejectProfile;
    const profileLoad = new Promise((_, reject) => { rejectProfile = reject; });
    const document = { getElementById: () => root, createElement: (tagName) => new Element(tagName) };
    vm.runInNewContext(source, {
        document, Error, Promise, AbortController,
        homeDesignerAPI: {
            loadDocument: async (_, scope) => scope.kind === 'profile'
                ? profileLoad
                : { scope, revision: 'global', rows: { inherited: false, effective: { shelves: [] } }, theme: { inherited: false, effective: {} } },
            applyDocument: async () => ({}),
            APIError: class APIError extends Error {},
        },
        homeDesignerStore: { createStore: () => ({ isDirty: () => false, buildApplyRequest: () => null, replaceWithSaved: () => {}, discard: () => {} }) },
    });
    await settle(); await settle(); await settle(); await settle();
    const profileSwitch = root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' });
    await settle();
    assert.equal(await root.homeDesigner.switchScope({ kind: 'global' }), true);
    rejectProfile(new Error('offline'));
    assert.equal(await profileSwitch, false);
    assert.deepEqual(status.children.map((child) => child.textContent), ['Home Designer is ready.']);
});

test('a stale conflict is cancelled and cannot expose Reload latest', async () => {
    // Break caught: a conflict from the old scope replacing a newer ready scope with a stale reload action.
    const source = await sourceWithModules();
    const root = new Element('section');
    root.dataset = { basePath: '/admin', isAdmin: 'true', profileId: '' };
    const status = new Element('div');
    status.dataset.homeDesignerStatus = '';
    root.append(status);
    let rejectApply;
    let applySignal;
    const applied = new Promise((_, reject) => { rejectApply = reject; });
    class ConflictError extends Error { constructor() { super('conflict'); this.code = 'revision_conflict'; } }
    const document = { getElementById: () => root, createElement: (tagName) => new Element(tagName) };
    vm.runInNewContext(source, {
        document, Error, Promise, AbortController,
        homeDesignerAPI: {
            loadDocument: async (_, scope) => ({ scope, revision: scope.kind, rows: { inherited: true, effective: { shelves: [] } }, theme: { inherited: true, effective: {} } }),
            applyDocument: async (_, __, options) => { applySignal = options.signal; return applied; },
            APIError: ConflictError,
        },
        homeDesignerStore: { createStore: (saved) => ({ isDirty: () => false, buildApplyRequest: () => ({ scope: saved.scope, expectedRevision: saved.revision, theme: { mode: 'custom', value: {} } }), replaceWithSaved: () => {}, discard: () => {} }) },
    });
    await settle(); await settle(); await settle(); await settle();
    const applying = root.homeDesigner.apply();
    await settle();
    await root.homeDesigner.switchScope({ kind: 'profile', profileId: 'profile-1' });
    assert.equal(applySignal.aborted, true);
    rejectApply(new ConflictError());
    assert.equal(await applying, false);
    assert.deepEqual(status.children.map((child) => child.textContent), ['Home Designer is ready.']);
});
