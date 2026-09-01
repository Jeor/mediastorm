import { createCatalogRows, findCatalogEntry } from './library.js';

const button = (label, className = 'btn btn-secondary') => {
    const element = document.createElement('button');
    element.type = 'button';
    element.className = className;
    element.textContent = label;
    return element;
};

const announce = (liveRegion, message) => {
    if (liveRegion) liveRegion.textContent = message;
};

export const isHomeDesignerDrop = (types) => {
    const offered = new Set(Array.from(types || []));
    return offered.has('application/x-home-designer-catalog') || offered.has('application/x-home-designer-row');
};

export const insertionIndex = (rows, targetID, after) => {
    const index = (rows || []).findIndex((row) => row.id === targetID);
    return index < 0 ? (rows || []).length : index + (after ? 1 : 0);
};

export const removalFocusTarget = (rows, index) => rows[index + 1]?.id || rows[index - 1]?.id || 'empty-outline';

const focusRow = (container, id) => requestAnimationFrame(() => container.querySelector(`[data-outline-row-id="${CSS.escape(id)}"]`)?.focus());

const moveRow = (container, dispatch, rows, id, to, liveRegion) => {
    const from = rows.findIndex((row) => row.id === id);
    if (from < 0 || to < 0 || to >= rows.length || from === to) return;
    dispatch({ type: 'rows/move', id, to });
    announce(liveRegion, `${rows[from].name || 'Row'} moved to position ${to + 1} of ${rows.length}.`);
    focusRow(container, id);
};

