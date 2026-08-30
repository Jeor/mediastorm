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

const focusRow = (container, id) => requestAnimationFrame(() => container.querySelector(`[data-outline-row-id="${CSS.escape(id)}"]`)?.focus());

const moveRow = (container, dispatch, rows, id, to, liveRegion) => {
    const from = rows.findIndex((row) => row.id === id);
    if (from < 0 || to < 0 || to >= rows.length || from === to) return;
    dispatch({ type: 'rows/move', id, to });
    announce(liveRegion, `${rows[from].name || 'Row'} moved to position ${to + 1} of ${rows.length}.`);
    focusRow(container, id);
};

export const renderOutline = (container, { state, dispatch, liveRegion, onSelect } = {}) => {
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
        createCatalogRows(entry, rows).forEach((row, offset) => dispatch({ type: 'rows/add', row, index: index + offset }));
        if (entry.catalogOnly) announce(liveRegion, 'Streaming service rows added.');
    };
    const readDrop = (event) => event.dataTransfer?.getData('application/x-home-designer-catalog') || event.dataTransfer?.getData('application/x-home-designer-row');
    list.addEventListener('dragover', (event) => {
        if (!readDrop(event)) return;
        event.preventDefault();
        list.classList.add('is-drop-target');
    });
    list.addEventListener('dragleave', () => list.classList.remove('is-drop-target'));
    list.addEventListener('drop', (event) => {
        const token = readDrop(event);
        if (!token) return;
        event.preventDefault();
        list.classList.remove('is-drop-target');
        const target = event.target.closest?.('[data-row-id]');
        const index = target ? rows.findIndex((row) => row.id === target.dataset.rowId) : rows.length;
        if (event.dataTransfer?.types.includes('application/x-home-designer-catalog')) {
            addAt(token, Math.max(0, index));
        } else {
            moveRow(container, dispatch, rows, token, Math.max(0, index), liveRegion);
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
        visibility.addEventListener('click', (event) => { event.stopPropagation(); dispatch({ type: 'rows/visibility', id: row.id, enabled: row.enabled === false }); });
        const up = button('Move up');
        up.disabled = index === 0;
        up.addEventListener('click', (event) => { event.stopPropagation(); moveRow(container, dispatch, rows, row.id, index - 1, liveRegion); });
        const down = button('Move down');
        down.disabled = index === rows.length - 1;
        down.addEventListener('click', (event) => { event.stopPropagation(); moveRow(container, dispatch, rows, row.id, index + 1, liveRegion); });
        const remove = button('Remove');
        remove.addEventListener('click', (event) => { event.stopPropagation(); dispatch({ type: 'rows/remove', id: row.id }); announce(liveRegion, `${row.name || 'Row'} removed.`); });
        controls.append(visibility, up, down, remove);
        item.append(name, stateText, controls);
        list.append(item);
    });
    if (!rows.length) {
        const empty = document.createElement('p');
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

const collectionEditor = (fieldset, row, rows, dispatch) => {
    const items = Array.isArray(row.collectionItems) ? row.collectionItems : [];
    const update = (next) => dispatch({ type: 'rows/field', id: row.id, path: 'collectionItems', value: next.map((item, order) => ({ ...item, order })) });
    const sources = rows.filter((candidate) => candidate.id !== row.id && candidate.type !== 'collection-hub');
    items.forEach((item, index) => {
        const line = document.createElement('div');
        line.className = 'home-designer-collection-item';
        const name = document.createElement('input');
        name.type = 'text'; name.value = item.name || ''; name.setAttribute('aria-label', `Collection ${index + 1} name`);
        name.addEventListener('change', () => update(items.map((candidate, position) => position === index ? { ...candidate, name: name.value } : candidate)));
        const source = document.createElement('select');
        source.setAttribute('aria-label', `Collection ${index + 1} source`);
        const none = document.createElement('option'); none.value = ''; none.textContent = 'Choose a source'; source.append(none);
        sources.forEach((candidate) => { const option = document.createElement('option'); option.value = candidate.id; option.textContent = candidate.name || candidate.id; source.append(option); });
        source.value = item.sourceShelfId || '';
        source.addEventListener('change', () => update(items.map((candidate, position) => position === index ? { ...candidate, sourceShelfId: source.value } : candidate)));
        const up = button('Move up'); up.disabled = index === 0;
        up.addEventListener('click', () => { const next = copy(items); [next[index - 1], next[index]] = [next[index], next[index - 1]]; update(next); });
        const down = button('Move down'); down.disabled = index === items.length - 1;
        down.addEventListener('click', () => { const next = copy(items); [next[index], next[index + 1]] = [next[index + 1], next[index]]; update(next); });
        const remove = button('Remove'); remove.addEventListener('click', () => update(items.filter((_, position) => position !== index)));
        line.append(name, source, up, down, remove);
        fieldset.append(line);
    });
    const add = button('Add collection');
    add.addEventListener('click', () => update([...items, { id: `collection-${items.length + 1}`, name: '', sourceShelfId: '', enabled: true }]));
    fieldset.append(add);
};

export const renderInspector = (container, { state, dispatch, onSelect } = {}) => {
    container.replaceChildren();
    const row = (state.rows || []).find((candidate) => candidate.id === state.selectionId);
    const heading = document.createElement('h2');
    heading.textContent = 'Inspector';
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
            collectionEditor(fieldset, row, state.rows || [], dispatch);
        } else {
            const control = fieldControl(field, row[field.path]);
            const id = `home-designer-${row.id}-${field.path}`;
            control.id = id; control.dataset.fieldPath = field.path; label.htmlFor = id;
            const error = errors.find((candidate) => candidate.path === field.path);
            if (error) { control.setAttribute('aria-invalid', 'true'); const message = document.createElement('p'); message.className = 'home-designer-field-error'; message.textContent = error.message; fieldset.append(message); }
            control.addEventListener(field.type === 'boolean' ? 'change' : 'input', () => dispatch({ type: 'rows/field', id: row.id, path: field.path, value: field.type === 'boolean' ? control.checked : field.type === 'number' ? (control.value === '' ? 0 : Number(control.value)) : control.value }));
            fieldset.append(label, control);
        }
        form.append(fieldset);
    });
    container.append(heading, form);
};
