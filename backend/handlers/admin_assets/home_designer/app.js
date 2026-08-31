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
    let store = null;
    let activeScope = initialScope;
    let operation = 0;
    let activeOperation = null;
    let editorModules = null;
    let previewModules = null;
    let previewController = null;
    let previewObserver = null;
    let previewProfileId = '';
    let previewPlatform = 'tv';
    let previewResults = {};
    let previewRenderToken = 0;
    let previewRowsSignature = '';
    let unsubscribe = null;
    let applyValidation = [];
    let beforeUnloadRegistered = false;
    let drawer = null;
    let drawerReturnFocus = null;
    let drawerBackground = [];
    const pendingDrafts = new Map();
    const drawerMedia = globalThis.matchMedia?.('(max-width: 1100px)');
    const warnBeforeUnload = (event) => { event.preventDefault(); event.returnValue = ''; return ''; };

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

    const focusFirstServerInvalid = (fields) => {
        const first = Array.isArray(fields) ? fields[0] : null;
        if (!first) return;
        if (first.section === 'rows' && first.rowId) {
            selectRow(first.rowId);
            const row = (store?.getState?.().rows || []).find((candidate) => candidate.id === first.rowId);
            const itemIndex = first.itemId ? row?.collectionItems?.findIndex((item) => item.id === first.itemId) : -1;
            const path = itemIndex >= 0 ? `collectionItems.${itemIndex}.${first.path}` : first.path;
            requestAnimationFrame(() => editor?.querySelector(`[data-field-path="${CSS.escape(path)}"]`)?.focus());
            return;
        }
        if (first.section === 'theme') {
            requestAnimationFrame(() => editor?.querySelector(`[data-theme-path="${CSS.escape(first.path)}"]`)?.focus());
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

    const previewSignature = (state) => JSON.stringify({ scope: state?.scope || {}, rows: state?.rows || [] });
    const invalidatePreview = () => {
        previewRenderToken += 1;
        previewResults = {};
        previewObserver?.disconnect();
        previewController?.invalidate();
    };

    const renderPreview = async (state, { schedule = true } = {}) => {
        const token = ++previewRenderToken;
        const host = editor?.querySelector('[data-home-designer-preview-host]');
        if (!host) return;
        if (!previewModules) previewModules = Promise.all([import('./theme.js'), import('./preview.js')]);
        const [theme, preview] = await previewModules;
        if (token !== previewRenderToken || !store || store.getState?.().revision !== state.revision) return;
        const renderer = previewPlatform === 'mobile' ? preview.renderMobilePreview : preview.renderTVPreview;
        renderer(host, state, { results: previewResults, onSelect: (id) => selectRow(id, 'preview'), onRetry: (id) => previewController?.retry(id) });
        host.querySelector('.home-preview-device') && theme.applyThemeVariables(host.querySelector('.home-preview-device'), state.theme);
        host.querySelectorAll('[data-preview-row-id]').forEach((row) => row.setAttribute('aria-current', String(state.selectionId === row.dataset.previewRowId)));
        previewObserver?.disconnect();
        const scheduleVisible = () => {
            const viewport = host.querySelector('.home-preview-content');
            const viewportBounds = viewport?.getBoundingClientRect?.();
            const visible = [...host.querySelectorAll('[data-preview-row-id]')].filter((element) => {
                const bounds = element.getBoundingClientRect?.();
                return !bounds || !viewportBounds || bounds.bottom >= viewportBounds.top && bounds.top <= viewportBounds.bottom;
            }).map((element) => element.dataset.previewRowId);
            previewController?.schedule({ scope: state.scope, profileId: previewProfileId, platform: previewPlatform, rows: state.rows, visibleRowIds: visible, theme: state.theme });
        };
        if (typeof IntersectionObserver === 'function') {
            previewObserver = new IntersectionObserver((entries) => {
                if (entries.some((entry) => entry.isIntersecting)) scheduleVisible();
            }, { root: host.querySelector('.home-preview-content') });
            host.querySelectorAll('[data-preview-row-id]').forEach((row) => previewObserver.observe(row));
        }
        if (schedule && previewController) requestAnimationFrame(scheduleVisible);
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
        const themePath = active?.closest?.('[data-home-designer-editor]') ? active.dataset?.themePath : null;
        const focusStart = typeof active?.selectionStart === 'number' ? active.selectionStart : null;
        const focusEnd = typeof active?.selectionEnd === 'number' ? active.selectionEnd : null;
        const state = store.getState();
        const validation = serverValidationByField(state);
        const serverRowValidation = validation.rows;
        const inspectorState = Object.keys(serverRowValidation).length
            ? { ...state, rowValidation: Object.fromEntries(Object.entries({ ...state.rowValidation, ...serverRowValidation }).map(([id, rowErrors]) => [id, [...(state.rowValidation?.[id] || []), ...(serverRowValidation[id] || [])]])) }
            : state;
        renderRowsMode(state, validation.sectionValidation.rows || []);
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
            state: inspectorState, dispatch: store.dispatch, onSelect: selectRow,
            sectionValidation: validation.sectionValidation,
            onFieldEdit: (identity) => clearServerValidation(identity),
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
        if (!previewModules) previewModules = Promise.all([import('./theme.js'), import('./preview.js')]);
        const [theme, preview] = await previewModules;
        theme.renderTheme(editor.querySelector('[data-home-designer-theme]'), {
            state, dispatch: store.dispatch, errors: validation.theme,
            onFieldEdit: (path) => clearServerValidation({ section: 'theme', path }),
            onReset: () => {
                if (!confirmReset('Reset Theme to the inherited appearance? This discards the Theme override.')) return;
                store.dispatch({ type: 'theme/reset' });
                editor.querySelector('[data-home-designer-live]').textContent = 'Theme reset to the inherited appearance.';
            },
        });
        if (!previewController) {
            previewController = preview.createPreviewController({
                fetchPreview: (request, options) => modules.then(([api]) => api.loadPreview(basePath, request, options)),
                onChange: (results) => { previewResults = results; void renderPreview(store?.getState?.() || state, { schedule: false }); },
            });
        }
        previewProfileId ||= state.previewProfiles?.[0]?.id || state.scope?.profileId || '';
        const profileSelect = editor.querySelector('[data-home-designer-preview-profile]');
        if (profileSelect && !profileSelect.dataset.ready) {
            profileSelect.dataset.ready = 'true';
            profileSelect.addEventListener('change', () => {
                previewProfileId = profileSelect.value;
                invalidatePreview();
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
            platformSelect.addEventListener('change', () => { previewPlatform = platformSelect.value === 'mobile' ? 'mobile' : 'tv'; invalidatePreview(); void renderPreview(store.getState()); });
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
        if (focusPath || themePath) requestAnimationFrame(() => {
            const replacement = focusPath
                ? editor.querySelector(`[data-field-path="${CSS.escape(focusPath)}"]`)
                : editor.querySelector(`[data-theme-path="${CSS.escape(themePath)}"]`);
            replacement?.focus();
            if (focusStart !== null && typeof replacement?.setSelectionRange === 'function') replacement.setSelectionRange(focusStart, focusEnd);
        });
    };

    const setBackgroundInert = (inert) => {
        if (inert && drawer) {
            const workspace = drawer.parentElement;
            const nodes = [
                ...Array.from(editor?.children || []).filter((node) => node !== workspace),
                ...Array.from(workspace?.children || []).filter((node) => node !== drawer),
            ];
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

    const guardDrawerFocus = (event) => {
        if (drawer && event.target && !drawer.contains(event.target)) drawer.querySelector('.home-designer-drawer-close, input, select, button, [href]')?.focus?.();
    };

    const closeDrawer = () => {
        if (!drawer) return;
        drawer.classList.remove('is-drawer-open');
        drawer.removeAttribute('role');
        drawer.removeAttribute('aria-modal');
        setBackgroundInert(false);
        document.body?.classList.remove('home-designer-drawer-open');
        document.removeEventListener?.('focusin', guardDrawerFocus, true);
        const returnFocus = drawerReturnFocus;
        drawer = null;
        drawerReturnFocus = null;
        requestAnimationFrame(() => returnFocus?.focus?.());
    };

    const openDrawer = (kind, trigger) => {
        if (drawerMedia && !drawerMedia.matches) return;
        const target = editor?.querySelector(`[data-home-designer-${kind}]`);
        if (!target) return;
        if (drawer && drawer !== target) closeDrawer();
        drawer = target;
        drawerReturnFocus = trigger || document.activeElement;
        target.classList.add('is-drawer-open');
        target.setAttribute('role', 'dialog');
        target.setAttribute('aria-modal', 'true');
        setBackgroundInert(true);
        document.body?.classList.add('home-designer-drawer-open');
        document.addEventListener?.('focusin', guardDrawerFocus, true);
        requestAnimationFrame(() => target.querySelector('.home-designer-drawer-close, input, select, button, [href]')?.focus?.());
    };

    drawerBackdrop?.addEventListener('click', closeDrawer);
    const onDrawerMediaChange = () => { if (drawer && drawerMedia && !drawerMedia.matches) closeDrawer(); };
    if (drawerMedia?.addEventListener) drawerMedia.addEventListener('change', onDrawerMediaChange);
    drawerMedia?.addListener?.(onDrawerMediaChange);

    const mountDrawerControls = () => {
        [['library', 'Row library'], ['inspector', 'Row inspector']].forEach(([kind, label]) => {
            const target = editor?.querySelector(`[data-home-designer-${kind}]`);
            if (!target || target.querySelector('.home-designer-drawer-close')) return;
            const close = document.createElement('button');
            close.type = 'button'; close.className = 'btn btn-secondary home-designer-drawer-close'; close.textContent = `Close ${label}`;
            close.addEventListener('click', closeDrawer);
            target.append(close);
        });
    };

    root.addEventListener?.('keydown', (event) => {
        if (drawer) {
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
        if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'z') {
            event.preventDefault();
            if (event.shiftKey) store?.redo?.(); else store?.undo?.();
        }
        if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'y') {
            event.preventDefault(); store?.redo?.();
        }
    });
    root.addEventListener?.('click', (event) => {
        const drawerButton = event.target?.closest?.('[data-home-designer-open-library], [data-home-designer-open-inspector]');
        if (drawerButton) {
            openDrawer(drawerButton.hasAttribute('data-home-designer-open-library') ? 'library' : 'inspector', drawerButton);
            return;
        }
        const link = event.target?.closest?.('a[href]');
        if (link && !event.defaultPrevented && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey && !confirmDiscard('Discard unsaved Home Designer changes and leave this page?')) event.preventDefault();
    });
    root.addEventListener?.('input', (event) => {
        const target = event.target;
        const key = draftKey(target);
        if (!key || !isEditableTarget(target) || ['checkbox', 'radio', 'color'].includes(target.type)) return;
        target.dataset.homeDesignerDraft = 'true';
        pendingDrafts.set(key, target.value);
        syncDirtyProtection();
    });
    root.addEventListener?.('change', (event) => {
        const target = event.target;
        if (!draftKey(target)) return;
        delete target.dataset.homeDesignerDraft;
        clearDraft(target);
    });

    const connectEditor = () => {
        unsubscribe?.();
        if (!store || !editor) return;
        previewRowsSignature = previewSignature(store.getState());
        unsubscribe = store.subscribe((state) => {
            syncDirtyProtection();
            const nextSignature = previewSignature(state);
            if (nextSignature !== previewRowsSignature) {
                previewRowsSignature = nextSignature;
                invalidatePreview();
            }
            renderEditor();
        });
        syncDirtyProtection();
        renderEditor();
    };

    const setWorkspaceLoading = (loading) => {
        if (editor) editor.hidden = Boolean(loading);
        root.setAttribute?.('aria-busy', String(Boolean(loading)));
    };

    const load = async (scope) => {
        invalidatePreview();
        previewRowsSignature = '';
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

    const bootstrap = async () => {
        if (!activeScope.kind || (activeScope.kind === 'profile' && !activeScope.profileId)) return;
        setWorkspaceLoading(true);
        const result = await load(activeScope);
        if (!result || !isCurrent(result.current)) return;
        if (result.error) showFailure();
        else {
            const [, { createStore }] = await modules;
            store = createStore(result.saved);
            connectEditor();
            setWorkspaceLoading(false);
            showReady();
        }
    };

    const switchScope = async (scope) => {
        if (!confirmDiscard('Discard unsaved Home Designer changes?')) return false;
        const previousScope = { ...activeScope };
        if (hasUnsavedWork()) {
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
        const [, { createStore }] = await modules;
        store = createStore(result.saved);
        connectEditor();
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
            applyingStore.replaceWithSaved(saved);
            applyValidation = [];
            clearErrors();
            clearDrafts();
            syncDirtyProtection();
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
                            const [, { createStore }] = await modules;
                            store = createStore(result.saved);
                            connectEditor();
                            setWorkspaceLoading(false);
                            clearErrors();
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
        discard: () => {
            if (!store) return false;
            store.discard();
            applyValidation = [];
            clearErrors();
            clearDrafts();
            syncDirtyProtection();
            showReady();
            return true;
        },
        switchScope,
    };

    root.querySelector('[data-home-designer-apply]')?.addEventListener('click', apply);
    root.querySelector('[data-home-designer-discard]')?.addEventListener('click', () => root.homeDesigner.discard());
    root.querySelector('[data-home-designer-undo]')?.addEventListener('click', () => store?.undo?.());
    root.querySelector('[data-home-designer-redo]')?.addEventListener('click', () => store?.redo?.());

    if (initialScope.kind === 'global' || initialScope.profileId) bootstrap();
}
