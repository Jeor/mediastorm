const defaults = {
    accentColor: '#3f66ff', textColor: '#f8fafc', secondaryTextColor: '#a7b0c0',
    backgroundColor: '#101522', modalBackgroundColor: '#1c2230', fontScale: 1,
    buttonStyle: 'soft', buttonRadius: 'rounded', highContrast: false, reduceOverlays: false,
};

const themeValue = (theme, field) => theme?.[field] ?? defaults[field];
const color = (value, fallback) => /^#[0-9a-f]{6}$/i.test(String(value)) ? String(value) : fallback;

export const applyThemeVariables = (device, theme = {}) => {
    if (!device?.style?.setProperty) return;
    const scale = Math.min(1.4, Math.max(0.8, Number(themeValue(theme, 'fontScale')) || 1));
    const radius = { square: '0', rounded: '0.55rem', pill: '999px' }[themeValue(theme, 'buttonRadius')] || '0.55rem';
    device.style.setProperty('--preview-accent', color(themeValue(theme, 'accentColor'), defaults.accentColor));
    device.style.setProperty('--preview-text', color(themeValue(theme, 'textColor'), defaults.textColor));
    device.style.setProperty('--preview-secondary-text', color(themeValue(theme, 'secondaryTextColor'), defaults.secondaryTextColor));
    device.style.setProperty('--preview-background', color(themeValue(theme, 'backgroundColor'), defaults.backgroundColor));
    device.style.setProperty('--preview-modal-background', color(themeValue(theme, 'modalBackgroundColor'), defaults.modalBackgroundColor));
    device.style.setProperty('--preview-font-scale', String(scale));
    device.style.setProperty('--preview-button-radius', radius);
    const buttonStyle = ['soft', 'outlined', 'filled'].includes(String(themeValue(theme, 'buttonStyle'))) ? String(themeValue(theme, 'buttonStyle')) : 'soft';
    device.style.setProperty('--preview-button-style', buttonStyle);
    if (device.dataset) device.dataset.previewButtonStyle = buttonStyle;
    device.style.setProperty('--preview-contrast', themeValue(theme, 'highContrast') ? '1.35' : '1');
    device.style.setProperty('--preview-overlay-opacity', themeValue(theme, 'reduceOverlays') ? '0' : '.52');
};

const label = (text) => { const item = document.createElement('label'); item.textContent = text; return item; };

const field = (name, path, value, onChange, type = 'text', options = []) => {
    const item = label(name);
    const input = document.createElement(type === 'select' ? 'select' : 'input');
    if (type === 'select') options.forEach(([optionValue, optionLabel]) => { const option = document.createElement('option'); option.value = optionValue; option.textContent = optionLabel; input.append(option); });
    else input.type = type;
    input.dataset.themePath = path;
    input.value = String(value ?? '');
    input.addEventListener('input', () => onChange(type === 'number' ? Number(input.value) : input.value));
    input.addEventListener('change', () => onChange(type === 'checkbox' ? input.checked : (type === 'number' ? Number(input.value) : input.value)));
    item._themeInput = input;
    item.append(input);
    return item;
};

const checkbox = (name, path, value, onChange) => {
    const item = label(name); const input = document.createElement('input'); input.type = 'checkbox'; input.dataset.themePath = path; input.checked = Boolean(value); input.addEventListener('change', () => onChange(input.checked)); item._themeInput = input; item.append(input); return item;
};

const renderPresets = (host, presets, dispatch) => {
    host.replaceChildren();
    (Array.isArray(presets) ? presets : []).forEach((preset) => {
        const button = document.createElement('button');
        button.type = 'button'; button.className = 'btn btn-secondary'; button.textContent = String(preset?.name || preset?.id || 'Preset');
        button.addEventListener('click', () => dispatch?.({ type: 'theme/replace', value: structuredClone(preset?.appearance || {}) }));
        host.append(button);
    });
};

