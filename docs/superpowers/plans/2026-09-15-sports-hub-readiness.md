# Sports Hub backend readiness plan

The coordinated plan and product decisions live in the frontend repository to prevent the two PRs from diverging:

- [Implementation plan](https://github.com/Jeor/mediastorm-frontend/blob/sports-hub-improvements-20260915/docs/superpowers/plans/2026-09-15-sports-hub-readiness.md)
- [Scope and acceptance decisions](https://github.com/Jeor/mediastorm-frontend/blob/sports-hub-improvements-20260915/docs/superpowers/specs/2026-09-15-sports-hub-readiness.md)
- Frontend draft: https://github.com/Jeor/mediastorm-frontend/pull/6
- Backend draft: https://github.com/Jeor/mediastorm/pull/8

Read both documents before execution. Use the existing `sports-hub-improvements-20260915` branches, execute inline, and keep the PRs in draft.

## Backend sequence

- [ ] Task 1: integrate required-filter Stremio catalog support and tests from the local publishing checkout.
- [ ] Task 2: support exact addon stream-option identity through discovery and playback as required by the frontend.
- [ ] Task 3: normalize reported quality and rank equally credible candidates without media probing.
- [ ] Task 4: validate missed-stream fixtures and add a conservative, manual-only fuzzy fallback.
- [ ] Task 5: support explicit pre-game discovery and available provider information; preserve profile/admin access filters.
- [ ] Tasks 7–8: complete paired verification, preserve existing profile/availability behavior, and prepare the final upstream diff.

The initial pre-game window is 15 minutes; game-information refresh is at most every 60 seconds while active. Opening details must not automatically search IPTV/addon streams. A new master Sports on/off switch is outside the tracked scope.

Commit and push each verified task to this draft branch and update its checklist. Upstream submission waits for a user request after the final review.
