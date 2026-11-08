# SMT Digital Twin Studio — OpenCode Development Plan

Version: 1.0 | Date: 2026-11-08 | Status: Approved for implementation

## 1. Mission
Build a browser-based, industrial-realistic **concept demonstration** of an ASMPT-inspired SMT electronics assembly line. This is an independently modeled demo, **not an official ASMPT product**, not an exact reproduction of proprietary machines, and not connected to production equipment. Show a credible line, event-driven PCB flow, fault propagation, traceability and replay. All synthetic metrics must be clearly marked **SIMULATED**.

### Non-goals (through v1.0)
- No live equipment connections, PLC commands, real MES integration or production control.
- No exact proprietary ASMPT geometry, logos, manuals or undocumented interfaces.
- No physics-accurate placement-head motion, production-grade OEE certification, AI predictive maintenance or backend cluster.
- No paid cloud APIs required.

## 2. Demo story and audience
Audience: industrial software engineers, prospective manufacturing customers, hiring/interview demonstrations.
Story: show complete line -> start work order -> track boards -> inspect DEK-inspired printer and SIPLACE-inspired mounter -> inject mounter fault -> upstream BLOCKED/downstream STARVED after buffers drain -> recover -> replay incident and inspect KPIs.

## 3. Production line (configurable)
1. PCB Loader
2. Solder Paste Printer (DEK-inspired, conceptual)
3. SPI (Solder Paste Inspection)
4. Pick & Place (SIPLACE-inspired, conceptual)
5. Reflow Oven
6. AOI
7. PCB Unloader

Model single-sided SMT, one primary conveyor path, finite buffers, FIFO board order initially. Process time, buffers, defect probability and simulation speed configurable. Reflow and AOI are generic devices; do not imply every station is an ASMPT product. Real line topology differs by customer.

## 4. User experience and visual requirements
Industrial realistic: white/gray metal cabinets, brushed aluminum, glass covers, conveyor rails, believable factory floor, industrial lights, restrained colors, physically based materials, soft shadows, readable labels. Avoid sci-fi holograms, excessive neon and exaggerated effects. Support 16:9 desktop at 1920x1080 and 1366x768, with usable 1280px layout.

Three views:
- Factory: entire line, board movement, status indicators, select machine.
- Machine: orbit/focus camera, named subassemblies, simplified process animation, details.
- Analytics: KPI charts, work order progress, alarms, event timeline and replay.

Persistent demo label: 'Concept demo · Synthetic data · Not affiliated with ASMPT'. Do not use third-party brand logos without permission.

## 5. Technology decisions
- Node.js current supported LTS; TypeScript strict; Vite; React.
- Three.js via @react-three/fiber and @react-three/drei.
- Zustand for UI/simulation state; pure TypeScript discrete-event simulator independent of React and rendering.
- Recharts for charts; Vitest for unit tests; Playwright for E2E; ESLint + Prettier.
- Blender for source .blend files; GLB/glTF 2.0 for browser assets. Initial models may be procedurally authored in Three.js, then progressively replaced by Blender assets.
- Local JSON fixtures and deterministic event logs; optional IndexedDB persistence only if needed after v0.5. No backend for v1.0.
- Pin exact dependency versions in lockfile, verify compatibility at implementation time. Use npm unless repository already establishes another package manager.

## 6. Architecture and invariants
```
Scenario Config + Seed
       | 
Discrete-event Simulation Core (virtual clock, queues, machines, boards)
       | emits immutable domain events
       v
Event Log ---> Replay Reducer ---> Derived Twin State
                                    |         |
                             React UI     R3F Scene
```

- Simulation is source of truth, not Three.js scene graph. Rendering consumes snapshots/events.
- Virtual simulation clock is independent of wall clock and frame rate; support pause, resume, speed 1x/2x/5x, reset and deterministic replay.
- Every event has id, schemaVersion, simTimeMs, sequence, type, payload; sequence is strictly monotonic per run.
- Every board has immutable boardId, workOrderId, current station, lifecycle status, timestamps and inspection results.
- Every machine has stable machineId, process duration, input/output buffer capacities and states RUNNING, IDLE, BLOCKED, STARVED, FAULT, OFFLINE (OFFLINE only for simulated communication scenarios).
- Do not teleport boards: transfer occurs only if downstream buffer has capacity; reflow input/output behavior should be simplified and documented.
- Replay must reconstruct the same state from seed/config and ordered events. No Math.random() in simulator; use seeded PRNG.
- Derived KPI definitions documented, no misleading fake real-time data.
- UI scene animation interpolates between valid simulation states; on seek/replay, reset interpolation to target snapshot.

