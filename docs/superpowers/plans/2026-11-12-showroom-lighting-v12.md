# Showroom lighting v1.2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the overview → close-up read as a lit showroom production line instead of flat-lit CAD: an authored in-scene environment map, a three-point rig, a reflective epoxy floor, and a backdrop that does not flatten the machines.

**Architecture:** Lighting becomes its own scene module (`src/scene/lighting.tsx`) built from drei lightformers baked into an environment map at load, with no downloaded HDRI. The floor becomes a blurred planar reflector, the CAD grid disappears, and the canvas goes transparent over a CSS gradient so the backdrop costs nothing per frame.

**Tech Stack:** React 19 + TypeScript strict, React Three Fiber, `@react-three/drei` 10 (`Environment`, `Lightformer`, `MeshReflectorMaterial`), three 0.180, Vitest, Playwright.

**Spec:** the Design section of this document.

## Global Constraints

- No downloaded HDRIs, textures or models: the environment is authored in code and original. The demo must still work with no network access.
- Keep the v1.1 pipeline intact: seven GLBs, anchor motion, eased push-in, `/api/assets` and schema version 2 are not touched by this round.
- Poses and camera easing keep deriving from virtual time; lighting must not add a render-frame leak into the business clock.
- `prefers-reduced-motion` concerns, post-processing, factory set dressing (fences, ceiling trusses, cable trays), texture baking and LODs stay out.

## Review Focus

- **A reflection pass that halves the frame rate on a customer laptop.** Planar reflection is the one new per-frame cost; measure and record frame time before/after, and keep the reflector's resolution and blur bounded.
- **Machines turning to mirrors or to mud.** With an environment map, `metalness`/`roughness` values that looked fine under flat ambient can read as chrome or as grey plastic. Check both the overview and the mechanism close-up.
- **The backdrop eating the silhouette.** A bright gradient behind near-white machines erases their edges; the floor and backdrop must separate, not blend.
- **Environment build failing or being re-rendered every frame.** `frames={1}` must actually bake once, and a WebGL context without float render targets must still start.
- **New network requests.** Any accidental CDN HDRI would pass visually and break the offline promise; the browser test pins same-origin only.

---

## Design

### Why

The v1.1 machines are the right shapes with the right state, but the lighting is one ambient light at 1.25 plus one directional at 2.0. Everything reads as white plastic, metal has nothing to reflect, shadows are the only depth cue, and the background is a flat `#d9ded9` band with a CAD grid drawn on it. For a customer demo that pushes in, this is the remaining reason the line looks like a diagram rather than equipment.

### Scope

In: authored environment map with three softboxes; key/fill/rim rig with the key shadowing; exposure tune; blurred planar floor reflection; grid removed; CSS gradient backdrop with matching fog; a measured frame-time comparison; screenshots.

Out: post-processing (SSAO, bloom, DOF), set dressing, texture baking, material re-authoring in Blender, camera changes, backend changes.

### Decisions

- Environment is built from drei `Lightformer` rectangles inside `<Environment frames={1} resolution={256}>` — one bake, held as a PMREM. No asset file, no CDN.
- Floor uses `MeshReflectorMaterial` at `resolution={512}`, mirror strength low and blur high, so it reads as polished epoxy rather than a mirror.
- The backdrop is CSS behind a transparent canvas; the scene keeps fog tuned to the gradient's mid-tone so distance still reads.
- Tone mapping stays ACES; exposure becomes a named constant so it is one number to tune, not a magic value in JSX.

### Acceptance

1. Overview shows separated silhouettes against the backdrop, visible reflections in the floor, and no grid.
2. Close-up keeps cabinet edges readable, glass shows a sheen, and no shadow acne returns.
3. The page makes zero cross-origin requests and logs no console errors.
4. Frame time is measured before and after and reported; the reflector settings are adjusted if the overview drops below roughly half the v1.1 rate on this machine, and the measurement's environment limits are stated.
5. `npm run typecheck`, `npm run lint`, `npm test`, `npm run test:e2e`, `cd backend && go test ./...` all pass, with real output.

