# Visual polish v1.1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build all seven stations from one original Blender pipeline and animate each through its own named anchors, so the customer demo's overview → push-in lands on a machine whose mechanism is visibly working.

**Architecture:** One Blender script emits seven PBR `.blend` sources and GLBs with a fixed anchor naming contract. The runtime loads every station from GLB, captures each anchor's rest pose once, and applies bounded offsets derived from the existing snapshot-driven `machineMotion`. Camera presets become typed data with a mechanism framing, and preset changes ease instead of jumping.

**Tech Stack:** Blender 5.2.3 LTS (Windows build, invoked from WSL), glTF 2.0, React 19 + TypeScript strict, React Three Fiber, Vitest, Playwright, Go 1.26 + Postgres migrations.

**Spec:** the Design section of this document.

## Global Constraints

- Original generic geometry only. No proprietary CAD, downloaded models, logos or brand names. Text on plaques stays generic.
- Units are metres, `export_yup=True`, transforms applied. Seven `.blend` + seven `.glb` files.
- Each GLB stays under 2 MiB and at most 40 meshes; seven machines share one material vocabulary.
- Blender is `/mnt/e/blender-5.2.3-windows-x64/blender.exe` (5.2.3 LTS), run from WSL against `\\wsl.localhost\Ubuntu\home\enming\demo-projects\smt-digital-twin`. The script must stay compatible with Blender 4.2+ APIs.
- The simulation clock stays authoritative: anchor poses and camera easing derive from `scene.simTimeMs` and tween elapsed time. No render-frame delta reaches the business clock.
- Historical acceptance records are evidence: `docs/ACCEPTANCE.md`, `docs/IMPLEMENTATION_BASELINE.md`, `docs/PROJECT_SUMMARY.md`, `docs/W1_GAP_REPORT.md` are not edited.
- Remote (W4) and historical replay views stay static by design in this round; that limitation is documented, not silently changed.

## Review Focus

- **A missing or duplicated anchor name.** `getObjectByName` returns `null` or the wrong node, so a part freezes or the wrong part moves, with no error. Pinned in Task 3 by a node-set assertion and a duplicate-name check.
- **One of the five new GLBs failing to load.** The line must stay interactive with a procedural fallback. Pinned in Task 2 by a forced-404 test on `loader`.
- **Tween interrupted by a drag or a reset.** The camera must not jump, overshoot, or fight `OrbitControls`. Pinned in Task 4 by `tweenProgress` clamping/completion tests and a browser check that drags mid-tween and asserts no console error.
- **Simulation paused or stopped while a station is selected.** Poses must freeze at the same value on every render and not drift with wall time. Pinned in Task 3 with a fixed `simTimeMs`.
- **Backend advertising a model URL whose file is absent.** `/api/assets` must name the seven files that actually ship. Pinned in Task 6.

---

## Design

### Why

Two stations use Blender GLBs and five use a procedural shell, so the close-up exposes a visual seam. The GLB anchor groups (`Gantry_X`, `Placement_Head`, `Feeders`) are exported but never animated; motion is a separate set of boxes pasted over a static shell. Shadow acne stripes the cabinet tops. A customer demo that pushes in does not survive being looked at closely.

### Scope

In: seven kinds on one pipeline with per-kind parts; detail and material vocabulary; anchors animated from the snapshot; eased push-in landing on a mechanism framing; shadow-acne, signal-lens and selection fixes; `/api/assets` and the seed row corrected; tests and docs.

Out: HDRI/IBL, floor reflection, SSAO/bloom, factory set dressing, normal/roughness texture baking, LODs, remote/replay animation.

### Contracts

Anchor node names per GLB (only the listed subset is required):

