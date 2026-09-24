# Sports coverage expansion — implementation and remaining gaps

## Achieved coverage

All 340 targets have a row in coverage-tracker.csv: 313 original manifest entries, 21 named cricket expansion groups and six W6 sports. This is not a claim of complete coverage.

- 205 of 313 manifest entries are integrated with verified limited capabilities: 204 ESPN schedules and the official CFL API replacing the stale ESPN CFL feed.
- Seven additional verified cricket series are activated (23805,24200,24627,24619,24620,24621,24623); three international-series target groups are partially served. Existing IPL remains intact. The persisted discovery endpoint returns candidates separately and explicitly reports incomplete discovery; dropdown rows never activate themselves.
- Runtime catalog: 267 active entries = 55 existing + 205 manifest + seven cricket series. All 267 actual HTTP descriptors pass frontend validation; 247 matchup entries retain their lanes with synthetic client events, while racing/cycling use separate event contracts.
- Added optional capabilities: 103 standings integrations including CFL, 10 identity-verified ESPN summaries, 186 limited ESPN team-identity lists, and one official CFL team catalog. No new live-score capability is advertised solely from a schedule/final snapshot.

## Remaining coverage blockers

Tracker totals: 208 limited targets, 87 candidate targets with insufficient current provider data, 24 blocked targets, and 21 archival dispositions. No aliases were inferred from similar names. The 24 blocked targets comprise 18 named cricket competition groups and all six W6 investigations. W6 boxing means expansion beyond the existing limited boxing feed, not removal of current boxing.

Each unresolved target retains endpoint evidence and a next action. Stale/empty ESPN schedules, unverified recurring cricket series mappings, anti-bot/HTML-only organizer discovery, and access-dependent paid APIs remain genuine source gaps. Current MMA promotions such as Cage Warriors, KSW and Rizin are explicitly marked active organizations with stale ESPN coverage, not retired competitions. Historical Olympic editions and retired organizations are not live-polled. No Nuvio metadata dependency, subscription purchase or untested paid integration was added.

## Behavior and resilience

Backend metadata identifies provider, application sport, adapter, implementation state and capabilities separately. Legacy IDs, explicit enabled-league lists and profile follows are preserved. Admin Select all saves the explicit all-supported sentinel; future validated additions join that mode only. Unsupported candidates stay out of normal polling and frontend filters.

Captured provider fixtures exercise every activated ESPN schedule through HTTP decoding and normalization. Generic periods, cricket innings and raw over/ball strings, team golf, college school names, colorful artwork fallback, spoilers, progressive streams and player returns stay on family-specific paths. Current summaries/standings are optional and bounded; incomplete team catalogs are labeled as such. Multi-day cricket merges bounded overlapping series data.

Metadata requests coalesce and share four slots across ESPN hosts, honor Retry-After, cache schedules/catalogs at different cadences and retain prior data on failure. Dated requests no longer hold a global lock during network work; every waiter can cancel independently. Detected limit/count/page truncation retains available games and reports partial/stale coverage rather than claiming completeness. Unknown provider pagination remains a limitation; no guessed continuation URLs are followed.

## Verification and limits

- Full backend normal test suite passed after review fixes (82 tested packages). Sports service and focused sports-handler race tests passed after all fixes. Broad handler race run exposed an unchanged HLS test-harness strings.Builder race in hls_direct_cast_test.go:390; this is outside the sports diff and prevents claiming an all-backend race-clean run.
- Frontend: 146 suites / 942 tests passed; TypeScript passed; changed-file lint had no errors (two existing warnings).
- 1,000-event behavior tests cover mocked iOS/Android phone/tablet, web and TV conditions, retaining all events while bounding mounted cards. These are automated simulations, not native-device tests.
- Production web, iOS/Android JS and TV-configured JS exports passed. These are not native application binaries.
- Actual backend binary built. With an isolated PostgreSQL instance, /watch/sports and the hashed production JS asset returned HTTP 200. Browser startup reached sign-in. Authenticated Sports Hub browser interaction and native device/remote interaction remain unverified.
- Actual GetLeagues and settings HTTP handler integration confirmed all-supported and explicit NFL/MLB settings roundtrips; raw contract reports are retained under evidence/coverage-*.json.

## Deployment and review

Deploy the backend and rebuilt frontend together. Backend-only deployment cannot update installed mobile/TV renderers. Package the web export under STRMR_WEB_APP_DIR (the standard /watch prefix), rebuild/release native apps through existing platform pipelines, then perform authenticated phone/tablet/TV acceptance checks. No database schema migration was introduced; existing settings/team persistence is reused. Existing hand-picked server league selections remain unchanged until an administrator chooses new entries or Select all.

Work is confined to Jeor forks. Recorded upstream baseline commits remain backend41cd8125 and frontend44763258. Dedicated fork review-base branches preserve those baselines so draft PRs do not include unrelated drift from the forks' older default branches. No upstream branch or PR is changed.

Source evidence: validated-catalog.json, source-dispositions.json, evidence/optional-capabilities.json, evidence/cricket-target-validation.json and w6-findings.json. The handoff and raw provider responses are included for reproducibility; large fixture files are evidence, not application assets.
