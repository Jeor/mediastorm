export const bandForWidth = (width) => Number(width) >= 1440 ? 'wide' : Number(width) >= 1100 ? 'standard' : 'compact';

export const createWorkspaceState = (width = 0) => ({
    mode: 'preview', band: bandForWidth(width), libraryOpen: false,
    contextTool: null, dragging: false,
});

export const reduceWorkspace = (state, action) => {
    if (action.type === 'resize') {
        const band = bandForWidth(action.width);
        const exclusive = band !== 'wide' && state.libraryOpen && state.contextTool;
        return { ...state, band, libraryOpen: exclusive ? false : state.libraryOpen };
    }
    if (action.type === 'edit/start') return { ...state, mode: 'edit' };
    if (action.type === 'edit/cancel' || action.type === 'edit/applied') {
        return { ...state, mode: 'preview', libraryOpen: false, contextTool: null, dragging: false };
    }
    if (state.mode !== 'edit') return state;
    if (action.type === 'tool/library') {
        const libraryOpen = action.open === true ? true : action.open === false ? false : !state.libraryOpen;
        return {
            ...state, libraryOpen,
            contextTool: libraryOpen && state.band !== 'wide' ? null : state.contextTool,
        };
    }
    if (action.type === 'tool/inspector' || action.type === 'tool/theme') {
        const wanted = action.type === 'tool/theme' ? 'theme' : 'inspector';
        const contextTool = action.open === true ? wanted : action.open === false ? null : state.contextTool === wanted ? null : wanted;
        return {
            ...state, libraryOpen: state.band === 'wide' ? state.libraryOpen : false,
            contextTool,
        };
    }
    if (action.type === 'drag/start') return {
        ...state, dragging: true, libraryOpen: state.band === 'compact' ? false : state.libraryOpen,
    };
    if (action.type === 'drag/end') return { ...state, dragging: false };
    return state;
};
