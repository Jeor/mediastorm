# Independent provider revalidation — September 24, 2026

All **313** manifest candidates received an independent ordinary scoreboard request and a dated request. Every response in that completed run was HTTP 200; activation nevertheless requires returned league identity, a dated current/relevant-season event, stable event identity and named participants. **203** meet that minimum and are marked **limited**, not complete or production-certified. **110** remain candidates. A second pass checked 134 unresolved entries using the previous provider calendar day or a bounded 2026 date range. No live-score capability is asserted from final/scheduled fixtures.

`validated-catalog.json` includes every candidate, additive descriptor fields, exact request URLs, retrieval timestamps, HTTP status, response SHA256/size, season, event IDs/dates/status/participants and fixture paths. `evidence/` retains provider-shaped compact payloads: first three default/dated events (including all their competitions/participants), plus relevant chosen event; second-pass season responses retain all returned events. Hashes describe original response bytes, not reserialized compact files. Fixtures establish observed behavior, not complete league schedules. Date filtering by actual timestamps remains necessary: a UTC early-morning event can belong to the preceding ESPN scoreboard day.

`coverage-tracker.csv` retains all **340** original rows. It explicitly distinguishes provider-fixture validation from backend, UI and device testing, which this evidence task did not run. Cricket target findings are maintained separately by the cricket work package; its fresh 500-row discovery snapshot is `evidence/cricket-dropdown.json`. W6 reference findings are in `w6-findings.json`. Official NCAA pages requiring human verification were not bypassed. Accessible NCAA indoor result HTML is useful reference evidence but not a certified generic results API. Genius documents CFL support, but no authenticated access was available. Free boxing season data returned only 15 entries and incomplete statuses; it does not justify expanding the existing limited schedule claim.

No entry is labeled archival or alias merely because its current default sample is old. Candidate rows retain their last returned dates and the 2026 probe, allowing a reviewer to distinguish a stale provider from a retired competition or a periodic tournament. No duplicate nonempty provider league IDs were observed. Olympic and international tournament cycles require separate season evidence; no yearly assumptions were imposed on historical samples.

Reproduction: run `python3 docs/sports-coverage/probe_coverage.py`, then `python3 docs/sports-coverage/refine_coverage.py`, then `python3 docs/sports-coverage/build_tracker.py`. Network requests are bounded to four workers for the first pass and two for refinement, have 25-second timeouts and 16 MiB response bounds, and honor bounded Retry-After delays without immediate retries. The first attempt's custom client header produced 403 responses; it was stopped and preserved in `initial-client-failures.json`. The standard urllib client, which had already succeeded in the initial connectivity check, completed the validation. No authentication, protected content or private provider references were accessed.

| Provider sport | Limited dated samples | Unresolved candidates |
| --- | ---: | ---: |
| australian-football | 1 | 0 |
| baseball | 6 | 5 |
| basketball | 6 | 5 |
| field-hockey | 1 | 0 |
| football | 1 | 2 |
| golf | 5 | 3 |
| hockey | 4 | 1 |
| lacrosse | 4 | 0 |
| mma | 6 | 41 |
| racing | 2 | 0 |
| rugby | 9 | 5 |
| soccer | 155 | 47 |
| volleyball | 2 | 0 |
| water-polo | 1 | 1 |
