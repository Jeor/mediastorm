# CFL public API adapter

The official stats.cfl.ca site's public API configuration and client route identify `https://api.stats.cfl.ca/fixtures/{year}?view=summary`. Removing the summary projection returns scores, status, broadcasts and team IDs with HTTP 200 and no credentials. The 2026 sample contains 95 fixtures: nine preseason, 81 regular season, two semifinal and three final fixtures. Sixty-three have explicit Finished status (one nested-quoted); 32 omit status. Missing status is not replaced with a final or live inference based on time or scores.

The adapter keeps canonical catalog identity `espn:football:cfl` while using provider `cfl`. Event and team IDs use the `cfl:` namespace. It returns generic football matchups, not NFL field/down diagrams. It preserves known UTC start timestamps, reported score zeros, literal unknown/postponed status labels, playoff TBD participants, broadcast labels, and source-provided phase groups. `finals` is not automatically interpreted as a championship game.

Team metadata uses the same shared bounded HTTP client as fixtures and has an independent two-second deadline. A missing team endpoint leaves fixture IDs and scores intact with team-ID placeholders. Root's shared client caches `/teams` for 24 hours. Logos accept only HTTPS URLs returned on `content.cfl.ca`; inline SVG and arbitrary hosts are omitted. No provider references or Genius IDs are fetched.

Root integration must route the CFL descriptor to `fetchCFLScoreboardDate`, set provider `cfl`, and advertise schedule/final-result only. Current live state transitions, period scoring and detail/standings integration remain unvalidated or unimplemented. The adapter returns adjacent UTC days for the existing final local-day filter. Upcoming request caching should be approximately 15 minutes; metadata refresh must not enter the score-critical path repeatedly.

Tests use compact captured fixture and team samples and a local fake provider. All sports service tests passed after this module (`go test ./services/sports -count=1`). No UI/device verification is claimed.
