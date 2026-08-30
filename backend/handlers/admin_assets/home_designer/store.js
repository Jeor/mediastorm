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

const documentState = (document) => ({
    scope: clone(document.scope),
    revision: document.revision,
    rows: normalizeRows(sectionValue(document.rows)),
    theme: sectionValue(document.theme),
    rowsMode: document.rows?.inherited ? 'inherit' : 'custom',
    themeMode: document.theme?.inherited ? 'inherit' : 'custom',
});

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
    scope: clone(baseline.scope),
    revision: baseline.revision,
    rows: normalizeRows(working.rows),
    theme: clone(working.theme),
    rowsMode: working.rowsMode,
    themeMode: working.themeMode,
    selectionId,
});

const publicState = (state) => ({
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
            const seen = new Set();
            const invalid = [];
            state.rows.shelves.forEach((row) => {
                const id = String(row.id ?? '').trim();
                if (!id || !String(row.name ?? '').trim() || seen.has(id)) invalid.push(id);
                seen.add(id);
            });
            return invalid;
        },
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
                        state.rowsMode = 'custom';
                        state.rows.shelves.push(clone(action.row ?? {}));
                        state.selectionId = action.row?.id ?? null;
                        break;
                    case 'rows/remove':
                        state.rowsMode = 'custom';
                        state.rows.shelves = state.rows.shelves.filter((candidate) => candidate.id !== action.id);
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
                        state.rowsMode = 'inherit';
                        break;
                    case 'theme/field':
                        state.themeMode = 'custom';
                        updateField(state.theme, action.path, action.value);
                        break;
                    case 'theme/customize':
                        state.themeMode = 'custom';
                        break;
                    case 'theme/reset':
                        state.themeMode = 'inherit';
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
