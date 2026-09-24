# API and stream-search parameter audit — 2026-09-24

## Live read-only verification

250 production-shaped ESPN scoreboard requests returned HTTP200 and the expected provider league slug. Requests included the current date, limit200, football DivisionI group90 and basketball DivisionI group50 where applicable; racing uses its separate undated scoreboard contract. The audit checks routing/identity, not season completeness or guaranteed live coverage.

18 additional requests returned HTTP200 with valid JSON: official CFL fixtures/teams/standings, MotoGP seasons and discovered current-season categories/events, eleven ASO cycling stage feeds, and the existing boxing date/league endpoint. Other organizer paths retain their existing captured-fixture regressions; they were not freshly probed in this audit.

Raw request URLs, timestamps, response hashes and identity results: evidence/request-parameter-audit.json and evidence/organizer-parameter-audit.json. Reproduce from the repository root with audit_sports_requests.py and audit_sports_organizers.py; requests are bounded to four workers and do not fetch media streams or use private addon credentials.

## Automated contracts

Every newly activated ESPN schedule fixture now validates the actual request host/path against independent endpoint evidence, as well as date/group/limit parameters. New NASCAR fixtures exercise GetRaceBoard HTTP dispatch. All298 optional endpoint fixtures validate the actual teams, summaries and standings request contracts: limit1000, raw provider eventID, and current season respectively. Existing CFL, MotoGP, cycling, boxing and college-group tests also pass.

Addon HTTP regressions check golf tournament names, college school/team names, tennis doubles accents/slashes, race names and cricket opponents. They verify Stremio extras remain correctly encoded in the path, including ampersands and spaces, and do not become unintended HTTP query parameters. Existing Xtream selected-category/caching tests pass. These are controlled addon tests, not a claim that every user's configured addon returned a playable stream.

## Corrections

1. Pregame summary validation and self-event exclusion now compare raw provider IDs, allowing canonical namespaced app IDs without discarding correct summaries.
2. Golf tournament addon searches use the tournament title instead of an athlete copied from the leaderboard. Team matchups keep opponent searches; fallback catalog matching remains intact.
3. MotoGP's runtime provider is labeled organizer, consistent with its dedicated API acquisition path.

## Results

- Full backend suite:82 tested packages passed.
- Focused API/search regressions passed with the race detector.
- Frontend runtime/API regression subset:3 suites/25 tests passed; no frontend code changed.
- Independent standards/spec reviews found no remaining issues in these fixes.
- No native-device or live playback tests were performed in this audit. Source completeness blockers recorded by the coverage tracker remain unchanged.
