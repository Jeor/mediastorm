const root = document.getElementById('homeDesignerRoot');
const status = root?.querySelector('[data-home-designer-status]');

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

    const load = async (scope) => {
        const current = beginOperation();
        try {
            const [{ loadDocument }, { createStore }] = await modules;
            const saved = await loadDocument(basePath, scope, { signal: current.controller.signal });
            if (!isCurrent(current)) return null;
            store = createStore(saved);
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

    if (initialScope.kind === 'global' || initialScope.profileId) bootstrap();
}
