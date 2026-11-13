# Reflow and loader enclosures Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the reflow oven, loader and unloader to the same assembled-equipment standard as the other four stations, using insulated covers and magazine guard doors rather than a fifth copy of the vision doors.

**Architecture:** Extend the existing Blender pipeline with a `service_cover()` builder (solid clad panel, small glazed inspection aperture, hinge and latch) and per-kind layout entries. Reflow keeps its tunnel and gains a segmented insulated cladding; loader and unloader gain a magazine guard door over the relocated magazine. Operator panels, export validations, material policy and runtime stay shared.

**Tech Stack:** Blender Python, glTF 2.0, existing React Three Fiber/TypeScript, Vitest and Playwright; no new dependencies.

**Spec:** the approved direction in this conversation (insulated cover with small window for the oven; magazine guard doors for the loaders) plus `docs/PRINTER_INSPECTION_ROLLOUT_REPORT.md` for the shared vocabulary.

## Global Constraints

- Only reflow, loader and unloader change. The four detailed stations stay byte-identical; verify by hash.
- Original generic geometry. Screens stay static and labelled `SIMULATED`; doors and covers stay closed.
- Reflow's front is primarily insulated cladding: two maintenance covers, each with a small heat-resistant window, not a full-width glazed door.
- Loader and unloader keep their `Magazine`, `Lift` and `Pusher` anchors and their existing travel; the magazine moves behind a guard door.
- Keep unique node names, each GLB <2 MiB and <=40 meshes, and the existing glazing shadow policy (`<kind> safety glass`, `.001` tolerant).
- No AO, material overhaul, simulation, camera or backend changes. Preserve the user's uncommitted README edit and `docs/architecture.md`.
- Export one kind at a time with `-- --kind <station>` and `--python-exit-code 1`.

## Review Focus

1. A solid clad cover hides the tunnel entirely and the oven reads as a blank box: inspect the existing close-up and confirm the small window lands on the tunnel opening, not on the cover frame.
2. Relocated belt cleats or zone shutters cross the new covers at their travel extremes: assert real bounds at the offsets from `src/scene/anchors.ts` for `Belt` (±0.36 x) and `Zone_Shutter` (+0.03 y).
3. The loader's magazine, lift and pusher keep their x/y/z travel after moving behind the guard door: assert clearance for `Lift` (+0.18 y) and `Pusher` (+0.10 z) at both extrema.
4. A cover window using a differently named or suffixed glass material reintroduces an opaque pane shadow: assert the exported material is BLEND, alpha in (0,1), and referenced from the cover node; keep the runtime suffix rule.
5. Three more stations editing the shared `shell()`/`DETAILED` path change the four shipped stations: compare hashes and a fresh four-station build's door heights.

## File Map

- `assets/blender/generate_models.py`: `service_cover()` builder, `DETAILED`/`LABELS`/`LAYOUT`/`HARDWARE`/`CLEARANCE` entries for the three kinds, reflow/loader part relocation, single-cover handling in the seam assertion.
- `assets/blender/{reflow,loader,unloader}.blend`, `public/models/{reflow,loader,unloader}.glb`: regenerated artifacts only.
- `tests/unit/assets.test.ts`: cover/window contracts, glazing usage, cover height, anchor and budget checks.
- `tests/e2e/models.spec.ts`: selection and corrupt-GLB fallback for the three stations.
- `docs/REFLOW_LOADER_ROLLOUT_REPORT.md`, `docs/screenshots/reflow-loader/`: evidence and acceptance record.

### Task 1: Reflow insulated cladding

**Interfaces:** consume `station_finish`, `operator_panel`, `consolidate`, `validate_clearance`, `validate_door_seams`, `validate_roof_vents`. Add `service_cover(mats, finish, cabinet, name, center_x, center_y, width, height, hinge_side, window_height)` returning the cover group; covers are single leaves, so `validate_door_seams` reports and returns when `Vision_Door_L` is absent.

