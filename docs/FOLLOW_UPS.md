# Follow-up candidates

Date: 2026-11-13. Written after the seven-station visual pass and the materials round merged to `main`.

This is a menu, not a plan. Every item needs its own agreed scope before implementation, per `AGENTS.md`. Costs are relative to this repository, not calendar estimates; risks are what a customer demo would lose if the work went wrong.

State this list assumes:

- Seven stations built from one Blender pipeline, each with framed doors or insulated covers plus a side operator panel (`docs/MOUNTER_SAMPLE_REPORT.md`, `docs/PRINTER_INSPECTION_ROLLOUT_REPORT.md`, `docs/REFLOW_LOADER_ROLLOUT_REPORT.md`).
- Matte powder coat, authored showroom lighting, blurred planar floor reflection, no post-processing (`docs/MATERIALS_AO_REPORT.md`).
- Browser demo and the W1–W6 integration track complete; nothing is pushed to production.
- Bundle 1,679 kB minified / 478 kB gzip; headless SwiftShader 1.9 callbacks/s default, 3.4 with `?quality=low`.

## 1. Cross-station board transfer animation

**Why:** the README and `PROJECT_SUMMARY.md` both list instantaneous transfers and cross-station PCB jumps as a known visual gap. It is the most visible remaining mismatch with the "follow one board" story.

**Work:** interpolate board position between station exits and the next entry using the existing virtual-time snapshot; no simulator change. Needs a transfer-duration convention that does not imply a real belt speed.

**Cost:** medium — touches the board layer and one visual contract, plus tests for seek/replay resets.

**Risk:** medium. Motion driven by anything other than `simTimeMs` would break replay determinism, which is a core invariant. A wrong duration makes boards appear to race or crawl.

**Depends on:** none.

## 2. Reflow thermal staging and zone visuals

**Why:** the oven currently shows a tunnel and belt cleats. The three zone shutters exist but carry no readable meaning; a customer asking "what are the zones?" has nothing to see.

**Work:** tint or label the three zones, optionally animate a heat glow that follows the board position from the simulator. Must stay clearly conceptual, not a thermal model.

**Cost:** low to medium — geometry labels plus a state mapping; no simulation change if zones derive from existing `processTimeMs` progress.

**Risk:** low. The main risk is implying a validated thermal profile; any label must say illustrative.

## 3. Status-light treatment (bloom for lamps only)

**Why:** the signal tower changes colour but reads as coloured plastic rather than a lit lamp. This was the only visual idea worth keeping from the discarded AO round.

**Work:** either an emissive-only bloom pass, or a cheaper fake: a small additive sprite under each lens. The cheap version avoids reintroducing a composer and its tone-mapping/MSAA traps documented in `docs/DECISIONS.md` v1.6.

**Cost:** low for the sprite, medium for a real bloom pass.

**Risk:** medium for bloom — the project plan explicitly warns against neon; a strong glow breaks the restrained look. The fake sprite is reversible in one file.

**Depends on:** none. Do **not** re-add `n8ao`/postprocessing for this alone.

## 4. Factory set dressing

**Why:** the line sits on an empty reflective floor with a gradient backdrop. Fences, cable trays, a ceiling truss or floor markings would sell scale.

**Work:** procedural geometry in the existing pipeline or a separate scene module; must respect the shadow budget and the 40-mesh asset limit per GLB.

**Cost:** medium to high — new geometry, new draw calls, and a new place for LOD decisions.

**Risk:** medium. Set dressing is the easiest way to make a demo look busier but not better; it also competes with the machines for attention and for frame time.

**Depends on:** item 5 if cost becomes a problem.

## 5. LOD and draw-call budget

**Why:** every GLB is a single LOD; the scene renders all seven at full detail at every distance. The overview is the most common view.

**Work:** a distant variant per station (or a merged proxy), selected by camera distance; measure with `renderer.info`.

**Cost:** medium to high — doubles the asset pipeline output and adds a runtime switch.

**Risk:** low visually if the swap distance is chosen well; medium in pipeline complexity, because every future model needs two exports.

**Depends on:** item 4 benefits from it.

## 6. Deferred review minors (cleared 2026-11-13)

These were recorded during the three rollout reviews and cleared in one pass; the list is kept for traceability. Cladding is now derived from each layout (`fill_front`), covers have a centre stile and a sealed header/base, window frames stand proud of the panel face, hinges clear the side wall, the `Pusher` clearance tuple only carries its z travel, the seam validator reads leaf names from `LAYOUT`, the powder-coat contract runs on all seven exports, CI now asserts the travel envelope for every station from the GLB accessors, and the browser console guard walks all seven stations.

- Reflow covers leave 10–40 mm unsealed slots against the header and between leaves.
- Cover window frame trim sits inside the panel thickness, so it does not read as a surround.
- One outer cover hinge is inset 15 mm inside the side wall.
- `Pusher` clearance tuple carries an unused y value; the `CLEARANCE` comment still describes two elements.
- The powder-coat material contract checks only the mounter GLB, not all seven.
- `validate_door_seams` hardcodes the `Vision_Door_L/R` names.
- One loader test assertion (`expect(title).toMatch(/DEMO/)`) is tautological.
- Cladding fill around single-cover stations is hard-coded per kind rather than derived from the layout.
- The clearance envelope is a per-station constant, not derived from the enclosure meshes; there is no CI test for bounds at travel extrema (the check only runs inside Blender).
- The e2e console-error guard still covers only the overview and mounter close-up, not every station.

**Cost:** low — mostly geometry constants plus a handful of tests.

**Risk:** low. Regenerating assets touches every GLB, so it should ride with a visual round rather than alone.

## 7. Real integration track

**Why:** `PROJECT_SUMMARY.md` records W1–W6 as implemented but awaiting formal acceptance, and lists production hardening and real equipment integration as the next value steps.

**Work, in order of value:** (a) formal acceptance sign-off for W2–W6 with any gap corrections; (b) authentication/TLS/deployment topology for the backend; (c) OPC UA/PLC adapters and real MES mappings.

**Cost:** high and open-ended.

**Risk:** high — this is where "concept demo" becomes infrastructure; it needs credentials, security review and a real customer environment, none of which the repository has.

**Depends on:** an explicit new scope. Out of bounds for the visual track.

## 8. Presentation and documentation polish

- Guided tour captions reference the previous, plainer visuals in one or two places; re-read them against the current close-ups.
- `docs/PROJECT_SUMMARY.md` predates the seven-station rebuild and should point at the three rollout reports plus the materials report.
- `README.md` bundle/frame numbers are current but the "Known limitations" list still describes the pre-rebuild models.

**Cost:** low.

**Risk:** low, but stale docs are how a handover loses trust; this is the cheapest item on the list.

## Recommended order

1. **Item 8** — cheap, and it fixes evidence that is already slightly out of date.
2. **Item 6** — clear the minors while the geometry context is fresh, bundled with any small visual change.
3. **Item 1** — the largest remaining gap in the demo story.
4. **Item 3 (sprite version)** — one file, immediate improvement to the status lights.
5. **Items 2 and 4** — visual richness, only after the above.
6. **Item 5** — when set dressing or a new station makes cost measurable.
7. **Item 7** — only with a new agreed scope and a customer environment.
