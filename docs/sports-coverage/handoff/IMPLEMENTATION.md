# Sports coverage expansion specification

Status: ready for implementation, with source-validation gates. Prepared September 24, 2026.

## 1. Problem and intended result

Mediastorm supports selected sports and leagues but omits many competitions ESPN lists, including entire team-sport families. Adding backend league IDs alone is insufficient: frontend allowlists discard unfamiliar IDs, specialized normalizers assume particular sports, and some provider directories contain old or incomplete feeds.

Deliver usable schedules, event cards, scores, event details and stream selection for every missing competition with a verified source. Preserve all existing sports features across phone, tablet, web and TV. Account explicitly for every missing directory entry and broader cricket target, including entries that are historical, duplicates, inaccessible or insufficiently covered.

### Scope interpretation

- **Required inventory:** all 313 entries in `missing-leagues.json`, plus cricket expansion and the additional W6 sports. No arbitrary cap on leagues or implementation waves.
- **Active integration:** enable a competition only after validating its identity, current or relevant season, date behavior and minimum display contract.
- **Historical competitions:** retain a documented archival disposition, with optional history support if existing date navigation supports it. Do not poll retired leagues as live competitions.
- **Aliases:** map duplicate provider aliases to one canonical competition when evidence establishes equivalence. Do not deduplicate based only on similar names.
- **Unavailable sources:** remain tracked with evidence and the next required action. They are unresolved targets, not “implemented.” Continue all independent feasible work.
- **Paid providers:** source options only until authorized access is supplied. The specification does not authorize subscriptions or credential acquisition.

## 2. Baseline and repositories

Backend catalog baseline: `godver3/mediastorm@41cd8125fc5d06e70ec31d8b4c1c4e616414397a`. Frontend main revision recorded during inspection: `godver3-org/mediastorm-frontend@447632588eed70c7bdfd24a4c3ce3e851ee93bf2`.

Implement in Jeor's forks. Verify remote relationships and current branches rather than assuming older worktree names still point to current code. Preserve unrelated changes. Keep backend and frontend changes separately reviewable, with a shared contract and coverage manifest. No upstream submission until explicitly requested.

The earlier mobile rendering mitigation is in frontend commit `035592e5`, branch `fix/mobile-sports-list-virtualization` in Jeor's fork. Determine whether its equivalent is already present before integrating it; do not duplicate or revert it. Native crash resolution was never established by that change alone.

### Existing seams to inspect

Backend paths, relative to the repository root:

- `backend/services/sports/service.go`: `League`, `LeagueCatalog`, enabled league selection, scoreboard cache/refresh and team catalog.
- `backend/services/sports/espn.go`: provider JSON parsing and matchup normalization.
- `backend/services/sports/scoreboard_coverage.go`: per-bout and tennis competition expansion.
- `backend/services/sports/detail.go`: event enrichment currently includes hardcoded league dispatch.
- `backend/services/sports/standings.go`, `team_context.go`, `team_detail.go`, `pregame.go`, `dates.go`: optional data, date selection and caches.
- Existing racing, golf/cricket, boxing and cycling implementations: discover their current locations; preserve their contracts.
- `backend/models/sports.go`, sports detail/preference models, sports handlers, user settings and routes.

Frontend paths:

- `features/sports/sports-model.ts`: closed competition registry and league-ID type; sport/detail types.
- `features/sports/sports-backend-source.ts`: rejects or filters league IDs outside that registry; has PGA/boxing special cases for events without two named teams.
- Sports backend API client, follows/pins/preferences, availability, date filtering and source interfaces.
- `components/sports/SportsMobileHub.tsx`, `SportsMobileScoreboard.tsx`, `SportsHubTV.tsx`, showcase cards and sport detail components.
- Sports filters, icon maps, card artwork, scoreboard replay/presentation, stream matcher and player return navigation.

These are inspection targets, not a mandate to preserve outdated hardcoded dispatch. Search for all league/sport allowlists and special cases before editing.

## 3. User stories

