# Source map and API research

Checked September 24, 2026. See the adjacent JSON probe logs for exact requests and observations. All candidate endpoints must be revalidated before production activation.

## 1. Evidence levels

- **Directory:** ESPN lists the sport/league identifier. This establishes a candidate, not usable live coverage.
- **Scoreboard sample:** an ordinary request returned one or more events. Inspect their actual dates, participant types and empty fields.
- **Supplementary sample:** a summary, team list or standings request was inspected. A 200 response can still be empty or incomplete.
- **Documented alternative:** the provider publishes relevant documentation, but authenticated access/data was not exercised here.
- **Reference only:** an official website useful for verifying schedules, names, results or source discovery. It is not a confirmed API integration.

No source received an end-to-end production certification in this audit. Do not use the word “verified” without specifying which level was verified.

## 2. Primary ESPN endpoint families

ESPN's existing public website APIs are the first choice because Mediastorm already uses them. They are not being represented as a documented developer service with guaranteed availability, universal coverage or a published request quota.

| Purpose | URL template | Evidence and constraints |
| --- | --- | --- |
| Sport discovery | `https://sports.core.api.espn.com/v2/sports?limit=100` | Returned 17 sport entries in the coverage audit; not the entirety of ESPN editorial/streaming coverage |
| League discovery | `https://site.api.espn.com/apis/site/v2/leagues/dropdown?region=us&lang=en&sport={sport}` | Returned named league metadata and slugs; use the manifest's exact identifiers |
| Core league index | `https://sports.core.api.espn.com/v2/sports/{sport}/leagues?limit=1000` | Returned reference lists; differs from site dropdowns, especially cricket/rugby |
| Scoreboard | `https://site.api.espn.com/apis/site/v2/sports/{sport}/{slug}/scoreboard` | 53 sampled pairs returned events; not necessarily today's events |
| Dated scoreboard | Same path plus `?dates=YYYYMMDD` | Single-date PLL request succeeded. Verify each adapter; do not assume all long date ranges work |
| Event summary | Same sport/slug prefix plus `/summary?event={providerEventId}` | Team-sport samples generally returned data; sampled LPGA, TGL and parent PFL requests returned 404 |
| Team list | Same prefix plus `/teams` | Some return 50-team pages or zero entries. IPL request returned 404. Do not assume complete team support |
| Standings | `https://site.api.espn.com/apis/v2/sports/{sport}/{slug}/standings` | Note the different `/apis/v2/` prefix. Empty responses are common for some competitions |

### Response fields and useful information

Scoreboards commonly provide `leagues`, `events`, event `season`, `status`, `competitions`, `venue`, `competitors`, `broadcasts` and `notes`. Team competitors can include school/location, name, short display name, logos/colors, record, ranking, score and linescores. Athlete competitors and nested competitions require their own adapters.

Summaries may add `header`, `gameInfo`, `boxscore`, `leaders`, `standings`, `rosters`, `plays` or scoring plays. Presence varies with sport, event state and coverage. Preserve these fields opportunistically; do not make them prerequisites for cards or scores.

Use returned public image URLs such as `team.logo`, `team.logos[].href`, `leagues[].logos[].href` or athlete headshots when present. Do not synthesize CDN paths from IDs. The samples include internal/private `$ref` hosts that must not be followed blindly. More detail does not justify recursively resolving every reference in a scoreboard.

Every missing entry has candidate scoreboard/summary/team/standings URLs in `missing-leagues.json` and `.csv`. Fields marked `not-tested` or `not-established` are genuinely unverified.

## 3. Missing sport families and league expansions

In the table below, pairs are `ESPN sport / league slug`. “Sampled” means scoreboard data was returned, not complete current-season or live coverage.

