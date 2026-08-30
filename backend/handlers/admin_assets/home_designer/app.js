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

    const load = async (scope, replaceWorkingCopy = false) => {
        const token = ++operation;
        const [{ loadDocument }, { createStore }] = await modules;
        const saved = await loadDocument(basePath, scope);
        if (token !== operation) return null;
        if (store && !replaceWorkingCopy) store.replaceWithSaved(saved);
        else store = createStore(saved);
        return saved;
    };

    const bootstrap = async () => {
        if (!activeScope.kind || (activeScope.kind === 'profile' && !activeScope.profileId)) return;
        try {
            if (!await load(activeScope, true)) return;
            showReady();
        } catch {
            showFailure();
        }
    };

    const switchScope = async (scope) => {
        if (store?.isDirty() && typeof globalThis.confirm === 'function' && !globalThis.confirm('Discard unsaved Home Designer changes?')) return false;
        activeScope = scope;
        try {
            if (!await load(activeScope, true)) return false;
            showReady();
            return true;
        } catch {
            showFailure();
            return false;
        }
    };

    const apply = async () => {
        if (!store) return false;
        const request = store.buildApplyRequest();
        if (!request) return true;
        const [{ applyDocument, APIError }] = await modules;
        const applyingStore = store;
        const token = ++operation;
        try {
            const saved = await applyDocument(basePath, request);
            if (token !== operation || store !== applyingStore) return false;
            applyingStore.replaceWithSaved(saved);
            showReady();
            return true;
        } catch (error) {
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
                    try {
                        await load(request.scope, true);
                        showReady();
                    } catch {
                        showFailure();
                    }
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
