# Materials and ambient occlusion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the demo's "white plastic" look by correcting the powder-coat material and adding screen-space ambient occlusion in the recesses and under the machines.

**Architecture:** Retune the shared powder-coat material in the existing Blender pipeline and regenerate all seven GLBs (same geometry). Add one post-processing pass to the existing Canvas, gated by the existing `?quality=low` switch, so weak hardware keeps the v1.5 frame rate. Everything else — simulation, camera, assets, backend — stays untouched.

**Tech Stack:** Blender Python materials, glTF 2.0 PBR, React Three Fiber 9.8, three 0.180, `@react-three/postprocessing` + `n8ao`.

**Spec:** this document plus the user's stated complaint that the line reads as pale plastic, and the earlier decision that the demo machine has frame-rate headroom.

## Global Constraints

- Geometry does not change: the same meshes, anchors and door/covers as `6b16446`. Only material factors change in the GLBs.
- AO must be optional at runtime: `?quality=low` skips it entirely, including the extra render targets.
- No downloaded textures, HDRIs or fonts. Any AO or material variation is computed at runtime or baked from the existing geometry.
- Keep the demo offline: the new packages are bundled; the existing cross-origin test must stay green.
- All telemetry stays labelled SIMULATED; no new UI, no camera or simulation changes.
- Preserve the user's uncommitted README edit and `docs/architecture.md`.
- Record measured frame cost with the software-rasteriser caveat; make no claim about the customer's GPU.

## Review Focus

1. Everything becomes muddy grey and the machines lose their light-value contrast: inspect overview and close-ups; the aim is less plastic, not darker.
2. AO darkens the interior of the glazed enclosures into unreadability: check the mounter/printer/SPI close-ups with AO on.
3. AO pass leaks into the transparent panes or the floor reflection (a classic double-render artifact): inspect a reflective-floor view and the door glazing.
4. The new pass breaks on resize, on a WebGL context without float render targets, or when `?quality=low` is set after load: pin with browser checks and a unit test for the switch.
5. Bundle/runtime regression: no cross-origin requests, no console errors, and a measured software-rasteriser cost recorded rather than assumed.

## File Map

- `assets/blender/generate_models.py`: powder-coat colour/metalness/roughness; no geometry edits.
- `assets/blender/*.blend`, `public/models/*.glb`: all seven regenerated.
- `src/scene/lighting.tsx` or a new `src/scene/AO.tsx`: the AO pass, gated by a pure `aoEnabled(search)` helper.
- `src/scene/Factory.tsx`: mount the pass inside the Canvas.
- `tests/unit/lighting.test.ts` (or `ao.test.ts`): `aoEnabled` cases.
- `tests/unit/assets.test.ts`: material-factor contract for the powder coat.
- `tests/e2e/factory.spec.ts`: overview capture with AO and with `?quality=low`; console/offline guard stays.
- `docs/MATERIALS_AO_REPORT.md`, `docs/screenshots/materials-ao/`: evidence.
- `README.md`, `docs/DECISIONS.md`: document the new pass and the switch.

### Task 1: Powder-coat material

**Interfaces:** `material(name, color, metallic, roughness)` in the generator keeps its signature; only the values passed for `Powder coated warm grey` change.

- [x] Add a unit assertion that the exported powder-coat material's `baseColorFactor` luminance is below 0.5 and `metallicFactor` at or below 0.1, and that its `roughnessFactor` is between 0.5 and 0.7. Run `npx vitest run tests/unit/assets.test.ts` and confirm failure against today's values (0.68/0.72/0.69, metallic 0.27).
- [x] Change the powder coat to `(.46, .49, .47)`, metallic `.04`, roughness `.62`; keep every other material as authored. Regenerate all seven kinds one at a time.
- [x] Re-run the asset suite and confirm the new contract passes; inspect overview and mounter close-up to check the line is less plastic against the white plinth/base contrast, and that the earlier stations' glazing still reads.
- [x] Commit generator plus the seven regenerated artifact pairs.

### Task 2: Ambient occlusion pass

**Interfaces:** add `aoEnabled(search: string): boolean` in `src/scene/ao.ts`, true unless `quality=low`; `src/scene/AO.tsx` default-exports a component rendering the N8AO effect; `Factory.tsx` mounts it inside `<Canvas>` after `<LightRig />`.

- [x] Install `@react-three/postprocessing` and `n8ao`; record the resolved versions and bundle delta.
- [x] Write `aoEnabled` tests first (default true, `?quality=low` false, `?source=remote` true) and watch them fail.
- [x] Implement `aoEnabled` and the `AO` component. Read the query string once at module scope the way `lighting.tsx` does, so the pass cannot flip mid-session. Configure N8AO conservatively: half-resolution, `aoRadius` around 0.6, `distanceFalloff` 0.8, `intensity` 1.5 — tune only by inspection.
- [x] Mount it in `Factory.tsx` and verify by capturing overview, mounter and printer close-ups with AO on and with `?quality=low`; check the reflective floor and the glazing for leaking or doubled shading.
- [x] Extend the e2e guard to assert zero console errors and zero cross-origin requests in both modes, and that the canvas renders in both.
- [x] Run typecheck, lint, full unit suite, build and the full e2e suite; record the bundle-size change and software-rasteriser callback rate for AO on and off.
- [x] If AO makes the enclosures unreadable or the reflection doubles, reduce intensity/resolution before considering any geometry or lighting change.

### Task 3: Evidence and acceptance

- [x] Capture the final overview and one close-up per station family at 1600×900, default and `?quality=low`.
- [x] Write `docs/MATERIALS_AO_REPORT.md` with the settings, measured frame cost, bundle delta, before/after screenshots and limitations.
- [x] Update `README.md` (rendering paragraph) and `docs/DECISIONS.md` (AO pass, switch and material change).
- [x] Run one fresh whole-branch review; fix Critical/Important findings with RED→GREEN tests.
- [x] Present the overview and close-ups for acceptance. Do not merge or push without an explicit instruction.

## Self-review

Two independent visual levers (material and AO) are split so either can be rejected without losing the other. The AO switch reuses the existing quality flag, so no new user-facing control appears. The material contract pins the intended direction rather than an exact colour, and every task ends with an inspection step because the complaint is visual.