| Family | Primary candidates | Other information and implementation notes |
| --- | --- | --- |
| Australian rules football | `australian-football / afl` — sampled | Summary, teams and standings returned data. Scoreboard has team logos/colors. Squiggle is a documented secondary source for fixtures, scores and standings; official AFL site is a reference |
| Lacrosse | `lacrosse / pll`, `nll`, `mens-college-lacrosse`, `womens-college-lacrosse` — all sampled | PLL summary/teams/standings returned data. NCAA and official PLL/NLL sites are reference sources for seasons and event completeness |
| Volleyball | `volleyball / mens-college-volleyball`, `womens-college-volleyball` — sampled | Women's summary/teams/standings returned data. Preserve match sets and individual set points. API-Sports has separate volleyball documentation; confirm exact league coverage before considering it |
| Field hockey | `field-hockey / womens-college-field-hockey` — sampled | Summary and standings returned data, but the team-list response contained zero teams. Scoreboard team identities still exist; do not claim exhaustive team following without a team discovery solution |
| Water polo | `water-polo / mens-college-water-polo`, `womens-college-water-polo` — sampled | Men's summary returned data; teams and standings were empty. NCAA provides official schedule/results references |
| Baseball and softball | `baseball / college-baseball`, `college-softball`, `world-baseball-classic`, `llb`, `lls`, `caribbean-series` — sampled; winter/Olympic entries in manifest | College baseball/softball summaries include boxscore and plays. Team lists returned 50 entries, requiring completeness checks. Display softball as its own sport despite the ESPN path |
| Basketball | `basketball / nba-development`, `nba-summer-las-vegas`, `fiba`, `nbl`, `mens-olympics-basketball`, `womens-olympics-basketball` — sampled; other summer variants in manifest | G League summary/teams/standings returned data. Verify edition/gender from event metadata for FIBA and Olympics; a broad directory name is not enough |
| American football | `football / ufl`, `cfl` — sampled; XFL directory entry requires archival/alias assessment | UFL summary/teams/standings returned data. CFL sample was from 2022: current official source validation is necessary before enabling current coverage |
| Ice hockey | `hockey / mens-college-hockey`, `womens-college-hockey` — sampled; Olympic and World Cup entries in manifest | Men's college summary/teams/standings returned data; team list was 50 entries. Preserve college school identities and competition-specific periods |
| Soccer | All 202 missing slugs in manifest | Eight representative scoreboards were sampled, including `eng.fa`, `eng.league_cup`, `uefa.euro`, `fifa.worldq.uefa`, `eng.w.1`, `uefa.wchampions`, `usa.usl.1`, `conmebol.libertadores`. FA Cup summary and teams returned data; season standings endpoint was empty |
| Rugby union | `rugby / 244293` Rugby Championship, `271937` Champions Cup, `270557` URC, `289237` Women's World Cup, `289262` MLR — sampled | Other missing slugs are in the manifest. Rugby Championship summary/teams/standings returned data. Official World Rugby, competition and union sites can establish the relevant season/cycle |
| Golf | `golf / lpga`, `eur`, `liv`, `champions-tour`, `ntw`, `tgl` — sampled; Olympic entries in manifest | LPGA/DP World examples have athlete lists. Several other examples only establish a schedule. TGL is team-based with multiple competitions and six returned teams; TGL standings returned data. Tested LPGA/TGL summary requests returned 404 |
| Racing | `racing / nascar-secondary`, `nascar-truck` — sampled | Current examples had no competitors; validate dated completed sessions before claiming race-result support. Reuse the existing racing adapter only after checking league gates and session handling |
| MMA | `mma / pfl`, `ofc`, `rizin`, `ksw`, `ifc` — sampled; all other promotion slugs in manifest | These contain athlete bouts. Rizin/KSW/Invicta returned older events. PFL parent summary request returned 404. Retain parent/bout IDs and validate supported detail routes; never invent an outcome from empty scores |
| Boxing | Existing TheSportsDB adapter | Existing source is a limited schedule, not ESPN-equivalent live statistics. Validate returned dates and coverage before widening claims. Official promoter/bout schedules are reference sources; any replacement API needs evidence |

### Representative supplementary endpoint results

| Pair | Summary HTTP | Team entries returned | Standings payload |
| --- | ---: | ---: | --- |
| AFL | 200 | 19 | Nonempty |
| NCAA baseball | 200 | 50 | Nonempty |
| NCAA softball | 200 | 50 | Nonempty |
| G League | 200 | 34 | Nonempty |
| NCAA women's field hockey | 200 | 0 | Nonempty |
| UFL | 200 | 8 | Nonempty |
| LPGA | 404 | 0 | Empty |
| TGL | 404 | 6 | Nonempty |
| NCAA men's hockey | 200 | 50 | Nonempty |
| PLL | 200 | 12 | Nonempty |
| PFL | 404 | 0 | Empty |
| Rugby Championship | 200 | 4 | Nonempty |
| FA Cup | 200 | 124 | Empty |
| NCAA women's volleyball | 200 | 50 | Nonempty |
| NCAA men's water polo | 200 | 0 | Empty |
| IPL | 200 | Teams HTTP 404 | Nonempty |

Counts above describe the returned response, not authoritative current roster counts. “Nonempty” means the observed standings structure was present; full row correctness and season matching remain to be validated. The samples include future and historical events, so a failed summary is not proof that no summary can ever exist for that competition.

