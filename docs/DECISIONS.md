# Decisions and assumptions

- v0.1 contains only a static visualization; simulation, telemetry, charts and production controls are deferred to their planned phases.
- Seven conceptual machines are original procedural geometry. The printer and mounter are inspiration only; oven and inspection equipment are generic. No proprietary assets are loaded.
- Layout, bounding dimensions and focus presets are typed data in `src/data/stations.ts`; named scene groups provide a future GLB replacement seam.
- Twenty instanced static board props are illustrative and are not tied to a production state. The tower light is decorative, not a status indicator.
- Camera presets change immediately rather than animating. Rendering does not advance any business clock.
- E2E screenshot is recorded as a deterministic viewport smoke artifact, not a cross-platform pixel comparison; graphics drivers can differ.

## v0.2
- The simulator has one FIFO input queue and one processing/output-hold slot per station. A completed board occupies that slot while the downstream queue is full; only capacity-checked transfers change ownership. A downstream-first settlement pass deterministically handles simultaneous completions.
- The loader produces boards on demand into its bounded input queue. Reflow is a single-slot station rather than a physically accurate continuous oven. SPI/AOI record seeded synthetic pass/fail results without scrapping boards.
- A monotonic browser timer supplies elapsed wall time; the engine advances explicit virtual time independently of React and rendering. Completion stops the virtual clock at the final completion event. Reset clears the event log and restores the seed.
- Board positions interpolate within a processing station from virtual time, but model transfers are instantaneous and visual positions jump between station anchors. Continuous transfer animation is not implemented; this is a known visual limitation.

## v0.3
- Illustrative mechanisms derive their bounded pose from each machine's processing interval and the simulation snapshot's virtual time. No rendering-frame delta advances the process. Pausing the simulation freezes its snapshot; stopped stations use a fixed pose.
- Station selection uses existing station-specific camera presets; refocus re-applies the selected preset. Mechanism geometry and coordinates are conceptual, not measurements.
- The simulator has no transfer duration or reservation state. Visual smoothing across station boundaries would place a board on a conveyor while the model already considers it elsewhere; the discontinuity is retained and explicitly reported rather than presenting it as physically valid movement. Fault injection and historical seek are deferred by the roadmap to v0.5 and cannot be end-to-end verified here.

## v0.4
- Analytics are derived from the existing snapshot and event log; no synthetic values are fabricated by the dashboard. Window bounds are inclusive, and the divisor is the actual elapsed virtual time if the selected window reaches before time zero.
- Quality is a board-level metric over completed, inspected boards in the completion window; all recorded inspections must pass. Failed inspection events are shown as alerts, not as equipment faults. Exact formulas are in README.
- PCB history filters immutable events by board ID. Selecting an in-process board highlights its mesh and current station; completed boards remain traceable off-line. Reset clears board selection and the log.

## v0.5
- Injected fault holds the active PCB and its remaining processing time; recovery shifts its processing start by the fault duration, preserving the illustrative pose. A completed blocked board stays held until downstream input capacity permits transfer. The faulted input queue remains bounded and may receive boards.
- `src/domain/replay.ts` reduces recorded events at or before a target virtual timestamp. Same-timestamp events are applied in sequence; selecting a timestamp cannot isolate events within that timestamp. Replay pauses the live simulation on entry, is read-only and filters dashboard events at the seek time; returning to live leaves it paused. Reset discards the in-memory log.
- Cross-station visual jumps are still not resolved. Replay event import/export and fault scheduling in advance are not implemented; manual injection occurs at the current virtual time.

## v0.6
- Original generic printer and mounter assets are generated with Blender 4.2 LTS using `assets/blender/generate_models.py`; no proprietary models, textures or logos are used. Each GLB has a 2 MiB budget enforced by unit tests.
- Static GLB shells are loaded locally, with the original procedural shell retained while loading and on failure. Process animation remains a separate snapshot-driven procedural overlay. Named GLB motion anchors are placeholders, not animated nodes; separate LODs are not yet implemented.
- The WSL2 headless SwiftShader run measured 21.1 browser animation callbacks/s over 5.033 s at 1366×768. It is not a representative hardware GPU performance baseline. Exact reproduction and limitations are in README.

## v1.0
- The presentation starts a fresh simulation at 1× and uses exact virtual-time fault (70s) and recovery (145s) boundaries, ending at 210s. Timer chunk size cannot alter the scripted events. Pausing adds pause/resume events, so the complete log is identical only for runs with the same user interactions.
- Selecting Exit pauses the run and exposes existing manual controls and historical replay. Camera chapter transitions are immediate preset changes; no rendered-frame clock advances simulation.
- Vite's current build is intended for deployment at a URL root. Local GLBs and procedural fallback avoid external service dependencies; optional offline caching is not implemented.
# W4 decisions (2026-11-09)

