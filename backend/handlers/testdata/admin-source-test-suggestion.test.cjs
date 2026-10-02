const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const template = fs.readFileSync(path.join(__dirname, '../admin_templates/settings.html'), 'utf8');
const source = template.slice(template.indexOf('    async function testProvider('), template.indexOf('    async function attemptTestArtifactDelete('));
const close = template.slice(template.indexOf('    function closeTestModal('), template.indexOf('    // Close modal on Escape key'));
function setup(result = {}) {
  const elements = new Map();
  const getElementById = id => {
    if (!elements.has(id)) elements.set(id, { style: {}, textContent: '', classList: { add() {}, remove() {} } });
    return elements.get(id);
  };
  const item = { name: 'Zeus', type: 'stremio-direct', url: 'https://addon.example/manifest.json', options: '', skipNameFiltering: false };
  const context = vm.createContext({
    document: { getElementById, body: { style: {} } },
    basePath: '/admin', schema: { torrentScrapers: {} },
    currentSettings: { torrentScrapers: [item] }, selectedUserId: '', selectedClientId: '',
    getValue: () => context.currentSettings.torrentScrapers,
    setValue: (key, value) => { context.changedPaths.push(key); context.currentSettings.torrentScrapers[Number(key.split('.')[1])].skipNameFiltering = value; },
    renderSettings: () => { context.renders++; }, renders: 0, changedPaths: [],
    escapeHtml: value => value,
    fetch: async (_url, request) => { context.payload = JSON.parse(request.body); return { json: async () => result }; },
  });
  vm.runInContext(source + close, context);
  return { context, item, getElementById };
}
const numeric = { success: true, message: 'Source works', nameMatchingSuggestion: {
  setting: 'skipNameFiltering', message: '2 of 2 streams have numeric filenames.', alreadyEnabled: false,
} };

test('numeric source test suggests an explicit change and preserves the normal save flow', async () => {
  const { context, item, getElementById } = setup(numeric);
  await context.testProvider('torrentScrapers', 0);
  assert.equal(context.payload.skipNameFiltering, false);
  assert.equal(context.payload.options, '');
  assert.equal(item.skipNameFiltering, false);
  assert.equal(getElementById('testModalSuggestionButton').style.display, 'inline-flex');
  assert.match(getElementById('testModalSuggestion').textContent, /numeric filenames/);
  context.applyTestSuggestion();
  assert.equal(item.skipNameFiltering, true);
  assert.deepEqual(context.changedPaths, ['torrentScrapers.0.skipNameFiltering']);
  assert.equal(context.renders, 1);
  assert.match(getElementById('testModalSuggestion').textContent, /Save changes/);
  assert.equal(getElementById('testModalSuggestionButton').style.display, 'none');
});

test('a reordered source keeps the suggestion attached to the tested object', async () => {
  const { context, item } = setup(numeric);
  await context.testProvider('torrentScrapers', 0);
  const other = { name: 'Other', skipNameFiltering: false };
  context.currentSettings.torrentScrapers.unshift(other);
  context.applyTestSuggestion();
  assert.equal(item.skipNameFiltering, true);
  assert.equal(other.skipNameFiltering, false);
  assert.deepEqual(context.changedPaths, ['torrentScrapers.1.skipNameFiltering']);
});

test('changed source, removed source, or changed context cannot apply a stale suggestion', async () => {
  for (const change of [
    c => { c.currentSettings.torrentScrapers[0].url = 'https://other.example'; },
    c => { c.currentSettings.torrentScrapers[0].apiKey = 'changed'; },
    c => { c.currentSettings.torrentScrapers[0].config = { filter: '2' }; },
    c => { c.currentSettings.torrentScrapers = []; },
    c => { c.selectedUserId = 'another-user'; },
    c => { c.selectedClientId = 'another-client'; },
  ]) {
    const { context, item, getElementById } = setup(numeric);
    await context.testProvider('torrentScrapers', 0);
    change(context);
    context.applyTestSuggestion();
    assert.equal(item.skipNameFiltering, false);
    assert.equal(context.changedPaths.length, 0);
    assert.match(getElementById('testModalSuggestion').textContent, /Test the source again/);
  }
});

test('already enabled displays diagnostics without an enable button', async () => {
  const { context, getElementById } = setup({ ...numeric, nameMatchingSuggestion: { ...numeric.nameMatchingSuggestion, alreadyEnabled: true } });
  await context.testProvider('torrentScrapers', 0);
  assert.equal(getElementById('testModalSuggestion').style.display, 'block');
  assert.equal(getElementById('testModalSuggestionButton').style.display, 'none');
});

test('other test results and closing the modal clear the previous suggestion', async () => {
  const { context, item, getElementById } = setup(numeric);
  await context.testProvider('torrentScrapers', 0);
  context.showTestResultModal('Other', true, 'Working', null);
  assert.equal(getElementById('testModalSuggestion').style.display, 'none');
  context.applyTestSuggestion();
  assert.equal(item.skipNameFiltering, false);
  await context.testProvider('torrentScrapers', 0);
  context.closeTestModal();
  context.applyTestSuggestion();
  assert.equal(item.skipNameFiltering, false);
});

test('failed tests never suggest enabling a setting', async () => {
  const { context, getElementById } = setup({ ...numeric, success: false, error: 'Failed' });
  await context.testProvider('torrentScrapers', 0);
  assert.equal(getElementById('testModalSuggestionButton').style.display, 'none');
});