1. As a viewer, I want every supported sport and competition selectable so I can follow events outside the original catalog.
2. As an administrator, I want to enable supported competitions without losing my saved selections when the catalog expands.
3. As a viewer, I want my profile's sports and following choices preserved so another person's preferences do not change my hub.
4. As a college sports viewer, I want school names and logos so I can identify teams without recognizing mascots.
5. As a viewer, I want correct local dates, scheduled times and multi-day events so I can find the event I intend to watch.
6. As a viewer, I want sport-appropriate scores and periods so cricket, volleyball, golf and AFL remain understandable.
7. As a viewer, I want available pregame records, rankings, venue facts and standings while I wait, without fabricated information.
8. As a viewer, I want postponements, cancellations, delays and missing data distinguished from a normal empty schedule.
9. As a viewer, I want preseason, regular season, playoffs and championships labeled from reported data so I understand the event context.
10. As a viewer, I want scores to appear promptly while optional details and streams load independently.
11. As a viewer, I want each available stream or alternative selectable and playback to return to the event details, including after an error.
12. As a mobile viewer, I want all sports enabled without the application rendering every event at once or crashing.
13. As a TV viewer, I want remote focus and Back/Menu behavior to match the visible controls as data arrives.
14. As a spoiler-sensitive viewer, I want results, rankings or winner indicators handled consistently with the existing spoiler policy.
15. As a viewer, I want clear limited/stale/unavailable coverage states so a provider outage is not mistaken for no events.
16. As an administrator, I want per-source health and failure information so incomplete coverage can be diagnosed without collecting private stream URLs.

## 4. Competition registry and capabilities

### One effective catalog

Use the backend catalog as the runtime source of competition identity, name, sport, provider and supported capabilities. Keep known frontend entries as compatibility/presentation overrides, but do not reject otherwise valid backend competitions merely because an ID is absent from a compile-time union. Use runtime validation and safe default presentation; do not replace validation with unchecked type casts.

Preserve every existing league ID. New IDs in the manifest use `espn:{sport}:{slug}` as **proposed** globally distinct identifiers; reconcile with current project conventions once, before persistence. Do not rename existing soccer, rugby, racing, IPL or other IDs. Provider IDs must remain separate from canonical Mediastorm IDs.

Represent provider sport, application sport and adapter family separately. In particular:

- ESPN `football` is application American football; `australian-football` is AFL.
- ESPN `hockey` maps to ice hockey; field hockey is a different sport.
- ESPN places college/Little League softball under `baseball`; display it as softball and use its own competition metadata.
- `golf/tgl` is a team-match contract, not an ordinary athlete leaderboard.
- Cricket competition identity, season and individual tour/series identity must not be collapsed.

Add optional registry metadata for college status, gender/division where reported, event family, provider references, aliases and season discovery. The minimum capability set should distinguish schedule, live score, final result, period/set/innings detail, standings, team catalog, players, play-by-play and artwork. Capabilities are supported behaviors backed by evidence, not just inferred from sport names.

Maintain separate implementation status (`candidate`, `validated`, `limited`, `archival`, `alias`, `blocked`) and operational health (`healthy`, `stale`, `unavailable`). A provider outage must not permanently remove a validated competition.

### Persistence and compatibility

Extend current models/settings additively. Store provider-to-canonical identity mappings using existing persistence infrastructure; inspect whether an existing table/config can serve this purpose before adding tables. If new tables are required, provide migrations and upgrade tests. Legacy API fields and event IDs must keep working for older clients.

Preserve saved enabled-league sets and follows. For an explicit “all supported” preference, ensure newly validated entries become selectable under the documented policy; do not rewrite an explicit hand-picked set to all leagues. Candidate and archival entries must not enter the normal refresh loop simply because the catalog grew.

## 5. Acquisition, discovery and normalization

### ESPN

Reuse the existing HTTP client, cache, API usage tracking and source health handling. Endpoint templates and observed limitations are in SOURCES.md. Treat these public website endpoints as undocumented interfaces with no assumed service guarantee.

Validate each missing entry with dated samples and the official competition schedule where necessary. An HTTP 200 response containing the last event from 2022 does not establish current coverage. Distinguish an off-season or four-year tournament cycle from a stale provider feed. Inspect returned season and event metadata; do not assume a directory label identifies the precise gender or edition of an event.