- Keep remote optional and read-only; local simulation, replay and presentation are retained and paused on remote selection.
- Use W3's atomic WS snapshot before deltas, rather than a separate REST snapshot that could race the subscription. Never interpret a fresh snapshot as repairing an uncertain/gapped backend.
- Show observed and flow status independently; avoid fabricating queue or processing data. Animate only visually after an observed exit and adjacent entry.
- Validate the browser's original local Origin at each proxy before setting the upstream Origin. The user explicitly approved this correction to the W4 plan.
- W3 does not supply station ordering, so W4 assumes the standard seven-station order; alternate backend topology is unsupported.

## v1.1 (2026-11-12)
- All seven stations are generated by one original Blender pipeline (`assets/blender/generate_models.py`, verified with Blender 5.2.3 LTS); no downloaded models, textures or logos are used. Each GLB holds the anchor names listed in `src/data/anchors.ts` and is capped at 2 MiB and 40 meshes by a unit test.
- GLB anchors are live motion nodes. The runtime captures each anchor's authored position once and writes `rest + bounded offset` every frame, so repeated frames cannot drift and the same virtual time always produces the same pose. Offsets remain illustrative, not machine coordinates.
- Reflow belt travel is one authored cleat pitch (0.36 m) per process cycle, so the wrap seam coincides with the cycle reset instead of teleporting mid-cycle.
- Camera presets ease over 1.2 s with an in-out cubic curve; a pointer drag cancels the ease and hands control back to `OrbitControls`. The station preset targets the process window, not the whole cabinet, and the whole-machine `camera` preset is retained as data.
- Remote (W4) views and their historical replay render the seven models statically. `remoteScene` still emits `localMotion: null` and the projection carries no per-station process phase, so adding motion there would require a new contract. Local replay keeps driving poses from recorded virtual time. This is a recorded limitation, not a silent behaviour change.
- Shadow acne is removed with `bias -0.0004`, `normalBias 0.02` and a 2048 map tightened to ±12 m. Signal-tower colour comes from `src/scene/status.ts` for both paths; emissive is set on the cloned GLB lens material only. Selection is a ground ring, not a floor plate.
- `/api/assets` reports seven `glb` entries and migration `002_v11_assets.sql` corrects the seed rows; the supported schema version is now 2.
- Historical acceptance records (`ACCEPTANCE.md`, `IMPLEMENTATION_BASELINE.md`, `PROJECT_SUMMARY.md`, `W1_GAP_REPORT.md`) were deliberately left untouched: they record the evidence of their own phases. v1.1 evidence lives in this entry and in README.

## v1.2 (2026-11-12)
- The environment is authored in code with drei `Lightformer` rectangles baked once (`frames={1}`, 256) into a PMREM. No HDRI file and no CDN: the demo keeps working with no network, and the `factory.spec.ts` test pins zero cross-origin requests.
- Exposure is a single constant (`EXPOSURE = 0.85`) in `src/scene/lighting.tsx`; the key light at 2.6 casts shadows, fill 0.35 and rim 0.6 do not. Ambient is 0.12 — the environment supplies the indirect light.
- The floor is `MeshReflectorMaterial` with heavy blur and `mirror 0.12`, reading as polished epoxy. The plane is 400 m so its edge sits past the fog; the scene background is a solid `#dde4dd` with fog in the same colour, which also gives the mirror something to reflect. The CSS gradient on `.viewport` is only the WebGL-off fallback and is not visible in the shipped camera presets.
- Frame cost was measured in one session on headless SwiftShader at 1366×768, two 5 s runs each: v1.1 at `5962e63` **9.1 fps**; this build **3.4 fps**; this build with `?quality=low` **8.2 fps**. A plain-floor variant measured ~4.3 in review, so the planar reflection is nearly free and the **environment/IBL is the 2.4× cost**. `?quality=low` skips the `<Environment>` bake and recovers the baseline; it is the supported fallback for a weak demo laptop. These are software-rasteriser numbers, not a GPU baseline; measure the actual demo machine before accepting the cost.
- Demo-hardware check (2026-11-12): the target presentation machine runs the overview at **150–180 fps**, which is refresh-rate capped, so the environment cost is absorbed there and `?quality=low` is not needed. The SwiftShader numbers above stay the CI reference, not a laptop baseline.
- Post-processing (SSAO, bloom, DOF), set dressing (fences, ceiling trusses, cable trays), texture baking and LODs stay deferred. Nothing in the v1.1 asset pipeline, anchor motion, camera easing or backend changed in this round.

