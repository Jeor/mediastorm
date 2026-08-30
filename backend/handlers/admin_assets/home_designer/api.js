export class APIError extends Error {
    constructor(status, code, message, fields = []) {
        super(message);
        this.name = 'APIError';
        this.status = status;
        this.code = code;
        this.fields = fields;
    }
}

const endpoint = (basePath, suffix) => `${String(basePath || '').replace(/\/+$/, '')}${suffix}`;

const jsonRequest = async (url, options, fetchImpl) => {
    const response = await fetchImpl(url, {
        credentials: 'same-origin',
        headers: { Accept: 'application/json', 'Content-Type': 'application/json', ...(options.headers ?? {}) },
        ...options,
    });
    let body = null;
    try {
        body = await response.json();
    } catch {
        // The server should return JSON, but an intermediary must not turn a recoverable HTTP error into a JSON parsing failure.
    }
    if (!response.ok) {
        throw new APIError(response.status, body?.code ?? 'request_failed', body?.message ?? `Home Designer request failed (${response.status})`, Array.isArray(body?.fields) ? body.fields : []);
    }
    return body;
};

export const loadDocument = (basePath, scope, { signal, fetchImpl = fetch } = {}) => {
    const query = new URLSearchParams({ scope: scope.kind });
    if (scope.profileId) query.set('profileId', scope.profileId);
    return jsonRequest(`${endpoint(basePath, '/api/home-designer')}?${query.toString()}`, { method: 'GET', signal }, fetchImpl);
};

export const loadPreview = (basePath, request, { signal, fetchImpl = fetch } = {}) => jsonRequest(
    endpoint(basePath, '/api/home-designer/preview'),
    { method: 'POST', body: JSON.stringify(request), signal },
    fetchImpl,
);

export const applyDocument = (basePath, request, { signal, fetchImpl = fetch } = {}) => jsonRequest(
    endpoint(basePath, '/api/home-designer'),
    { method: 'PUT', body: JSON.stringify(request), signal },
    fetchImpl,
);