Query appropriate dates using tested provider behavior. Default ESPN scoreboards can return past or future dates; filter by the selected local-day interval after parsing actual start/end instants. Preserve multi-day cricket/golf events that overlap the interval, including ongoing events that started earlier. Handle season years that differ from calendar years. Do not apply one numeric preseason/postseason mapping to every sport.

Implement pagination/truncation handling per endpoint. Team list samples commonly stop at 50 entries; HTTP 200 plus 50 teams is not a complete catalog. Do not fix truncation by raising every scoreboard request to an arbitrarily huge limit. Check existing college division/group behavior and retain completeness fixes.

### Event and score identity

Keep provider, sport, competition, event, sub-event and participant identities independently. For MMA, use individual bout IDs under the card and preserve the parent title for stream matching. For racing, preserve weekend/session identity. For TGL and multi-event meets, do not take only `competitions[0]` or generate duplicate top-level cards for every nested result.

Preserve scores as display strings plus optional structured values. Never coerce `161/5 (18/20 ov, target 156)`, `E`, `-8`, a timed result or a judged score into a generic integer. Decode provider booleans defensively where samples show string booleans, while rejecting malformed values. Missing, scheduled zero and a genuine score of zero are distinct states.

Keep trustworthy provider labels for round, competition phase, leg, playoff series, championship, format and weather interruptions. Do not infer a final result or championship winner solely from time passing or a participant being listed first.

### Optional information

Fetch summaries, standings, rankings, roster/player data and venue facts lazily or in bounded cached background work. A missing optional endpoint must not prevent the schedule or score from appearing. Expose unavailable capabilities honestly and avoid rendering empty or misleading tabs.

Prefer images and colors already included in scoreboards/team catalogs. Continue using team or event logos over locally composed colorful backgrounds. Use a generic sport icon when images are absent. Fetch images at bounded display sizes with cache reuse; no eager download of every event's full-resolution art.

Provider references may point to internal/private hosts: the AFL sample contains `espn.pvt` references and cricket includes a legacy reference host. Do not blindly follow arbitrary `$ref` or image URLs. Use validated public endpoints on the project's provider allowlist; omit inaccessible enrichment. This also prevents unbounded recursive fetches.

### Cricket

Implement a registry of recurring competitions and a discovery layer for dated series/tours. Use the site league dropdown as one discovery input, not an exhaustive source: the observed response has 500 rows and some empty slugs. Never synthesize an endpoint from an empty ID or assume a root/`all` scoreboard exists.

Initial named targets: men's and women's ICC World Cups/T20 World Cups, Champions Trophy and World Test Championship where data is available; international Test/ODI/T20I series including the Ashes; BBL/WBBL, PSL, SA20, CPL, MLC, The Hundred, WPL, BPL, LPL and ILT20. Resolve actual provider competition/series IDs from source evidence, not guesses. Retain IPL.

Normalize innings, runs, wickets, overs/balls, target, batting side, result and rain/abandonment/draw/tie/no-result states. An over value such as `18.4` is an over-and-ball representation, not a decimal quantity. Preserve raw provider text when ball counts or format are uncertain. A Test can span days and four team innings; do not collapse it to one home/away numeric score.

## 6. Rendering and interaction

Use the existing responsive designs. Add a renderer per event family, not per league. Common team cards should remain shared, with optional set/period/innings panels. Avoid silently forcing unknown events through MLB, NFL or PGA diagrams.

Sport-specific acceptance details:

- **AFL:** team names, total points and reported goals/behinds when supplied; quarter/status labels. Do not use NFL drives or touchdowns.
- **Lacrosse / field hockey / water polo:** correct reported periods and scores; league-specific time/shot-clock rules only when supported. Do not hardcode a common period count across divisions/genders.
- **Volleyball:** match sets won plus per-set points where supplied; distinguish current set from the overall match result.
- **Baseball / softball:** innings and optional runners/counts; format-dependent inning lengths, extra innings and run rules. Do not assume every competition is a nine-inning MLB game.
- **American football variants:** CFL/UFL field/rule differences; generic score/detail panels are preferable to an incorrect NFL field visualization.
- **Golf:** athlete leaderboards, rounds, holes, cut/withdrawal/disqualification and playoff status where supplied; TGL has a separate team presentation.
- **Soccer:** league/cup identity, penalties versus regulation score, aggregate/leg and group context when present.
- **MMA/boxing:** event card plus bout identities, participants and reported outcome; absent round-by-round statistics remain absent.
- **Olympic/multi-sport entries:** preserve gender and event discipline; an Olympic league entry does not imply all Olympic sports are supported.
- **Meets/judged/timed sports in W6:** multi-participant results, precision and units must be represented explicitly rather than inventing a two-team matchup.

