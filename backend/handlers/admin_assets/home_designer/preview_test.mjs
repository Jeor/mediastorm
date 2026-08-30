import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const moduleFromFile = async (name) => {
    const source = await readFile(new URL(`./${name}`, import.meta.url), 'utf8');
    return import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);
};

const fakeTimers = () => {
    let now = 0;
    let nextID = 1;
    const entries = new Map();
    return {
        setTimeout(callback, delay) { const id = nextID++; entries.set(id, { callback, due: now + delay }); return id; },
        clearTimeout(id) { entries.delete(id); },
        tick(milliseconds) {
            now += milliseconds;
            [...entries.entries()].filter(([, entry]) => entry.due <= now).sort(([, left], [, right]) => left.due - right.due)
                .forEach(([id, entry]) => { entries.delete(id); entry.callback(); });
        },
    };
};

const row = (id, extra = {}) => ({ id, name: id, enabled: true, order: 0, ...extra });
const deferred = () => {
    let resolve;
    let reject;
    return { promise: new Promise((nextResolve, nextReject) => { resolve = nextResolve; reject = nextReject; }), resolve, reject };
};
const settle = () => new Promise((resolve) => setImmediate(resolve));

test('preview controller debounces visible enabled rows and caps their items at twelve', async () => {
    // Break caught: a render immediately requests every row or forwards an unsafe oversized item count.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const requests = [];
    const controller = createPreviewController({
        timers,
        fetchPreview: async (request) => { requests.push(request); return { rows: [{ id: request.rows.value.shelves[0].id, status: 'ready', items: [] }] }; },
    });
    controller.schedule({ scope: { kind: 'profile', profileId: 'profile-a' }, profileId: 'profile-a', platform: 'tv', rows: [row('first', { limit: 99 }), row('disabled', { enabled: false })], visibleRowIds: ['first', 'disabled'] });
    assert.equal(requests.length, 0);
    timers.tick(249);
    assert.equal(requests.length, 0);
    timers.tick(1);
    await settle();
    assert.equal(requests.length, 1);
    assert.equal(requests[0].rows.value.shelves[0].limit, 12);
    assert.deepEqual(requests[0].scope, { kind: 'profile', profileId: 'profile-a' });
});

test('preview controller cancels superseded row requests and ignores their stale responses', async () => {
    // Break caught: a slow response for an older row configuration overwriting the current preview.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const first = deferred();
    const second = deferred();
    const calls = [];
    const controller = createPreviewController({ timers, fetchPreview: (request, { signal }) => { calls.push({ request, signal }); return calls.length === 1 ? first.promise : second.promise; } });
    controller.schedule({ profileId: 'profile-a', platform: 'tv', rows: [row('watchlist', { name: 'Old' })], visibleRowIds: ['watchlist'] });
    timers.tick(250);
    await settle();
    controller.schedule({ profileId: 'profile-a', platform: 'tv', rows: [row('watchlist', { name: 'New' })], visibleRowIds: ['watchlist'] });
    timers.tick(250);
    await settle();
    assert.equal(calls.length, 2);
    assert.equal(calls[0].signal.aborted, true);
    first.resolve({ rows: [{ id: 'watchlist', status: 'ready', items: [{ title: 'Old' }] }] });
    await settle();
    assert.equal(controller.getRows().watchlist?.items?.[0]?.title, undefined);
    second.resolve({ rows: [{ id: 'watchlist', status: 'ready', items: [{ title: 'New' }] }] });
    await settle();
    assert.equal(controller.getRows().watchlist.items[0].title, 'New');
});

test('preview controller caches successful results by profile, platform, and normalized row configuration', async () => {
    // Break caught: cache entries bleeding across profiles/platforms or missing equivalent normalized configurations.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const calls = [];
    const controller = createPreviewController({ timers, fetchPreview: async (request) => { calls.push(request); return { rows: [{ id: request.rows.value.shelves[0].id, status: 'ready', items: [] }] }; } });
    const schedule = (profileId, platform, input) => {
        controller.schedule({ profileId, platform, rows: [input], visibleRowIds: [input.id] }); timers.tick(250);
    };
    schedule('profile-a', 'tv', row('watchlist', { limit: 30, order: 8 })); await settle();
    schedule('profile-a', 'tv', row('watchlist', { order: 0, limit: 12 })); await settle();
    schedule('profile-a', 'mobile', row('watchlist', { limit: 12 })); await settle();
    schedule('profile-b', 'tv', row('watchlist', { limit: 12 })); await settle();
    assert.equal(calls.length, 3);
});

test('preview controller clears visible row data before switching preview profiles', async () => {
    // Break caught: presentation-safe content for one profile remaining visible while another profile is loading.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const controller = createPreviewController({ timers, fetchPreview: async (request) => ({ rows: [{ id: request.rows.value.shelves[0].id, status: 'ready', items: [{ title: request.previewProfileId }] }] }) });
    controller.schedule({ profileId: 'profile-a', platform: 'tv', rows: [row('watchlist')], visibleRowIds: ['watchlist'] });
    timers.tick(250); await settle();
    assert.equal(controller.getRows().watchlist.items[0].title, 'profile-a');
    controller.schedule({ profileId: 'profile-b', platform: 'tv', rows: [row('watchlist')], visibleRowIds: ['watchlist'] });
    assert.equal(controller.getRows().watchlist, undefined);
});