---

## File Structure

| file | responsibility |
|---|---|
| `src/scene/lighting.tsx` | environment bake, light rig, exposure constant |
| `src/scene/Floor.tsx` | reflective floor + grid removal owns only the ground plane |
| `src/scene/Factory.tsx` | canvas flags, composition of rig/floor, fog |
| `src/styles/main.css` | `.viewport` backdrop gradient |
| `tests/e2e/factory.spec.ts` | no console errors, no cross-origin requests, screenshot |
| `README.md`, `docs/DECISIONS.md` | v1.2 record and any tuning trade-off |

---

### Task 1: Reflective floor and no grid

**Files:**
- Create: `src/scene/Floor.tsx`
- Modify: `src/scene/Factory.tsx`
- Test: `tests/e2e/factory.spec.ts`

**Interfaces:**
- Produces: `Floor()` rendering a 100 m plane at `y = 0` with `receiveShadow`, and the removal of `gridHelper` and the 100 m `meshStandardMaterial` plane from `Factory`.

- [ ] **Step 1: Write the failing browser test**

In `factory.spec.ts`, add a test that loads `/`, waits for the canvas, asserts no console errors, and asserts every request the page made is same-origin (`new URL(url).origin === location.origin`), capturing `test-results/v12-floor.png`.

- [ ] **Step 2: Run it to confirm it passes for the wrong reason**

Run: `npx playwright test tests/e2e/factory.spec.ts -g "same-origin"`
Expected: PASS — this is a guard, not a driver. The floor change is visual; the guard must be in place before the scene changes so a regression is attributable.

- [ ] **Step 3: Build `Floor.tsx`**

A `mesh` rotated `-π/2` with `planeGeometry args=[100,100]`, `receiveShadow`, and `MeshReflectorMaterial` with `resolution={512}`, `blur={[320, 80]}`, `mixBlur={0.9}`, `mixStrength={6}`, `roughness={0.9}`, `depthScale={1.1}`, `minDepthThreshold={0.4}`, `maxDepthThreshold={1.5}`, `color="#c6cec7"`, `metalness={0}`, `mirror={0.35}`. Replace the plains plane and delete `<gridHelper>` in `Factory`.

- [ ] **Step 4: Verify the frame still renders**

Run: `npx playwright test tests/e2e/factory.spec.ts`
Expected: PASS, all tests, no console errors. Read the screenshot: reflections under the machines, no grid lines, no z-fighting between floor and plinth.

- [ ] **Step 5: Measure frame time**

Record average `requestAnimationFrame` intervals over 3 s at the overview with a throwaway Playwright script; write the number in the commit body. If the overview is slower than roughly half the v1.1 rate, first drop `resolution` to 256 and re-measure.

- [ ] **Step 6: Commit**

```bash
git add src/scene/Floor.tsx src/scene/Factory.tsx tests/e2e/factory.spec.ts
git commit -m "feat: replace the grid floor with a blurred planar reflection"
```

---

### Task 2: Authored environment and three-point rig

**Files:**
- Create: `src/scene/lighting.tsx`
- Modify: `src/scene/Factory.tsx`
- Test: `tests/e2e/factory.spec.ts`

**Interfaces:**
- Produces: `EXPOSURE = 0.95`, `LightRig()` rendering the baked environment plus key/fill/rim, and the removal of `ambientLight` and the ad-hoc `directionalLight` from `Factory`.

- [ ] **Step 1: Write the guard**

Extend the Task 1 test to also assert the page requests nothing outside `localhost` and still logs no console errors once the rig is in.

- [ ] **Step 2: Build `lighting.tsx`**

