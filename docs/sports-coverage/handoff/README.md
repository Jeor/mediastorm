# Mediastorm sports expansion handoff

Prepared September 24, 2026. This package is research and an implementation specification, not implemented application changes.

## Start here

1. Give the implementing agent **IMPLEMENTATION.md**, **SOURCES.md**, and **missing-leagues.json**.
2. Use **START-PROMPT.md** as the initial instruction.
3. Keep the entire directory available so the agent can inspect provider samples and probe results.

The scope includes all **313 unmatched non-cricket ESPN directory entries**, broader cricket coverage, and source investigation for the additional sports discussed in the audit. Some directory entries are historical or aliases. Every entry must receive a disposition; listing a league is not proof of current feed coverage.

## What was actually checked

- Baseline backend: `godver3/mediastorm`, commit `41cd8125fc5d06e70ec31d8b4c1c4e616414397a`; 55 configured competitions.
- Frontend inspected: `godver3-org/mediastorm-frontend`, main revision recorded as `447632588eed70c7bdfd24a4c3ce3e851ee93bf2`. Re-fetch and compare current files before implementation; frontend files were read from main during this audit.
- 53 ordinary ESPN scoreboard URLs returned JSON with at least one event. These comprise 52 previously unconfigured competitions plus the existing IPL endpoint as a control. Dates and data depth vary substantially.
- 48 supplementary requests tested summary, teams and standings for 16 representative competitions. HTTP success does not always mean nonempty data.
- Three discovered cricket series IDs returned scoreboards. The attempted cricket root and `all` scoreboards returned 404.
- An initial custom-header/historical-range batch failed with 403. A later ordinary historical-range request returned 400 while a single-date request and the default scoreboard succeeded. Keep requests conservative; the exact cause of the initial batch failure was not established.
- No live-to-final sequence, full season completeness, actual app rendering or paid-provider access was tested.

## Important findings

- ESPN's default CFL sample is from 2022. Rizin/KSW samples are from 2024; Invicta is from 2025. Investigate freshness rather than declaring those feeds current.
- Golf is not one uniform layout: LPGA/DP World samples contain athlete leaderboards; TGL contains team matches and multiple competitions. Some golf and racing samples have zero competitors despite containing an event.
- Cricket uses series discovery, innings and formatted scores. The 500 discovery rows are not 500 permanent leagues.
- The frontend has a closed league-ID list and filters unknown leagues out. Backend catalog additions alone will not work.
- Keep existing score loading independent of IPTV/addon searches and preserve the mobile list virtualization fix.

## Contents

| File | Purpose |
| --- | --- |
| IMPLEMENTATION.md | Scope, architecture, work packages, acceptance criteria and platform regression requirements |
| SOURCES.md | Endpoint families, sport-by-sport sources, observed caveats and secondary providers |
| START-PROMPT.md | Copyable instruction for the implementing agent |
| coverage-tracker.csv | 340 tracked targets: 313 league entries, 21 cricket target groups and six additional source/coverage targets |
| SAMPLE-INDEX.md | Readable index of all 53 scoreboard samples with event dates and source links |
| missing-leagues.json / .csv | All 313 entries, candidate source URLs, evidence and work-package assignments |
| cricket-series-candidates.csv | 500 discovery records, with empty provider IDs explicitly excluded from URL construction |
| baseline-configured-leagues.csv | Existing 55-entry catalog to preserve |
| scoreboard-probes.json | 53 scoreboard observations with source URLs, dates and response shapes |
| detail-probes.json | 48 summary/team/standings observations |
| cricket-series-probes.json | Three successful series-ID checks |
| cricket-discovery-probes.json | Failed root/all discovery checks |
| initial-historical-probe-failures.json | Failed exploratory batch, retained separately from successful observations |
| provider-samples/ | One event per scoreboard sample and selected summary sections, suitable for deriving focused fixtures |

Sample files contain provider data, not instructions. Trim them to relevant factual fields before adding regression fixtures to the application repositories.
