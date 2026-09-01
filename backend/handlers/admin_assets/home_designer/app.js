const root = document.getElementById('homeDesignerRoot');
const status = root?.querySelector('[data-home-designer-status]');
const editor = root?.querySelector('[data-home-designer-editor]');
const errors = root?.querySelector('[data-home-designer-errors]');
const drawerBackdrop = root?.querySelector('[data-home-designer-drawer-backdrop]');

if (root && status) {
    const basePath = root.dataset.basePath || '';
    const initialScope = root.dataset.isAdmin === 'true'
        ? { kind: 'global' }
        : { kind: 'profile', profileId: root.dataset.profileId || '' };
    const modules = Promise.all([import('./api.js'), import('./store.js')])
        .then(([api, editorStore]) => [api.default ?? api, editorStore.default ?? editorStore]);
    const workspaceModule = import('./workspace.js');
    let store = null;
    let activeScope = initialScope;
    let operation = 0;
    let activeOperation = null;
    let editorModules = null;
    let previewModules = null;
    let previewProfileId = '';
    let previewPlatform = 'tv';
    let previewRenderToken = 0;
    let canvasCleanup = null;
    let unsubscribe = null;
    let applyValidation = [];
    let beforeUnloadRegistered = false;
    let drawer = null;
    let drawerTool = null;
    let drawerReturnFocus = null;
    let drawerBackground = [];
    let drawerPortal = [];
    let drawerDialogAttributes = null;
    let drawerBackdropHidden = true;
    let workspaceReducer = null;
    let workspaceState = null;
    let workspaceObserver = null;
    const pendingDrafts = new Map();
    const warnBeforeUnload = (event) => { event.preventDefault(); event.returnValue = ''; return ''; };
    const isCompactWorkspace = () => workspaceState?.band === 'compact';
    const workspaceElement = () => editor?.querySelector('.home-designer-workspace') || editor;
    const workspaceToolButton = (tool) => [...(editor?.querySelectorAll('[data-home-designer-tool]') || [])]
        .find((button) => button.dataset.homeDesignerTool === tool) || null;

    const isUsableFocusTarget = (target) => {
        if (!target?.focus || target.isConnected === false) return false;
        for (let current = target; current; current = current.parentElement) {
            if (current.hidden || current.inert || current.getAttribute?.('aria-hidden') === 'true') return false;
        }
        return true;
    };

    const fallbackFocusTarget = (tool) => {
        const toolButton = workspaceToolButton(tool);
        if (isUsableFocusTarget(toolButton)) return toolButton;
        const canvas = editor?.querySelector('[data-home-designer-canvas]');
        const canvasControl = canvas?.querySelector('button, [href], [tabindex]');
        return isUsableFocusTarget(canvasControl) ? canvasControl : isUsableFocusTarget(canvas) ? canvas : null;
    };

    const renderWorkspace = () => {
        if (!editor || !workspaceState) return;
        const editing = workspaceState.mode === 'edit';
        editor.dataset.mode = workspaceState.mode;
        editor.dataset.band = workspaceState.band;
        const stateClasses = [
            'is-band-wide', 'is-band-standard', 'is-band-compact',
            'is-library-open', 'is-inspector-open', 'is-theme-open', 'is-dragging',
        ];
        editor.classList.remove(...stateClasses);
        editor.classList.add(`is-band-${workspaceState.band}`);
        if (editing && workspaceState.libraryOpen) editor.classList.add('is-library-open');
        if (editing && workspaceState.contextTool === 'inspector') editor.classList.add('is-inspector-open');
        if (editing && workspaceState.contextTool === 'theme') editor.classList.add('is-theme-open');
        if (editing && workspaceState.dragging) editor.classList.add('is-dragging');
        [
            '[data-home-designer-undo]',
            '[data-home-designer-redo]',
            '[data-home-designer-cancel]',
            '[data-home-designer-apply]',
            '[data-home-designer-rows-controls]',
            '[data-home-designer-tool]',
        ].forEach((selector) => editor.querySelectorAll(selector).forEach((element) => { element.hidden = !editing; }));
        editor.querySelector('[data-home-designer-edit]')?.toggleAttribute('hidden', editing);
        const library = findDesignerElement('[data-home-designer-library]');
        const context = findDesignerElement('[data-home-designer-context-drawer]');
        const inspector = findDesignerElement('[data-home-designer-inspector]');
        const theme = findDesignerElement('[data-home-designer-theme]');
        if (library) library.hidden = !editing || !workspaceState.libraryOpen;
        if (context) context.hidden = !editing || !workspaceState.contextTool;
        if (inspector) inspector.hidden = !editing || workspaceState.contextTool !== 'inspector';
        if (theme) theme.hidden = !editing || workspaceState.contextTool !== 'theme';
        editor.querySelectorAll('[data-home-designer-tool]').forEach((button) => {
            const active = button.dataset.homeDesignerTool === 'library'
                ? workspaceState.libraryOpen
                : workspaceState.contextTool === button.dataset.homeDesignerTool;
            button.setAttribute('aria-pressed', String(active));
        });
    };

    const dispatchWorkspace = (action) => {
        if (!workspaceState || !workspaceReducer) return workspaceState;
        const previousMode = workspaceState.mode;
        const previousBand = workspaceState.band;
        workspaceState = workspaceReducer(workspaceState, action);
        renderWorkspace();
        const drawerIsInactive = (tool) => (
            previousBand !== workspaceState.band && !isCompactWorkspace()
            || tool === 'library' && !workspaceState.libraryOpen
            || tool !== 'library' && !workspaceState.contextTool
        );
        const shouldCloseDrawer = drawer && drawerIsInactive(drawerTool);
        const closeInactiveDrawer = (activeDrawer, tool) => {
            if (drawer !== activeDrawer || !drawerIsInactive(tool)) return;
            closeDrawer(false);
            renderWorkspace();
        };
        if (shouldCloseDrawer) {
            const activeDrawer = drawer;
            const activeTool = drawerTool;
            if (action.type === 'drag/start' && activeTool === 'library') requestAnimationFrame(() => closeInactiveDrawer(activeDrawer, activeTool));
            else closeInactiveDrawer(activeDrawer, activeTool);
        }
        if (!drawer && previousBand !== workspaceState.band && isCompactWorkspace() && workspaceState.mode === 'edit') {
            const activeTool = workspaceState.libraryOpen ? 'library' : workspaceState.contextTool;
            if (activeTool) {
                const inlineDrawer = activeTool === 'library'
                    ? findDesignerElement('[data-home-designer-library]')
                    : findDesignerElement('[data-home-designer-context-drawer]') || findDesignerElement(`[data-home-designer-${activeTool}]`);
                const returnTarget = inlineDrawer?.contains?.(document.activeElement) ? workspaceToolButton(activeTool) : null;
                openDrawer(activeTool, returnTarget);
            }
        }
        if (workspaceState.mode !== previousMode) void renderEditor();
        return workspaceState;
    };

    const openWorkspaceTool = (tool) => {
        const workspace = dispatchWorkspace({ type: `tool/${tool}`, open: true });
        if (workspace?.band === 'compact') openDrawer(tool);
        return workspace;
    };

    const initializeWorkspace = async () => {
        if (!editor || workspaceState) return;
        const workspace = await workspaceModule;
        if (!editor || workspaceState) return;
        const observedWorkspace = workspaceElement();
        workspaceReducer = workspace.reduceWorkspace;
        workspaceState = workspace.createWorkspaceState(observedWorkspace.getBoundingClientRect().width);
        renderWorkspace();
        if (typeof ResizeObserver === 'function') {
            workspaceObserver = new ResizeObserver((entries = []) => {
                const entry = entries.find?.((candidate) => candidate.target === observedWorkspace) || entries[0];
                dispatchMeasuredWorkspaceWidth(entry?.contentRect?.width ?? observedWorkspace.getBoundingClientRect().width);
            });
            workspaceObserver.observe(observedWorkspace);
        }
    };

    const dispatchMeasuredWorkspaceWidth = (width) => {
        const measured = Number(width);
        if (!Number.isFinite(measured) || measured <= 0 || editor?.hidden || root.getAttribute?.('aria-busy') === 'true') return workspaceState;
        return dispatchWorkspace({ type: 'resize', width: measured });
    };

    const remeasureWorkspace = () => {
        const observedWorkspace = workspaceElement();
        if (observedWorkspace) dispatchMeasuredWorkspaceWidth(observedWorkspace.getBoundingClientRect().width);
    };

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

    const clearErrors = () => errors?.replaceChildren();
    const clearValidationState = () => {
        applyValidation = [];
        clearErrors();
    };
    const showActionError = (text, actionLabel = '', action = null) => {
        if (!errors) {
            showMessage('home-designer-error', text);
            return;
        }
        errors.replaceChildren();
        const alert = document.createElement('div');
        alert.className = 'home-designer-error';
        alert.setAttribute('role', 'alert');
        alert.tabIndex = -1;
        const message = document.createElement('p');
        message.textContent = text;
        alert.append(message);
        if (actionLabel && action) {
            const button = document.createElement('button');
            button.className = 'btn btn-primary';
            button.type = 'button';
            button.textContent = actionLabel;
            button.addEventListener('click', action);
            alert.append(button);
        }
        errors.append(alert);
        requestAnimationFrame(() => alert.focus?.());
    };

    const hasUnsavedWork = () => Boolean(store?.isDirty?.() || pendingDrafts.size);
    const syncDirtyProtection = () => {
        const dirty = hasUnsavedWork();
        if (dirty === beforeUnloadRegistered) return;
        if (dirty) globalThis.addEventListener?.('beforeunload', warnBeforeUnload);
        else globalThis.removeEventListener?.('beforeunload', warnBeforeUnload);
        beforeUnloadRegistered = dirty;
    };

    const confirmDiscard = (message) => !hasUnsavedWork() || typeof globalThis.confirm !== 'function' || globalThis.confirm(message);
    const confirmReset = (message) => typeof globalThis.confirm !== 'function' || globalThis.confirm(message);
    const draftKey = (target) => target?.dataset?.themePath ? `theme:${target.dataset.themePath}` : target?.dataset?.fieldPath ? `rows:${target.dataset.fieldPath}` : '';
    const clearDraft = (target) => { const key = draftKey(target); if (key) pendingDrafts.delete(key); syncDirtyProtection(); };
    const clearDrafts = () => { pendingDrafts.clear(); syncDirtyProtection(); };
    const isEditableTarget = (target) => Boolean(target?.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target?.tagName));
    const findDesignerElement = (selector) => editor?.querySelector(selector)
        || (drawer?.matches?.(selector) ? drawer : drawer?.querySelector?.(selector))
        || null;
    const focusDesignerPath = (attribute, path) => {
        if (!editor && !drawer) return;
        findDesignerElement(`[${attribute}="${CSS.escape(path)}"]`)?.focus();
    };
    const focusThemeValidation = (path) => {
        if (!editor && !drawer) return;
        const input = findDesignerElement(`[data-theme-path="${CSS.escape(path)}"]`);
        (input || findDesignerElement('[data-home-designer-theme-validation]'))?.focus();
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

    const selectRow = (id) => {
        store?.dispatch({ type: 'selection/select', id });
        if (workspaceState?.mode === 'edit') openWorkspaceTool('inspector');
        requestAnimationFrame(() => {
            editor?.querySelector(`[data-preview-row-id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: 'nearest' });
        });
    };

    const focusFirstInvalid = () => {
        const validation = store?.getRowValidation?.() || {};
        const [id, errors] = Object.entries(validation)[0] || [];
        if (!id || !errors?.[0]) return;
        selectRow(id);
        openWorkspaceTool('inspector');
        requestAnimationFrame(() => focusDesignerPath('data-field-path', errors[0].path));
    };

    const focusFirstServerInvalid = (fields) => {
        const first = Array.isArray(fields) ? fields[0] : null;
        if (!first) return;
        if (first.section === 'rows' && first.rowId) {
            selectRow(first.rowId);
            openWorkspaceTool('inspector');
            const row = (store?.getState?.().rows || []).find((candidate) => candidate.id === first.rowId);
            const itemIndex = first.itemId ? row?.collectionItems?.findIndex((item) => item.id === first.itemId) : -1;
            const path = itemIndex >= 0 ? `collectionItems.${itemIndex}.${first.path}` : first.path;
            requestAnimationFrame(() => focusDesignerPath('data-field-path', path));
            return;
        }
        if (first.section === 'theme') {
            openWorkspaceTool('theme');
            requestAnimationFrame(() => focusThemeValidation(first.path));
            return;
        }
        if (first.section === 'scope') {
            requestAnimationFrame(() => editor?.querySelector('[data-home-designer-scope]')?.focus());
            return;
        }
        if (first.section === 'rows') requestAnimationFrame(() => editor?.querySelector('[data-home-designer-rows-controls] button:not([disabled])')?.focus());
    };

    const validationKey = (error) => [error?.section || '', error?.rowId || '', error?.itemId || '', error?.path || ''].join(':');
    const serverValidationByField = (state) => {
        const rows = {};
        const theme = [];
        const sectionValidation = {};
        applyValidation.forEach((error) => {
            if (!error?.section || !error?.path) return;
            if (error.section === 'theme') { theme.push(error); return; }
            if (error.section !== 'rows') { (sectionValidation[error.section] ||= []).push(error); return; }
            if (!error.rowId) { (sectionValidation.rows ||= []).push(error); return; }
            const row = (state.rows || []).find((candidate) => candidate.id === error.rowId);
            let path = error.path;
            if (error.itemId && row?.collectionItems) {
                const index = row.collectionItems.findIndex((item) => item.id === error.itemId);
                if (index >= 0) path = `collectionItems.${index}.${error.path}`;
            }
            (rows[error.rowId] ||= []).push({ ...error, path });
        });
        return { rows, theme, sectionValidation };
    };

    const clearServerValidation = (identity) => {
        const key = validationKey(identity);
        const previous = applyValidation.length;
        applyValidation = applyValidation.filter((error) => validationKey(error) !== key);
        if (previous !== applyValidation.length && !applyValidation.length) clearErrors();
    };

    const renderPreview = async (state) => {
        const token = ++previewRenderToken;
        const host = editor?.querySelector('[data-home-designer-preview-host]');
        if (!host) return;
        if (!previewModules) previewModules = Promise.all([import('./theme.js'), import('./preview.js')]);
        const [[theme, preview], [, , canvas]] = await Promise.all([previewModules, editorModules]);
        if (token !== previewRenderToken || !store || store.getState?.().revision !== state.revision) return;
        const renderer = previewPlatform === 'mobile' ? preview.renderMobilePreview : preview.renderTVPreview;
        canvasCleanup?.();
        canvasCleanup = null;
        renderer(host, state, { editing: workspaceState?.mode === 'edit', onSelect: selectRow });
        host.querySelector('.home-preview-device') && theme.applyThemeVariables(host.querySelector('.home-preview-device'), state.theme);
        host.querySelectorAll('[data-preview-row-id]').forEach((row) => row.setAttribute('aria-current', String(state.selectionId === row.dataset.previewRowId)));
        canvasCleanup = canvas.mountCanvasInteractions(host, {
            state, getState: store.getState, editing: workspaceState?.mode === 'edit', dispatch: store.dispatch,
            liveRegion: editor.querySelector('[data-home-designer-live]'),
            onSelect: selectRow,
            onCustomize: (id) => { selectRow(id); openWorkspaceTool('inspector'); },
            onCatalogDrop: addCatalogAt,
            onDragStart: () => dispatchWorkspace({ type: 'drag/start' }),
            onDragEnd: () => dispatchWorkspace({ type: 'drag/end' }),
        });
    };

    const renderRowsMode = (state, sectionErrors = []) => {
        const controls = editor?.querySelector('[data-home-designer-rows-controls]');
        if (!controls) return;
        controls.replaceChildren();
        const message = document.createElement('p');
        message.textContent = state.rowsMode === 'inherit' ? 'Inheriting the global row configuration.' : 'Customized row configuration.';
        const customize = document.createElement('button');
        customize.type = 'button'; customize.className = 'btn btn-secondary'; customize.textContent = 'Customize rows';
        customize.disabled = state.rowsMode === 'custom';
        customize.addEventListener('click', () => {
            store.dispatch({ type: 'rows/customize' });
            editor.querySelector('[data-home-designer-live]').textContent = 'Rows now use a custom configuration.';
            requestAnimationFrame(() => editor.querySelector('[data-home-designer-rows-controls] button:not([disabled])')?.focus());
        });
        controls.append(message, customize);
        sectionErrors.forEach((error, index) => {
            const fieldError = document.createElement('p');
            fieldError.className = 'home-designer-field-error';
            fieldError.id = `home-designer-rows-error-${index}`;
            fieldError.textContent = error.message;
            controls.append(fieldError);
            customize.setAttribute('aria-invalid', 'true');
            customize.setAttribute('aria-describedby', fieldError.id);
        });
        if (state.scope?.kind !== 'global') {
            const reset = document.createElement('button');
            reset.type = 'button'; reset.className = 'btn btn-secondary'; reset.textContent = 'Reset to inherited';
            reset.disabled = state.rowsMode === 'inherit';
            reset.addEventListener('click', () => {
                if (!confirmReset('Reset Rows to the inherited configuration? This discards the Rows override.')) return;
                store.dispatch({ type: 'rows/reset' });
                editor.querySelector('[data-home-designer-live]').textContent = 'Rows reset to the inherited configuration.';
                requestAnimationFrame(() => editor.querySelector('[data-home-designer-rows-controls] button:not([disabled])')?.focus());
            });
            controls.append(reset);
        }
    };

    const configureCatalog = (entry, index) => {
        store.dispatch({ type: 'catalog/configure', token: entry.type, index, values: {} });
        openWorkspaceTool('inspector');
        requestAnimationFrame(() => findDesignerElement('[data-home-designer-inspector] [data-field-path]')?.focus());
    };

    const handleAddedRow = (id) => {
        selectRow(id);
        requestAnimationFrame(() => {
            const field = findDesignerElement('[data-home-designer-inspector]')?.querySelector('[data-field-path]');
            const row = editor?.querySelector(`[data-preview-row-id="${CSS.escape(id)}"]`);
            (field || row)?.focus?.();
        });
    };

    const addCatalogAt = async (token, index, values) => {
        const [library] = await editorModules;
        const state = store.getState();
        const entry = library.findCatalogEntry(state.catalog, token);
        if (!entry) return false;
        const insertion = Number.isInteger(index) ? index : state.rows.length;
        const configured = entry.catalogOnly && values !== undefined;
        if (entry.catalogOnly && !configured) {
            configureCatalog(entry, insertion);
            return true;
        }
        const rows = library.createCatalogRows(entry, state.rows, values);
        rows.forEach((row, offset) => store.dispatch({ type: 'rows/add', row, index: insertion + offset }));
        if (configured) store.dispatch({ type: 'catalog/cancel' });
        if (rows[0]) handleAddedRow(rows[0].id);
        return rows.length > 0;
    };

    const renderEditor = async () => {
        if (!editor || !store) return;
        if (!editorModules) editorModules = Promise.all([import('./library.js'), import('./inspector.js'), import('./canvas.js')]);
        const [library, inspector] = await editorModules;
        if (!store) return;
        const active = document.activeElement;
        const activeInDesigner = editor.contains?.(active) || drawer?.contains(active);
        const focusPath = activeInDesigner ? active?.dataset?.fieldPath : null;
        const themePath = activeInDesigner ? active?.dataset?.themePath : null;
        const focusStart = typeof active?.selectionStart === 'number' ? active.selectionStart : null;
        const focusEnd = typeof active?.selectionEnd === 'number' ? active.selectionEnd : null;
        const state = store.getState();
        const validation = serverValidationByField(state);
        const serverRowValidation = validation.rows;
        const inspectorState = Object.keys(serverRowValidation).length
            ? { ...state, rowValidation: Object.fromEntries(Object.entries({ ...state.rowValidation, ...serverRowValidation }).map(([id, rowErrors]) => [id, [...(state.rowValidation?.[id] || []), ...(serverRowValidation[id] || [])]])) }
            : state;
        renderRowsMode(state, validation.sectionValidation.rows || []);
        library.renderLibrary(findDesignerElement('[data-home-designer-library]'), {
            state,
            onAdd: addCatalogAt,
            onDragStart: () => dispatchWorkspace({ type: 'drag/start' }),
            onDragEnd: () => dispatchWorkspace({ type: 'drag/end' }),
        });
        inspector.renderInspector(findDesignerElement('[data-home-designer-inspector]'), {
            state: inspectorState, dispatch: store.dispatch, onSelect: selectRow,
            sectionValidation: validation.sectionValidation,
            onFieldEdit: (identity) => clearServerValidation(identity),
            onCatalogSubmit: (entry, values) => {
                const current = store.getState();
                const index = Number.isInteger(current.catalogSelection?.index) ? current.catalogSelection.index : current.rows.length;
                return addCatalogAt(current.catalogSelection?.token || entry.type, index, values);
            },
        });
        if (!previewModules) previewModules = Promise.all([import('./theme.js'), import('./preview.js')]);
        const [theme] = await previewModules;
        theme.renderTheme(findDesignerElement('[data-home-designer-theme]'), {
            state, dispatch: store.dispatch, errors: validation.theme,
            onFieldEdit: (path) => clearServerValidation({ section: 'theme', path }),
            onReset: () => {
                if (!confirmReset('Reset Theme to the inherited appearance? This discards the Theme override.')) return false;
                clearServerValidation({ section: 'theme', path: 'mode' });
                clearServerValidation({ section: 'theme', path: 'value' });
                store.dispatch({ type: 'theme/reset' });
                editor.querySelector('[data-home-designer-live]').textContent = 'Theme reset to the inherited appearance.';
                return true;
            },
        });
        previewProfileId ||= state.previewProfiles?.[0]?.id || state.scope?.profileId || '';
        const profileSelect = editor.querySelector('[data-home-designer-preview-profile]');
        if (profileSelect && !profileSelect.dataset.ready) {
            profileSelect.dataset.ready = 'true';
            profileSelect.addEventListener('change', () => {
                previewProfileId = profileSelect.value;
                void renderPreview(store.getState());
            });
        }
        if (profileSelect) {
            profileSelect.replaceChildren();
            (state.previewProfiles || []).forEach((profile) => { const option = document.createElement('option'); option.value = profile.id; option.textContent = profile.displayName || profile.id; option.selected = profile.id === previewProfileId; profileSelect.append(option); });
        }
        const platformSelect = editor.querySelector('[data-home-designer-preview-platform]');
        if (platformSelect && !platformSelect.dataset.ready) {
            platformSelect.dataset.ready = 'true';
            platformSelect.addEventListener('change', () => { previewPlatform = platformSelect.value === 'mobile' ? 'mobile' : 'tv'; void renderPreview(store.getState()); });
        }
        if (platformSelect) platformSelect.value = previewPlatform;
        const scopeSelect = editor.querySelector('[data-home-designer-scope]');
        if (scopeSelect && !scopeSelect.dataset.ready) {
            scopeSelect.dataset.ready = 'true';
            scopeSelect.addEventListener('change', async () => {
                const [kind, profileId = ''] = scopeSelect.value.split(':', 2);
                const next = kind === 'profile' ? { kind, profileId } : { kind: 'global' };
                if (!await switchScope(next)) scopeSelect.value = activeScope.kind === 'profile' ? `profile:${activeScope.profileId}` : 'global';
            });
        }
        if (scopeSelect) {
            scopeSelect.replaceChildren();
            if (state.permissions?.canEditGlobal) {
                const option = document.createElement('option'); option.value = 'global'; option.textContent = 'Server defaults'; scopeSelect.append(option);
            }
            (state.previewProfiles || []).forEach((profile) => {
                const option = document.createElement('option'); option.value = `profile:${profile.id}`; option.textContent = profile.displayName || profile.id; scopeSelect.append(option);
            });
            scopeSelect.value = state.scope?.kind === 'profile' ? `profile:${state.scope.profileId}` : 'global';
            const existingScopeError = scopeSelect.parentElement?.querySelector?.('.home-designer-scope-error');
            existingScopeError?.remove();
            const scopeError = validation.sectionValidation.scope?.[0];
            if (scopeError) {
                const message = document.createElement('p');
                message.className = 'home-designer-field-error home-designer-scope-error';
                message.id = 'home-designer-scope-error';
                message.textContent = scopeError.message;
                scopeSelect.setAttribute('aria-invalid', 'true');
                scopeSelect.setAttribute('aria-describedby', message.id);
                scopeSelect.parentElement?.append(message);
            } else {
                scopeSelect.removeAttribute('aria-invalid');
                scopeSelect.removeAttribute('aria-describedby');
            }
        }
        const undoButton = root.querySelector('[data-home-designer-undo]');
        const redoButton = root.querySelector('[data-home-designer-redo]');
        if (undoButton) undoButton.disabled = !(store.canUndo?.());
        if (redoButton) redoButton.disabled = !(store.canRedo?.());
        mountDrawerControls();
        void renderPreview(state);
        const applyButton = root.querySelector('[data-home-designer-apply]');
        if (applyButton) {
            const valid = store.isApplyValid?.() ?? true;
            applyButton.disabled = !valid;
            applyButton.setAttribute('aria-invalid', String(!valid));
            applyButton.setAttribute('aria-disabled', String(!valid));
        }
        editor.hidden = false;
        renderWorkspace();
        if (focusPath || themePath) requestAnimationFrame(() => {
            const replacement = focusPath
                ? findDesignerElement(`[data-field-path="${CSS.escape(focusPath)}"]`)
                : findDesignerElement(`[data-theme-path="${CSS.escape(themePath)}"]`);
            replacement?.focus();
            if (focusStart !== null && typeof replacement?.setSelectionRange === 'function') replacement.setSelectionRange(focusStart, focusEnd);
        });
    };

    const setBackgroundInert = (inert) => {
        if (inert && drawer) {
            const nodes = [];
            for (let child = drawer; child?.parentElement; child = child.parentElement) {
                const parent = child.parentElement;
                nodes.push(...Array.from(parent.children || []).filter((node) => node !== child && node !== drawerBackdrop));
            }
            drawerBackground = nodes.map((node) => ({ node, inert: Boolean(node.inert), ariaHidden: node.getAttribute?.('aria-hidden') }));
            drawerBackground.forEach(({ node }) => { node.inert = true; node.setAttribute?.('aria-hidden', 'true'); });
            if (drawerBackdrop) drawerBackdrop.hidden = false;
            return;
        }
        drawerBackground.forEach(({ node, inert: wasInert, ariaHidden }) => {
            node.inert = wasInert;
            if (ariaHidden === null || ariaHidden === undefined) node.removeAttribute?.('aria-hidden');
            else node.setAttribute?.('aria-hidden', ariaHidden);
        });
        drawerBackground = [];
        if (drawerBackdrop) drawerBackdrop.hidden = true;
    };

    const portalDrawer = (target) => {
        const body = document.body;
        if (!body) return;
        const nodes = [drawerBackdrop, target].filter(Boolean);
        drawerPortal = nodes.map((node) => ({ node, parent: node.parentNode, nextSibling: node.nextSibling }));
        drawerPortal.forEach(({ node }) => body.append(node));
    };

    const restoreDrawerPortal = () => {
        drawerPortal.forEach(({ node, parent, nextSibling }) => {
            if (!parent) return;
            parent.insertBefore(node, nextSibling?.parentNode === parent ? nextSibling : null);
        });
        drawerPortal = [];
    };

    const guardDrawerFocus = (event) => {
        if (drawer && event.target && !drawer.contains(event.target)) drawer.querySelector('.home-designer-drawer-close, input, select, button, [href]')?.focus?.();
    };

    const closeDrawer = (synchronizeWorkspace = true) => {
        if (!drawer) return;
        const tool = drawerTool;
        drawer.classList.remove('is-drawer-open');
        ['role', 'aria-modal'].forEach((name) => {
            const value = drawerDialogAttributes?.[name];
            if (value === null || value === undefined) drawer.removeAttribute(name);
            else drawer.setAttribute(name, value);
        });
        setBackgroundInert(false);
        document.body?.classList.remove('home-designer-drawer-open');
        document.removeEventListener?.('focusin', guardDrawerFocus, true);
        document.removeEventListener?.('keydown', handleKeyboard);
        detachDrawerDelegates(drawer);
        restoreDrawerPortal();
        if (drawerBackdrop) drawerBackdrop.hidden = drawerBackdropHidden;
        const returnFocus = drawerReturnFocus;
        drawer = null;
        drawerTool = null;
        drawerReturnFocus = null;
        drawerDialogAttributes = null;
        if (synchronizeWorkspace && tool && isCompactWorkspace()) dispatchWorkspace({ type: `tool/${tool}`, open: false });
        requestAnimationFrame(() => {
            const target = isUsableFocusTarget(returnFocus) ? returnFocus : fallbackFocusTarget(tool);
            target?.focus?.();
        });
    };

    const openDrawer = (kind, trigger) => {
        if (!isCompactWorkspace()) return;
        const directTarget = findDesignerElement(`[data-home-designer-${kind}]`);
        const target = kind === 'library'
            ? directTarget
            : findDesignerElement('[data-home-designer-context-drawer]') || directTarget;
        if (!target) return;
        if (drawer === target) { drawerTool = kind; return; }
        if (drawer && drawer !== target) closeDrawer(false);
        drawer = target;
        drawerTool = kind;
        const activeElement = document.activeElement;
        drawerReturnFocus = trigger || (target.contains?.(activeElement) ? workspaceToolButton(kind) : activeElement);
        drawerDialogAttributes = { role: target.getAttribute('role'), 'aria-modal': target.getAttribute('aria-modal') };
        drawerBackdropHidden = Boolean(drawerBackdrop?.hidden);
        portalDrawer(target);
        target.classList.add('is-drawer-open');
        target.setAttribute('role', 'dialog');
        target.setAttribute('aria-modal', 'true');
        setBackgroundInert(true);
        document.body?.classList.add('home-designer-drawer-open');
        document.addEventListener?.('focusin', guardDrawerFocus, true);
        document.addEventListener?.('keydown', handleKeyboard);
        attachDrawerDelegates(target);
        requestAnimationFrame(() => target.querySelector('.home-designer-drawer-close, input, select, button, [href]')?.focus?.());
    };

    drawerBackdrop?.addEventListener('click', closeDrawer);

    const mountDrawerControls = () => {
        [['library', 'Row library'], ['context-drawer', 'Home Designer controls']].forEach(([kind, label]) => {
            const target = findDesignerElement(`[data-home-designer-${kind}]`)
                || (kind === 'context-drawer' ? findDesignerElement('[data-home-designer-inspector]') : null);
            if (!target || target.querySelector('.home-designer-drawer-close')) return;
            const close = document.createElement('button');
            close.type = 'button'; close.className = 'btn btn-secondary home-designer-drawer-close'; close.textContent = `Close ${label}`;
            close.addEventListener('click', closeDrawer);
            target.append(close);
        });
    };

    const handleKeyboard = (event) => {
        if (drawer?.contains(event.target)) {
            if (event.key === 'Escape') { event.preventDefault(); closeDrawer(); return; }
            if (event.key === 'Tab') {
                const focusable = [...drawer.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')];
                if (!focusable.length) return;
                const first = focusable[0]; const last = focusable.at(-1);
                if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
                else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
            }
        }
        if (isEditableTarget(event.target)) return;
        if (workspaceState?.mode !== 'edit') return;
        if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'z') {
            event.preventDefault();
            if (event.shiftKey) store?.redo?.(); else store?.undo?.();
        }
        if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'y') {
            event.preventDefault(); store?.redo?.();
        }
    };
    root.addEventListener?.('keydown', handleKeyboard);
    const handleClick = (event) => {
        const toolButton = event.target?.closest?.('[data-home-designer-tool], [data-home-designer-open-library], [data-home-designer-open-inspector]');
        if (toolButton) {
            const legacyDrawer = toolButton.hasAttribute?.('data-home-designer-open-library') || toolButton.hasAttribute?.('data-home-designer-open-inspector');
            const kind = toolButton.dataset?.homeDesignerTool || (toolButton.hasAttribute?.('data-home-designer-open-library') ? 'library' : toolButton.hasAttribute?.('data-home-designer-open-inspector') ? 'inspector' : '');
            if (legacyDrawer) { openDrawer(kind, toolButton); return; }
            const state = dispatchWorkspace({ type: `tool/${kind}` });
            const opened = kind === 'library' ? state?.libraryOpen : state?.contextTool === kind;
            if (opened && state?.band === 'compact') openDrawer(kind, toolButton);
            else if (!opened && drawer) closeDrawer(false);
            return;
        }
        const link = event.target?.closest?.('a[href]');
        if (link && !event.defaultPrevented && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey && !confirmDiscard('Discard unsaved Home Designer changes and leave this page?')) event.preventDefault();
    };
    const handleInput = (event) => {
        const target = event.target;
        const key = draftKey(target);
        if (!key || !isEditableTarget(target) || ['checkbox', 'radio', 'color'].includes(target.type)) return;
        target.dataset.homeDesignerDraft = 'true';
        pendingDrafts.set(key, target.value);
        syncDirtyProtection();
    };
    const handleChange = (event) => {
        const target = event.target;
        if (!draftKey(target)) return;
        delete target.dataset.homeDesignerDraft;
        clearDraft(target);
    };
    const handleWorkspaceDragStart = () => dispatchWorkspace({ type: 'drag/start' });
    const handleWorkspaceDragEnd = () => dispatchWorkspace({ type: 'drag/end' });
    const attachDrawerDelegates = (target) => {
        target?.addEventListener?.('click', handleClick);
        target?.addEventListener?.('input', handleInput);
        target?.addEventListener?.('change', handleChange);
        target?.addEventListener?.('dragstart', handleWorkspaceDragStart);
        target?.addEventListener?.('dragend', handleWorkspaceDragEnd);
    };
    const detachDrawerDelegates = (target) => {
        target?.removeEventListener?.('click', handleClick);
        target?.removeEventListener?.('input', handleInput);
        target?.removeEventListener?.('change', handleChange);
        target?.removeEventListener?.('dragstart', handleWorkspaceDragStart);
        target?.removeEventListener?.('dragend', handleWorkspaceDragEnd);
    };
    root.addEventListener?.('click', handleClick);
    root.addEventListener?.('input', handleInput);
    root.addEventListener?.('change', handleChange);
    root.addEventListener?.('dragstart', handleWorkspaceDragStart);
    root.addEventListener?.('dragend', handleWorkspaceDragEnd);

    const connectEditor = () => {
        unsubscribe?.();
        if (!store || !editor) return;
        unsubscribe = store.subscribe(() => {
            syncDirtyProtection();
            renderEditor();
        });
        syncDirtyProtection();
        renderEditor();
    };

    const setWorkspaceLoading = (loading) => {
        if (editor) editor.hidden = Boolean(loading);
        root.setAttribute?.('aria-busy', String(Boolean(loading)));
        if (!loading) {
            if (typeof requestAnimationFrame === 'function') requestAnimationFrame(remeasureWorkspace);
            else remeasureWorkspace();
        }
    };

    const load = async (scope) => {
        const current = beginOperation();
        try {
            const [{ loadDocument }, { createStore }] = await modules;
            const saved = await loadDocument(basePath, scope, { signal: current.controller.signal });
            if (!isCurrent(current)) return null;
            return { current, saved };
        } catch (error) {
            return isCurrent(current) ? { current, error } : null;
        }
    };

    const replaceWithLoadedDocument = async (saved) => {
        clearValidationState();
        clearDrafts();
        const [, { createStore }] = await modules;
        store = createStore(saved);
        connectEditor();
    };

    const bootstrap = async () => {
        if (!activeScope.kind || (activeScope.kind === 'profile' && !activeScope.profileId)) return;
        setWorkspaceLoading(true);
        const result = await load(activeScope);
        if (!result || !isCurrent(result.current)) return;
        if (result.error) showFailure();
        else {
            await replaceWithLoadedDocument(result.saved);
            setWorkspaceLoading(false);
            showReady();
        }
    };

    const switchScope = async (scope) => {
        if (!confirmDiscard('Discard unsaved Home Designer changes?')) return false;
        const previousScope = { ...activeScope };
        if (hasUnsavedWork()) {
            clearValidationState();
            store?.discard?.();
            clearDrafts();
        }
        setWorkspaceLoading(true);
        const result = await load(scope);
        if (!result || !isCurrent(result.current)) {
            setWorkspaceLoading(false);
            return false;
        }
        if (result.error) {
            setWorkspaceLoading(false);
            activeScope = previousScope;
            void renderEditor();
            showFailure();
            return false;
        }
        activeScope = { ...scope };
        await replaceWithLoadedDocument(result.saved);
        setWorkspaceLoading(false);
        showReady();
        return true;
    };

    const apply = async () => {
        if (!store || root.getAttribute?.('aria-busy') === 'true') return false;
        if (!(store.isApplyValid?.() ?? true)) {
            showActionError('Complete the highlighted row settings before applying.');
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
            clearValidationState();
            clearDrafts();
            applyingStore.replaceWithSaved(saved);
            syncDirtyProtection();
            dispatchWorkspace({ type: 'edit/applied' });
            showReady();
            return true;
        } catch (error) {
            if (!isCurrent(current)) return false;
            const [{ APIError }] = await modules;
            if (!isCurrent(current)) return false;
                if (error instanceof APIError && error.code === 'revision_conflict') {
                    showActionError('This document changed elsewhere. Reload the latest version before applying again.', 'Reload latest', async () => {
                        if (!isCurrent(current) || !sameScope(activeScope, request.scope)) return;
                        setWorkspaceLoading(true);
                        const result = await load(activeScope);
                        if (!result || !isCurrent(result.current)) { setWorkspaceLoading(false); return; }
                        if (result.error) { setWorkspaceLoading(false); showFailure(); }
                        else {
                            await replaceWithLoadedDocument(result.saved);
                            setWorkspaceLoading(false);
                            showReady();
                        }
                    });
            } else if (error instanceof APIError && (error.status === 422 || error.code === 'validation_error')) {
                applyValidation = Array.isArray(error.fields) ? error.fields : [];
                showActionError(error.message || 'Correct the highlighted settings before applying.');
                void renderEditor();
                focusFirstServerInvalid(applyValidation);
            } else {
                showActionError('Home Designer could not apply changes. Your edits are still here.', 'Retry', apply);
            }
            return false;
        }
    };

    root.homeDesigner = {
        get store() { return store; },
        apply,
        cancel: () => {
            if (!store || !confirmDiscard('Discard unsaved Home Designer changes?')) return false;
            clearValidationState();
            clearDrafts();
            store.discard();
            syncDirtyProtection();
            dispatchWorkspace({ type: 'edit/cancel' });
            showReady();
            return true;
        },
        discard: () => {
            if (!store) return false;
            clearValidationState();
            clearDrafts();
            store.discard();
            syncDirtyProtection();
            showReady();
            return true;
        },
        switchScope,
    };

    root.querySelector('[data-home-designer-edit]')?.addEventListener('click', () => dispatchWorkspace({ type: 'edit/start' }));
    root.querySelector('[data-home-designer-apply]')?.addEventListener('click', apply);
    root.querySelector('[data-home-designer-cancel]')?.addEventListener('click', () => root.homeDesigner.cancel());
    root.querySelector('[data-home-designer-undo]')?.addEventListener('click', () => store?.undo?.());
    root.querySelector('[data-home-designer-redo]')?.addEventListener('click', () => store?.redo?.());

    void initializeWorkspace();
    if (initialScope.kind === 'global' || initialScope.profileId) bootstrap();
}