const reconcileTheme = (controls, state, dispatch) => {
    controls.mode.textContent = state?.themeMode === 'inherit' ? 'Theme inherits the global appearance.' : 'Theme uses a custom appearance.';
    controls.customize.disabled = state?.themeMode === 'custom';
    if (controls.reset) controls.reset.disabled = state?.themeMode === 'inherit';
    renderPresets(controls.presets, state?.themePresets, dispatch);
    controls.inputs.forEach((input, path) => {
        if (input === document.activeElement) return;
        const value = themeValue(state?.theme, path);
        if (input.type === 'checkbox') input.checked = Boolean(value);
        else input.value = String(value ?? '');
    });
};

export const renderTheme = (host, { state, dispatch, onReset } = {}) => {
    if (!host) return;
    const documentKey = `${state?.scope?.kind || ''}:${state?.scope?.profileId || ''}:${state?.revision || ''}`;
    if (host._homeDesignerTheme?.documentKey === documentKey) {
        reconcileTheme(host._homeDesignerTheme, state, dispatch);
        return;
    }
    host.replaceChildren();
    const section = document.createElement('section'); section.className = 'home-designer-theme-controls';
    const heading = document.createElement('h2'); heading.textContent = 'Theme'; section.append(heading);
    const mode = document.createElement('p'); mode.dataset.themeMode = ''; section.append(mode);
    const actions = document.createElement('div'); actions.className = 'home-designer-theme-actions';
    const customize = document.createElement('button'); customize.type = 'button'; customize.className = 'btn btn-secondary'; customize.dataset.themeCustomize = ''; customize.textContent = 'Customize theme'; customize.addEventListener('click', () => dispatch?.({ type: 'theme/customize' })); actions.append(customize);
    let reset = null;
    if (state?.scope?.kind !== 'global') { reset = document.createElement('button'); reset.type = 'button'; reset.className = 'btn btn-secondary'; reset.dataset.themeReset = ''; reset.textContent = 'Reset to inherited'; reset.addEventListener('click', () => { if (onReset) onReset(); else dispatch?.({ type: 'theme/reset' }); }); actions.append(reset); }
    section.append(actions);
    const presetList = document.createElement('div'); presetList.className = 'home-designer-theme-presets'; section.append(presetList);
    const fields = document.createElement('div'); fields.className = 'home-designer-theme-fields';
    const update = (path) => (value) => dispatch?.({ type: 'theme/field', path, value });
    const inputs = new Map();
    const addField = (path, control) => { inputs.set(path, control._themeInput); fields.append(control); };
    [['Accent color', 'accentColor'], ['Text color', 'textColor'], ['Secondary text', 'secondaryTextColor'], ['Background', 'backgroundColor'], ['Modal background', 'modalBackgroundColor']]
        .forEach(([name, path]) => addField(path, field(name, path, themeValue(state?.theme, path), update(path), 'color')));
    addField('fontScale', field('Font scale', 'fontScale', themeValue(state?.theme, 'fontScale'), update('fontScale'), 'number'));
    addField('buttonStyle', field('Button style', 'buttonStyle', themeValue(state?.theme, 'buttonStyle'), update('buttonStyle'), 'select', [['soft', 'Soft'], ['outlined', 'Outlined'], ['filled', 'Filled']]));
    addField('buttonRadius', field('Button radius', 'buttonRadius', themeValue(state?.theme, 'buttonRadius'), update('buttonRadius'), 'select', [['square', 'Square'], ['rounded', 'Rounded'], ['pill', 'Pill']]));
    addField('highContrast', checkbox('High contrast', 'highContrast', themeValue(state?.theme, 'highContrast'), update('highContrast')));
    addField('reduceOverlays', checkbox('Reduce overlays', 'reduceOverlays', themeValue(state?.theme, 'reduceOverlays'), update('reduceOverlays')));
    section.append(fields); host.append(section);
    host._homeDesignerTheme = { documentKey, mode, customize, reset, presets: presetList, inputs };
    reconcileTheme(host._homeDesignerTheme, state, dispatch);
};