| kind | anchors |
|---|---|
| all | `Cabinet`, `Conveyor`, `PCB_Anchor`, `Signal_Tower`, `Signal_Lens` |
| printer | `Stencil`, `Squeegee`, `Clamp` |
| mounter | `Gantry_X`, `Gantry_Y`, `Placement_Head`, `Feeders` |
| spi, aoi | `Camera_Bridge`, `Camera_Head` |
| reflow | `Tunnel`, `Belt`, `Zone_Shutter` |
| loader, unloader | `Magazine`, `Lift`, `Pusher` |

Each anchor is a parent group at its authored rest pose. The runtime stores that rest position once and applies offsets only. `Signal_Lens` carries its own material so state colour and emissive can change without touching authored materials.

### Decisions already made

- Mechanism framing puts the moving part at roughly 40% of frame, aimed at the process window, not the whole cabinet.
- Reflow boards stay the existing instanced PCB mesh; the GLB belt moves beneath them.
- Plaques and nameplates are geometry this round; text is generic and unbranded.
- Remote mode is out of scope and is documented as static.

---

## File Structure

| file | responsibility |
|---|---|
| `src/data/anchors.ts` | the anchor contract as data, imported by tests, data layer and scene |
| `src/data/stations.ts` | station layout plus `mechanismCamera` presets |
| `src/scene/anchors.ts` | rest-pose capture, per-kind offsets, bounded application |
| `src/scene/cameraTween.ts` | pure easing and preset interpolation |
| `src/scene/status.ts` | station state → signal colour, shared by both render paths |
| `src/scene/Factory.tsx` | loading, anchor animation wiring, camera rig, lighting |
| `assets/blender/generate_models.py` | one shell builder + seven part sets |
| `backend/internal/api/http.go` | asset registry response |
| `backend/migrations/002_v11_assets.sql` | seed correction |

---

### Task 1: Seven-model Blender pipeline and the anchor contract

**Files:**
- Create: `src/data/anchors.ts`
- Modify: `assets/blender/generate_models.py`
- Regenerate: `assets/blender/{loader,printer,spi,mounter,reflow,aoi,unloader}.blend`, `public/models/{...}.glb`
- Test: `tests/unit/assets.test.ts`

**Interfaces:**
- Produces: `export const commonAnchors: readonly string[]`, `export const stationAnchors: Readonly<Record<StationId, readonly string[]>>`, `export const artifactNames: readonly string[]`.

- [ ] **Step 1: Write the contract data**

```ts
// src/data/anchors.ts
export const artifactNames = ['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader'] as const
export type StationId = (typeof artifactNames)[number]
export const commonAnchors = ['Cabinet', 'Conveyor', 'PCB_Anchor', 'Signal_Tower', 'Signal_Lens'] as const
export const stationAnchors: Readonly<Record<StationId, readonly string[]>> = {
  loader: [...commonAnchors, 'Magazine', 'Lift', 'Pusher'],
  printer: [...commonAnchors, 'Stencil', 'Squeegee', 'Clamp'],
  spi: [...commonAnchors, 'Camera_Bridge', 'Camera_Head'],
  mounter: [...commonAnchors, 'Gantry_X', 'Gantry_Y', 'Placement_Head', 'Feeders'],
  reflow: [...commonAnchors, 'Tunnel', 'Belt', 'Zone_Shutter'],
  aoi: [...commonAnchors, 'Camera_Bridge', 'Camera_Head'],
  unloader: [...commonAnchors, 'Magazine', 'Lift', 'Pusher'],
}
```

- [ ] **Step 2: Write the failing asset test**

Replace `tests/unit/assets.test.ts` with a loop over `artifactNames` that asserts, per model: `.blend` magic `BLENDER`; GLB magic `0x46546c67` and version 2; size under 2 MiB; every name in `stationAnchors[name]` present in `json.nodes`; no duplicate node names; `json.meshes.length` between 6 and 40.

- [ ] **Step 3: Run it to confirm it fails**

Run: `npx vitest run tests/unit/assets.test.ts`
Expected: FAIL — `ENOENT` for `assets/blender/loader.blend`.

- [ ] **Step 4: Rewrite `generate_models.py`**

