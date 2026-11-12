# Mounter vision-door sample Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver one inspectable mounter sample with large framed observation doors and a separate side-mounted screen/button panel.

**Architecture:** Extend the existing Blender `build(kind)` pipeline with a mounter-only enclosure and detail assembly. Export the mounter source/GLB only, retain named motion anchors, and use the existing runtime loader and virtual-time animation.

**Tech Stack:** Blender 4.2+ Python APIs (installed Windows Blender executable via WSL), glTF 2.0, existing React Three Fiber runtime, Vitest and Playwright.

**Spec:** Approved design in chat (A, then mounter-first sample); captured in Design below. Visual reference: `.superpowers/brainstorm/438678-1794517143/content/door-panel-study.html` (illustration, not a rendering contract).

## Global Constraints

- One mounter sample only; expansion to other stations waits for visual acceptance.
- Original generic geometry; telemetry and static HMI content visibly labeled `SIMULATED`.
- Preserve all names in `stationAnchors.mounter`, node-name uniqueness, <2 MiB GLB and <=40 meshes. Consolidate static geometry instead of increasing the budget.
- Keep simulation, replay semantics, camera easing, backend and other station assets intact.
- Doors stay closed; buttons and screen are visual details, not operational controls.
- Preserve the user's uncommitted README badge and untracked `docs/architecture.md`.
- No new rendering dependency or AO in this sample. Existing default and `?quality=low` lighting remain available.

## Review Focus

- A translucent pane in front of a solid block still conceals the mechanism: inspect front and oblique close-ups (Task 2).
- Moving head/beam crosses the new frame: check exported bounds at motion extrema and inspect a running cycle (Tasks 1–2).
- Alpha glazing casts an opaque shadow or suffers ordering artifacts: inspect default/low lighting and adjust glazing-only shadow handling if necessary (Task 2).
- Export adds meshes or drops anchors: asset-contract checks must retain existing limits (Task 1).
- Detail reads only in the mockup, not the installed camera: use the existing focus preset and verify screen, hardware and internal motion together (Task 2).

## Design

The user selected A: large smoke-tinted observation windows with narrow graphite frames and a separate side operator panel. Add door gaps, rubber seals, two hinges per door, raised vertical handles, an inset display with a layered bezel, three physical buttons, and a red mushroom emergency-stop on a yellow collar. Display a modest static diagram/status layout with `SIMULATED`, not invented live metrics. Small generic labels may use Blender text converted to geometry; no external fonts or downloaded textures.

The sample retains the mounter's overall footprint and existing conveyor/PCB alignment. Replace its solid upper housing with roof/back/side enclosure pieces and an open front cavity. Two closed doors sit in front of the mechanism; reserve the right fixed strip for the operator panel. Move the mounter mechanism's authored depth into the cavity as necessary, keeping anchor names and existing offsets. Keep the existing feeder count and process behavior.

Builder calls: retain the current cabinet palette; distinguish satin metal hardware, graphite trim, matte seals and mildly transparent smoke glazing. Prefer glTF alpha blending to transmission (no extra refractive render pass); tune alpha by actual browser inspection. Join disconnected static parts by material within their semantic assemblies to meet the current mesh budget. Do not merge across animated anchors or the signal lens.

Success is a convincing existing-camera close-up, not exact reproduction of the SVG. Door construction, screen depth and button shapes must read clearly, with the mechanism visible behind the glass. Acceptance stops at the sample; seven-station rollout is a later decision. The reported 150–180 fps on the user's machine is an observation, not proof of refresh-rate capping or zero GPU cost.

## File Structure

- `assets/blender/generate_models.py`: selective CLI export, mounter-specific enclosure/details, geometry consolidation and export validation.
- `assets/blender/mounter.blend`, `public/models/mounter.glb`: regenerated sample artifacts.
- `tests/unit/assets.test.ts`: retain budgets/anchors; add sample structure/glazing checks.
- `tests/e2e/models.spec.ts`: mounter load/interaction/fallback coverage.
- `src/scene/Factory.tsx`: only if required, glazing-specific shadow handling keyed to `Mounter safety glass`.
- `docs/MOUNTER_SAMPLE_REPORT.md`: exact reproduction commands, screenshots, sizes/mesh counts, measured checks and pending visual acceptance.