Audit all hardcoded sport/category maps: filters, following selectors, icons, colors, league logos, team naming, pregame sections, spoiler masking and stream matcher aliases. Display college school names consistently on every platform, including newly added NCAA competitions.

Streams remain a separate workflow. Include league/competition aliases, team school names, athlete names and parent tournament/card labels in existing matching, with sport/date context to avoid false positives. Metadata expansion supplies no broadcast rights or media URLs. Reuse the existing player/HUD and return-to-details route for success, cancellation and error exits.

## 7. Performance and resilience

Use a bounded scheduler rather than polling all 313 candidates every few seconds. Starting implementation defaults, to tune against actual provider limits:

- At most four concurrent metadata requests per provider across all clients; share requests for the same cache key.
- Live scores: approximately 30–60 seconds where provider conditions permit; selected detail requests can reuse the freshest cached response.
- Upcoming schedules: approximately 15 minutes; nearby starts can refresh more often.
- Catalogs/team identities: approximately 24 hours; standings 15–60 minutes; archive data much longer.
- Apply jitter, cancellation, bounded backoff and Retry-After. Avoid retries of permanent missing capabilities on every render.

These are proposed defaults, not asserted ESPN quotas. Provider-specific published limits override them. Include response-size limits and compressed-response handling without making large valid event days appear empty. Keep the last good data on temporary errors and distinguish stale data from an actual empty result.

Virtualize phone/tablet event lists and large tables; preserve TV's spatial navigation constraints. Stabilize row identities and focus ordering when results arrive asynchronously. Unmount or pause unnecessary subscriptions when backgrounded. Do not let an open drawer/filter reset the visible event set to “no live events.”

## 8. Work packages

Complete these as sequential reviewable increments; independent work may be organized according to repository instructions.

### W0 — Baseline, evidence and tracker

- Compare current branches with recorded baselines; retain existing fixes.
- Import the full manifest into a durable coverage tracker.
- Revalidate provider samples and classify current/seasonal/archive/alias/source-blocked entries.
- Derive compact fixtures from captured samples. Keep source date, URL and capability evidence with each target.

### W1 — Shared catalog and capability handling

- Add additive backend descriptors/capabilities and safe frontend runtime catalog support.
- Refactor hardcoded enrichment dispatch only as needed to route by supported adapter capability.
- Implement schema validation, identity rules, settings migration, partial-failure states and bounded scheduling.
- Prove a newly described test competition traverses the public backend contract and appears correctly in the frontend without being filtered out.

### W2 — Existing matchup families and all soccer gaps

- Process every soccer entry in the manifest: 202 entries, not just the eight sampled representatives.
- Add validated NCAA baseball/softball/hockey, international baseball/basketball, G League/summer leagues, UFL, additional rugby and remaining existing-family entries.
- Check cups, qualifiers, lower divisions, international and women's competitions and their distinct identities.
- Resolve CFL current coverage through W6 if ESPN cannot supply it.

### W3 — Entirely missing team sports

- AFL; college and professional lacrosse; volleyball; field hockey; water polo.
- Implement score-specific presentation and rules without inappropriate inherited sport diagrams.
- Support school identity, current-season completeness and optional details independently.

### W4 — Golf and cricket

- All remaining eight golf directory entries, with separate TGL handling and Olympic season validation.
- Cricket series discovery and every named target in section 5, including women's cricket.
- Treat absent leaderboards/players as limited coverage while retaining valid schedules; do not claim detailed support from a schedule sample.

### W5 — Racing and combat

- NASCAR second-tier and Trucks through the racing/session adapter, with season results and participant validation.
- Process all 47 non-UFC MMA directory entries. Validate historical/current promotion status and event freshness; preserve bout-level IDs.
- Audit boxing completeness and optional alternate sources. Keep the existing limited-coverage label unless stronger evidence warrants a capability upgrade.

