const clone = (value) => structuredClone(value);

const equal = (left, right) => {
    if (left === right) return true;
    if (!left || !right || typeof left !== 'object' || typeof right !== 'object') return false;
    const leftKeys = Object.keys(left);
    const rightKeys = Object.keys(right);
    return leftKeys.length === rightKeys.length && leftKeys.every((key) => Object.hasOwn(right, key) && equal(left[key], right[key]));
};

const normalizeRows = (value) => {
    const rows = clone(value || {});
    rows.shelves = Array.isArray(rows.shelves) ? rows.shelves : [];
    rows.shelves.sort((left, right) => (left.order ?? 0) - (right.order ?? 0));
    rows.shelves.forEach((row, index) => { row.order = index; });
    return rows;
};

const sectionValue = (section) => clone(section?.override ?? section?.effective ?? {});

const documentState = (document) => {
    const envelope = clone(document);
    delete envelope.scope;
    delete envelope.revision;
    delete envelope.rows;
    delete envelope.theme;
    return {
        envelope,
        scope: clone(document.scope),
        revision: document.revision,
        rows: normalizeRows(sectionValue(document.rows)),
        rowsEffective: normalizeRows(document.rows?.effective ?? {}),
        theme: sectionValue(document.theme),
        themeEffective: clone(document.theme?.effective ?? {}),
        rowsMode: document.rows?.inherited ? 'inherit' : 'custom',
        themeMode: document.theme?.inherited ? 'inherit' : 'custom',
    };
};

const workingState = (state) => ({
    rows: normalizeRows(state.rows),
    theme: clone(state.theme),
    rowsMode: state.rowsMode,
    themeMode: state.themeMode,
});

const sectionEqual = (mode, value, baselineMode, baselineValue) => mode === baselineMode && (
    mode === 'inherit' || equal(value, baselineValue)
);

const stateFrom = (baseline, working, selectionId = null) => ({
    envelope: clone(baseline.envelope),
    scope: clone(baseline.scope),
    revision: baseline.revision,
    rows: normalizeRows(working.rows),
    rowsEffective: normalizeRows(baseline.rowsEffective),
    theme: clone(working.theme),
    themeEffective: clone(baseline.themeEffective),
    rowsMode: working.rowsMode,
    themeMode: working.themeMode,
    selectionId,
});

const publicState = (state) => ({
    ...clone(state.envelope),
    ...clone(state),
    rows: clone(state.rows.shelves),
    rowsSettings: clone(state.rows),
});

const updateField = (target, path, value) => {
    const parts = path.split('.').filter(Boolean);
    if (!parts.length) return false;
    let parent = target;
    for (const part of parts.slice(0, -1)) {
        if (!parent[part] || typeof parent[part] !== 'object') parent[part] = {};
        parent = parent[part];
    }
    parent[parts.at(-1)] = clone(value);
    return true;
};

const pathValue = (target, path) => path.split('.').filter(Boolean).reduce((value, part) => value && typeof value === 'object' ? value[part] : undefined, target);

const rowValidation = (rows, catalog) => {
    const errors = {};
    const catalogEntries = Array.isArray(catalog) ? catalog : [];
    const builtins = new Map(catalogEntries.filter((entry) => entry?.type === 'builtin').map((entry) => [entry?.default?.id, entry]));
    const entries = new Map(catalogEntries.filter((entry) => entry?.type && entry.type !== 'builtin').map((entry) => [entry.type, entry]));
    const ids = new Set();
    rows.forEach((row) => {
        const id = String(row?.id ?? '').trim();
        const rowErrors = [];
        if (!id) rowErrors.push({ path: 'id', message: 'Row ID is required' });
        else if (ids.has(id)) rowErrors.push({ path: 'id', message: 'Row ID must be unique' });
        ids.add(id);
        if (!String(row?.name ?? '').trim()) rowErrors.push({ path: 'name', message: 'Name is required' });
        const entry = row?.type === 'builtin' ? builtins.get(id) : entries.get(row?.type);
        (entry?.fields || []).forEach((field) => {
            if (!field?.required) return;
            const value = pathValue(row, field.path);
            if (value === undefined || value === null || String(value).trim() === '') {
                rowErrors.push({ path: field.path, message: `${field.label || field.path} is required` });
            }
        });
        if (rowErrors.length) errors[id] = rowErrors;
    });
    return errors;
};

const nextRowID = (candidate, rows) => {
    const wanted = String(candidate?.id ?? '').trim();
    if (!wanted) return wanted;
    const occupied = new Set(rows.map((row) => String(row?.id ?? '').trim()));
    if (!occupied.has(wanted)) return wanted;
    if (candidate?.type === 'builtin') return null;
    let suffix = 2;
    while (occupied.has(`${wanted}-${suffix}`)) suffix += 1;
    return `${wanted}-${suffix}`;
};