test('preview controller retains successful rows when another row fails and retries only the failed row', async () => {
    // Break caught: one failed request clearing neighbouring cards or Retry rescheduling the entire preview.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const calls = [];
    const controller = createPreviewController({
        timers,
        fetchPreview: async (request) => {
            calls.push(request.rows.value.shelves[0].id);
            if (calls.at(-1) === 'broken' && calls.filter((id) => id === 'broken').length === 1) throw new Error('offline');
            return { rows: [{ id: request.rows.value.shelves[0].id, status: 'ready', items: [{ title: request.rows.value.shelves[0].id }] }] };
        },
    });
    controller.schedule({ profileId: 'profile-a', platform: 'tv', rows: [row('good'), row('broken')], visibleRowIds: ['good', 'broken'] });
    timers.tick(250); await settle(); await settle();
    assert.equal(controller.getRows().good.status, 'ready');
    assert.equal(controller.getRows().broken.status, 'error');
    controller.retry('broken'); await settle();
    assert.deepEqual(calls, ['good', 'broken', 'broken']);
    assert.equal(controller.getRows().good.items[0].title, 'good');
    assert.equal(controller.getRows().broken.status, 'ready');
});

test('theme variables remain scoped to the preview device and map contrast plus overlay choices', async () => {
    // Break caught: live theme editing recoloring the admin shell or ignoring the accessibility choices shown in the mock app.
    const { applyThemeVariables } = await moduleFromFile('theme.js');
    const values = new Map();
    const device = { dataset: {}, style: { setProperty: (name, value) => values.set(name, value) } };
    applyThemeVariables(device, { accentColor: '#112233', fontScale: 1.2, highContrast: true, reduceOverlays: true, buttonRadius: 'pill', buttonStyle: 'filled' });
    assert.equal(values.get('--preview-accent'), '#112233');
    assert.equal(values.get('--preview-font-scale'), '1.2');
    assert.equal(values.get('--preview-contrast'), '1.35');
    assert.equal(values.get('--preview-overlay-opacity'), '0');
    assert.equal(values.get('--preview-button-radius'), '999px');
    assert.equal(device.dataset.previewButtonStyle, 'filled');
});

test('preview requests retain the exact edited scope and do not cap visible rows', async () => {
    // Break caught: the authorization scope being omitted or an accidental 12-row limit hiding valid visible shelves.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const requests = [];
    const controller = createPreviewController({ timers, fetchPreview: async (request) => { requests.push(request); return { rows: [{ id: request.rows.value.shelves[0].id, status: 'ready', items: [] }] }; } });
    const rows = Array.from({ length: 13 }, (_, index) => row(`row-${index}`));
    const scope = { kind: 'profile', profileId: 'edited-scope' };
    controller.schedule({ scope, profileId: 'profile-a', platform: 'tv', rows, visibleRowIds: rows.map((item) => item.id) });
    timers.tick(250); await settle(); await settle();
    assert.equal(requests.length, 13);
    assert.deepEqual(requests[0].scope, scope);
});

test('preview controller schedules only a newly visible enabled row after another row has resolved', async () => {
    // Break caught: losing the lazy observer lifecycle or re-fetching an already resolved visible row after a scroll.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const calls = [];
    const controller = createPreviewController({ timers, fetchPreview: async (request) => { calls.push(request.rows.value.shelves[0].id); return { rows: [{ id: calls.at(-1), status: 'ready', items: [] }] }; } });
    const rows = [row('first'), row('second'), row('disabled', { enabled: false })];
    const base = { scope: { kind: 'profile', profileId: 'scope-a' }, profileId: 'profile-a', platform: 'tv', rows };
    controller.schedule({ ...base, visibleRowIds: ['first'] }); timers.tick(250); await settle();
    controller.schedule({ ...base, visibleRowIds: ['first', 'second', 'disabled'] }); timers.tick(250); await settle();
    assert.deepEqual(calls, ['first', 'second']);
});

