# Coverage implementation graph

Approved specification: handoff/IMPLEMENTATION.md. All 340 tracker rows remain in scope.

- W0-baseline: preserve previous progressive search fixes, compare recorded upstream revisions, confirm mobile SectionList virtualization. Complete before implementation.
- W0-evidence: independently revalidate all 313 candidates, cricket discovery, and W6 references; record exact endpoints, dates, response evidence and conservative source dispositions. No activation from HTTP success alone.
- W1-backend: additive runtime descriptor/capability schema, catalog activation gates, provider scheduler, public API coverage. Depends on W0-baseline; activation depends on W0-evidence.
- W1-frontend: runtime catalog validation, preserve existing IDs/overrides, new sport families, filters/preferences, score strings and safe details. Depends on agreed W1 contract below.
- W2-W5-backend: adapters, normalization and optional enrichment with captured fixtures; all active candidates plus cricket discovery. Depends on W1-backend and W0-evidence.
- W2-W5-frontend: family presentation/navigation regression tests. Depends on W1-frontend and adapter contracts.
- W6: investigate source gaps; evidence-backed blockers remain unresolved, never represented as supported.
- W7: integration review, per-target tracker, builds/platform checks and fork-only PR preparation. Depends on all feasible preceding work.

## Additive descriptor contract

Existing /sports/leagues fields remain: id,name,sport,category,eventKind,supportsTeams,enabled.
Add provider (string), providerSport (string), providerLeague (string), applicationSport (string), college (bool), adapter (string), implementationStatus (validated|limited|candidate|archival|alias|blocked), capabilities (string[]), aliases (string[] optional), coverageNote (string optional).
New IDs use espn:{providerSport}:{slug}. Existing canonical IDs are preserved. Frontend only accepts descriptors with validated/limited status and schedule capability; missing additive fields on known legacy IDs retain compatibility. Unknown payloads without validated descriptors are rejected.
Application sport mappings: football->american-football, hockey->ice-hockey, rugby->rugby-union, racing->motorsport; college-softball/lls->softball; otherwise provider sport slug. Generic team sports never receive NFL/MLB-specific diagrams unless genuinely applicable.