Structure: `material(name, color, metallic, roughness)`; `group(name, parent)`; `box(...)` with 2-segment bevel; `shell(kind)` for the shared cabinet, plinth, service doors with seams and hinges, control bezel, conveyor bed and rails, signal mast and its own-material lens, and the `PCB_Anchor` group placed at the fixed board line `y = 1.08` so authoring agrees with the instanced PCB mesh; `parts(kind)` adding the per-kind anchors from the contract table — feeder rack and 12 feeder slots for `mounter`, stencil frame, squeegee carriage and clamps for `printer`, camera bridge and head for `spi`/`aoi`, tunnel walls, three zone shutters and a cleated belt for `reflow`, magazine stack, lift platform and pusher for `loader`/`unloader`. Warning plaques and nameplates are flat bevelled boxes in a dedicated material, no text. Material vocabulary: powder-coated warm grey, dark graphite, satin aluminium, smoked glazing, safety amber, plaque white.

- [ ] **Step 5: Generate the seven assets**

```bash
cd /mnt/e/blender-5.2.3-windows-x64 && ./blender.exe -b --factory-startup \
  --python "\\\\wsl.localhost\\Ubuntu\\home\\enming\\demo-projects\\smt-digital-twin\\assets\\blender\\generate_models.py"
```
Expected: seven `Saved as` + `Finished glTF 2.0 export` pairs, no traceback.

- [ ] **Step 6: Run the test to verify it passes**

Run: `npx vitest run tests/unit/assets.test.ts`
Expected: PASS for all seven models.

- [ ] **Step 7: Commit**

```bash
git add src/data/anchors.ts assets/blender public/models tests/unit/assets.test.ts
git commit -m "feat: generate all seven stations from one Blender pipeline"
```

---

### Task 2: Every station renders from its GLB

**Files:**
- Modify: `src/scene/Factory.tsx` (`AssetShell`, `Machine`)
- Test: `tests/e2e/models.spec.ts`

**Interfaces:**
- Consumes: `artifactNames`, `stationAnchors` from Task 1.
- Produces: `AssetShell({ kind, selected })`; the procedural shell remains only on the load-failure path.

- [ ] **Step 1: Write the failing browser test**

In `tests/e2e/models.spec.ts` add a test that loads `/`, collects successful `/models/*.glb` responses, and asserts all seven `artifactNames` were fetched; add a second test that routes `**/models/loader.glb` to an invalid body, clicks the Loader station, and asserts the canvas stays visible and the station detail heading appears.

- [ ] **Step 2: Run it to confirm it fails**

Run: `npx playwright test tests/e2e/models.spec.ts`
Expected: FAIL — only `printer.glb` and `mounter.glb` are requested.

- [ ] **Step 3: Load all seven**

`Machine` chooses `AssetShell` for every station instead of branching on `printer`/`mounter`; `AssetShell` fetches `${import.meta.env.BASE_URL}models/${kind}.glb` as today, keeps a per-instance material/geometry disposal on unmount, and renders `ProceduralShell` only while loading or after a failure. Remove the now-unused per-kind conditional.

- [ ] **Step 4: Run it to verify it passes**

Run: `npx playwright test tests/e2e/models.spec.ts`
Expected: PASS, both tests.

- [ ] **Step 5: Commit**

```bash
git add src/scene/Factory.tsx tests/e2e/models.spec.ts
git commit -m "feat: load all seven station models with procedural fallback"
```

---

### Task 3: Anchor-driven mechanism motion

**Files:**
- Create: `src/scene/anchors.ts`
- Modify: `src/scene/Factory.tsx`, `src/scene/motion.ts` (import shared pose type only)
- Test: `tests/unit/anchors.test.ts`

