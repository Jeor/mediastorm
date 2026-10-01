// Personal list shelves reuse the remote-shelf transport supported by existing
// app builds. The backend resolves this URL against the requesting profile.
const CUSTOM_LIST_SHELF_PREFIX = 'mediastorm:custom-list:';

function customListShelfId(shelf) {
  return shelf?.listUrl?.startsWith(CUSTOM_LIST_SHELF_PREFIX)
    ? shelf.listUrl.slice(CUSTOM_LIST_SHELF_PREFIX.length)
    : '';
}

function homeShelfTypeLabel(shelf) {
  return customListShelfId(shelf) ? 'Custom List' : getShelfTypeLabel(shelf.type);
}

async function loadProfileCustomListOptions(selectId, selectedListId = '') {
  const select = document.getElementById(selectId);
  if (!select) return;
  const profileId = selectedUserId;
  select.disabled = true;
  select.dataset.profileId = '';
  select.replaceChildren(new Option(profileId ? 'Loading lists…' : 'Choose a person first', ''));
  if (!profileId) return;
  try {
    const response = await fetch(`${basePath}/api/profiles/${encodeURIComponent(profileId)}/custom-lists`);
    if (!response.ok) throw new Error('Could not load custom lists');
    const lists = (await response.json()) || [];
    // A profile change or rerender may replace this field during the request.
    if (selectedUserId !== profileId || document.getElementById(selectId) !== select) return;
    select.replaceChildren();
    for (const list of lists) select.add(new Option(list.name, list.id));
    if (selectedListId && !lists.some(list => list.id === selectedListId)) {
      const missing = new Option('Unavailable list — choose another list', selectedListId);
      missing.disabled = true;
      select.add(missing);
    }
    if (selectedListId) select.value = selectedListId;
    if (!lists.length && !selectedListId) select.add(new Option('No custom lists yet — create one in the app', ''));
    select.disabled = lists.length === 0;
    select.dataset.profileId = profileId;
    if (selectId === 'newShelfCustomList' && document.getElementById('newShelfType')?.value === 'custom-list') onProfileCustomListChange();
  } catch (error) {
    if (selectedUserId !== profileId || document.getElementById(selectId) !== select) return;
    select.replaceChildren(new Option('Could not load lists — select Custom List again to retry', ''));
    showToast(error.message, 'error');
  }
}

function onProfileCustomListChange() {
  const select = document.getElementById('newShelfCustomList');
  const name = document.getElementById('newShelfName');
  if (select?.value && name) name.value = select.selectedOptions[0].textContent;
}

function selectedProfileCustomList(selectId) {
  const select = document.getElementById(selectId);
  if (!selectedUserId || !userSettings || select?.dataset.profileId !== selectedUserId || select.disabled || !select.value || select.selectedOptions[0]?.disabled) {
    alert('Choose a custom list belonging to the selected person.');
    return '';
  }
  return select.value;
}

function addProfileCustomListShelf() {
  const listId = selectedProfileCustomList('newShelfCustomList');
  const name = document.getElementById('newShelfName')?.value?.trim();
  if (!listId) return;
  if (!name) { alert('Please enter a name'); return; }
  const shelves = getEditableHomeShelves();
  shelves.push({
    id: 'custom-list-' + Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join(''),
    name,
    enabled: true,
    order: Math.max(...shelves.map(shelf => shelf.order || 0), -1) + 1,
    type: 'mdblist',
    listUrl: CUSTOM_LIST_SHELF_PREFIX + listId,
    limit: parseInt(document.getElementById('newShelfLimit')?.value || '0', 10) || 0,
    hideUnreleased: !!document.getElementById('newShelfHideUnreleased')?.checked,
  });
  renderSettings();
}

function saveProfileCustomListSource(shelf) {
  const listId = selectedProfileCustomList('editShelfCustomList');
  if (!listId) return false;
  shelf.listUrl = CUSTOM_LIST_SHELF_PREFIX + listId;
  return true;
}