## v1.6 (2026-11-13)
- Powder coat is matte paint: base `(.46, .49, .47)`, metallic `0.04`, roughness `0.62`. The previous values (`.68/.72/.69`, metallic `0.27`) made a non-metal surface read as plastic because of the metallic sheen, not because of its lightness alone.
- Ambient occlusion was implemented (`n8ao` + `SMAAPass` + `OutputPass`), reviewed, then removed. Measured against `?ao=off` at the same camera it changed the overview by 2.2 and the mounter close-up by 8.5 grey levels (of 255); the key light's shadow map already supplies the contact shading. It cost 165 kB of bundle and about a tenth of the frame on headless SwiftShader. Both packages and the pass are gone.
- If AO is revisited: N8AO draws into float targets and bypasses three's tone mapping and MSAA, so it needs `gammaCorrection = false` with `SMAAPass` + `OutputPass`, and a gate on `EXT_color_buffer_float` — without that gate the stage renders blank on contexts without float targets.
- `?quality=low` remains the environment-bake switch: ~1.9 callbacks/s by default vs ~3.4 with it, at 1366×768 on headless SwiftShader. Bundle is 1,679 kB minified / 478 kB gzip.

## v1.7 (2026-11-13)
- The deferred review minors were cleared: front cladding is derived from each station's cover layout instead of literals, reflow covers meet in a stile with a sealed header and base, cover window frames stand proud of the panel face, and the outboard hinge clears the side wall.
- The travel envelope is now asserted twice: inside Blender before export, and in Vitest from the exported GLB accessors with the offsets taken from `src/scene/anchors.ts`. The CI test was verified to fail when the envelope is tightened.
- The browser console/cross-origin guard now visits all seven stations instead of the overview and mounter only.

## v1.8 (2026-11-13)
- Cross-station board motion is a visual interpolation: `src/scene/transfers.ts` diffs successive scene snapshots, records a hop when a board's station changes, and derives the position from `scene.simTimeMs` only. Pausing freezes the hop; a backwards seek clears in-flight transfers.
- Hop duration is `max(clamp(|dx| * 110, 350, 900), 2 × poll gap)` and is explicitly illustrative. The stretch guarantees a frame in flight at 2× and 5×, where a 350 ms hop would fit inside one 200–500 ms poll. The simulator still transfers boards instantaneously between buffers; no event payload, KPI or replay semantic changed.
- A hop starts at the poll that discovers the move, from where the board was last drawn, because the engine's transfer event is not exposed to the scene. x and z are both eased, so entering a queue neither jogs backwards nor snaps sideways. The model can lead the picture by up to one poll; the hop itself always lasts at least two polls.
- Boards travel on the real conveyor line (`z 0.12`), so most of a hop is hidden inside the machine enclosures. Routing them in front of the cabinets was rejected as physically wrong.
- Remote W4 boards keep the server-supplied `visual` interpolation and are outside this change; scenes containing them are skipped by the transfer model, and switching source clears any in-flight hops.

## v1.9 (2026-11-13)
- The seven signal towers draw a soft sprite halo over the lens, sized and tinted by state: FAULT 0.60/0.46 m, BLOCKED and STARVED 0.52/0.42, RUNNING 0.46/0.40, idle 0.12/0.26. Texture is a runtime radial gradient; nothing is downloaded.
- Normal blending, not additive: an additive halo clips to white on the pale backdrop, so the fault halo read as a white bloom. Depth testing stays on and depth writing is off, so machines occlude it; the sprite is not clickable.
- `SIGNAL_LENS` is the single source for the authored lens position and a unit test asserts it against every GLB, so a future tower edit cannot leave the halo floating.
- A post-processing bloom was rejected: render targets, tone-mapping handling and a quality switch for seven small lamps is the trade the ambient-occlusion round already declined.
- The halo is a visual cue only: no light is cast, exposure does not change.

## v2.0 (2026-11-13)
- The reflow oven shows three conceptual zones as small header lamps Z1–Z3. The active one is `floor(progress * 3)` from the same `processTimeMs` the mechanism already uses; an empty oven is dark and a fault freezes the zone.
- The labels are runtime canvas textures and the lamps are scene geometry at the authored header position, so this round needed no GLB regeneration and no simulator change.
- No temperatures, dwell times or recipes are shown: the zones illustrate process staging only.
