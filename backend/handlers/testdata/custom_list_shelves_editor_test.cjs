const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../static/admin-custom-list-shelves-v1.js'), 'utf8');
const template = fs.readFileSync(path.join(__dirname, '../admin_templates/settings.html'), 'utf8');
function templateFunction(name) {
  const start = template.indexOf(`    function ${name}(`);
  const end = template.indexOf('\n    }', start) + '\n    }'.length;
  assert.ok(start >= 0 && end > start, name);
  return template.slice(start, end);
}

class Option {
  constructor(text, value) { this.textContent = text; this.value = value; this.disabled = false; }
}
class Select {
  constructor() { this.dataset = {}; this.options = []; this.disabled = false; this.style = {}; }
  replaceChildren(...options) { this.options = options; this.value = options[0]?.value || ''; }
  add(option) { this.options.push(option); if (this.options.length === 1) this.value = option.value; }
  get selectedOptions() { return this.options.filter(option => option.value === this.value); }
}
function fixture() {
  const elements = {
    newShelfType: { value: 'custom-list' },
    newShelfCustomList: new Select(),
    editShelfCustomList: new Select(),
    newShelfName: { value: '' },
    newShelfLimit: { value: '8' },
    newShelfHideUnreleased: { checked: true },
  };
  const requests = [], alerts = [], errors = [];
  const context = vm.createContext({
    selectedUserId: 'profile', userSettings: {},
    currentSettings: { homeShelves: { shelves: [{ id: 'watchlist', name: 'Saved', enabled: true, order: 0 }] } },
    basePath: '/prefix/account', perUserSections: ['homeShelves.shelves'], editingHomeView: 'all',
    Option, document: { getElementById: id => elements[id] },
    crypto: { getRandomValues: bytes => { bytes.fill(12); return bytes; } },
    renderSettings() {}, closeEditShelfModal() {}, ensureBuiltInHomeShelfConfig: shelves => shelves,
    getShelfTypeLabel: type => type === 'mdblist' ? 'MDBList' : type,
    alert: text => alerts.push(text), showToast: text => errors.push(text),
    fetch: url => new Promise(resolve => requests.push({ url, resolve })),
  });
  vm.runInContext(source + '\n' + templateFunction('ensureProfileShelves') + '\n' + templateFunction('getEditableHomeShelves') + '\n' + templateFunction('addCustomShelf') + '\n' + templateFunction('saveEditedShelf'), context);
  const run = code => vm.runInContext(code, context);
  return { context, elements, requests, alerts, errors, run };
}
function respond(request, lists, ok = true) { request.resolve({ ok, json: async () => lists }); }

test('loads the selected profile through the account route and safely uses list names as text', async () => {
  const { run, requests, elements } = fixture();
  const pending = run("loadProfileCustomListOptions('newShelfCustomList')");
  assert.equal(elements.newShelfCustomList.disabled, true);
  assert.equal(requests[0].url, '/prefix/account/api/profiles/profile/custom-lists');
  respond(requests[0], [{ id: 'list-1', name: '<img onerror="bad()"> Favorites' }]);
  await pending;
  assert.equal(elements.newShelfCustomList.disabled, false);
  assert.equal(elements.newShelfName.value, '<img onerror="bad()"> Favorites');
  run('addCustomShelf()');
  assert.equal(run('currentSettings.homeShelves.shelves.length'), 1);
  assert.equal(run('userSettings.homeShelves.shelves.length'), 2);
  const shelf = run('userSettings.homeShelves.shelves[1]');
  assert.equal(shelf.listUrl, 'mediastorm:custom-list:list-1');
  assert.equal(shelf.type, 'mdblist');
  assert.equal(shelf.limit, 8);
  assert.equal(shelf.hideUnreleased, true);
  assert.equal(shelf.order, 1);
  assert.equal(run('homeShelfTypeLabel(userSettings.homeShelves.shelves[1])'), 'Custom List');
});

test('editing preserves the configured list and saves a replacement list', async () => {
  const { run, requests, elements, context } = fixture();
  context.shelf = { type: 'mdblist', listUrl: 'mediastorm:custom-list:list-2' };
  const pending = run("loadProfileCustomListOptions('editShelfCustomList', customListShelfId(shelf))");
  respond(requests[0], [{ id: 'list-1', name: 'First' }, { id: 'list-2', name: 'Second' }]);
  await pending;
  assert.equal(elements.editShelfCustomList.value, 'list-2');
  elements.editShelfCustomList.value = 'list-1';
  assert.equal(run('saveProfileCustomListSource(shelf)'), true);
  assert.equal(context.shelf.listUrl, 'mediastorm:custom-list:list-1');
});

