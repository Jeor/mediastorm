const { test } = require("node:test");
const assert = require("node:assert/strict");
const {
  getDescription,
  renderButton,
} = require("../static/admin-shelf-info-v1.js");

test("renamed and legacy built-ins retain their content explanation", () => {
  const upcoming = getDescription({
    id: "my-upcoming",
    name: "My renamed shelf",
  });
  assert.match(upcoming, /next unwatched episode has not aired/);
  assert.match(
    getDescription({ id: "calendar" }),
    /recent releases and upcoming releases/,
  );
  assert.match(
    getDescription({ id: "gemini-recs" }),
    /No AI provider is required/,
  );
});

test("recent release help reflects the rendered shelf’s source selection without editing it", () => {
  const shelf = {
    id: "my-recently-aired",
    calendarSources: { watchlist: false, history: true, topTrending: true },
  };
  const original = JSON.stringify(shelf);
  assert.match(
    getDescription(shelf),
    /Selected sources: Continue Watching, Top Trending\./,
  );
  assert.equal(JSON.stringify(shelf), original);
  assert.match(
    getDescription({ id: shelf.id }),
    /Selected sources: Watchlist\./,
  );
  assert.match(
    getDescription({ id: shelf.id, calendarSources: { watchlist: false } }),
    /Selected sources: none\./,
  );
});

test("custom help covers source and release filtering, with a fallback for unknown sources", () => {
  assert.match(
    getDescription({ id: "list-1", type: "tmdb", hideUnreleased: true }),
    /TMDB.*Hide Unreleased is enabled/,
  );
  assert.match(
    getDescription({ id: "legacy", listUrl: "https://mdblist.com/list" }),
    /MDBList/,
  );
  assert.match(getDescription({ id: "future-source" }), /configured source/);
  assert.match(
    getDescription({ id: "hub", type: "collection-hub" }),
    /Cards linking to the shelves/,
  );
});

test("info buttons safely escape user names in accessible labels and data attributes", () => {
  const button = renderButton({
    id: "watchlist",
    name: 'A "quote" <img onerror="bad()"> & \'name\'',
  });
  assert.match(button, /type="button"/);
  assert.match(button, /aria-haspopup="dialog"/);
  assert.match(button, /aria-label="About A &quot;quote&quot; &lt;img/);
  assert.ok(!button.includes("<img"));
  assert.ok(!button.includes('onerror="bad()"'));
});
