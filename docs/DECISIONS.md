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
