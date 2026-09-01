const text = (value) => String(value ?? '').trim();

const copy = (value) => structuredClone(value);

const categoryFor = (entry) => {
    if (entry.type === 'genre' || entry.type === 'decade') return 'Genres';
    if (entry.type === 'streaming-service' || entry.type === 'library') return 'Services';
    if (entry.category === 'Built-in') return 'Personal';
    return 'Lists';
};

const catalogToken = (entry) => entry.type === 'builtin' ? `builtin:${entry.default?.id || ''}` : entry.type;

const nextID = (base, rows) => {
    const used = new Set((rows || []).map((row) => text(row.id)));
    if (!used.has(base)) return base;
    let suffix = 2;
    while (used.has(`${base}-${suffix}`)) suffix += 1;
    return `${base}-${suffix}`;
};

export const defaultInsertionIndex = (rows = [], selectionId = null) => {
    const selected = rows.findIndex((row) => row.id === selectionId);
    return selected < 0 ? rows.length : selected + 1;
};

const streamingLists = {
    netflix: ['https://mdblist.com/lists/snoak/netflix-top-10-movies/json', 'https://mdblist.com/lists/snoak/netflix-top-10-shows/json'],
    disney: ['https://mdblist.com/lists/snoak/disney-plus-top-10-movies/json', 'https://mdblist.com/lists/snoak/disney-plus-top-10-tv-shows/json'],
    amazon: ['https://mdblist.com/lists/snoak/amazon-prime-top-10-movies/json', 'https://mdblist.com/lists/snoak/amazon-prime-top-10-tv-shows/json'],
    appletv: ['https://mdblist.com/lists/snoak/apple-tv-top-10-movies/json', 'https://mdblist.com/lists/snoak/apple-tv-top-10-tv-shows/json'],
    paramount: ['https://mdblist.com/lists/snoak/paramount-plus-top-10-movies/json', 'https://mdblist.com/lists/snoak/paramount-plus-top-10-tv-shows/json'],
    hbomax: ['https://mdblist.com/lists/snoak/hbo-top-10-movies-2/json', 'https://mdblist.com/lists/snoak/hbo-top-10-tv-shows/json'],
    hulu: ['https://mdblist.com/lists/snoak/top-hulu-movies/json', 'https://mdblist.com/lists/snoak/top-tv-shows-hulu/json'],
    crunchyroll: ['https://mdblist.com/lists/snoak/trending-anime-movies/json', 'https://mdblist.com/lists/snoak/trending-anime-shows/json'],
};

const optionLabel = (field, value) => field?.options?.find((option) => option.value === value)?.label || value;

export const findCatalogEntry = (catalog, token) => {
    const [kind, builtinID] = text(token).split(':', 2);
    return (catalog || []).find((entry) => entry.type === kind && (kind !== 'builtin' || entry.default?.id === builtinID)) || null;
};

export const createCatalogInstance = (entry, rows = []) => {
    if (!entry?.available) return null;
    const row = copy(entry.default || {});
    const builtin = entry.type === 'builtin';
    const baseID = text(row.id) || text(entry.type) || 'row';
    if (builtin && rows.some((candidate) => text(candidate.id) === baseID)) return null;
    row.id = builtin ? baseID : nextID(baseID, rows);
    row.type = builtin ? 'builtin' : (text(row.type) || entry.type);
    row.name = text(row.name) || text(entry.name) || row.id;
    row.enabled = row.enabled !== false;
    delete row.order;
    return row;
};

export const expandStreamingService = (entry, rows = [], values = {}) => {
    const serviceField = entry?.fields?.find((field) => field.path === 'service');
    const mediaField = entry?.fields?.find((field) => field.path === 'media');
    const service = values.service || serviceField?.options?.[0]?.value || 'netflix';
    const media = values.media || mediaField?.options?.[0]?.value || 'movies';
    const lists = streamingLists[service];
    if (!lists) return [];
    const label = optionLabel(serviceField, service);
    const wanted = media === 'both' ? [['movies', lists[0], 'Movies'], ['shows', lists[1], 'TV shows']] :
        media === 'shows' ? [['shows', lists[1], 'TV shows']] : [['movies', lists[0], 'Movies']];
    const used = new Set(rows.map((row) => text(row.id)));
    let instance = 'streaming-service';
    let suffix = 2;
    while (wanted.some(([kind]) => used.has(`${instance}-${kind}`))) instance = `streaming-service-${suffix++}`;
    return wanted.map(([kind, listURL, suffix], index) => ({
        id: `${instance}-${kind}`,
        type: 'mdblist', name: wanted.length > 1 ? `${label} ${suffix}` : label, enabled: true, listUrl: listURL,
    }));
};

