# Printer and inspection doors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring printer, SPI and AOI close-ups to the accepted mounter sample's level of enclosure and operator-panel detail.

**Architecture:** Reuse the existing Blender pipeline and factor its door/panel construction into shared functions with explicit station layout parameters. Export only printer, SPI and AOI; preserve existing anchor names, virtual-time motion offsets and camera presets. Register their glazing with the existing runtime shadow policy.

**Tech Stack:** Blender Python, glTF 2.0, existing React Three Fiber/TypeScript, Vitest and Playwright; no new dependencies.

**Spec:** Design below, based on the user's approval of the three-station rollout and the accepted mounter sample in `docs/MOUNTER_SAMPLE_REPORT.md`.

## Global Constraints

- This round covers printer, SPI and AOI only. Preserve the four other stations' shipped `.blend`/`.glb` files byte-for-byte.
- Original generic geometry; HMI explicitly marked `SIMULATED`, with static station-specific content and no operational controls.
- Doors remain closed; no AO, material overhaul, backend changes, simulation changes or camera changes.
- Keep required anchor names unique; each GLB <2 MiB and <=40 meshes. Record primitive counts as well.
- Preserve the user's unrelated README change and `docs/architecture.md`.
- Use documented `-- --kind <station>` export commands with `--python-exit-code 1`; never blindly regenerate all assets.

## Design

User-approved direction: carry the mounter's graphite frames, seals, hinges, raised handles, inset display and physical buttons across the three stations, adapting observation-window proportions to their mechanisms. Success is a consistent overview followed by convincing close-ups, rather than three copies of the mounter.

Builder defaults proposed for this rollout:
- Printer: two broad, relatively low observation doors; the stencil/worktable and moving squeegee remain visible behind the glazing. Side-fixed operator panel labeled `PRINT / DEMO` and `SIMULATED`.
- SPI and AOI: one broad observation door with a raised handle and two hinges; a more enclosed upper cover around the camera bridge. Shared exterior vocabulary, with static screen diagrams and `SPI / DEMO` or `AOI / DEMO` labels distinguishing process purpose. No claims of real optical shielding or machine accuracy.
- All three get genuinely hollow upper housings and layered operator panels. Mechanisms move inside the cavity rather than on the front face. Rest depths and beam lengths may be adjusted to accommodate existing travel; runtime motion stays unchanged.
- Reuse alpha-blended smoke glazing with no shadow casting, retaining the mounter's treatment. Mechanical gaps and hardware thickness must read in existing focus presets.

## Review Focus

1. Inspection bridge sweeps +/-0.57 m and can hit a side wall after relocation: validate real mesh bounds at both travel extrema (Tasks 1–2).
2. A different glazing name or suffixed Blender material reintroduces opaque pane shadows: test exported material use and the runtime policy, including full-pipeline naming behavior (Tasks 1–2).
3. Common-helper extraction changes the accepted mounter: compare its construction path and preserve its shipped artifact hashes (Task 1).
4. Windows/HMI appear clear in isolation but disappear in the existing camera: inspect same-camera default/low-quality screenshots and running snapshots (Task 3).
5. A failed new model breaks station selection or simulation: parameterize corrupt-model browser checks for all three stations (Task 3).

## File Map

- `assets/blender/generate_models.py`: shared enclosure/door/panel helpers, kind dispatch, mechanism depth/length adjustments and export geometry validation.
- `assets/blender/{printer,spi,aoi}.blend`, `public/models/{printer,spi,aoi}.glb`: only regenerated artifacts.
- `src/scene/assetMaterials.ts`, `tests/unit/asset-materials.test.ts`: glazing shadow policy.
- `tests/unit/assets.test.ts`: exported node, material usage, anchor and budget contracts.
- `tests/e2e/models.spec.ts`: selection and fallback coverage.
- `docs/PRINTER_INSPECTION_ROLLOUT_REPORT.md`, `docs/screenshots/printer-inspection/`: reproducible evidence and visual acceptance record.

### Task 1: Shared construction and printer

**Interfaces:** retain `build(kind)` and `shell(mats, kind)`. Extract `operator_panel(mats, cabinet, label, material_prefix)` and `vision_door(mats, cabinet, name, center, width, height, glass, handle_side)` from the sample; the mounter adapter supplies its existing dimensions. Produce `detailed_enclosure(mats, cabinet, kind)` for the three new stations. Material names `<kind> safety glass` use lowercase kind; retain `Mounter safety glass` compatibility.

