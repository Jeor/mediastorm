const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const path = require("node:path");
const source = fs.readFileSync(
  path.join(__dirname, "../static/admin-home-views-v1.js"),
  "utf8",
);
function fixture() {
  const context = vm.createContext({
    currentSettings: {
      homeShelves: {
        shelves: [{ id: "watchlist", name: "Saved", enabled: true, order: 0 }],
      },
    },
    userSettings: null,
    selectedUserId: null,
    renderSettings() {},
    escapeHtml: String,
  });
  vm.runInContext(source, context);
  return { context, run: (code) => vm.runInContext(code, context) };
}
test("customizing a view copies Home without changing it and reset restores live inheritance", () => {
  const { run } = fixture();
  run(
    "setEditingHomeView('movies'); setHomeViewMode('custom'); editableAlternateHomeView().shelves[0].name='Films'",
  );
  assert.equal(run("currentSettings.homeShelves.shelves[0].name"), "Saved");
  assert.equal(run("effectiveHomeView().shelves[0].name"), "Films");
  run("setEditingHomeView('shows')");
  assert.equal(run("effectiveHomeView().shelves[0].name"), "Saved");
  run(
    "setEditingHomeView('movies');setHomeViewMode('inherit');currentSettings.homeShelves.shelves[0].name='Updated'",
  );
  assert.equal(run("effectiveHomeView().shelves[0].name"), "Updated");
});
test("profile view definitions can inherit Home or global view independently", () => {
  const { run } = fixture();
  run(
    "setEditingHomeView('movies');setHomeViewMode('custom');editableAlternateHomeView().shelves[0].name='Global movies';selectedUserId='profile';userSettings={homeShelves:{shelves:[{id:'watchlist',name:'Profile',enabled:true,order:0}]}}",
  );
  assert.equal(run("effectiveHomeView().shelves[0].name"), "Global movies");
  run("setHomeViewMode('inherit')");
  assert.equal(run("effectiveHomeView().shelves[0].name"), "Profile");
  run("setHomeViewMode('global')");
  assert.equal(run("effectiveHomeView().shelves[0].name"), "Global movies");
});
test("hero selection is saved only in the selected view", () => {
  const { run } = fixture();
  run(
    "setEditingHomeView('movies');setHomeViewHero('tvTopShelfMode','watchlist')",
  );
  assert.equal(
    run("currentSettings.homeShelves.views.movies.tvTopShelfMode"),
    "shelf",
  );
  assert.equal(
    run("currentSettings.homeShelves.views.movies.tvTopShelfSourceId"),
    "watchlist",
  );
  assert.equal(run("currentSettings.homeShelves.tvTopShelfMode"), undefined);
});
test("custom profile views are not dimmed when regular Home inherits global shelves", () => {
  const { run } = fixture();
  run("selectedUserId='profile';userSettings={};setEditingHomeView('movies')");
  assert.equal(run("isAlternateHomeViewInherited()"), true);
  run("setHomeViewMode('custom')");
  assert.equal(run("isAlternateHomeViewInherited()"), false);
  assert.equal(run("userSettings.homeShelves.shelves"), undefined);
  run("setEditingHomeView('shows')");
  assert.equal(run("isAlternateHomeViewInherited()"), true);
  run("setEditingHomeView('movies');setHomeViewMode('inherit')");
  assert.equal(run("isAlternateHomeViewInherited()"), true);
});
test("alternate layout fields use standard full-width form controls and associated labels", () => {
  const { run } = fixture();
  run("setEditingHomeView('movies');setHomeViewMode('custom')");
  const html = run("renderHomeViewControls()");
  assert.equal((html.match(/class="form-input"/g) || []).length, 3);
  assert.equal((html.match(/class="form-select"/g) || []).length, 2);
  assert.equal((html.match(/class="form-group"/g) || []).length, 5);
  for (const id of [
    "itemCap",
    "homeShelfScale",
    "homeHeroScale",
    "tv-hero",
    "mobile-hero",
  ]) {
    assert.ok(html.includes('for="home-view-' + id + '"'));
    assert.ok(html.includes('id="home-view-' + id + '"'));
  }
});