test('a profile change or replaced field discards stale responses', async () => {
  for (const replace of [false, true]) {
    const { run, context, requests, elements } = fixture();
    const pending = run("loadProfileCustomListOptions('newShelfCustomList')");
    if (replace) elements.newShelfCustomList = new Select();
    else context.selectedUserId = 'another-profile';
    respond(requests[0], [{ id: 'private-list', name: 'Private' }]);
    await pending;
    assert.equal(elements.newShelfCustomList.options.some(option => option.value === 'private-list'), false);
    assert.equal(elements.newShelfName.value, '');
  }
});

test('empty and failed loads cannot create a shelf', async () => {
  for (const ok of [true, false]) {
    const { run, requests, elements, alerts, errors } = fixture();
    const pending = run("loadProfileCustomListOptions('newShelfCustomList')");
    respond(requests[0], [], ok);
    await pending;
    assert.equal(elements.newShelfCustomList.disabled, true);
    run('addCustomShelf()');
    assert.equal(alerts.length, 1);
    assert.equal(errors.length, ok ? 0 : 1);
    assert.equal(run('userSettings.homeShelves'), undefined);
  }
});

test('a deleted selected list stays unavailable rather than silently selecting another', async () => {
  const { run, requests, elements, context } = fixture();
  context.shelf = { listUrl: 'mediastorm:custom-list:deleted' };
  const pending = run("loadProfileCustomListOptions('editShelfCustomList', 'deleted')");
  respond(requests[0], [{ id: 'other-list', name: 'Another list' }]);
  await pending;
  assert.equal(elements.editShelfCustomList.value, 'deleted');
  assert.equal(run('saveProfileCustomListSource(shelf)'), false);
  assert.equal(context.shelf.listUrl, 'mediastorm:custom-list:deleted');
});

test('global scope cannot fetch or add a personal list', async () => {
  const { run, context, requests, alerts } = fixture();
  context.selectedUserId = '';
  await run("loadProfileCustomListOptions('newShelfCustomList')");
  run('addCustomShelf()');
  assert.equal(requests.length, 0);
  assert.equal(alerts.length, 1);
  assert.equal(run('currentSettings.homeShelves.shelves.length'), 1);
});

test('switching shelf types while loading does not overwrite another source name', async () => {
  const { run, requests, elements } = fixture();
  const pending = run("loadProfileCustomListOptions('newShelfCustomList')");
  elements.newShelfType.value = 'mdblist';
  elements.newShelfName.value = 'Weekly movies';
  respond(requests[0], [{ id: 'list-1', name: 'Favorites' }]);
  await pending;
  assert.equal(elements.newShelfName.value, 'Weekly movies');
});


test('the shelf edit form saves title, source and options without URL validation', async () => {
  const { run, requests, elements, context } = fixture();
  context.userSettings = { homeShelves: { shelves: [{ id: 'shelf-1', name: 'Old', type: 'mdblist', listUrl: 'mediastorm:custom-list:list-1', order: 0 }] } };
  const pending = run("loadProfileCustomListOptions('editShelfCustomList', 'list-1')");
  respond(requests[0], [{ id: 'list-1', name: 'First' }, { id: 'list-2', name: 'Second' }]);
  await pending;
  elements.editShelfCustomList.value = 'list-2';
  elements.editShelfName = { value: 'Renamed' };
  elements.editShelfLimit = { value: '12' };
  elements.editShelfHideUnreleased = { checked: true };
  run("saveEditedShelf('shelf-1')");
  assert.equal(run('userSettings.homeShelves.shelves[0].name'), 'Renamed');
  assert.equal(run('userSettings.homeShelves.shelves[0].listUrl'), 'mediastorm:custom-list:list-2');
  assert.equal(run('userSettings.homeShelves.shelves[0].limit'), 12);
  assert.equal(run('userSettings.homeShelves.shelves[0].hideUnreleased'), true);
});

test('adding a custom list to an alternate page only changes that page', async () => {
  const { run, requests } = fixture();
  run(fs.readFileSync(path.join(__dirname, '../static/admin-home-views-v1.js'), 'utf8'));
  run("setEditingHomeView('movies');setHomeViewMode('custom')");
  const pending = run("loadProfileCustomListOptions('newShelfCustomList')");
  respond(requests[0], [{ id: 'list-1', name: 'Favorites' }]);
  await pending;
  run('addCustomShelf()');
  assert.equal(run('userSettings.homeShelves.views.movies.shelves.length'), 2);
  assert.equal(run('currentSettings.homeShelves.shelves.length'), 1);
  assert.equal(run('userSettings.homeShelves.shelves'), undefined);
});
