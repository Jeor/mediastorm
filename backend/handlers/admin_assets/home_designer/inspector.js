import { findCatalogEntry } from './library.js';

const text = (value) => String(value ?? '').trim();
const copy = (value) => structuredClone(value);

const button = (label, className = 'btn btn-secondary') => {
    const element = document.createElement('button');
    element.type = 'button';
    element.className = className;
    element.textContent = label;
    return element;
};

const fieldControl = (field, value) => {
    if (field.type === 'select') {
        const select = document.createElement('select');
        select.className = 'form-select';
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
    if (input.type !== 'checkbox') input.className = 'form-input';
    if (field.type === 'boolean') input.checked = Boolean(value);
    else input.value = value ?? '';
    return input;
};

const fieldError = (fieldset, error, control = null) => {
    if (!error) return;
    const message = document.createElement('p');
    message.className = 'home-designer-field-error';
    message.id = `home-designer-field-error-${error.path.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
    message.textContent = error.message;
    control?.setAttribute?.('aria-invalid', 'true');
    control?.setAttribute?.('aria-describedby', message.id);
    fieldset.append(message);
};

const collectionEditor = (fieldset, row, rows, dispatch, errors = [], onFieldEdit) => {
    const items = Array.isArray(row.collectionItems) ? row.collectionItems : [];
    const update = (next) => dispatch({ type: 'rows/field', id: row.id, path: 'collectionItems', value: next.map((item, order) => ({ ...item, order })) });
    const sources = rows.filter((candidate) => candidate.id !== row.id && candidate.type !== 'collection-hub' && candidate.id !== 'streaming-services');
    items.forEach((item, index) => {
        const line = document.createElement('div');
        line.className = 'home-designer-collection-item';
        const set = (path, value) => {
            onFieldEdit?.({ section: 'rows', rowId: row.id, itemId: item.id, path });
            update(items.map((candidate, position) => position === index ? { ...candidate, [path]: value } : candidate));
        };
        const errorAt = (path) => errors.find((error) => error.path === `collectionItems.${index}.${path}`);
        const input = (path, label, type = 'text') => {
            const control = document.createElement('input');
            control.type = type;
            control.className = 'form-input';
            control.value = item[path] ?? '';
            control.dataset.fieldPath = `collectionItems.${index}.${path}`;
            control.setAttribute('aria-label', `Collection ${index + 1} ${label}`);
            const error = errorAt(path);
            control.addEventListener('change', () => set(path, type === 'number' ? (control.value === '' ? 0 : Number(control.value)) : control.value));
            line.append(control);
            fieldError(line, error, control);
        };
        input('id', 'ID');
        input('name', 'name');
        const source = document.createElement('select');
        source.className = 'form-select';
        source.setAttribute('aria-label', `Collection ${index + 1} source`);
        source.dataset.fieldPath = `collectionItems.${index}.sourceShelfId`;
        const none = document.createElement('option'); none.value = ''; none.textContent = 'Choose a source'; source.append(none);
        const usedByOtherItems = new Set(items.filter((_, position) => position !== index).map((candidate) => candidate.sourceShelfId));
        sources.filter((candidate) => candidate.id === item.sourceShelfId || !usedByOtherItems.has(candidate.id)).forEach((candidate) => { const option = document.createElement('option'); option.value = candidate.id; option.textContent = candidate.name || candidate.id; source.append(option); });
        source.value = item.sourceShelfId || '';
        const sourceError = errorAt('sourceShelfId');
        source.addEventListener('change', () => set('sourceShelfId', source.value));
        line.append(source);
        fieldError(line, sourceError, source);
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

export const renderInspector = (container, { state, dispatch, onSelect, onCatalogSubmit, onFieldEdit, sectionValidation = {} } = {}) => {
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
    (sectionValidation.rows || []).forEach((error) => fieldError(form, error));
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
            collectionEditor(fieldset, row, state.rows || [], dispatch, errors, onFieldEdit);
        } else {
            const control = fieldControl(field, row[field.path]);
            const id = `home-designer-${row.id}-${field.path}`;
            control.id = id; control.dataset.fieldPath = field.path; label.htmlFor = id;
            const error = errors.find((candidate) => candidate.path === field.path);
            fieldError(fieldset, error, control);
            control.addEventListener('change', () => { onFieldEdit?.({ section: 'rows', rowId: row.id, path: field.path }); dispatch({ type: 'rows/field', id: row.id, path: field.path, value: field.type === 'boolean' ? control.checked : field.type === 'number' ? (control.value === '' ? 0 : Number(control.value)) : control.value }); });
            fieldset.append(label, control);
        }
        form.append(fieldset);
    });
    container.append(heading, form);
};