**Interfaces:**
- Consumes: `machineMotion(machine, simTimeMs) → { cycle: number; sweep: number; lift: number }`; `stationAnchors` from Task 1.
- Produces: `captureAnchors(root: THREE.Object3D, names: readonly string[]): Map<string, THREE.Vector3>`, `anchorOffsets(kind: string, pose: MechanismPose): Record<string, Partial<Record<'x' | 'y' | 'z', number>>>`, `applyAnchorOffsets(root, rest, offsets): void`. `MechanismPose = { cycle: number; sweep: number; lift: number }` is declared in `src/scene/motion.ts` as `machineMotion`'s return type.

- [ ] **Step 1: Write the failing unit test**

Three assertions on a hand-built `THREE.Group` tree: `captureAnchors` records every requested anchor and ignores unknown names; `anchorOffsets` stays inside the authored bounds for `cycle` 0, 0.5, 1 (mounter `Gantry_X |x| ≤ 0.45`, `Placement_Head |y| ≤ 0.12`, reflow `Belt |x| ≤ 0.5`); applying offsets then an empty record restores the captured rest positions exactly.

- [ ] **Step 2: Run it to confirm it fails**

Run: `npx vitest run tests/unit/anchors.test.ts`
Expected: FAIL — cannot resolve `src/scene/anchors`.

- [ ] **Step 3: Implement the module**

Offsets per kind, copied from the constants the current overlay boxes use: mounter `Gantry_X x = sweep * 0.45`, `Placement_Head y = -lift * 0.12`, `Feeders y = lift * 0.01`; printer `Squeegee x = sweep * 0.48`, `Clamp y = lift * 0.02`; spi/aoi `Camera_Bridge x = sweep * 0.57`, `Camera_Head y = -lift * 0.05`; reflow `Belt x = ((cycle * 0.5) % 0.25)`, `Zone_Shutter y = lift * 0.03`; loader/unloader `Lift y = lift * 0.18`, `Pusher z = sweep * 0.1`. Unknown kinds return `{}`. `applyAnchorOffsets` writes `rest + offset` and never accumulates, so repeated frames cannot drift.

- [ ] **Step 4: Run it to verify it passes**

Run: `npx vitest run tests/unit/anchors.test.ts`
Expected: PASS.

- [ ] **Step 5: Wire it into the scene**

`AssetShell` captures anchors once per loaded GLB (keyed by station id), then a `useFrame` callback applies offsets from `machineMotion(machine, scene.simTimeMs)` when a snapshot exists and an empty pose otherwise. Delete the overlay `ProcessAssembly` call for GLB stations; keep `ProcessAssembly` rendering only alongside `ProceduralShell` so the failure path still shows something moving. Remove `ProcessAssembly`'s `printer`/`mounter` branches if they become unreachable.

- [ ] **Step 6: Run the scene tests**

Run: `npm run typecheck && npx playwright test tests/e2e/factory.spec.ts tests/e2e/motion.spec.ts`
Expected: PASS, no console errors.

- [ ] **Step 7: Commit**

```bash
git add src/scene tests/unit/anchors.test.ts
git commit -m "feat: animate mechanism anchors from the simulation snapshot"
```

---

### Task 4: Mechanism presets and an eased push-in

**Files:**
- Modify: `src/data/stations.ts`, `src/scene/Factory.tsx` (`CameraRig`)
- Create: `src/scene/cameraTween.ts`
- Test: `tests/unit/camera-tween.test.ts`, `tests/unit/stations.test.ts`

**Interfaces:**
- Produces: `CameraPreset = { position: Vec3; target: Vec3 }`, `EASE_DURATION_MS = 1200`, `easeInOutCubic(t: number): number`, `tweenProgress(elapsedMs: number, durationMs?: number): { t: number; done: boolean }`, `lerpPreset(from: CameraPreset, to: CameraPreset, t: number): CameraPreset`, and `Station.mechanismCamera: CameraPreset`.

- [ ] **Step 1: Write the failing unit tests**