## 4. Cricket source discovery

### Confirmed paths

- Existing IPL: [scoreboard](https://site.api.espn.com/apis/site/v2/sports/cricket/8048/scoreboard).
- Discovery: [site cricket league dropdown](https://site.api.espn.com/apis/site/v2/leagues/dropdown?region=us&lang=en&sport=cricket).
- Pakistan in England Test Series 2026: [series 23805](https://site.api.espn.com/apis/site/v2/sports/cricket/23805/scoreboard), sample event `1496584` dated September 9, 2026.
- Ashes 2027: [series 24627](https://site.api.espn.com/apis/site/v2/sports/cricket/24627/scoreboard), sample event `1547175` dated June 18, 2027.
- Bangladesh in South Africa 2026/27: [series 24200](https://site.api.espn.com/apis/site/v2/sports/cricket/24200/scoreboard), sample event `1525662` dated November 15, 2026.

These series numbers are verified examples, not a permanent shortlist. Refresh discovery and map each series to its recurring competition/tour identity. No universal ESPN cricket endpoint was established: both `/sports/cricket/all/scoreboard` and `/sports/cricket/scoreboard` returned 404. The core league index returned zero while the site dropdown returned 500 entries.

The IPL sample contains `score: "161/5 (18/20 ov, target 156)"`, innings-specific runs/wickets/overs and a string `winner: "true"`. The implementation must preserve structured innings and safe display text. Other cricket formats require separate fixtures; three successful Test series lookups do not validate all international or domestic cricket.

### Additional discovery/reference sources

- [ESPNcricinfo competition selector](https://stats.espncricinfo.com/sachinat20/engine/stats/index.html?class=6%3Bfilter%3Dadvanced%3Bfilter_box%3Dseason%3Bfilter_box%3Dteam%3Bfilter_box%3Dtrophy%3Btype%3Dteam): helps identify recurring competitions such as BBL, PSL, SA20, CPL, MLC and The Hundred. Its trophy IDs must not be assumed interchangeable with scoreboard series IDs.
- [ESPNcricinfo series directory](https://www.espncricinfo.com/series): reference/discovery only; this audit's web request returned 403. Do not depend on scraping it or bypassing its access controls.
- [Sportradar cricket documentation](https://developer.sportradar.com/cricket/reference/cricket-overview): documented authenticated alternative with schedules, tournament/season discovery, summaries and coverage-dependent ball-by-ball data. Credentials/entitlements were not supplied or tested.

## 5. Alternative APIs and official information sources

### Squiggle — AFL alternative

[API documentation](https://api.squiggle.com.au/). Documented examples:

```text
https://api.squiggle.com.au/?q=teams
https://api.squiggle.com.au/?q=games;year=2026
https://api.squiggle.com.au/?q=standings;year=2026
```

Provides basic fixtures, match scores and standings. Use explicit season parameters and follow its identification/request requirements. `standings` represents actual standings; `ladder` is a model prediction and must not be shown as the real table. A separate SSE service exists, but start with the simpler documented REST interface. Documentation reviewed; data endpoints were not exercised in this package. No advanced Champion Data statistics are implied.

### TheSportsDB — metadata and limited schedules

[Documentation](https://www.thesportsdb.com/documentation). Candidate operations:

```text
https://www.thesportsdb.com/api/v1/json/{key}/all_leagues.php
https://www.thesportsdb.com/api/v1/json/{key}/lookupleague.php?id={leagueId}
https://www.thesportsdb.com/api/v1/json/{key}/lookupteam.php?id={teamId}
https://www.thesportsdb.com/api/v1/json/{key}/lookupevent.php?id={eventId}
https://www.thesportsdb.com/api/v1/json/{key}/eventsday.php?d={YYYY-MM-DD}&l={leagueId}
https://www.thesportsdb.com/api/v1/json/{key}/eventsseason.php?id={leagueId}&s={season}
```

Useful for logos, league/team metadata and schedule fallback after identity validation. The current boxing adapter uses league `4445`. The public test key is `123`; free methods are restricted, and generic team search is not broadly available under that key. Premium access supports additional functionality, including live-score methods. Do not infer full event coverage or live scoring from a successful free response. Prefer returned artwork URLs and retain provenance. No new credentials or paid access were used here.

### API-Sports — documented optional sport APIs

- [AFL documentation](https://api-sports.io/documentation/afl/v1)
- [Volleyball documentation](https://api-sports.io/documentation/volleyball/v1)
- [Rugby information](https://api-sports.io/sports/rugby)

These are alternative provider leads, not validated substitutes. Before integration, inspect the provider's current authentication, league/season availability and quota documentation with authorized access. Confirm the exact target competition is covered; a provider supporting volleyball does not establish NCAA volleyball or every professional league.

### CFL and Genius Sports

- [Official CFL statistics](https://stats.cfl.ca/stats/teams)
- [CFL's official data partnership announcement](https://press.cfl.ca/cfl-and-genius-sports-unveil-cutting-edge-official-data-and-technology-ecosystem)
- [Genius developer centre](https://developer.geniussports.com/)
- [Genius statistics API documentation](https://geniussports.atlassian.net/wiki/spaces/BID/pages/3990454841/Statistics%2BAPI)

Use these to establish current coverage and authorized data access. The announcement describes a new official ecosystem launched in 2023; the sampled ESPN CFL response contains a 2022 event. That makes ESPN freshness a required investigation, not proof that a specific commercial integration must be purchased. A current authenticated CFL feed was not tested here.

### NCAA and additional Olympic/meet sports

[NCAA women's volleyball scoreboard](https://www.ncaa.com/scoreboard/volleyball-women/d1) links official schedules, rankings, statistics, brackets and other sports. It is a reference page, not a documented generic JSON API. [NCAA women's gymnastics](https://www.ncaa.org/championship/national-collegiate/womens-gymnastics/) provides championship information. The [NCAA statistics site](https://stats.ncaa.org/) returned 403 during this audit.

For wrestling, gymnastics, swimming/diving and track and field, identify the actual official meet/results source and its authorized machine-readable interface before writing an adapter. NCAA public pages can anchor identity and completeness checks, but do not assert they supply an unrestricted API. Timed/judged meets need different event/result models. Source discovery for these sports remains open; no fabricated endpoints are supplied.

Concrete reference entry points discovered through NCAA's sport navigation:

| Target | Official reference | Status in this audit |
| --- | --- | --- |
| Wrestling | https://www.ncaa.com/sports/wrestling-men | Official linked page; automated fetch showed a JavaScript verification page |
| Gymnastics | https://www.ncaa.com/sports/gymnastics-women | Official linked page; use the NCAA.org championship reference above as well |
| Swimming/diving | https://www.ncaa.com/sports/swimming-men | Official linked page; no machine-readable results feed verified |
| Track and field | https://www.ncaa.com/sports/trackfield-outdoor-men | Official linked page; indoor, outdoor and gender categories need explicit discovery |

These pages are starting points for identifying the result provider, not scraping instructions or confirmation of public API access. Cover both men's and women's competitions where available; do not extrapolate a men's URL into a claimed women's API.

## 6. Information and artwork strategy

| Information | Preferred source | Fallback / restriction |
| --- | --- | --- |
| Team/school identity | Scoreboard team metadata, then validated team catalog | Preserve college school naming; use country/location as school only when competition metadata says it is collegiate |
| Team/event logo and colors | Existing ESPN/organizer response image fields | Locally composed colorful background plus generic sport icon; optional validated TheSportsDB metadata |
| Team records/rankings | Scoreboard records and curated rank; optional standings | Preserve scope and unknown values; do not display sentinel ranks as real rankings |
| Pregame comparison | Cached team context, standings and already-fetched summaries | Omit unavailable fields; do not block initial detail rendering |
| Venue/location | Scoreboard venue or summary gameInfo | Photos only if actually supplied and usable; no inferred venue photo URLs |
| Players/rosters | Summary or team/player endpoint validated for that competition | Athlete names/headshots from scoreboard when available; no N+1 roster fetch for every card |
| Phase/round/championship | Provider season, type, notes, round, series and sub-event metadata | Literal provider labels when canonical interpretation is uncertain |
| Broadcast label | Reported broadcasts and existing event metadata | Labels assist existing search; they are not playable stream URLs |

Record provenance and available image usage terms for any new artwork source. Reuse the existing team/event-logo design rather than making new venue imagery a prerequisite.

## 7. Reproduction and unresolved checks

The JSON probe files contain exact URLs. Re-run a small sample manually or through the project's test harness first; use a bounded client, explicit timeouts and source-aware caching. Do not launch hundreds of detail/roster requests for every directory entry in one live startup path.

Remaining provider checks include current-season completeness, scheduled/live/final transitions, detailed results for competitor-less golf/racing samples, missing team catalogs, richer combat feeds, full cricket competition discovery and every directory-only manifest entry. The initial failed historical batch must not be presented as evidence that those leagues lack ESPN scoreboards—the ordinary scoreboard batch succeeded.