export const createCatalogRows = (entry, rows = [], values = {}) => entry?.catalogOnly ? expandStreamingService(entry, rows, values) : [createCatalogInstance(entry, rows)].filter(Boolean);

export const filterCatalog = (catalog, query = '', category = 'All') => {
    const normalized = text(query).toLowerCase();
    return (catalog || []).filter((entry) => (category === 'All' || categoryFor(entry) === category) &&
        (!normalized || `${entry.name || ''} ${entry.description || ''} ${entry.category || ''}`.toLowerCase().includes(normalized)));
};

const button = (label, className = 'btn') => {
    const element = document.createElement('button');
    element.type = 'button';
    element.className = className;
    element.textContent = label;
    return element;
};

// renderLibrary owns only its container. State changes flow through the supplied
// store dispatcher, allowing app.js to re-render the complete editor surface.
export const renderLibrary = (container, { state, dispatch, onAdd, onConfigure } = {}) => {
    container.replaceChildren();
    const heading = document.createElement('h2');
    heading.textContent = 'Add a row';
    const search = document.createElement('input');
    search.type = 'search';
    search.className = 'form-input home-designer-library-search';
    search.placeholder = 'Search rows';
    search.setAttribute('aria-label', 'Search row library');
    const filters = document.createElement('div');
    filters.className = 'home-designer-library-filters';
    const entries = document.createElement('div');
    entries.className = 'home-designer-library-entries';
    let query = '';
    let category = 'All';

    const renderEntries = () => {
        entries.replaceChildren();
        filterCatalog(state.catalog, query, category).forEach((entry) => {
            const card = document.createElement('article');
            card.className = 'home-designer-library-entry';
            const title = document.createElement('h3');
            title.textContent = entry.name;
            const description = document.createElement('p');
            description.textContent = entry.description || '';
            const controls = document.createElement('div');
            controls.className = 'home-designer-entry-actions';
            if (entry.available) {
                const add = button('Add', 'btn btn-secondary');
                add.dataset.catalogType = catalogToken(entry);
                const alreadyAdded = entry.type === 'builtin' && (state.rows || []).some((row) => row.id === entry.default?.id);
                add.disabled = alreadyAdded;
                add.draggable = !alreadyAdded;
                add.addEventListener('dragstart', (event) => {
                    if (alreadyAdded) return;
                    event.dataTransfer?.setData('application/x-home-designer-catalog', catalogToken(entry));
                    event.dataTransfer.effectAllowed = 'copy';
                });
                add.addEventListener('click', () => {
                    const current = state.rows || [];
                    const index = defaultInsertionIndex(current, state.selectionId);
                    if (entry.catalogOnly) {
                        onConfigure?.(entry, index);
                        return;
                    }
                    const added = createCatalogRows(entry, current);
                    added.forEach((row, offset) => dispatch?.({ type: 'rows/add', row, index: index + offset }));
                    if (added[0]) onAdd?.(added[0].id, entry);
                });
                controls.append(add);
                if (alreadyAdded) {
                    const added = document.createElement('p');
                    added.className = 'home-designer-row-state';
                    added.textContent = 'Already added';
                    controls.append(added);
                }
            } else {
                const reason = document.createElement('p');
                reason.className = 'home-designer-unavailable';
                reason.textContent = entry.unavailableReason || 'This row is not available.';
                controls.append(reason);
                if (entry.setupPath) {
                    const setup = document.createElement('a');
                    setup.className = 'btn btn-secondary';
                    setup.href = entry.setupPath;
                    setup.textContent = 'Set up';
                    controls.append(setup);
                }
            }
            card.append(title, description, controls);
            entries.append(card);
        });
        if (!entries.children.length) {
            const empty = document.createElement('p');
            empty.textContent = 'No rows match this filter.';
            entries.append(empty);
        }
    };

    ['All', 'Personal', 'Genres', 'Services', 'Lists'].forEach((name) => {
        const filter = button(name, 'btn btn-secondary');
        filter.setAttribute('aria-pressed', String(name === category));
        filter.addEventListener('click', () => {
            category = name;
            [...filters.children].forEach((candidate) => candidate.setAttribute('aria-pressed', String(candidate.textContent === name)));
            renderEntries();
        });
        filters.append(filter);
    });
    search.addEventListener('input', () => { query = search.value; renderEntries(); });
    container.append(heading, search, filters, entries);
    renderEntries();
};