### Task 1: Build and export the detailed mounter

**Interfaces:** retain `build(kind)` and existing required anchor nodes. Add `shell(mats, kind)` dispatching only mounter-specific upper enclosure work; `mounter_enclosure(mats, cabinet)` creates the doors and operator assembly. Add CLI `--kind mounter` after Blender's `--`; no argument still exports all kinds.

- [ ] Extend `assets.test.ts` with a mounter-only test asserting unique nodes `Vision_Door_L`, `Vision_Door_R`, `Operator_Panel`, `HMI_Display`, `Emergency_Stop`; assert `Mounter safety glass` uses `alphaMode: 'BLEND'` with alpha strictly between 0 and 1. Existing anchors and budgets remain mandatory.
- [ ] Run `npx vitest run tests/unit/assets.test.ts`; confirm the new sample test fails on missing detail nodes, not a tooling error.
- [ ] Add the selective CLI entry point and implement `mounter_enclosure(mats, cabinet)`. Keep non-mounter shell construction identical. Name detail assemblies as above; use separate glazing material, physical bezel/button profiles, and converted static screen text.
- [ ] Position `mounter_parts(mats)` behind the glazing. Add export-time mounter bounds validation in scene coordinates: at x offsets -0.45 and +0.45, head lift 0 and -0.12, and feeder lift 0 and +0.01, moving parts stay within the free enclosure space and behind the panes. Preserve conveyor height and PCB anchor.
- [ ] Consolidate static geometry within assemblies; export only mounter using `/mnt/e/blender-5.2.3-windows-x64/blender.exe -b --factory-startup --python <Windows-path-to-script> -- --kind mounter`. Resolve the Windows path with `wslpath -w`; record the actual successful command in the report.
- [ ] Run `npx vitest run tests/unit/assets.test.ts tests/unit/anchors.test.ts`; record pass/fail, GLB bytes and mesh count. Confirm hashes for the other six source/GLB pairs match their pre-export values.
- [ ] Review the diff and commit only generator, sample artifacts and asset tests on an isolated sample branch.

### Task 2: Verify the sample in the existing presentation

**Interfaces:** consumes Task 1's mounter GLB; uses existing `Pick & Place`, `Start simulation`, pause and reset controls. No new customer-facing UI.

- [ ] Extend `models.spec.ts` to check mounter load and selection, and parameterize the corrupt-model fallback test to include mounter. Verify the scene remains interactive and simulation count progresses under fallback.
- [ ] Capture same-camera before/after mounter screenshots at 1600×900, plus final overview and oblique close-up. Test default lighting and `?quality=low`. Inspect door gaps, hardware, HMI label, glazing transparency and mechanism clearance during a cycle; record findings rather than treating a screenshot capture as visual proof.
- [ ] If glass produces opaque shadows, disable castShadow only for meshes using `Mounter safety glass` in `Factory.tsx`; preserve opaque-part shadows. Repeat the affected visual check.
- [ ] Measure identical-duration rAF callback samples before/after with the same viewport/browser settings; report them as callback rate, with software-renderer limitations. Do not infer target-machine cost from these samples.
- [ ] Run `npm run typecheck`, `npm run lint`, `npm test`, `npm run build`, and `npm run test:e2e`. Capture actual exit codes and full summaries; do not pipe errors away. Investigate failures rather than increasing timeouts.
- [ ] Write `docs/MOUNTER_SAMPLE_REPORT.md` with commands, measured results and screenshots, including static-HMI and closed-door limitations. Confirm only approved paths changed and commit the verified sample.
- [ ] Present the actual sample close-up for user acceptance. Stop before other stations, merge or PR unless requested.

## Plan Self-review

Both tasks cover the approved scope. Artifact tests protect the existing asset contract; export bounds checks and manual moving-camera checks cover enclosure clearance. Visual inspection is explicitly required for transparency and readability, which node-name tests cannot establish. Existing pipeline and runtime are reused without a broad refactor.
