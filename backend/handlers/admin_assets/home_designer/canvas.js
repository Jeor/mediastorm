const button = (label, marker) => {
    const element = document.createElement('button');
    element.type = 'button';
    element.className = 'btn btn-secondary home-designer-canvas-control';
    element.textContent = label;
    element.dataset[marker] = '';
    return element;
};

const announce = (liveRegion, message) => {
    if (liveRegion) liveRegion.textContent = message;
};

export const isCanvasDrop = (types) => {
    const offered = new Set(Array.from(types || []));
    return offered.has('application/x-home-designer-catalog') || offered.has('application/x-home-designer-row');
};

export const insertionIndex = (rows, targetID, after) => {
    const index = (rows || []).findIndex((row) => row.id === targetID);
    return index < 0 ? (rows || []).length : index + (after ? 1 : 0);
};

export const moveDestination = (rows, sourceID, rawIndex) => {
    const source = (rows || []).findIndex((row) => row.id === sourceID);
    return Math.max(0, Math.min(rawIndex - (source >= 0 && source < rawIndex ? 1 : 0), (rows || []).length - 1));
};

export const edgeScrollDelta = (pointerY, rect, threshold = 48, maximum = 18) =>
    pointerY < rect.top + threshold ? -maximum : pointerY > rect.bottom - threshold ? maximum : 0;

const rowFromTarget = (target) => target?.closest?.('[data-preview-row-id]') ||
    (target?.dataset?.previewRowId ? target : null);

const focusSelectedRow = (host, getState) => {
    const selected = getState?.()?.selectionId;
    if (!selected) return;
    const escaped = globalThis.CSS?.escape ? CSS.escape(selected) : String(selected).replace(/"/g, '\\"');
    host.querySelector(`[data-preview-row-id="${escaped}"]`)?.focus();
};

export const mountCanvasInteractions = (host, {
    state = {}, getState, editing = false, dispatch = () => {}, liveRegion,
    onSelect, onCustomize, onCatalogDrop, onDragStart, onDragEnd,
} = {}) => {
    if (!host || !editing) return () => {};
    const viewport = host.querySelector('.home-preview-content') || host;
    const rows = state.rows || [];
    const indicator = document.createElement('div');
    indicator.className = 'home-designer-drop-indicator';
    indicator.setAttribute('aria-hidden', 'true');
    let pendingIndex = null;
    const clearInsertion = () => { pendingIndex = null; indicator.remove(); };
    const showInsertion = (index) => {
        pendingIndex = Math.max(0, Math.min(index, rows.length));
        indicator.dataset.dropIndex = String(pendingIndex);
        if (typeof viewport.insertBefore === 'function') {
            const children = [...viewport.children].filter((child) => child !== indicator);
            viewport.insertBefore(indicator, children[pendingIndex] || null);
        }
    };
    const customize = (id) => onCustomize?.(id);
    const controls = viewport.querySelectorAll('[data-preview-row-id]');
    controls.forEach((section) => {
        const id = section.dataset.previewRowId;
        const row = rows.find((candidate) => candidate.id === id);
        if (!row) return;
        const index = rows.indexOf(row);
        const actions = document.createElement('div');
        actions.className = 'home-designer-canvas-actions';
        const drag = button('Drag row', 'homeDesignerDragHandle');
        drag.draggable = true;
        drag.setAttribute('aria-label', `Drag ${row.name || 'row'}`);
        drag.addEventListener('dragstart', (event) => {
            event.dataTransfer?.setData('application/x-home-designer-row', row.id);
            if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
            onDragStart?.(row.id);
        });
        const visibility = button(row.enabled === false ? 'Show' : 'Hide', 'homeDesignerVisibility');
        visibility.addEventListener('click', (event) => {
            event.stopPropagation(); customize(row.id);
            dispatch({ type: 'rows/visibility', id: row.id, enabled: row.enabled === false });
            announce(liveRegion, `${row.name || 'Row'} is now ${row.enabled === false ? 'visible' : 'hidden'}.`);
        });
        const up = button('Move Up', 'homeDesignerMoveUp');
        up.disabled = index === 0;
        up.addEventListener('click', (event) => {
            event.stopPropagation(); customize(row.id); dispatch({ type: 'rows/move', id: row.id, to: index - 1 });
            announce(liveRegion, `${row.name || 'Row'} moved to position ${index} of ${rows.length}.`);
        });
        const down = button('Move Down', 'homeDesignerMoveDown');
        down.disabled = index === rows.length - 1;
        down.addEventListener('click', (event) => {
            event.stopPropagation(); customize(row.id); dispatch({ type: 'rows/move', id: row.id, to: index + 1 });
            announce(liveRegion, `${row.name || 'Row'} moved to position ${index + 2} of ${rows.length}.`);
        });
        const remove = button('Remove', 'homeDesignerRemove');
        remove.addEventListener('click', (event) => {
            event.stopPropagation(); customize(row.id); dispatch({ type: 'rows/remove', id: row.id });
            announce(liveRegion, `${row.name || 'Row'} removed.`);
            requestAnimationFrame(() => focusSelectedRow(host, getState));
        });
        section.addEventListener('click', (event) => {
            if (!event.target?.closest?.('button')) onSelect?.(row.id);
        });
        actions.append(drag, visibility, up, down, remove);
        section.append(actions);
    });
    const onDragOver = (event) => {
        if (!isCanvasDrop(event.dataTransfer?.types)) return;
        event.preventDefault();
        const target = rowFromTarget(event.target);
        const rect = target?.getBoundingClientRect?.();
        const after = rect ? event.clientY > rect.top + rect.height / 2 : false;
        showInsertion(insertionIndex(rows, target?.dataset?.previewRowId, after));
        const delta = edgeScrollDelta(event.clientY, viewport.getBoundingClientRect(), 48, 18);
        if (delta !== 0) viewport.scrollBy({ top: delta, behavior: 'auto' });
    };
    const onDrop = async (event) => {
        if (!isCanvasDrop(event.dataTransfer?.types)) return;
        event.preventDefault();
        const target = rowFromTarget(event.target);
        const rawIndex = pendingIndex ?? insertionIndex(rows, target?.dataset?.previewRowId, false);
        const catalogToken = event.dataTransfer?.getData('application/x-home-designer-catalog');
        const rowID = event.dataTransfer?.getData('application/x-home-designer-row');
        clearInsertion();
        if (catalogToken) await onCatalogDrop?.(catalogToken, rawIndex);
        else if (rowID) { customize(rowID); dispatch({ type: 'rows/move', id: rowID, to: moveDestination(rows, rowID, rawIndex) }); }
    };
    const finishDrag = () => { clearInsertion(); onDragEnd?.(); };
    const onKeyDown = (event) => { if (event.key === 'Escape') finishDrag(); };
    viewport.addEventListener('dragover', onDragOver);
    viewport.addEventListener('drop', onDrop);
    viewport.addEventListener('dragend', finishDrag);
    viewport.addEventListener('keydown', onKeyDown);
    return () => {
        viewport.removeEventListener('dragover', onDragOver);
        viewport.removeEventListener('drop', onDrop);
        viewport.removeEventListener('dragend', finishDrag);
        viewport.removeEventListener('keydown', onKeyDown);
        clearInsertion();
    };
};
