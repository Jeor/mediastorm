const clone = (value) => structuredClone(value);

const stable = (value) => {
    if (Array.isArray(value)) return `[${value.map(stable).join(',')}]`;
    if (!value || typeof value !== 'object') return JSON.stringify(value);
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stable(value[key])}`).join(',')}}`;
};

export const normalizePreviewRow = (row) => {
    const next = clone(row || {});
    next.id = String(next.id || '');
    next.name = String(next.name || 'Untitled row');
    next.enabled = next.enabled !== false;
    next.order = 0;
    next.limit = Math.min(12, Math.max(1, Number.parseInt(next.limit, 10) || 12));
    return next;
};

const previewRequest = (scope, profileId, platform, row, theme) => ({
    scope: clone(scope),
    previewProfileId: profileId,
    platform,
    rows: { mode: 'custom', value: { shelves: [row] } },
    theme: theme ? { mode: 'custom', value: clone(theme) } : undefined,
});

export const createPreviewController = ({ fetchPreview, timers = globalThis, onChange = () => {}, debounceMs = 250 } = {}) => {
    const cache = new Map();
    const results = new Map();
    const pending = new Map();
    let timeout = null;
    let generation = 0;
    let current = null;
    const emit = () => onChange(Object.fromEntries(results));
    const keyFor = (scope, profileId, platform, row) => `${stable(scope)}|${profileId}|${platform}|${stable(row)}`;
    const cancelPending = (id) => {
        const request = pending.get(id);
        if (request) request.controller.abort();
        pending.delete(id);
    };

    const requestRow = async (context, row) => {
        const key = keyFor(context.scope, context.profileId, context.platform, row);
        const cached = cache.get(key);
        if (cached) {
            results.set(row.id, clone(cached));
            emit();
            return;
        }
        cancelPending(row.id);
        const controller = new AbortController();
        const request = { controller, key, generation: context.generation };
        pending.set(row.id, request);
        results.set(row.id, { id: row.id, name: row.name, status: 'loading', items: [] });
        emit();
        try {
            const response = await fetchPreview(previewRequest(context.scope, context.profileId, context.platform, row, context.theme), { signal: controller.signal });
            if (pending.get(row.id) !== request || current?.generation !== context.generation || controller.signal.aborted) return;
            const result = clone((response?.rows || []).find((candidate) => candidate?.id === row.id) || { id: row.id, name: row.name, status: 'empty', items: [] });
            result.id = row.id;
            result.name ||= row.name;
            result.items = Array.isArray(result.items) ? result.items.slice(0, 12) : [];
            cache.set(key, result);
            results.set(row.id, result);
        } catch (error) {
            if (pending.get(row.id) !== request || current?.generation !== context.generation || controller.signal.aborted) return;
            results.set(row.id, { id: row.id, name: row.name, status: 'error', items: [], message: 'Content is unavailable for this row.' });
        } finally {
            if (pending.get(row.id) === request) pending.delete(row.id);
            emit();
        }
    };

    const run = () => {
        timeout = null;
        const context = current;
        if (!context) return;
        const visible = new Set(context.rows.map((row) => row.id));
        [...pending.keys()].filter((id) => !visible.has(id)).forEach(cancelPending);
        [...results.keys()].filter((id) => !context.activeRowIDs.has(id)).forEach((id) => results.delete(id));
        context.rows.forEach((row) => { void requestRow(context, row); });
    };

    const invalidate = () => {
        if (timeout !== null) timers.clearTimeout(timeout);
        timeout = null;
        generation += 1;
        pending.forEach((request) => request.controller.abort());
        pending.clear(); results.clear(); current = null; emit();
    };

    return {
        schedule({ scope, profileId, platform, rows, visibleRowIds, theme } = {}) {
            const nextScope = clone(scope || {});
            const nextProfile = String(profileId || '');
            const nextPlatform = String(platform || 'tv');
            const visible = new Set(visibleRowIds || []);
            const enabledRows = (Array.isArray(rows) ? rows : []).filter((row) => row?.enabled !== false).map(normalizePreviewRow);
            const selected = enabledRows.filter((row) => visible.has(row.id));
            const baseKey = `${stable(nextScope)}|${nextProfile}|${nextPlatform}|${stable(enabledRows)}`;
            const visibleKey = stable(selected);
            if (current?.baseKey === baseKey && current.visibleKey === visibleKey) return;
            if (current?.baseKey !== baseKey) {
                invalidate();
            }
            current = {
                scope: nextScope, profileId: nextProfile, platform: nextPlatform, rows: selected,
                activeRowIDs: new Set(enabledRows.map((row) => row.id)), theme: clone(theme || {}),
                baseKey, visibleKey, generation: generation || ++generation,
            };
            if (timeout !== null) timers.clearTimeout(timeout);
            timeout = timers.setTimeout(run, debounceMs);
        },
        retry(id) {
            const context = current;
            const row = context?.rows.find((candidate) => candidate.id === id);
            if (row) void requestRow(context, row);
        },
        getRows: () => Object.fromEntries([...results.entries()].map(([id, value]) => [id, clone(value)])),
        invalidate,
        clear: invalidate,
    };
};