`easeInOutCubic` clamps outside `[0,1]`, returns 0 at 0, 1 at 1, 0.5 at 0.5, and is monotonic across ten samples. `tweenProgress` returns `done: false` below the duration, `{ t: 1, done: true }` at and beyond it, and never returns a `t` outside `[0,1]` for negative elapsed values. `lerpPreset` returns the endpoints exactly and the midpoint componentwise. `stations.test.ts` asserts all seven stations have a `mechanismCamera`, every mechanism target differs from the overview target, `target[1]` lies in `[0.8, 1.9]`, and every preset is distinct from its station's `camera`.

- [ ] **Step 2: Run them to confirm they fail**

Run: `npx vitest run tests/unit/camera-tween.test.ts tests/unit/stations.test.ts`
Expected: FAIL — module missing and `mechanismCamera` undefined.

- [ ] **Step 3: Implement the tween module and the presets**

`easeInOutCubic(t) = t < 0.5 ? 4t³ : 1 - (-2t + 2)³ / 2` after clamping. `tweenProgress(elapsedMs, durationMs = EASE_DURATION_MS)` returns `t = min(1, max(0, elapsedMs / durationMs))` and `done = elapsedMs >= durationMs`. `lerpPreset` interpolates the six numbers. `stations.ts` gains `mechanismCamera` per station: same station `x` offset, `y ≈ 1.55`, `z ≈ 2.9`, target `[x, 1.55, 0.95]` aiming at the process window, with `spi`/`aoi` slightly higher and `reflow` aimed at the tunnel mouth.

- [ ] **Step 4: Run them to verify they pass**

Run: `npx vitest run tests/unit/camera-tween.test.ts tests/unit/stations.test.ts`
Expected: PASS.

- [ ] **Step 5: Replace the instant jump in `CameraRig`**

On station/reset change, capture the current `camera.position` and `controls.target` as the tween start and the destination preset as the end. Accumulate elapsed time and call `tweenProgress` from a `useFrame` callback; apply `lerpPreset` to both position and target, then `controls.update()`. At `done`, stop and leave `OrbitControls` in charge. Subscribe to the controls' `start` event to cancel an in-flight tween so a drag never fights the animation. Reset tweens to `overviewCamera`. Guard against `NaN` when a preset repeats.

- [ ] **Step 6: Verify in the browser**

Run: `npx playwright test tests/e2e/factory.spec.ts tests/e2e/presentation.spec.ts`
Expected: PASS. Capture `test-results/v11-pushin.png` after selecting `mounter`.

- [ ] **Step 7: Commit**

```bash
git add src/data/stations.ts src/scene/cameraTween.ts src/scene/Factory.tsx tests/unit
git commit -m "feat: ease camera presets onto each station's mechanism"
```

---

### Task 5: Close-up render defects

**Files:**
- Create: `src/scene/status.ts`
- Modify: `src/scene/Factory.tsx`
- Test: `tests/unit/status.test.ts`, extend `tests/e2e/factory.spec.ts`

**Interfaces:**
- Produces: `signalColor(state: string): string` and `signalIntensity(state: string): number`.

- [ ] **Step 1: Write the failing unit test**

`signalColor` maps `FAULT → #e14e4e`, `BLOCKED`/`STARVED → #e1a545`, `RUNNING → #53b879`, everything else (including `UNKNOWN`, `IDLE`, `null`) → `#87928c`; `signalIntensity` returns a value in `[0, 1]` for every state.

- [ ] **Step 2: Run it to confirm it fails**

Run: `npx vitest run tests/unit/status.test.ts`
Expected: FAIL — module missing.

- [ ] **Step 3: Implement `status.ts` and use it in both render paths**

Procedural shell uses `signalColor` for its tower box; GLB stations look up `Signal_Lens` after load, clone its material once, and set `color` plus `emissive`/`emissiveIntensity` from the same functions on state change. Cloned materials are disposed on unmount.

- [ ] **Step 4: Fix the shadow acne and the selection affordance**