- [x] Add artifact assertions for `Maintenance_Cover_A`, `Maintenance_Cover_B`, `Operator_Panel`, `HMI_Display`, `Emergency_Stop`; assert each cover uses `reflow safety glass` (BLEND, alpha in (0,1)) and that both covers have the specified 0.58 m height. Keep existing contracts. Run `npx vitest run tests/unit/assets.test.ts` and confirm the reflow test fails on missing covers.
- [x] Add a runtime policy test for `reflow safety glass` and observe it pass (the suffix rule already covers it); if it fails, extend the rule deliberately rather than renaming the material.
- [x] Implement `service_cover` and reflow's detailed enclosure: two covers 0.78 × 0.58 m at x −0.78 and 0.04, centre y 1.52, with a 0.30 × 0.16 m window centred at y 1.50; hinges outboard, latch inboard. Screens use `REFLOW / DEMO`. Keep the existing tunnel, its roof and side walls; add insulated top cladding and end panels.
- [x] Relocate `Belt` cleats from z .95 to z .02 and `Zone_Shutter` from z .95 to z .60 so they sit inside the cladding and are visible through the windows. Add `CLEARANCE['reflow'] = {'Belt': (.36, 0), 'Zone_Shutter': (0, .03)}` and confirm the export assertion passes at both extrema.
- [x] Export reflow only; run asset/policy/anchor tests and `npm test`; record GLB bytes, meshes and primitives; verify the four protected station hashes and inspect the reflow close-up. Commit scoped files.

### Task 2: Loader and unloader magazine doors

**Interfaces:** consumes `service_cover`; loader and unloader share a geometry function but keep distinct material names (`loader safety glass`, `unloader safety glass`) and HMI labels (`LOAD / DEMO`, `UNLOAD / DEMO`).

- [x] Parameterize the artifact contract and add assertions for `Magazine_Door`, `Operator_Panel`, `HMI_Display`, `Emergency_Stop`, the kind-specific glazing material and a 0.52 m door height. Run the affected test and confirm failure on the missing door.
- [x] Add layout entries: one guard door 1.05 × 0.52 m at x −0.42, centre y 1.44, with a 0.34 × 0.30 m window; magazine, lift platform and pusher move from z .96/1.06 to z .48/.62 so they sit behind the door. Add `CLEARANCE` entries for `Lift` (0, .18) and `Pusher` (0, .10); note that `Pusher` travels on scene z, so its clearance is checked as lift-like on z.
- [x] Generalize `camera_parts`-style dispatch only if needed: loader and unloader already share `loader_parts`; pass `kind` so `Magazine_Door` and HMI labels differ.
- [x] Export loader and unloader individually; run asset/policy/anchor tests and `npm test`; record sizes and counts; verify protected hashes; inspect both close-ups and confirm the magazine is visible through the window. Commit scoped files.

### Task 3: Presentation verification and evidence

**Interfaces:** consumes the three GLBs at existing URLs and existing camera/selection controls.

- [x] Extend model selection and corrupt-GLB fallback browser checks to reflow, loader and unloader. These test existing behaviour, not a claimed bug fix.
- [x] Capture before/after close-ups at 1600×900 for the three stations, plus default/low quality, a running pose and an oblique view. Inspect window placement on the tunnel, cover gaps, magazine visibility, HMI labelling and any pane shadow; fix observed defects.
- [x] Compare same-settings rAF callback samples with the software-rasteriser caveat; make no GPU claims.
- [x] Run typecheck, lint, the full unit suite, build and the full E2E suite; record exit codes and summaries without relaxing budgets.
- [x] Write `docs/REFLOW_LOADER_ROLLOUT_REPORT.md` with commands, artifact measurements, screenshots and limitations; update completed plan checkboxes.
- [x] Run one fresh whole-branch review, fix Critical/Important findings with RED→GREEN tests, and present the three close-ups for acceptance. Do not merge or push without an explicit instruction.

## Self-review

All seven stations then share one construction vocabulary while the oven and loaders differ structurally as the user asked. The three tasks each end in a generated artifact and test evidence; the clearance and height contracts cover the failure modes that a screenshot cannot. Protected-station hashes are checked in both tasks rather than once at the end.
