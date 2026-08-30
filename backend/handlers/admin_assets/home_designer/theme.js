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
    item.append(input);
    return item;
};

const checkbox = (name, path, value, onChange) => {
    const item = label(name); const input = document.createElement('input'); input.type = 'checkbox'; input.dataset.themePath = path; input.checked = Boolean(value); input.addEventListener('change', () => onChange(input.checked)); item.append(input); return item;
};

export const renderTheme = (host, { state, dispatch } = {}) => {
    if (!host) return;
    const scopeKey = `${state?.scope?.kind || ''}:${state?.scope?.profileId || ''}`;
    if (host.dataset.themeScope === scopeKey && document.activeElement?.dataset?.themePath) return;
    host.replaceChildren();
    host.dataset.themeScope = scopeKey;
    const section = document.createElement('section'); section.className = 'home-designer-theme-controls';
    const heading = document.createElement('h2'); heading.textContent = 'Theme'; section.append(heading);
    const mode = document.createElement('p'); mode.textContent = state?.themeMode === 'inherit' ? 'Theme inherits the global appearance.' : 'Theme uses a custom appearance.'; section.append(mode);
    const actions = document.createElement('div'); actions.className = 'home-designer-theme-actions';
    const customize = document.createElement('button'); customize.type = 'button'; customize.className = 'btn btn-secondary'; customize.textContent = 'Customize theme'; customize.disabled = state?.themeMode === 'custom'; customize.addEventListener('click', () => dispatch?.({ type: 'theme/customize' })); actions.append(customize);
    if (state?.scope?.kind !== 'global') { const reset = document.createElement('button'); reset.type = 'button'; reset.className = 'btn btn-secondary'; reset.textContent = 'Reset to inherited'; reset.disabled = state?.themeMode === 'inherit'; reset.addEventListener('click', () => dispatch?.({ type: 'theme/reset' })); actions.append(reset); }
    section.append(actions);
    const presets = Array.isArray(state?.themePresets) ? state.themePresets : [];
    if (presets.length) {
        const presetList = document.createElement('div'); presetList.className = 'home-designer-theme-presets';
        presets.forEach((preset) => { const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-secondary'; button.textContent = String(preset?.name || preset?.id || 'Preset'); button.addEventListener('click', () => dispatch?.({ type: 'theme/replace', value: structuredClone(preset?.appearance || {}) })); presetList.append(button); });
        section.append(presetList);
    }
    const fields = document.createElement('div'); fields.className = 'home-designer-theme-fields';
    const update = (path) => (value) => dispatch?.({ type: 'theme/field', path, value });
    [['Accent color', 'accentColor'], ['Text color', 'textColor'], ['Secondary text', 'secondaryTextColor'], ['Background', 'backgroundColor'], ['Modal background', 'modalBackgroundColor']]
        .forEach(([name, path]) => fields.append(field(name, path, themeValue(state?.theme, path), update(path), 'color')));
    fields.append(field('Font scale', 'fontScale', themeValue(state?.theme, 'fontScale'), update('fontScale'), 'number'));
    fields.append(field('Button style', 'buttonStyle', themeValue(state?.theme, 'buttonStyle'), update('buttonStyle'), 'select', [['soft', 'Soft'], ['outlined', 'Outlined'], ['filled', 'Filled']]));
    fields.append(field('Button radius', 'buttonRadius', themeValue(state?.theme, 'buttonRadius'), update('buttonRadius'), 'select', [['square', 'Square'], ['rounded', 'Rounded'], ['pill', 'Pill']]));
    fields.append(checkbox('High contrast', 'highContrast', themeValue(state?.theme, 'highContrast'), update('highContrast')));
    fields.append(checkbox('Reduce overlays', 'reduceOverlays', themeValue(state?.theme, 'reduceOverlays'), update('reduceOverlays')));
    section.append(fields); host.append(section);
};