On the directional light set `shadow-bias={-0.0004}`, `shadow-normalBias={0.02}`, `shadow-mapSize={[2048, 2048]}`, and tighten `shadow-camera-{left,right,top,bottom}` to ±12/±10. Replace the faint floor plate with a thin `ringGeometry` selection ring in an emissive material at `y = 0.02`.

- [ ] **Step 5: Verify no acne in the close-up**

Run: `npx playwright test tests/e2e/factory.spec.ts` and capture `test-results/v11-mounter.png`. Inspect the cabinet tops for striping; re-run with `shadow-bias={-0.001}` if any remains, and record the chosen value in the task notes.

- [ ] **Step 6: Commit**

```bash
git add src/scene tests/unit/status.test.ts tests/e2e/factory.spec.ts
git commit -m "fix: remove shadow acne and drive signal lenses from station state"
```

---

### Task 6: Backend asset registry tells the truth

**Files:**
- Modify: `backend/internal/api/http.go`, `backend/internal/api/http_test.go`
- Create: `backend/migrations/002_v11_assets.sql`
- Modify: `backend/migrations/migrations.go`, `backend/internal/persistence/migrate.go`, `backend/internal/persistence/migrate_test.go`

**Interfaces:**
- Produces: `GET /api/assets` returning seven entries with `"representation": "glb"` and `"modelURL": "/models/<id>.glb"`.

- [ ] **Step 1: Write the failing Go test**

In `http_test.go`, assert the endpoint returns seven assets, every `Representation` is `"glb"`, and every `ModelURL` names one of the seven files that exist in `public/models` (read via a relative path from the test).

- [ ] **Step 2: Run it to confirm it fails**

Run: `cd backend && go test ./internal/api/ -run Assets`
Expected: FAIL — five entries report `procedural`.

- [ ] **Step 3: Update the registry and the migration**

`assets()` marks all seven as `glb` with `/models/<id>.glb`. Add `002_v11_assets.sql` updating the five procedural rows to `glb` with their URLs, embed it as `migrations.Second`, and extend `Migrate` to apply it when the recorded version is 1, insert version 2, and reject anything above 2. Update `migrate_test.go`'s expected version.

- [ ] **Step 4: Run the backend tests**

Run: `cd backend && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend
git commit -m "feat: report all seven station models from the asset registry"
```

---

### Task 7: Documentation and full verification

**Files:**
- Modify: `README.md`, `docs/DECISIONS.md`

- [ ] **Step 1: Document the decisions**

Add a "## v1.1" block to `docs/DECISIONS.md`: seven models from one pipeline, anchors are real motion nodes now, camera eases in 1.2 s, remote and replay views stay static (known limitation), shadow bias values, and that historical acceptance records were left untouched.

- [ ] **Step 2: Document regeneration and the asset map**

In `README.md`, add the exact Blender command from Task 1 Step 5 (noting the Windows build and the WSL path), the seven-model/anchor table, and the note that `ProceduralShell` is now only a failure fallback.

- [ ] **Step 3: Run every check**

```bash
npm run typecheck
npm run lint
npm test
npm run test:e2e
cd backend && go test ./...
```
Expected: all pass. Paste the real output in the completion report; do not summarise a command that was not run.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/DECISIONS.md
git commit -m "docs: record the v1.1 asset pipeline and its limits"
```

---

## Acceptance

1. Seven GLBs are requested on load; a forced 404 on any one keeps the line interactive.
2. Every model exports its contract anchors; unit tests enforce names, uniqueness, GLB validity and the 2 MiB budget.
3. Selecting each station eases to its mechanism preset and the mechanism moves with the snapshot; pausing freezes it and replaying the same virtual time reproduces the same pose.
4. No shadow acne on cabinet tops in the close-up; signal lenses follow station state on all seven.
5. `/api/assets` reports seven GLBs that exist on disk.
6. `npm run typecheck`, `npm run lint`, `npm test`, `npm run test:e2e` and `cd backend && go test ./...` pass, with real output.
