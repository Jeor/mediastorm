# Sports detail coverage and expansion roadmap

## September 2026 phase

| Sport | Provider / contract | Delivered fields | Limits |
|---|---|---|---|
| Golf | ESPN PGA scoreboard; `pga` | Tournament title/status, leaderboard scores to par, round strokes, hole strokes/to-par, event end date | PGA only initially. Provider ordering is not asserted to be a tied rank. Unplayed holes are absent. |
| Cricket | ESPN IPL scoreboard; `cricket-8048` | Complete score strings, batting innings, runs, wickets, overs, provider result summary | IPL only initially; no claim of all international/domestic cricket. Non-batting placeholders are omitted. |
| Boxing | TheSportsDB v1 day schedule; `boxing` | Event title, UTC start, venue, explicit final/postponed state when supplied | Limited public schedule, not comprehensive coverage. No inferred live status, punch stats, odds or fighter records. |
| Tennis | ESPN ATP/WTA scoreboard | Individual set scores and existing match totals | No verified serve speeds, serve percentage or live ball tracking. |
| NHL | ESPN summary plays | Shot/goal/missed/blocked coordinates, period/clock, team and description | Coordinates in feet, provider orientation. No attack-direction inference or power-play countdown. |
| Rugby union | ESPN scoreboard + summary | Scoring timeline, nested possession/territory/tries and other comparisons | Missing summary timeline retains scoreboard plays. Rugby league enrichment not claimed without schema verification. |
| MMA | ESPN UFC core | Height/reach/weight/stance/division, significant strikes, takedowns, control time and knockdowns when supplied | Four bounded requests per bout, cached. No fabricated winner probability, live tracking or unsupported records. |
| F1 | Existing official archive integration | Tyres, stints, timing and weather already available in archived sessions | Historical/recorded mode only, not a free live telemetry claim. |

Saved league selections remain unchanged. Enable `pga`, `cricket-8048`, and `boxing` through Sports settings on existing installations. Fresh defaults include them. Mobile and TV clients need this frontend plus the matching backend.

### Artwork

Use existing MaterialCommunityIcons and original vector rink, golf, cricket and ring graphics. Existing optional provider logos keep their normal fallback behavior; no generated athlete photos or downloaded promotional posters are required. Provider artwork availability is not a blanket redistribution license. Circuit reference assets retain existing attribution. The preview gallery uses original SVGs and captured factual API data, labels dates, and distinguishes archive data from live data.

### API evidence

- PGA: `https://site.api.espn.com/apis/site/v2/sports/golf/pga/scoreboard`
- IPL: `https://site.api.espn.com/apis/site/v2/sports/cricket/8048/scoreboard`
- Tennis: `https://site.api.espn.com/apis/site/v2/sports/tennis/atp/scoreboard?dates=20250907`
- NHL: `https://site.api.espn.com/apis/site/v2/sports/hockey/nhl/summary?event=401777460`
- Rugby: `https://site.api.espn.com/apis/site/v2/sports/rugby/180659/summary?event=600262`
- UFC: `https://sports.core.api.espn.com/v2/sports/mma/leagues/ufc/events/600054045/competitions/401772654/competitors/3166126/statistics`
- Boxing day filter: `https://www.thesportsdb.com/api/v1/json/123/eventsday.php?d=2026-09-19&l=4445`
- TheSportsDB limits: https://www.thesportsdb.com/documentation (free day endpoint limited to three events; premium coverage is separate).

Reduced regression fixtures retain only fields needed by the tested contracts. They are historical snapshots, never production fallback events.

## Later phases

1. Broaden existing sports: verify LPGA/DP World Tour and major international cricket competitions (Test/ODI/T20), then domestic competitions. Verify boxing provider completeness before claiming full coverage.
2. Add volleyball/beach volleyball, softball, badminton and table tennis. Require event identity, lifecycle, participants and set/period scores first.
3. Add athletics, swimming and gymnastics through meet/session/result contracts with heats, attempts and apparatus where supported. Do not force these into two-team cards.
4. Evaluate darts, snooker, handball and winter sports based on audience demand and provider completeness.
5. Investigate live F1, richer cycling and other premium data only after cost, credentials, redistribution and reliability are confirmed. No paid provider is enabled by this change.

Every sport must pass: verified provider samples for scheduled/live/final states (or an explicit limited schedule-only contract), stable IDs, timezone boundaries, zero-vs-missing tests, mobile long-text checks, TV focus/scroll checks, spoiler checks and source-failure recovery. Unsupported modules stay hidden; no synthetic stats in production.