test('preview invalidation aborts and suppresses deferred row results for row, profile, platform, and scope transitions', async () => {
    // Break caught: old presentation data rendering during any of the four synchronous context transitions.
    const { createPreviewController } = await moduleFromFile('preview.js');
    const timers = fakeTimers();
    const deferredRequests = [];
    const controller = createPreviewController({ timers, fetchPreview: (_, { signal }) => { const request = deferred(); deferredRequests.push({ request, signal }); return request.promise; } });
    const initial = { scope: { kind: 'profile', profileId: 'scope-a' }, profileId: 'profile-a', platform: 'tv', rows: [row('watchlist')], visibleRowIds: ['watchlist'] };
    controller.schedule(initial); timers.tick(250); await settle();
    const transitions = [
        () => controller.invalidate(),
        () => controller.schedule({ ...initial, profileId: 'profile-b' }),
        () => controller.schedule({ ...initial, platform: 'mobile' }),
        () => controller.schedule({ ...initial, scope: { kind: 'global' } }),
    ];
    for (const transition of transitions) {
        transition();
        const active = deferredRequests.at(-1);
        assert.equal(active.signal.aborted, true);
        active.request.resolve({ rows: [{ id: 'watchlist', status: 'ready', items: [{ title: 'stale' }] }] });
        await settle();
        assert.equal(controller.getRows().watchlist, undefined);
        controller.schedule(initial); timers.tick(250); await settle();
    }
});

test('TV and mobile plans keep row order while applying distinct top and card rules', async () => {
    // Break caught: platform changes becoming a generic renderer alias or mutating the shared row order.
    const { buildMobilePreviewPlan, buildTVPreviewPlan } = await moduleFromFile('preview.js');
    const state = { rows: [row('first'), row('collection', { type: 'collection-hub', order: 1 }), row('third', { order: 2 })], rowsSettings: { tvTopShelfMode: 'source', tvTopShelfSourceId: 'third', mobileTopShelfSourceId: 'collection', homeShelfScale: 1.2, homeHeroScale: 1.3 } };
    const tv = buildTVPreviewPlan(state, {});
    const mobile = buildMobilePreviewPlan(state, {});
    assert.deepEqual(tv.rows.map((item) => item.id), ['first', 'collection', 'third']);
    assert.deepEqual(mobile.rows.map((item) => item.id), ['first', 'collection', 'third']);
    assert.equal(tv.heroRowId, 'third');
    assert.equal(tv.rows[0].cardLayout, 'landscape');
    assert.equal(tv.rows[1].cardLayout, 'portrait');
    assert.equal(tv.shelfScale, 1.2);
    assert.equal(mobile.carouselRowId, 'collection');
    assert.ok(mobile.rows.every((item) => item.cardLayout === 'portrait'));
});

test('continuous theme input retains the same focused control across a live render', async () => {
    // Break caught: typing a number or using a color picker replacing its focused Theme control after each input event.
    const { renderTheme } = await moduleFromFile('theme.js');
    class Element {
        constructor(tagName) { this.tagName = tagName; this.children = []; this.dataset = {}; this.listeners = new Map(); this.value = ''; this.checked = false; }
        append(...children) { this.children.push(...children); }
        replaceChildren(...children) { this.children = children; }
        addEventListener(type, listener) { this.listeners.set(type, listener); }
        querySelectorAll(selector) {
            const match = selector.match(/^\[data-([a-z-]+)(?:="(.+)")?\]$/);
            const found = [];
            const datasetKey = match?.[1]?.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
            const visit = (node) => { if (match && Object.hasOwn(node.dataset || {}, datasetKey) && (!match[2] || node.dataset[datasetKey] === match[2])) found.push(node); node.children?.forEach(visit); };
            visit(this); return found;
        }
        focus() { document.activeElement = this; }
    }
    const previousDocument = globalThis.document;
    const document = { activeElement: null, createElement: (tagName) => new Element(tagName) };
    globalThis.document = document;
    try {
        const host = new Element('div');
        const actions = [];
        const base = { scope: { kind: 'profile', profileId: 'profile-a' }, revision: 'one', themeMode: 'inherit', theme: { fontScale: 1, accentColor: '#112233' }, themePresets: [] };
        renderTheme(host, { state: base, dispatch: (action) => actions.push(action) });
        const font = host.querySelectorAll('[data-theme-path="fontScale"]')[0];
        font.focus(); font.value = '1.2'; font.listeners.get('input')();
        renderTheme(host, { state: { ...base, themeMode: 'custom', theme: { ...base.theme, fontScale: 1.2 } }, dispatch: () => {} });
        assert.equal(actions[0].path, 'fontScale');
        assert.strictEqual(host.querySelectorAll('[data-theme-path="fontScale"]')[0], font);
        assert.strictEqual(document.activeElement, font);
        assert.equal(host.querySelectorAll('[data-theme-mode]')[0].textContent, 'Theme uses a custom appearance.');
        assert.equal(host.querySelectorAll('[data-theme-customize]')[0].disabled, true);
        assert.equal(host.querySelectorAll('[data-theme-reset]')[0].disabled, false);
    } finally { globalThis.document = previousDocument; }
});

test('preview-only CSS gives each persisted button style a distinct card/control treatment', async () => {
    // Break caught: buttonStyle being serialized into a variable but never consumed by the mock device.
    const css = await readFile(new URL('./home_designer.css', import.meta.url), 'utf8');
    for (const style of ['soft', 'outlined', 'filled']) {
        assert.match(css, new RegExp(`\\.home-preview-device\\[data-preview-button-style="${style}"\\] \\.home-preview-card`));
    }
    assert.doesNotMatch(css, /\.btn\[data-preview-button-style/);
});