const text = (value, fallback = '') => String(value ?? '').trim() || fallback;
const appendText = (parent, tag, value, className = '') => {
    const element = document.createElement(tag);
    if (className) element.className = className;
    element.textContent = value;
    parent.append(element);
    return element;
};

const schematicLabel = (item) => {
    const kind = text(item?.mediaType || item?.type, 'content').toLowerCase();
    if (kind === 'movie' || kind === 'movies' || kind === 'film') return { kind: 'movie', label: 'Movie' };
    if (['series', 'tv', 'show', 'episode'].includes(kind)) return { kind: 'series', label: 'Series' };
    if (['live', 'livetv', 'live-tv', 'channel', 'channels'].includes(kind)) return { kind: 'live', label: 'Live' };
    return { kind: 'content', label: 'Content' };
};

export const schematicKindForRow = (row = {}) => {
    const value = `${row.type || ''} ${row.mediaType || ''} ${row.id || ''}`.toLowerCase();
    if (value.includes('live') || value.includes('channel')) return 'live';
    if (value.includes('series') || value.includes('show') || value.includes('tv')) return 'series';
    if (value.includes('movie') || value.includes('film')) return 'movie';
    return 'content';
};

export const schematicItems = (row, count) => Array.from(
    { length: count },
    (_, index) => ({ id: `${row.id}-schematic-${index + 1}`, mediaType: schematicKindForRow(row) }),
);

const mediaCard = (item, layout) => {
    const card = document.createElement('article');
    const schematic = schematicLabel(item);
    card.className = `home-preview-card home-preview-card-${layout} home-preview-tile`;
    card.dataset.tileKind = schematic.kind;
    const surface = document.createElement('div');
    surface.className = 'home-preview-tile-surface';
    appendText(surface, 'span', schematic.label, 'home-preview-tile-label');
    card.append(surface);
    return card;
};

const previewRow = (row, onSelect, onRetry, cardLayout = 'landscape', presentation = '') => {
    const section = document.createElement('section');
    section.className = `home-preview-row ${presentation}`.trim();
    section.dataset.previewRowId = row.id;
    section.dataset.rowEnabled = String(row.enabled !== false);
    section.tabIndex = -1;
    if (row.enabled === false) section.style.opacity = '0.55';
    const heading = document.createElement('button'); heading.type = 'button'; heading.className = 'home-preview-row-heading'; heading.dataset.homeDesignerRowSelect = row.id; heading.textContent = text(row.name, 'Untitled row'); heading.addEventListener('click', () => onSelect?.(row.id)); section.append(heading);
    if (row.enabled === false) appendText(section, 'span', 'Hidden', 'home-preview-row-hidden');
    const items = Array.isArray(row.items) ? row.items : [];
    if (row.status === 'loading') {
        const skeletons = document.createElement('div'); skeletons.className = 'home-preview-items home-preview-skeletons';
        for (let index = 0; index < 4; index += 1) appendText(skeletons, 'div', 'Loading', 'home-preview-skeleton'); section.append(skeletons);
    } else if (row.status === 'error') {
        const error = document.createElement('div'); error.className = 'home-preview-row-error'; appendText(error, 'p', text(row.message, 'Content is unavailable for this row.'));
        const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-secondary'; retry.textContent = 'Retry'; retry.addEventListener('click', () => onRetry?.(row.id)); error.append(retry); section.append(error);
    } else if (!items.length) appendText(section, 'p', 'No items are available for this row.', 'home-preview-row-empty');
    else { const list = document.createElement('div'); list.className = 'home-preview-items'; items.forEach((item) => list.append(mediaCard(item, cardLayout))); section.append(list); }
    return section;
};

