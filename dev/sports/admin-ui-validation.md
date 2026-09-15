# Sports Hub settings

The browser sidebar now has **Settings → Sports Hub**:

- `/admin/sports`: server league availability and profile favorites.
- `/account/sports`: favorites for profiles owned by the signed-in account.
- A configured reverse-proxy base path is preserved.

Server availability is saved through the existing sports settings handler and starts a feed refresh. At least one league must stay enabled. Profile favorites use the existing revision-checked sports preference store; saves preserve athlete/school preferences and unknown existing IDs. Profile favorites do not change the server's enabled leagues.

The matching frontend change is in the `sportshub_frontend_integration_20260913` checkout. It includes favorite teams in **Following** using `{league}:{providerTeamId}` and reloads preferences on returning to Sports Hub. **All events** remains available. Deploy the backend and updated frontend together for favorite-team filtering; no database migration is required.

Profile controls expose every league in the backend catalog, with existing app ID aliases retained, plus the existing cycling competitions. Sports are derived from that catalog. Team lists load progressively (four requests at a time) through `/api/sports/catalog?teams=1&league=ID`, including disabled leagues, without enabling polling. Successful lists are cached; retry requests only target failed lists. A partial fetch does not erase existing favorites. Non-team competitions can be followed by sport or league; athlete/driver catalogs are not provided by this page.

Favorites now lead the page with Sports / Leagues / Teams views, sport and league filters, search, selected-only browsing, selection counts, and a separate collapsible server section. Desktop and 390px layouts were reviewed. A live catalog check loaded 2,116 league-scoped team entries across all 31 team-based leagues. Provider membership and totals may change.

The settings catalog is broader than the current frontend event renderers: saving a favorite does not add app support for a new event type or league. Those preferences remain stored for compatible app versions.

## Verification

From `backend/`, with Go 1.26.5:

```sh
go test ./handlers ./models ./services/user_settings ./api -count=1
go build -buildvcs=false -o /tmp/mediastorm-sports-backend .
```

The DOM behavior tests exercise the actual Go-rendered template. They require Node and jsdom (available in the paired frontend's dependencies). Set `NODE_PATH` to that checkout's `node_modules` or an isolated installation of jsdom.

```sh
mkdir -p /tmp/mediastorm-sports-ui
SPORTS_UI_RENDER_DIR=/tmp/mediastorm-sports-ui go test ./handlers -run TestSportsAdminTemplateRendersBothScopes -count=1
SPORTS_UI_RENDER_DIR=/tmp/mediastorm-sports-ui node ../dev/sports/admin-ui.test.cjs
```

Tests cover ownership, persisted favorites, stale revisions, preservation of stream scopes and profile data, empty availability, filtered team selections, unavailable catalogs, and late responses after profile switching. Browser checks were performed at desktop and 390-pixel phone widths using sample API responses, including both save actions and team search.

Server availability uses sport-level checkboxes with checked/partial/unchecked states, enabled counts, indented child leagues, and global Select all / Deselect all controls. Parent actions include every league in the sport even when a search hides some children. An empty selection cannot be saved. New and previously unconfigured servers default to all 50 catalog competitions; explicitly saved nonempty subsets remain unchanged. This increases initial provider polling coverage compared with the previous five-league default. Catalog/config parity and preservation of saved subsets are covered by regression tests.

Sport header buttons expand/collapse their child lists; only their separate checkbox toggles availability. Cycling now contributes its 12 organizer competitions to the catalog. Cycling requests honor those enabled IDs and skip disabled competitions, including cached schedules; generic ESPN scoreboard requests skip cycling. The former hard-coded cycling favorites fallback is deduplicated against the catalog.