- [x] Record baseline screenshots and SHA-256 hashes for all assets; run `npm test` before changing code. Work on a feature branch, using the established inline execution method.
- [x] Add printer artifact assertions for `Vision_Door_L`, `Vision_Door_R`, `Operator_Panel`, `HMI_Display`, `Emergency_Stop`. Verify door descendants contain primitives referencing the BLEND glazing material with alpha in (0,1). Keep anchor and budget checks. Run `npx vitest run tests/unit/assets.test.ts` and confirm missing-detail failure.
- [x] Add runtime policy tests asserting `printer safety glass` does not cast shadows while opaque materials do. Run `npx vitest run tests/unit/asset-materials.test.ts` and confirm failure before extending the policy.
- [x] Extract reusable builders while retaining the mounter's geometry and names; implement printer enclosure/panel. Relocate printer mechanisms behind its panes. Reuse or replace same-named materials deliberately so full-pipeline exports cannot acquire `.001` glazing names that bypass shadow policy.
- [x] Add export assertions derived from actual enclosure surfaces for moving-part clearance, separated door frames and non-coplanar vents. Check Squeegee +/-0.48 m and Clamp +0.02 m; do not claim these are full physical collision tests.
- [x] Export only printer with the documented Blender command. Run asset/material/anchor tests and `npm test`; verify four protected asset pairs' hashes and inspect printer close-up. Commit scoped files.

### Task 2: SPI and AOI adaptations

**Interfaces:** consumes Task 1 helpers; add `kind` to `camera_parts(mats, kind)` dispatch to select static labels/diagram details, preserving `Camera_Bridge` and `Camera_Head`.

- [x] Parameterize artifact contracts for SPI/AOI with `Vision_Door`, `Operator_Panel`, `HMI_Display`, `Emergency_Stop` and actual glazing primitive usage. Add their material names to policy tests. Run affected tests and observe expected failures.
- [x] Implement one-door inspection variants, shared operator hardware, station-specific HMI diagrams/labels, and hollow housings. Move camera mechanisms behind the pane; shorten the moving bridge if required to clear the side walls at +/-0.57 m. Check the head's -0.05 m vertical travel.
- [x] Validate geometry before export: door/frame clearances, pane depth, roof vents, movement extrema. Export SPI and AOI individually; inspect unique names, file sizes, mesh and primitive counts.
- [x] Run `npx vitest run tests/unit/assets.test.ts tests/unit/asset-materials.test.ts tests/unit/anchors.test.ts` and `npm test`. Verify protected artifacts remain unchanged; commit scoped files.

### Task 3: Presentation verification and acceptance evidence

**Interfaces:** consumes all three exported models at existing URLs and existing camera/selection controls.

- [x] Extend model selection and corrupt-model fallback browser checks to printer, SPI and AOI; assert selection and continued simulation after fallback. These test existing behavior, not a claimed RED-to-GREEN bug fix.
- [x] Capture before/after close-ups for each station at 1600x900 using existing camera presets. Inspect default and low quality, an oblique view and running/paused poses; record window visibility, hardware readability, pane shadows, overlaps and HMI labeling. Fix observed defects before acceptance.
- [x] Compare same-settings rAF callback samples before/after, identifying software rasterizer and measurement limitations. Avoid conclusions about the user's GPU from these numbers.
- [x] Run typecheck, lint, complete unit suite, build and complete E2E suite. Preserve exit codes and report actual summaries; do not relax budgets/timeouts to hide regressions.
- [x] Write the rollout report and commit screenshots plus exact generation commands, artifact measurements and limitations. Update plan checkboxes only for completed steps.
- [x] Obtain one fresh whole-branch review and address important findings. Present the three actual close-ups together for user acceptance; await explicit instruction before merge or PR.

## Self-review

The three assets share one construction path, so tasks are sequential rather than parallel. The shared-helper extraction and runtime glazing changes are scoped to the approved visual outcome. Tests protect artifact contracts; visual inspection remains necessary for readability and perceived depth. Inspection travel is wider than the mounter's and is explicitly covered rather than inheriting mounter bounds assumptions.
