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

const previewRequest = (profileId, platform, row, theme) => ({
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
    const keyFor = (profileId, platform, row) => `${profileId}|${platform}|${stable(row)}`;
    const cancelPending = (id) => {
        const request = pending.get(id);
        if (request) request.controller.abort();
        pending.delete(id);
    };

    const requestRow = async (context, row) => {
        const key = keyFor(context.profileId, context.platform, row);
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
            const response = await fetchPreview(previewRequest(context.profileId, context.platform, row, context.theme), { signal: controller.signal });
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
        const allowed = new Set(context.rows.map((row) => row.id));
        [...pending.keys()].filter((id) => !allowed.has(id)).forEach(cancelPending);
        [...results.keys()].filter((id) => !allowed.has(id)).forEach((id) => results.delete(id));
        context.rows.forEach((row) => { void requestRow(context, row); });
    };

    return {
        schedule({ profileId, platform, rows, visibleRowIds, theme } = {}) {
            const nextProfile = String(profileId || '');
            const nextPlatform = String(platform || 'tv');
            const visible = new Set(visibleRowIds || []);
            const selected = (Array.isArray(rows) ? rows : []).filter((row) => row?.enabled !== false && visible.has(row.id)).slice(0, 12).map(normalizePreviewRow);
            const nextKey = `${nextProfile}|${nextPlatform}|${stable(selected)}`;
            if (current?.contextKey === nextKey) return;
            if (current && (current.profileId !== nextProfile || current.platform !== nextPlatform)) {
                pending.forEach((request) => request.controller.abort());
                pending.clear(); results.clear(); emit();
            }
            current = { profileId: nextProfile, platform: nextPlatform, rows: selected, theme: clone(theme || {}), contextKey: nextKey, generation: ++generation };
            if (timeout !== null) timers.clearTimeout(timeout);
            timeout = timers.setTimeout(run, debounceMs);
        },
        retry(id) {
            const context = current;
            const row = context?.rows.find((candidate) => candidate.id === id);
            if (row) void requestRow(context, row);
        },
        getRows: () => Object.fromEntries([...results.entries()].map(([id, value]) => [id, clone(value)])),
        clear() {
            if (timeout !== null) timers.clearTimeout(timeout);
            timeout = null;
            pending.forEach((request) => request.controller.abort());
            pending.clear(); results.clear(); current = null; emit();
        },
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

const safeArtwork = (value) => {
    try { const url = new URL(String(value || ''), document.baseURI); return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : ''; } catch { return ''; }
};

const mediaCard = (item, layout) => {
    const card = document.createElement('article');
    card.className = `home-preview-card home-preview-card-${layout}`;
    const artwork = safeArtwork(item?.artworkUrl);
    if (artwork) {
        const image = document.createElement('img');
        image.className = 'home-preview-artwork'; image.src = artwork; image.alt = ''; image.loading = 'lazy'; card.append(image);
    } else appendText(card, 'div', 'Artwork unavailable', 'home-preview-artwork home-preview-artwork-fallback');
    const details = document.createElement('div'); details.className = 'home-preview-card-details';
    appendText(details, 'strong', text(item?.title, 'Untitled'));
    if (item?.subtitle) appendText(details, 'span', text(item.subtitle), 'home-preview-subtitle');
    if (Array.isArray(item?.badges) && item.badges.length) appendText(details, 'span', item.badges.map((badge) => text(badge)).filter(Boolean).join(' · '), 'home-preview-badges');
    if (Number.isFinite(Number(item?.progress)) && Number(item.progress) > 0) {
        const progress = document.createElement('progress'); progress.max = 100; progress.value = Math.min(100, Math.max(0, Number(item.progress))); progress.setAttribute('aria-label', 'Watched progress'); details.append(progress);
    }
    if (item?.sample) appendText(details, 'span', 'Sample', 'home-preview-sample');
    card.append(details);
    return card;
};

const previewRow = (row, onSelect, onRetry) => {
    const section = document.createElement('section');
    section.className = 'home-preview-row'; section.dataset.previewRowId = row.id;
    const heading = document.createElement('button'); heading.type = 'button'; heading.className = 'home-preview-row-heading'; heading.textContent = text(row.name, 'Untitled row'); heading.addEventListener('click', () => onSelect?.(row.id)); section.append(heading);
    const items = Array.isArray(row.items) ? row.items : [];
    if (row.status === 'loading') {
        const skeletons = document.createElement('div'); skeletons.className = 'home-preview-items home-preview-skeletons';
        for (let index = 0; index < 4; index += 1) appendText(skeletons, 'div', 'Loading', 'home-preview-skeleton'); section.append(skeletons);
    } else if (row.status === 'error') {
        const error = document.createElement('div'); error.className = 'home-preview-row-error'; appendText(error, 'p', text(row.message, 'Content is unavailable for this row.'));
        const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-secondary'; retry.textContent = 'Retry'; retry.addEventListener('click', () => onRetry?.(row.id)); error.append(retry); section.append(error);
    } else if (!items.length) appendText(section, 'p', 'No items are available for this row.', 'home-preview-row-empty');
    else { const list = document.createElement('div'); list.className = 'home-preview-items'; items.forEach((item) => list.append(mediaCard(item, row.layout === 'collection' ? 'portrait' : 'landscape'))); section.append(list); }
    return section;
};

const renderDevice = (host, state, platform, options = {}) => {
    if (!host) return;
    host.replaceChildren();
    const device = document.createElement('div'); device.className = `home-preview-device home-preview-${platform}`; device.dataset.platform = platform;
    const frame = document.createElement('div'); frame.className = 'home-preview-frame';
    if (platform === 'tv') appendText(frame, 'nav', 'Home\nSearch\nLibrary', 'home-preview-tv-rail');
    else appendText(frame, 'header', 'Home                         ◉', 'home-preview-mobile-top');
    const main = document.createElement('main'); main.className = 'home-preview-content';
    appendText(main, 'div', 'Featured tonight', 'home-preview-hero');
    (state.rows || []).filter((row) => row.enabled !== false).forEach((row) => main.append(previewRow(options.results?.[row.id] || { ...row, status: 'loading' }, options.onSelect, options.onRetry)));
    frame.append(main);
    if (platform === 'mobile') appendText(frame, 'nav', 'Home     Search     Library', 'home-preview-mobile-nav');
    device.append(frame); host.append(device);
};

export const renderTVPreview = (host, state, options) => renderDevice(host, state, 'tv', options);
export const renderMobilePreview = (host, state, options) => renderDevice(host, state, 'mobile', options);