### Suggested domain events
RUN_STARTED, RUN_PAUSED, RUN_RESUMED, BOARD_CREATED, BOARD_ENTERED, PROCESS_STARTED, PROCESS_COMPLETED, BOARD_TRANSFERRED, INSPECTION_RECORDED, MACHINE_STATE_CHANGED, FAULT_INJECTED, FAULT_CLEARED, WORK_ORDER_COMPLETED.

### Suggested types
```
Machine { id, stationType, state, processTimeMs, inputCapacity, outputCapacity }
Board { id, workOrderId, stationId, status, createdAtMs, history[] }
ProductionEvent { id, runId, sequence, simTimeMs, schemaVersion, type, payload }
Scenario { seed, workOrderQuantity, stations[], defectRate, speed }
```

## 7. Repository layout
```
smt-digital-twin/
  AGENTS.md
  docs/PROJECT_PLAN.md
  docs/ARCHITECTURE.md
  docs/DECISIONS.md
  docs/ACCEPTANCE.md
  public/models/               # optimized .glb
  assets/blender/              # source .blend + export scripts
  src/app/                    # routing, layouts
  src/components/             # panels, charts, controls
  src/scene/                  # R3F factory, machines, camera, boards
  src/simulation/             # engine, machine logic, seeded PRNG
  src/domain/                 # event types, reducers, selectors
  src/data/                   # scenarios, demo fixtures
  src/styles/
  tests/unit/
  tests/e2e/
  scripts/
```

## 8. Phased implementation plan

### v0.1 — Industrial Realistic Factory Scene
**Goal:** polished, navigable 3D line without relying on external assets.
**Tasks (in order):**
1. Bootstrap Vite React TypeScript strict, lint, format, Vitest, Playwright, README and CI-compatible scripts.
2. Define station IDs, layout coordinates, bounding boxes and camera presets in typed config.
3. Build floor, ambient/area lights, seven simplified machines, conveyor sections and 20 static PCB props with procedural geometry and PBR materials.
4. Implement orbit, zoom limits, click selection, camera focus/reset and responsive side panel.
5. Add semantic machine hierarchy and labels, status legend, demo disclaimer, loading/fallback/error handling.
6. Implement deterministic visual screenshot smoke test and basic accessibility for UI controls.
**Acceptance:** `npm install`, `npm run dev`, `npm run build`, `npm test`, `npm run test:e2e` work; 7 individually selectable stations; camera reset; no browser console errors; no proprietary model downloads; usable 1366x768; placeholder models clearly labeled conceptual.
**Stop and report before v0.2.**

### v0.2 — Deterministic Production Simulator
**Goal:** coherent board flow independent of UI.
**Tasks:** build virtual-clock event scheduler, seeded PRNG, work-order generator, board queues, process/transfer transitions, finite buffers, pause/resume/reset/speed; bind board movement to simulator state; display simulation clock and counts.
**Acceptance:** seed+scenario yields identical ordered event log; 100 boards complete with zero losses/duplicates under no faults; queues never exceed capacity; board cannot be at two stations; pause freezes sim time; speed affects wall-time only; unit tests cover boundaries.
**Stop and report before v0.3.**

### v0.3 — Machine Process Animations
**Goal:** convincing, explicitly illustrative motion.
**Tasks:** create procedural printer stencil movement, SPI scan beam, mounter gantry and feeder activity, reflow conveyor transit, AOI scan, loader/unloader motions; animate by machine state and virtual time; show close-up presets and 'illustrative animation' labels.
**Acceptance:** animations stop on FAULT/PAUSE, remain in sync after speed change and seek; no physically impossible PCB jumps; scene remains interactive on target desktop; motion clearly not represented as measured real machine coordinates.
**Stop and report before v0.4.**