export const createStore = (document) => {
    let baseline = documentState(clone(document));
    let state = stateFrom(baseline, baseline);
    let undoHistory = [];
    let redoHistory = [];
    const listeners = new Set();

    const notify = () => {
        const next = publicState(state);
        listeners.forEach((listener) => listener(next));
    };

    const synchronizeSelection = () => {
        if (state.selectionId && !state.rows.shelves.some((row) => row.id === state.selectionId)) state.selectionId = null;
    };

    const commit = (change) => {
        const before = workingState(state);
        const selection = state.selectionId;
        change();
        state.rows = normalizeRows(state.rows);
        synchronizeSelection();
        const after = workingState(state);
        if (equal(before, after)) {
            state.selectionId = selection;
            return false;
        }
        undoHistory = [...undoHistory.slice(-99), before];
        redoHistory = [];
        notify();
        return true;
    };

    const api = {
        getState: () => publicState(state),
        subscribe: (listener) => {
            listeners.add(listener);
            return () => listeners.delete(listener);
        },
        canUndo: () => undoHistory.length > 0,
        canRedo: () => redoHistory.length > 0,
        isDirty: () => !sectionEqual(state.rowsMode, state.rows, baseline.rowsMode, baseline.rows) ||
            !sectionEqual(state.themeMode, state.theme, baseline.themeMode, baseline.theme),
        getInvalidRowIDs: () => {
            return Object.keys(rowValidation(state.rows.shelves, state.envelope.catalog));
        },
        getRowValidation: () => clone(rowValidation(state.rows.shelves, state.envelope.catalog)),
        isApplyValid: () => Object.keys(rowValidation(state.rows.shelves, state.envelope.catalog)).length === 0,
        dispatch: (action) => {
            if (!action || typeof action.type !== 'string') return false;
            if (action.type === 'selection/select') {
                state.selectionId = state.rows.shelves.some((row) => row.id === action.id) ? action.id : null;
                notify();
                return true;
            }
            if (action.type === 'selection/clear') {
                state.selectionId = null;
                notify();
                return true;
            }
            return commit(() => {
                const row = state.rows.shelves.find((candidate) => candidate.id === action.id);
                switch (action.type) {
                    case 'rows/add':
                        {
                            const next = clone(action.row ?? {});
                            const id = nextRowID(next, state.rows.shelves);
                            if (id === null) break;
                            state.rowsMode = 'custom';
                            next.id = id;
                            const index = Number.isInteger(action.index)
                                ? Math.max(0, Math.min(action.index, state.rows.shelves.length))
                                : state.rows.shelves.length;
                            state.rows.shelves.splice(index, 0, next);
                            state.rows.shelves.forEach((candidate, position) => { candidate.order = position; });
                            state.selectionId = next.id ?? null;
                        }
                        break;
                    case 'rows/remove':
                        if (row) {
                            state.rowsMode = 'custom';
                            state.rows.shelves = state.rows.shelves.filter((candidate) => candidate.id !== action.id);
                        }
                        break;
                    case 'rows/move': {
                        if (!row) break;
                        state.rowsMode = 'custom';
                        const from = state.rows.shelves.indexOf(row);
                        const to = Math.max(0, Math.min(Number.isInteger(action.to) ? action.to : from, state.rows.shelves.length - 1));
                        state.rows.shelves.splice(from, 1);
                        state.rows.shelves.splice(to, 0, row);
                        state.rows.shelves.forEach((candidate, index) => { candidate.order = index; });
                        state.selectionId = row.id;
                        break;
                    }
                    case 'rows/visibility':
                        if (row) {
                            state.rowsMode = 'custom';
                            row.enabled = Boolean(action.enabled);
                        }
                        break;
                    case 'rows/field':
                        if (row) {
                            state.rowsMode = 'custom';
                            updateField(row, action.path, action.value);
                        }
                        break;
                    case 'rows/customize':
                        state.rowsMode = 'custom';
                        break;
                    case 'rows/reset':
                        if (state.scope.kind !== 'global') {
                            state.rowsMode = 'inherit';
                            state.rows = normalizeRows(baseline.rowsEffective);
                        }
                        break;
                    case 'theme/field':
                        state.themeMode = 'custom';
                        updateField(state.theme, action.path, action.value);
                        break;
                    case 'theme/customize':
                        state.themeMode = 'custom';
                        break;
                    case 'theme/replace':
                        state.themeMode = 'custom';
                        state.theme = clone(action.value ?? {});
                        break;
                    case 'theme/reset':
                        if (state.scope.kind !== 'global') {
                            state.themeMode = 'inherit';
                            state.theme = clone(baseline.themeEffective);
                        }
                        break;
                    default:
                        break;
                }
            });
        },
        undo: () => {
            if (!undoHistory.length) return false;
            const current = workingState(state);
            const previous = undoHistory.pop();
            redoHistory.push(current);
            state = stateFrom(baseline, previous, state.selectionId);
            synchronizeSelection();
            notify();
            return true;
        },
        redo: () => {
            if (!redoHistory.length) return false;
            const current = workingState(state);
            const next = redoHistory.pop();
            undoHistory.push(current);
            state = stateFrom(baseline, next, state.selectionId);
            synchronizeSelection();
            notify();
            return true;
        },
        discard: () => {
            state = stateFrom(baseline, baseline, state.selectionId);
            synchronizeSelection();
            undoHistory = [];
            redoHistory = [];
            notify();
        },
        buildApplyRequest: () => {
            const request = { scope: clone(state.scope), expectedRevision: state.revision };
            if (!sectionEqual(state.rowsMode, normalizeRows(state.rows), baseline.rowsMode, normalizeRows(baseline.rows))) {
                request.rows = state.rowsMode === 'inherit' ? { mode: 'inherit' } : { mode: 'custom', value: normalizeRows(state.rows) };
            }
            if (!sectionEqual(state.themeMode, state.theme, baseline.themeMode, baseline.theme)) {
                request.theme = state.themeMode === 'inherit' ? { mode: 'inherit' } : { mode: 'custom', value: clone(state.theme) };
            }
            return request.rows || request.theme ? request : null;
        },
        replaceWithSaved: (savedDocument) => {
            baseline = documentState(clone(savedDocument));
            state = stateFrom(baseline, baseline);
            undoHistory = [];
            redoHistory = [];
            notify();
        },
    };

    return api;
};