const resolvedRows = (state, { editing = false, schematicCount = 5 } = {}) => (state?.rows || [])
    .filter((row) => editing || row?.enabled !== false)
    .map((row) => ({
        ...row, id: row.id, name: row.name, enabled: row.enabled !== false,
        status: 'ready', items: schematicItems(row, schematicCount),
    }));

const topRow = (settings, mode, source, rows) => {
    if (String(settings?.[mode] || '').toLowerCase() === 'disabled') return null;
    const sourceID = String(settings?.[source] || '');
    return rows.find((row) => row.id === sourceID) || rows[0] || null;
};

const scale = (value) => Number.isFinite(Number(value)) && Number(value) > 0 ? Number(value) : 1;

export const buildTVPreviewPlan = (state, { editing = false } = {}) => {
    const rows = resolvedRows(state, { editing, schematicCount: 5 });
    const settings = state?.rowsSettings || {};
    const hero = topRow(settings, 'tvTopShelfMode', 'tvTopShelfSourceId', rows);
    return {
        heroRowId: hero?.id || null, heroTitle: hero?.name || 'Featured tonight', shelfScale: scale(settings.homeShelfScale), heroScale: scale(settings.homeHeroScale),
        rows: rows.map((row) => ({ ...row, cardLayout: row.layout === 'collection' || row.type === 'collection-hub' ? 'portrait' : 'landscape' })),
    };
};

export const buildMobilePreviewPlan = (state, { editing = false } = {}) => {
    const rows = resolvedRows(state, { editing, schematicCount: 4 });
    const settings = state?.rowsSettings || {};
    const carousel = topRow(settings, 'mobileTopShelfMode', 'mobileTopShelfSourceId', rows);
    return {
        carouselRowId: carousel?.id || null,
        rows: rows.map((row) => ({ ...row, cardLayout: 'portrait', presentation: row.id === carousel?.id ? 'home-preview-mobile-carousel' : '' })),
    };
};

const device = (platform) => {
    const element = document.createElement('div'); element.className = `home-preview-device home-preview-${platform}`; element.dataset.platform = platform; return element;
};

export const renderTVPreview = (host, state, options = {}) => {
    if (!host) return;
    host.replaceChildren();
    const plan = buildTVPreviewPlan(state, options);
    const preview = device('tv'); preview.style.setProperty('--preview-tv-shelf-scale', String(plan.shelfScale)); preview.style.setProperty('--preview-tv-hero-scale', String(plan.heroScale));
    const frame = document.createElement('div'); frame.className = 'home-preview-frame';
    appendText(frame, 'nav', 'Home\nSearch\nLibrary', 'home-preview-tv-rail');
    const main = document.createElement('main'); main.className = 'home-preview-content';
    appendText(main, 'div', plan.heroTitle, 'home-preview-hero');
    plan.rows.forEach((row) => main.append(previewRow(row, options.onSelect, options.onRetry, row.cardLayout, 'home-preview-tv-row')));
    frame.append(main);
    preview.append(frame); host.append(preview);
};

export const renderMobilePreview = (host, state, options = {}) => {
    if (!host) return;
    host.replaceChildren();
    const plan = buildMobilePreviewPlan(state, options);
    const preview = device('mobile');
    const frame = document.createElement('div'); frame.className = 'home-preview-frame';
    appendText(frame, 'header', 'Home                         ◉', 'home-preview-mobile-top');
    const main = document.createElement('main'); main.className = 'home-preview-content';
    const carousel = plan.rows.find((row) => row.id === plan.carouselRowId);
    if (carousel) main.append(previewRow(carousel, options.onSelect, options.onRetry, 'portrait', 'home-preview-mobile-carousel'));
    plan.rows.filter((row) => row.id !== plan.carouselRowId).forEach((row) => main.append(previewRow(row, options.onSelect, options.onRetry, 'portrait', 'home-preview-mobile-row')));
    frame.append(main);
    appendText(frame, 'nav', 'Home     Search     Library', 'home-preview-mobile-nav');
    preview.append(frame); host.append(preview);
};
