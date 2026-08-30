# Task 8 report: live themes and device previews

Implemented the Home Designer's local-only theme editing and TV/mobile preview surfaces.

- Added Theme controls for presets, colors, typography scale, button treatment, high contrast, and overlay reduction. Presets use one `theme/replace` action, preserving grouped undo; Rows and Theme inheritance remain independent.
- Added scoped preview CSS variables on `.home-preview-device` only, so live preview changes never recolor the admin editor. The mock applies selected contrast/overlay behavior while the editor retains its focus treatment and honors reduced motion.
- Added a debounced preview controller with a 250 ms delay, visible enabled-row scheduling, a 12-row/item bound, cancellation, stale-response suppression, normalized profile/platform/row cache keys, scoped result clearing, and per-row Retry. A failure leaves neighboring successful rows intact.
- Added safe DOM-only card rendering for artwork, metadata, badges, progress, samples, loading skeletons, empty rows, and local error/retry states.
- Replaced the placeholder with shared-order TV and mobile mock renderers. TV has a 16:9 rail/hero layout and landscape/portrait rules; mobile has a phone viewport, top treatment, portrait density, and bottom navigation. Selection remains synchronized with the outline.
- Registered `theme.js` and `preview.js` as embedded, allowlisted assets.

Verification:

- `node --test backend/handlers/admin_assets/home_designer/store_test.mjs backend/handlers/admin_assets/home_designer/preview_test.mjs` — PASS
- `node --test backend/handlers/home_designer_app_test.js` — PASS
- `node --check backend/handlers/admin_assets/home_designer/app.js && node --check backend/handlers/admin_assets/home_designer/theme.js && node --check backend/handlers/admin_assets/home_designer/preview.js` — PASS
- `podman run --rm --network=host -v /home/jeor/Documents/Mediastorm/Backend/mediastorm/.worktrees/home-designer:/src:Z -w /src/backend docker.io/library/golang:1.26.5 go test ./handlers -run 'HomeDesigner' -count=1` — PASS (exit 0)
