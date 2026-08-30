const root = document.getElementById('homeDesignerRoot');
const status = root?.querySelector('[data-home-designer-status]');
const editor = root?.querySelector('[data-home-designer-editor]');

if (root && status) {
    const basePath = root.dataset.basePath || '';
    const initialScope = root.dataset.isAdmin === 'true'
        ? { kind: 'global' }
        : { kind: 'profile', profileId: root.dataset.profileId || '' };
    const modules = Promise.all([import('./api.js'), import('./store.js')])
        .then(([api, editorStore]) => [api.default ?? api, editorStore.default ?? editorStore]);
    let store = null;
    let activeScope = initialScope;
    let operation = 0;
    let activeOperation = null;
    let editorModules = null;
    let unsubscribe = null;

    const sameScope = (left, right) => left?.kind === right?.kind && left?.profileId === right?.profileId;
    const beginOperation = () => {
        activeOperation?.controller.abort();
        const next = { token: ++operation, controller: new AbortController() };
        activeOperation = next;
        return next;
    };
    const isCurrent = (candidate) => activeOperation === candidate;

    const showMessage = (className, text) => {
        status.replaceChildren();
        const message = document.createElement('p');
        message.className = className;
        message.textContent = text;
        status.append(message);
    };

    const showFailure = () => {
        status.replaceChildren();
        const message = document.createElement('p');
        message.className = 'home-designer-error';
        message.textContent = 'Home Designer could not load. Try again.';
        const retry = document.createElement('button');
        retry.className = 'btn btn-primary';
        retry.type = 'button';
        retry.textContent = 'Retry';
        retry.addEventListener('click', bootstrap);
        status.append(message, retry);
    };

    const showReady = () => showMessage('home-designer-ready', 'Home Designer is ready.');

    const selectRow = (id, source = 'outline') => {
        store?.dispatch({ type: 'selection/select', id });
        requestAnimationFrame(() => {
            const counterpart = source === 'outline'
                ? editor?.querySelector(`[data-preview-row-id="${CSS.escape(id)}"]`)
                : editor?.querySelector(`[data-outline-row-id="${CSS.escape(id)}"]`);
            counterpart?.scrollIntoView({ block: 'nearest' });
        });
    };

    const focusFirstInvalid = () => {
        const validation = store?.getRowValidation?.() || {};
        const [id, errors] = Object.entries(validation)[0] || [];
        if (!id || !errors?.[0]) return;
        selectRow(id);
        requestAnimationFrame(() => editor?.querySelector(`[data-field-path="${CSS.escape(errors[0].path)}"]`)?.focus());
    };

    const renderPreviewPlaceholder = (state) => {
        const host = editor?.querySelector('[data-home-designer-preview-host]');
        if (!host) return;
        host.replaceChildren();
        (state.rows || []).forEach((row) => {
            const rowButton = document.createElement('button');
            rowButton.type = 'button';
            rowButton.className = 'home-designer-preview-row';
            rowButton.dataset.rowId = row.id;
            rowButton.dataset.previewRowId = row.id;
            rowButton.setAttribute('aria-current', String(state.selectionId === row.id));
            rowButton.textContent = row.name || 'Untitled row';
            rowButton.addEventListener('click', () => selectRow(row.id, 'preview'));
            host.append(rowButton);
        });
    };

    const renderRowsMode = (state) => {
        const controls = editor?.querySelector('[data-home-designer-rows-controls]');
        if (!controls) return;
        controls.replaceChildren();
        const message = document.createElement('p');
        message.textContent = state.rowsMode === 'inherit' ? 'Rows inherit the global configuration.' : 'Rows use a custom configuration.';
        const customize = document.createElement('button');
        customize.type = 'button'; customize.className = 'btn btn-secondary'; customize.textContent = 'Customize rows';
        customize.disabled = state.rowsMode === 'custom';
        customize.addEventListener('click', () => {
            store.dispatch({ type: 'rows/customize' });
            editor.querySelector('[data-home-designer-live]').textContent = 'Rows now use a custom configuration.';
            requestAnimationFrame(() => editor.querySelector('[data-home-designer-rows-controls] button:not([disabled])')?.focus());
        });
        controls.append(message, customize);
        if (state.scope?.kind !== 'global') {
            const reset = document.createElement('button');
            reset.type = 'button'; reset.className = 'btn btn-secondary'; reset.textContent = 'Reset to inherited';
            reset.disabled = state.rowsMode === 'inherit';
            reset.addEventListener('click', () => {
                store.dispatch({ type: 'rows/reset' });
                editor.querySelector('[data-home-designer-live]').textContent = 'Rows reset to the inherited configuration.';
                requestAnimationFrame(() => editor.querySelector('[data-home-designer-rows-controls] button:not([disabled])')?.focus());
            });
            controls.append(reset);
        }
    };

    const configureCatalog = (entry, index) => {
        store.dispatch({ type: 'catalog/configure', token: entry.type, index, values: {} });
        requestAnimationFrame(() => editor?.querySelector('[data-home-designer-inspector] [data-field-path]')?.focus());
    };

    const handleAddedRow = (id, entry) => {
        selectRow(id);
        if ((entry.fields || []).length) requestAnimationFrame(() => editor?.querySelector('[data-home-designer-inspector] [data-field-path]')?.focus());
    };

    const renderEditor = async () => {
        if (!editor || !store) return;
        if (!editorModules) editorModules = Promise.all([import('./library.js'), import('./outline.js')]);
        const [library, outline] = await editorModules;
        if (!store) return;
        const active = document.activeElement;
        const focusPath = active?.closest?.('[data-home-designer-editor]') ? active.dataset?.fieldPath : null;
        const focusStart = typeof active?.selectionStart === 'number' ? active.selectionStart : null;
        const focusEnd = typeof active?.selectionEnd === 'number' ? active.selectionEnd : null;
        const state = store.getState();
        renderRowsMode(state);
        library.renderLibrary(editor.querySelector('[data-home-designer-library]'), {
            state, dispatch: store.dispatch,
            onAdd: handleAddedRow,
            onConfigure: configureCatalog,
        });
        outline.renderOutline(editor.querySelector('[data-home-designer-outline]'), {
            state, dispatch: store.dispatch, liveRegion: editor.querySelector('[data-home-designer-live]'), onSelect: selectRow,
            onConfigure: configureCatalog, onAdd: handleAddedRow,
        });
        outline.renderInspector(editor.querySelector('[data-home-designer-inspector]'), {
            state, dispatch: store.dispatch, onSelect: selectRow,
            onCatalogSubmit: (entry, values) => {
                const rows = library.createCatalogRows(entry, state.rows, values);
                const index = Number.isInteger(state.catalogSelection?.index) ? state.catalogSelection.index : state.rows.length;
                rows.forEach((row, offset) => store.dispatch({ type: 'rows/add', row, index: index + offset }));
                store.dispatch({ type: 'catalog/cancel' });
                if (rows[0]) {
                    selectRow(rows[0].id);
                    requestAnimationFrame(() => editor.querySelector('[data-home-designer-inspector] [data-field-path]')?.focus());
                }
            },
        });
        renderPreviewPlaceholder(state);
        const applyButton = root.querySelector('[data-home-designer-apply]');
        if (applyButton) {
            const valid = store.isApplyValid?.() ?? true;
            applyButton.disabled = !valid;
            applyButton.setAttribute('aria-invalid', String(!valid));
            applyButton.setAttribute('aria-disabled', String(!valid));
        }
        editor.hidden = false;
        if (focusPath) requestAnimationFrame(() => {
            const replacement = editor.querySelector(`[data-field-path="${CSS.escape(focusPath)}"]`);
            replacement?.focus();
            if (focusStart !== null && typeof replacement?.setSelectionRange === 'function') replacement.setSelectionRange(focusStart, focusEnd);
        });
    };

    const connectEditor = () => {
        unsubscribe?.();
        if (!store || !editor) return;
        unsubscribe = store.subscribe(() => { renderEditor(); });
        renderEditor();
    };

    const load = async (scope) => {
        const current = beginOperation();
        try {
            const [{ loadDocument }, { createStore }] = await modules;
            const saved = await loadDocument(basePath, scope, { signal: current.controller.signal });
            if (!isCurrent(current)) return null;
            store = createStore(saved);
            connectEditor();
            return { current, saved };
        } catch (error) {
            return isCurrent(current) ? { current, error } : null;
        }
    };

    const bootstrap = async () => {
        if (!activeScope.kind || (activeScope.kind === 'profile' && !activeScope.profileId)) return;
        const result = await load(activeScope);
        if (!result || !isCurrent(result.current)) return;
        if (result.error) showFailure();
        else showReady();
    };

    const switchScope = async (scope) => {
        if (store?.isDirty() && typeof globalThis.confirm === 'function' && !globalThis.confirm('Discard unsaved Home Designer changes?')) return false;
        activeScope = { ...scope };
        const result = await load(activeScope);
        if (!result || !isCurrent(result.current)) return false;
        if (result.error) {
            showFailure();
            return false;
        }
        showReady();
        return true;
    };

    const apply = async () => {
        if (!store) return false;
        if (!(store.isApplyValid?.() ?? true)) {
            showMessage('home-designer-error', 'Complete the highlighted row settings before applying.');
            focusFirstInvalid();
            return false;
        }
        const request = store.buildApplyRequest();
        if (!request) return true;
        const applyingStore = store;
        const current = beginOperation();
        try {
            const [{ applyDocument, APIError }] = await modules;
            if (!isCurrent(current)) return false;
            const saved = await applyDocument(basePath, request, { signal: current.controller.signal });
            if (!isCurrent(current) || store !== applyingStore) return false;
            applyingStore.replaceWithSaved(saved);
            showReady();
            return true;
        } catch (error) {
            if (!isCurrent(current)) return false;
            const [{ APIError }] = await modules;
            if (!isCurrent(current)) return false;
            if (error instanceof APIError && error.code === 'revision_conflict') {
                status.replaceChildren();
                const message = document.createElement('p');
                message.className = 'home-designer-error';
                message.textContent = 'This document changed elsewhere. Reload the latest version before applying again.';
                const reload = document.createElement('button');
                reload.className = 'btn btn-primary';
                reload.type = 'button';
                reload.textContent = 'Reload latest';
                reload.addEventListener('click', async () => {
                    if (!isCurrent(current) || !sameScope(activeScope, request.scope)) return;
                    const result = await load(activeScope);
                    if (!result || !isCurrent(result.current)) return;
                    if (result.error) showFailure();
                    else showReady();
                });
                status.append(message, reload);
            } else {
                showMessage('home-designer-error', 'Home Designer could not apply changes. Try again.');
            }
            return false;
        }
    };

    root.homeDesigner = {
        get store() { return store; },
        apply,
        discard: () => {
            if (!store) return false;
            store.discard();
            showReady();
            return true;
        },
        switchScope,
    };

    root.querySelector('[data-home-designer-apply]')?.addEventListener('click', apply);
    root.querySelector('[data-home-designer-discard]')?.addEventListener('click', () => root.homeDesigner.discard());

    if (initialScope.kind === 'global' || initialScope.profileId) bootstrap();
}