### W6 — Sources outside a usable ESPN feed

- CFL: validate current official CFL/Genius or another authorized source; do not activate stale ESPN-only results as current.
- Missing/old MMA feeds: review current promotion sources or an authorized provider and reconcile identities explicitly.
- Wrestling, gymnastics, swimming/diving and track and field: investigate official NCAA/governing-body schedules/results and documented partner APIs. These lack a validated drop-in ESPN league mapping in this audit.
- Professional volleyball beyond NCAA, where desired within ESPN's wider coverage, also requires a distinct league/source mapping; NCAA support does not cover it.
- Use documented sources in SOURCES.md. Keep missing-source targets open with a clear reason and next step; do not fabricate an API, bypass access restrictions or convert an unstructured news page into a claimed live-score feed.

### W7 — Complete platform validation and release preparation

- Verify backend and frontend together, including the web bundle served by the backend.
- Produce one backend PR and one frontend PR in Jeor's repositories, with coverage tracker links and actual test evidence; no upstream submission yet.
- Document schema/version compatibility, backend image rebuilds for bundled web assets and new mobile/TV builds needed for client changes.
- Report implemented, limited, archival/alias and genuinely blocked targets separately. Any unresolved requested target means universal coverage is not yet complete.

## 9. Verification and acceptance criteria

### Backend tests

Use existing HTTP handler/service integration seams with an in-process fake provider. Assert observable API results; avoid tests that only mirror a catalog constant.

For each adapter family, cover scheduled, live, final, postponed/canceled, malformed/empty data and partial optional-data failure. For each added competition, retain a data-backed smoke fixture and freshness/season evidence. Test compressed data, string/number/boolean variations, competitor-less schedules and payload truncation.

Test local-midnight boundaries, DST, cross-midnight and multi-day events; postseason year versus calendar year; cup rounds, playoff legs/series and genuine championship labels. Test school names, duplicate event/bout IDs, aliases, archived feeds and renamed teams. Test settings upgrades and explicit selection preservation.

Demonstrate that one provider timeout/403/404/429 does not blank other sports, stall scores until stream lookup completes, or repeatedly fetch a known absent capability. Test request concurrency and cancellation through the shared provider client.

### Frontend tests

Test through backend-response normalization and rendered hub/detail behavior. Confirm valid new competitions survive mapping, filters and follows; unsupported payloads degrade safely. Test score strings/units, college names, spoiler masking, logo fallbacks and missing participant data.

Verify both phone and tablet paths on iOS and Android; desktop/narrow web; Apple TV/tvOS and Android TV focus/navigation paths. A mocked Platform.OS test is not native device verification.

Required regressions:

- 1,000-event synthetic hub retains all data while mounting only a bounded visible batch; scrolling and return navigation work.
- Stream picker lists all returned sources and alternatives as results grow; web viewport does not collapse; long lists scroll on every platform.
- Remote focus order matches visual order after dynamic insertion; More Options fits, scrolls and closes; no focus oscillation.
- Fullscreen player HUD buttons work; Back/Menu first dismisses controls where appropriate, then returns to details; error exits also return correctly.
- Opening drawers does not clear events; score display does not await IPTV/addon search; empty/offline/stale states are distinct.
- All-sports startup is exercised with a real native build and diagnostics when available, given the earlier iOS crash report. Report any unavailable device test explicitly.

### Completion evidence

The coverage tracker must include every manifest row, every named cricket target and every W6 sport. Record canonical ID, provider, endpoint, tested season/date, adapter, capabilities, current/archival assessment, API test, UI test and remaining limitation.

Run scoped backend tests and frontend tests/lint/type checks, then required build/export checks. Record baseline unrelated failures separately; do not disguise failures as passing. Rebuild the web export and verify it in the backend-served context before declaring the web integration fixed.

## 10. Out of scope

This work does not acquire streaming rights, scrape protected video, introduce Nuvio data, purchase API plans, build betting/prediction features, or replace the existing overall hub design. Sports outside the audit and W6 require a separate scope decision. Rich stats, imagery and live updates cannot be promised for a source that only provides schedules; capability labels must reflect that boundary.