export const renderOutline = (container, { state, dispatch, liveRegion, onSelect, onConfigure, onAdd, editable = true } = {}) => {
    container.replaceChildren();
    const heading = document.createElement('h2');
    heading.textContent = 'Composition';
    const list = document.createElement('ol');
    list.className = 'home-designer-outline-list';
    list.setAttribute('aria-label', 'Home row order');
    const rows = state.rows || [];
    const addAt = (token, index) => {
        const entry = findCatalogEntry(state.catalog, token);
        if (!entry) return;
        if (entry.catalogOnly) {
            onConfigure?.(entry, index);
            return;
        }
        const added = createCatalogRows(entry, rows);
        added.forEach((row, offset) => dispatch({ type: 'rows/add', row, index: index + offset }));
        if (added[0]) onAdd?.(added[0].id, entry);
    };
    let pendingIndex = null;
    const marker = document.createElement('li');
    marker.className = 'home-designer-drop-indicator';
    marker.setAttribute('aria-hidden', 'true');
    const showInsertion = (index) => {
        marker.remove();
        pendingIndex = index;
        list.dataset.dropIndex = String(index);
        [...list.children].forEach((item, position) => {
            item.classList.toggle('is-drop-before', position === index);
            item.classList.toggle('is-drop-after', index === rows.length && position === rows.length - 1);
        });
        list.insertBefore(marker, list.children[index] || null);
    };
    const clearInsertion = () => {
        pendingIndex = null;
        delete list.dataset.dropIndex;
        marker.remove();
        [...list.children].forEach((item) => item.classList.remove('is-drop-before', 'is-drop-after'));
    };
    if (editable) {
        list.addEventListener('dragover', (event) => {
            if (!isHomeDesignerDrop(event.dataTransfer?.types)) return;
            event.preventDefault();
            const target = event.target.closest?.('[data-row-id]');
            const rect = target?.getBoundingClientRect?.();
            const after = rect ? event.clientY > rect.top + rect.height / 2 : false;
            showInsertion(insertionIndex(rows, target?.dataset.rowId, after));
        });
        list.addEventListener('dragleave', (event) => { if (!list.contains(event.relatedTarget)) clearInsertion(); });
        list.addEventListener('drop', (event) => {
            if (!isHomeDesignerDrop(event.dataTransfer?.types)) return;
            event.preventDefault();
            const index = pendingIndex ?? rows.length;
            const catalogType = event.dataTransfer?.getData('application/x-home-designer-catalog');
            const rowID = event.dataTransfer?.getData('application/x-home-designer-row');
            clearInsertion();
            if (catalogType) {
                addAt(catalogType, Math.max(0, index));
            } else {
                const source = rows.findIndex((row) => row.id === rowID);
                moveRow(container, dispatch, rows, rowID, Math.max(0, index - (source >= 0 && source < index ? 1 : 0)), liveRegion);
            }
        });
    }

    rows.forEach((row, index) => {
        const item = document.createElement('li');
        item.className = 'home-designer-outline-row';
        item.dataset.rowId = row.id;
        item.setAttribute('aria-posinset', String(index + 1));
        item.setAttribute('aria-setsize', String(rows.length));
        item.draggable = editable;
        item.tabIndex = 0;
        item.setAttribute('aria-current', String(state.selectionId === row.id));
        item.dataset.outlineRowId = row.id;
        if (editable) item.addEventListener('dragstart', (event) => {
            event.dataTransfer?.setData('application/x-home-designer-row', row.id);
            event.dataTransfer.effectAllowed = 'move';
        });
        item.addEventListener('click', () => onSelect?.(row.id));
        item.addEventListener('keydown', (event) => {
            if ((event.key === 'Enter' || event.key === ' ') && !event.altKey) { event.preventDefault(); onSelect?.(row.id); }
            if (editable && event.altKey && event.key === 'ArrowUp') { event.preventDefault(); moveRow(container, dispatch, rows, row.id, index - 1, liveRegion); }
            if (editable && event.altKey && event.key === 'ArrowDown') { event.preventDefault(); moveRow(container, dispatch, rows, row.id, index + 1, liveRegion); }
        });
        const name = document.createElement('span');
        name.className = 'home-designer-row-name';
        name.textContent = row.name || 'Untitled row';
        const stateText = document.createElement('span');
        stateText.className = 'home-designer-row-state';
        stateText.textContent = row.enabled === false ? 'Hidden' : 'Visible';
        const controls = document.createElement('div');
        controls.className = 'home-designer-row-actions';
        const select = button('Select');
        select.addEventListener('click', (event) => { event.stopPropagation(); onSelect?.(row.id); });
        controls.append(select);
        if (editable) {
            const visibility = button(row.enabled === false ? 'Show' : 'Hide');
            visibility.setAttribute('aria-label', `${visibility.textContent} ${row.name || 'row'}`);
            visibility.addEventListener('click', (event) => { event.stopPropagation(); dispatch({ type: 'rows/visibility', id: row.id, enabled: row.enabled === false }); announce(liveRegion, `${row.name || 'Row'} is now ${row.enabled === false ? 'visible' : 'hidden'}.`); });
            const up = button('Move up');
            up.disabled = index === 0;
            up.addEventListener('click', (event) => { event.stopPropagation(); moveRow(container, dispatch, rows, row.id, index - 1, liveRegion); });
            const down = button('Move down');
            down.disabled = index === rows.length - 1;
            down.addEventListener('click', (event) => { event.stopPropagation(); moveRow(container, dispatch, rows, row.id, index + 1, liveRegion); });
            const remove = button('Remove');
            remove.addEventListener('click', (event) => {
                event.stopPropagation();
                const target = removalFocusTarget(rows, index);
                dispatch({ type: 'rows/remove', id: row.id });
                announce(liveRegion, `${row.name || 'Row'} removed.`);
                if (target !== 'empty-outline') {
                    onSelect?.(target);
                    focusRow(container, target);
                } else {
                    requestAnimationFrame(() => container.querySelector('[data-outline-empty]')?.focus());
                }
            });
            controls.append(visibility, up, down, remove);
        }
        item.append(name, stateText, controls);
        list.append(item);
    });
    if (!rows.length) {
        const empty = document.createElement('p');
        empty.dataset.outlineEmpty = '';
        empty.tabIndex = -1;
        empty.textContent = 'Add a row from the library to begin composing this home screen.';
        container.append(heading, empty, list);
        return;
    }
    container.append(heading, list);
};
