import { createCatalogRows, findCatalogEntry } from './library.js';

const text = (value) => String(value ?? '').trim();
const copy = (value) => structuredClone(value);

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

export const renderOutline = (container, { state, dispatch, liveRegion, onSelect, onConfigure, onAdd } = {}) => {
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

    rows.forEach((row, index) => {
        const item = document.createElement('li');
        item.className = 'home-designer-outline-row';
        item.dataset.rowId = row.id;
        item.setAttribute('aria-posinset', String(index + 1));
        item.setAttribute('aria-setsize', String(rows.length));
        item.draggable = true;
        item.tabIndex = 0;
        item.setAttribute('aria-current', String(state.selectionId === row.id));
        item.dataset.outlineRowId = row.id;
        item.addEventListener('dragstart', (event) => {
            event.dataTransfer?.setData('application/x-home-designer-row', row.id);
            event.dataTransfer.effectAllowed = 'move';
        });
        item.addEventListener('click', () => onSelect?.(row.id));
        item.addEventListener('keydown', (event) => {
            if ((event.key === 'Enter' || event.key === ' ') && !event.altKey) { event.preventDefault(); onSelect?.(row.id); }
            if (event.altKey && event.key === 'ArrowUp') { event.preventDefault(); moveRow(container, dispatch, rows, row.id, index - 1, liveRegion); }
            if (event.altKey && event.key === 'ArrowDown') { event.preventDefault(); moveRow(container, dispatch, rows, row.id, index + 1, liveRegion); }
        });
        const name = document.createElement('span');
        name.className = 'home-designer-row-name';
        name.textContent = row.name || 'Untitled row';
        const stateText = document.createElement('span');
        stateText.className = 'home-designer-row-state';
        stateText.textContent = row.enabled === false ? 'Hidden' : 'Visible';
        const controls = document.createElement('div');
        controls.className = 'home-designer-row-actions';
        const visibility = button(row.enabled === false ? 'Show' : 'Hide');
        visibility.setAttribute('aria-label', `${visibility.textContent} ${row.name || 'row'}`);
        visibility.addEventListener('click', (event) => { event.stopPropagation(); dispatch({ type: 'rows/visibility', id: row.id, enabled: row.enabled === false }); announce(liveRegion, `${row.name || 'Row'} is now ${row.enabled === false ? 'visible' : 'hidden'}.`); });
        const select = button('Select');
        select.addEventListener('click', (event) => { event.stopPropagation(); onSelect?.(row.id); });
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
        controls.append(select, visibility, up, down, remove);
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

const fieldControl = (field, value) => {
    if (field.type === 'select') {
        const select = document.createElement('select');
        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = field.required ? 'Select an option' : 'No selection';
        select.append(placeholder);
        (field.options || []).forEach((option) => {
            const choice = document.createElement('option');
            choice.value = option.value;
            choice.textContent = option.label;
            select.append(choice);
        });
        select.value = value ?? '';
        return select;
    }
    const input = document.createElement('input');
    input.type = field.type === 'number' ? 'number' : field.type === 'url' ? 'url' : field.type === 'boolean' ? 'checkbox' : 'text';
    if (field.type === 'boolean') input.checked = Boolean(value);
    else input.value = value ?? '';
    return input;
};

const fieldError = (fieldset, error) => {
    if (!error) return;
    const message = document.createElement('p');
    message.className = 'home-designer-field-error';
    message.textContent = error.message;
    fieldset.append(message);
};

const collectionEditor = (fieldset, row, rows, dispatch, errors = []) => {
    const items = Array.isArray(row.collectionItems) ? row.collectionItems : [];
    const update = (next) => dispatch({ type: 'rows/field', id: row.id, path: 'collectionItems', value: next.map((item, order) => ({ ...item, order })) });
    const sources = rows.filter((candidate) => candidate.id !== row.id && candidate.type !== 'collection-hub' && candidate.id !== 'streaming-services');
    items.forEach((item, index) => {
        const line = document.createElement('div');
        line.className = 'home-designer-collection-item';
        const set = (path, value) => update(items.map((candidate, position) => position === index ? { ...candidate, [path]: value } : candidate));
        const errorAt = (path) => errors.find((error) => error.path === `collectionItems.${index}.${path}`);
        const input = (path, label, type = 'text') => {
            const control = document.createElement('input');
            control.type = type;
            control.value = item[path] ?? '';
            control.dataset.fieldPath = `collectionItems.${index}.${path}`;
            control.setAttribute('aria-label', `Collection ${index + 1} ${label}`);
            const error = errorAt(path);
            if (error) control.setAttribute('aria-invalid', 'true');
            control.addEventListener('change', () => set(path, type === 'number' ? (control.value === '' ? 0 : Number(control.value)) : control.value));
            line.append(control);
            fieldError(line, error);
        };
        input('id', 'ID');
        input('name', 'name');
        const source = document.createElement('select');
        source.setAttribute('aria-label', `Collection ${index + 1} source`);
        source.dataset.fieldPath = `collectionItems.${index}.sourceShelfId`;
        const none = document.createElement('option'); none.value = ''; none.textContent = 'Choose a source'; source.append(none);
        const usedByOtherItems = new Set(items.filter((_, position) => position !== index).map((candidate) => candidate.sourceShelfId));
        sources.filter((candidate) => candidate.id === item.sourceShelfId || !usedByOtherItems.has(candidate.id)).forEach((candidate) => { const option = document.createElement('option'); option.value = candidate.id; option.textContent = candidate.name || candidate.id; source.append(option); });
        source.value = item.sourceShelfId || '';
        const sourceError = errorAt('sourceShelfId');
        if (sourceError) source.setAttribute('aria-invalid', 'true');
        source.addEventListener('change', () => set('sourceShelfId', source.value));
        line.append(source);
        fieldError(line, sourceError);
        const enabled = document.createElement('input');
        enabled.type = 'checkbox'; enabled.checked = item.enabled !== false; enabled.setAttribute('aria-label', `Collection ${index + 1} enabled`);
        enabled.addEventListener('change', () => set('enabled', enabled.checked));
        line.append(enabled);
        input('logoUrl', 'logo URL', 'url');
        input('heroArtUrl', 'hero art URL', 'url');
        input('logoScale', 'logo scale', 'number');
        input('tintColor', 'tint color');
        const up = button('Move up'); up.disabled = index === 0;
        up.addEventListener('click', () => { const next = copy(items); [next[index - 1], next[index]] = [next[index], next[index - 1]]; update(next); });
        const down = button('Move down'); down.disabled = index === items.length - 1;
        down.addEventListener('click', () => { const next = copy(items); [next[index], next[index + 1]] = [next[index + 1], next[index]]; update(next); });
        const remove = button('Remove'); remove.addEventListener('click', () => update(items.filter((_, position) => position !== index)));
        line.append(up, down, remove);
        fieldset.append(line);
    });
    const add = button('Add collection');
    add.disabled = items.length >= 20;
    add.addEventListener('click', () => {
        const ids = new Set(items.map((item) => text(item.id)));
        let suffix = 1;
        while (ids.has(`collection-${suffix}`)) suffix += 1;
        update([...items, { id: `collection-${suffix}`, name: '', sourceShelfId: '', enabled: true }]);
    });
    fieldset.append(add);
};

export const renderInspector = (container, { state, dispatch, onSelect, onCatalogSubmit } = {}) => {
    container.replaceChildren();
    const heading = document.createElement('h2');
    heading.textContent = 'Inspector';
    if (state.catalogSelection) {
        const entry = findCatalogEntry(state.catalog, state.catalogSelection.token);
        const form = document.createElement('form');
        form.addEventListener('submit', (event) => { event.preventDefault(); onCatalogSubmit?.(entry, state.catalogSelection.values); });
        const title = document.createElement('h3'); title.textContent = `Configure ${entry?.name || 'row'}`;
        form.append(title);
        (entry?.fields || []).forEach((field) => {
            const fieldset = document.createElement('fieldset');
            const label = document.createElement('label'); label.textContent = field.label || field.path;
            const control = fieldControl(field, state.catalogSelection.values?.[field.path]);
            const id = `home-designer-catalog-${field.path}`;
            control.id = id; control.dataset.fieldPath = field.path; label.htmlFor = id;
            control.addEventListener('change', () => dispatch({ type: 'catalog/field', path: field.path, value: field.type === 'boolean' ? control.checked : control.value }));
            fieldset.append(label, control);
            form.append(fieldset);
        });
        const actions = document.createElement('div');
        const cancel = button('Cancel'); cancel.addEventListener('click', () => dispatch({ type: 'catalog/cancel' }));
        const submit = button('Add configured rows', 'btn btn-primary'); submit.type = 'submit';
        actions.append(cancel, submit); form.append(actions);
        container.append(heading, form);
        return;
    }
    const row = (state.rows || []).find((candidate) => candidate.id === state.selectionId);
    if (!row) {
        const help = document.createElement('p');
        help.textContent = 'Select a row to edit its settings.';
        container.append(heading, help);
        return;
    }
    const entry = findCatalogEntry(state.catalog, row.type === 'builtin' ? `builtin:${row.id}` : row.type);
    const errors = state.rowValidation?.[row.id] || [];
    const form = document.createElement('form');
    form.addEventListener('submit', (event) => event.preventDefault());
    const title = document.createElement('h3'); title.textContent = row.name || entry?.name || 'Untitled row';
    form.append(title);
    if (errors.length) {
        const summary = document.createElement('div');
        summary.className = 'home-designer-validation-summary';
        const message = document.createElement('p'); message.textContent = 'Complete the required fields before applying.'; summary.append(message);
        errors.forEach((error) => { const link = button(error.message); link.addEventListener('click', () => { onSelect?.(row.id); requestAnimationFrame(() => container.querySelector(`[data-field-path="${CSS.escape(error.path)}"]`)?.focus()); }); summary.append(link); });
        form.append(summary);
    }
    const fields = [{ path: 'name', type: 'text', label: 'Name', required: true }, ...(entry?.fields || []).filter((field) => field.path !== 'name')];
    fields.forEach((field) => {
        const fieldset = document.createElement('fieldset');
        const label = document.createElement('label'); label.textContent = field.label || field.path;
        if (field.type === 'collection') {
            fieldset.append(label);
            fieldError(fieldset, errors.find((error) => error.path === field.path));
            collectionEditor(fieldset, row, state.rows || [], dispatch, errors);
        } else {
            const control = fieldControl(field, row[field.path]);
            const id = `home-designer-${row.id}-${field.path}`;
            control.id = id; control.dataset.fieldPath = field.path; label.htmlFor = id;
            const error = errors.find((candidate) => candidate.path === field.path);
            if (error) { control.setAttribute('aria-invalid', 'true'); const message = document.createElement('p'); message.className = 'home-designer-field-error'; message.textContent = error.message; fieldset.append(message); }
            control.addEventListener('change', () => dispatch({ type: 'rows/field', id: row.id, path: field.path, value: field.type === 'boolean' ? control.checked : field.type === 'number' ? (control.value === '' ? 0 : Number(control.value)) : control.value }));
            fieldset.append(label, control);
        }
        form.append(fieldset);
    });
    container.append(heading, form);
};
