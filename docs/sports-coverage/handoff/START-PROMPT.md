# Implementation prompt

Implement the Mediastorm sports coverage expansion described in the attached handoff.

Read IMPLEMENTATION.md, SOURCES.md, missing-leagues.json, and the evidence files first. Inspect the current Jeor backend and frontend repositories and their applicable repository instructions. Compare with the recorded upstream baselines before editing; preserve existing uncommitted work and previously completed sports fixes. Work in Jeor's forks and do not submit or push changes upstream until explicitly requested.

The objective is complete, usable coverage of the missing sports and leagues wherever a verified source exists—not just a larger dropdown. Process every entry in the manifest, implement active supported entries, resolve aliases and archival entries explicitly, and record genuine source/access blockers with evidence. Also implement cricket competition/series discovery and investigate the additional sports listed in work package W6. Do not silently stop after the first implementation wave or claim all coverage is complete while unresolved targets remain.

Use existing ESPN and organizer integrations first. Do not introduce a Nuvio dependency. Paid providers in SOURCES.md are alternatives requiring separately authorized credentials; do not purchase access or claim those integrations work without testing them.

Implement the backend catalog, score normalization, schedules, optional details/standings, preferences and team identities together with frontend filtering, cards, details and playback navigation. Preserve college school names, team/event logos on colorful backgrounds, spoiler mode, progressive loading, alternative-stream visibility, TV focus order and native player controls. Keep metadata work independent of stream discovery. Preserve or integrate the mobile Sports Hub virtualization change.

Maintain a coverage tracker covering all 313 manifest entries, named cricket expansion targets and W6 sports. Track endpoint evidence, current-season coverage, supported capabilities, platform verification and remaining blockers for each target. Use focused backend HTTP/service tests and frontend behavior tests with captured provider fixtures. Exercise all enabled sports with a large event set on iOS, Android, web and TV paths; clearly distinguish automated tests from actual device tests.

Follow the work packages and acceptance criteria in IMPLEMENTATION.md. Continue through every feasible work package, keeping changes reviewable as a backend PR and a frontend PR in Jeor's repositories. Finish with the actual coverage achieved, remaining source blockers, test/build results, deployment requirements and the exact changes ready for review. Do not expose unavailable leagues as fully supported merely because their URLs return HTTP 200.