```tsx
export const EXPOSURE = 0.95
export function LightRig() {
  return <>
    <Environment resolution={256} frames={1}>
      <color attach="background" args={['#2a3230']} />
      {/* overhead softbox, front fill, rear rim */}
      <Lightformer form="rect" intensity={2.4} position={[0, 10, 0]} scale={[14, 6, 1]} rotation={[Math.PI / 2, 0, 0]} color="#f2f6ef" />
      <Lightformer form="rect" intensity={0.9} position={[9, 4, 8]} scale={[8, 8, 1]} rotation={[0, -Math.PI / 4, 0]} color="#e8efe9" />
      <Lightformer form="rect" intensity={1.2} position={[-8, 5, -9]} scale={[10, 6, 1]} rotation={[0, Math.PI / 5, 0]} color="#dfe8e2" />
    </Environment>
    <ambientLight intensity={0.22} />
    <directionalLight position={[6, 12, 8]} intensity={1.9} castShadow shadow-mapSize={[2048, 2048]} shadow-bias={-0.0004} shadow-normalBias={0.02} shadow-camera-left={-12} shadow-camera-right={12} shadow-camera-top={10} shadow-camera-bottom={-10} />
    <directionalLight position={[-7, 5, 6]} intensity={0.35} color="#d9e6da" />
    <directionalLight position={[0, 6, -10]} intensity={0.6} color="#eef3ea" />
  </>
}
```

- [ ] **Step 3: Wire it in and set exposure**

`Factory` renders `<LightRig />` instead of `ambientLight` + `directionalLight`, and the `Canvas` takes `gl={{ alpha: true, toneMappingExposure: EXPOSURE }}`. Remove the `<color attach="background">`.

- [ ] **Step 4: Verify**

Run: `npx playwright test tests/e2e/factory.spec.ts`
Expected: PASS. Read `test-results/v12-floor.png`: machines separated from the backdrop, metal edges catching the softboxes, cabinet fronts not blown out. If everything is too bright, lower `EXPOSURE` in 0.05 steps and re-shoot; record the final value in the commit body.

- [ ] **Step 5: Commit**

```bash
git add src/scene/lighting.tsx src/scene/Factory.tsx tests/e2e/factory.spec.ts
git commit -m "feat: light the line from an authored environment and three-point rig"
```

---

### Task 3: Gradient backdrop and fog

**Files:**
- Modify: `src/styles/main.css`, `src/scene/Factory.tsx`
- Test: `tests/e2e/factory.spec.ts`

**Interfaces:**
- Produces: `.viewport` radial gradient and fog tuned to its mid-tone; no scene background colour.

- [ ] **Step 1: Set the backdrop**

`.viewport` background becomes `radial-gradient(130% 95% at 50% 0%, #eef2ec 0%, #d3dbd4 48%, #b7c1b9 100%)`. Fog becomes `['#cfd7d0', 38, 92]`.

- [ ] **Step 2: Shoot overview and close-up**

Run: `npx playwright test tests/e2e/factory.spec.ts` and capture both framings with a throwaway script. Check: machine silhouettes separate at the horizon, the floor meets the backdrop without a hard band, and the selected machine's mechanism is still readable.

- [ ] **Step 3: Commit**

```bash
git add src/styles/main.css src/scene/Factory.tsx
git commit -m "feat: give the viewport a gradient backdrop with matching fog"
```

---

### Task 4: Documentation and full verification

**Files:**
- Modify: `README.md`, `docs/DECISIONS.md`

- [ ] **Step 1: Record the decisions**

`DECISIONS.md` v1.2: environment authored in code with one bake, no CDN; reflector settings and the measured frame-time trade-off; exposure constant; backdrop is CSS with matching fog; post-processing and set dressing deliberately deferred.

- [ ] **Step 2: Update README**

Point the "browser demo" description at the new lighting (environment bake, reflective floor), and state that there are still no external assets or network calls.

- [ ] **Step 3: Run everything**

```bash
npm run typecheck
npm run lint
npm test
npm run test:e2e
cd backend && go test ./...
```
Expected: all pass; paste the real output in the completion report.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/DECISIONS.md
git commit -m "docs: record the v1.2 showroom lighting"
```
