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
    controller.schedule({ profileId: 'profile-a', platform: 'tv', rows: [row('first', { limit: 99 }), row('disabled', { enabled: false })], visibleRowIds: ['first', 'disabled'] });
    assert.equal(requests.length, 0);
    timers.tick(249);
    assert.equal(requests.length, 0);
    timers.tick(1);
    await settle();
    assert.equal(requests.length, 1);
    assert.equal(requests[0].rows.value.shelves[0].limit, 12);
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
    const device = { style: { setProperty: (name, value) => values.set(name, value) } };
    applyThemeVariables(device, { accentColor: '#112233', fontScale: 1.2, highContrast: true, reduceOverlays: true, buttonRadius: 'pill' });
    assert.equal(values.get('--preview-accent'), '#112233');
    assert.equal(values.get('--preview-font-scale'), '1.2');
    assert.equal(values.get('--preview-contrast'), '1.35');
    assert.equal(values.get('--preview-overlay-opacity'), '0');
    assert.equal(values.get('--preview-button-radius'), '999px');
});
