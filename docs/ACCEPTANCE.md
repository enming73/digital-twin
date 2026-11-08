# v0.1 acceptance checklist

- Seven conceptual stations, individually selectable from the scene or accessible list.
- Camera orbit, zoom limits, station focus and overview reset.
- Procedural industrial materials, conveyors, floor, lights and 20 static PCB props.
- Persistent independent-demo disclaimer, conceptual labels, static/simulated data indication and WebGL fallback.
- Unit layout checks and browser smoke coverage, including screenshot capture at 1366×768.
- Verify with `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`, and `npm run test:e2e`. See README for setup and limitations.

# v0.2 acceptance checklist

- Seeded deterministic event log, virtual-time scheduler and configurable bounded FIFO stations: covered by `tests/unit/simulation.test.ts`.
- 100-board completion, no duplicate IDs or occupied slots, queue capacity, blocked state, pause, speed, reset and invalid inputs: covered by unit tests.
- Simulated time, counts, station states, start/pause/reset/speed and scene board positions: browser smoke coverage in `tests/e2e/simulation.spec.ts`.
- Known gap: visual board transfers between station anchors are instantaneous rather than continuously interpolated. Interactive replay and fault injection are later phases.

# v0.3 acceptance status

- Illustrative seven-station mechanisms, virtual-time pose calculation, close-up refocus and explicit labels implemented. Unit tests cover pose determinism, pause, non-running state and speed equivalence; browser smoke test covers the label and refocus control.
- **Not fully accepted:** cross-station PCB positions still jump; no fault injection or historical seek exists to exercise fault/seek animation behavior end-to-end. Target-desktop frame rate has not been measured. The phase cannot be called fully complete until these acceptance gaps are addressed or explicitly deferred.

# v0.4 acceptance status

- Mock work-order progress, simulated machine details, windowed throughput/WIP/cycle/pass rate, completion chart, inspection alerts and board event history implemented.
- Unit fixtures verify KPI calculations, inclusive window boundary and no-data handling; browser coverage checks searching an existing board and reset.
- Completed boards are searchable after they leave the 3D scene. Equipment faults and replay remain later-phase features. Selection highlighting and full-route browser traversal require further direct end-to-end verification.

# v0.5 acceptance status

- Mounter fault freezes current work; unit tests cover bounded propagation, downstream exhaustion, recovery, unique board completion, fault pose and replay/KPI equivalence at a historical timestamp.
- Read-only replay provides virtual-time seek, event markers and 1×/2×/5× playback. Browser smoke covers fault injection/clear and replay entry/exit.
- Limitations: fault scheduling in advance and event import/export are not available. Automated tests do not yet exhaustively prove propagation across every station or exercise all replay seek/playback boundaries in a browser. Cross-station PCB visual jumps from v0.3 remain.

# v0.6 acceptance status

- Two original Blender source files and GLBs exist; unit tests inspect GLB headers, named nodes, mesh presence and a 2 MiB per-asset budget. Browser tests cover local model requests and interaction when a model is corrupt.
- **Not fully accepted:** GLB motion anchors are placeholders while process animation remains procedural; dedicated LOD variants and representative desktop performance/regression measurements are missing. A headless SwiftShader diagnostic is documented but is not a representative GPU benchmark. Existing cross-station visual jumps remain.

# v1.0 acceptance status

- A 210-second scripted presentation drives fixed-time fault/recovery, camera presets, narration and KPI summary; unit tests check deterministic logs across different advancement chunks and fault propagation. Browser smoke covers start, pause, restart, exit and local-only cold start.
- **Not fully accepted:** a complete 3½-minute presentation has not been exercised end-to-end in a browser; earlier cross-station visual jumps, v0.6 representative-GPU performance/LOD gaps, and build size warning remain. Static deployment is documented but has not been verified on an external host.