### v0.4 — Twin Dashboard & Mock MES
**Goal:** explain production operations in 3D and charts.
**Tasks:** mock work orders, board IDs and routing; machine details; throughput, WIP, cycle time, quality pass rate; alarms; board search and process history; chart time windows; selected machine/board cross-highlighting.
**Acceptance:** user can locate a board and its full route; work-order progress agrees with simulator; KPI formulas documented and validated against event fixtures; no invented values presented as live industrial telemetry.
**Stop and report before v0.5.**

### v0.5 — Fault Injection & Historical Replay
**Goal:** show cause/effect and reconstruct past state.
**Tasks:** inject mounter fault at selected virtual time, freeze processing safely, let finite buffers propagate BLOCKED/STARVED; clear fault; event timeline, seek, playback speed, replay reducer, optional event export/import JSON.
**Acceptance:** deterministic failure scenario; downstream becomes STARVED only after its available WIP is exhausted; upstream becomes BLOCKED only after relevant buffer fills; no lost/duplicated boards; seeking to same timestamp reproduces same state/KPIs; documented recovery semantics.
**Stop and report before v0.6.**

### v0.6 — Blender Asset Upgrade & Performance
**Goal:** replace key placeholders with professional, original industrial-realistic models.
**Tasks:** author generic printer and mounter in Blender; use named mesh parts (Cabinet, Conveyor, PCB_Anchor, Gantry_X, Gantry_Y, Placement_Head, Feeders, Signal_Tower); assign PBR materials, apply transforms, export GLB with stable naming and metadata; add LOD/mesh optimization and asset size budget; preserve procedural fallback.
**Acceptance:** 2 Blender source assets + 2 GLBs under version control (subject to size); exported parts discoverable by name; interaction and animation unchanged; no trademark/proprietary geometry; baseline performance recorded on representative desktop hardware, with regression checks.
**Stop and report before v1.0.**

### v1.0 — Presentation Mode & Release
**Goal:** polished 3–5 minute guided demonstration.
**Tasks:** scripted camera tour, narrative captions, start/pause/replay controls, deterministic fault scenario, KPI summary, reset-to-start, graceful asset loading, documentation, screenshots, static deployment, optional offline demo mode.
**Acceptance:** cold-start browser demo runs without external service/API; guided scenario can be repeated identically; all unit/E2E/build checks pass; README explains demo limitations, provenance, controls and deployment.

## 9. Acceptance metrics and performance
- Functional correctness first. 100-board no-fault test: no loss/duplication, exactly one terminal outcome per board.
- Browser targets: 1366x768 and 1920x1080; pointer selection and keyboard-accessible UI controls.
- Performance target (aspirational, measured): 30+ FPS on an explicitly recorded mid-range desktop; use instancing for repeated PCBs, dispose geometry/materials, cap pixel ratio, avoid React rerenders per frame.
- No uncaught errors, missing assets, uncontrolled console warnings or blocked critical interactions.
- Add deterministic tests for same seed, buffer saturation, fault propagation, replay equivalence, KPI calculation and reset.

## 10. Risks and mitigations
- Model accuracy / IP: conceptual original generic assets, no official logo or proprietary CAD.
- Simulator credibility: discrete events, finite buffers, explicit simplified assumptions.
- Performance: low-poly procedural first, GLB budgets, instancing, adaptive shadows.
- Coupled UI/engine: pure simulation package, event/reducer contracts, no direct mesh mutation from domain layer.
- Demo scope creep: explicit phase gates and no live MES/backend until after v1.0.
- Branding: clearly label 'ASMPT-inspired concept', not official partnership or verified performance.

## 11. Development workflow for OpenCode
For each phase:
1. Read `AGENTS.md`, this plan, existing source, and decisions before editing.
2. Produce a brief implementation plan and list of files to touch.
3. Implement only the current phase; prefer small cohesive commits/changes.
4. Run lint, typecheck, unit tests, build and relevant E2E tests; report commands and results honestly.
5. Update README, `docs/DECISIONS.md`, `docs/ACCEPTANCE.md` and changelog if applicable.
6. Summarize completed work, limitations, tests, screenshots if available, and next phase.
7. **Do not start the next phase without explicit user approval.**

## 12. Definition of done for every phase
- Requirements and acceptance checks demonstrably met.
- Tests added for newly introduced business logic.
- TypeScript strict checks pass; no unexplained warnings.
- No placeholder functionality silently passed off as complete.
- New configuration and UI actions documented.
- No unexpected cloud spend or network dependency.
- Working demo remains usable after changes.
