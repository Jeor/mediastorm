const root = document.getElementById('homeDesignerRoot');

if (root) {
    const basePath = root.dataset.basePath || '';
    const isAdmin = root.dataset.isAdmin === 'true';
    const profileID = root.dataset.profileId || '';

    const loadDocument = async () => {
        const params = new URLSearchParams({ scope: isAdmin ? 'global' : 'profile' });
        if (!isAdmin && profileID) params.set('profileId', profileID);

        const response = await fetch(`${basePath}/api/home-designer?${params.toString()}`, {
            credentials: 'same-origin',
            headers: { Accept: 'application/json' },
        });
        if (!response.ok) throw new Error(`Home Designer load failed (${response.status})`);
        return response.json();
    };

    const showFailure = () => {
        root.replaceChildren();
        const message = document.createElement('p');
        message.className = 'home-designer-error';
        message.textContent = 'Home Designer could not load. Try again.';
        const retry = document.createElement('button');
        retry.className = 'btn btn-primary';
        retry.type = 'button';
        retry.textContent = 'Retry';
        retry.addEventListener('click', bootstrap);
        root.append(message, retry);
    };

    const bootstrap = async () => {
        try {
            await loadDocument();
            const loading = root.querySelector('.home-designer-loading');
            if (loading) loading.textContent = 'Home Designer is ready.';
        } catch {
            showFailure();
        }
    };

    if (isAdmin || profileID) bootstrap();
}
