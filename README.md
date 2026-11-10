# SMT Digital Twin Studio

**An industrial-realistic SMT line digital twin: browser-first, deterministic and entirely synthetic.**

> Concept demo · All telemetry is **SIMULATED** · Not affiliated with ASMPT · No real equipment, PLC, MES or production data.

The repository contains one shared 3D front end with two ways to run it:

1. **Standalone browser demo** (default) — React + React Three Fiber, seven conceptual SMT stations, a virtual-time simulation engine, analytics, fault/replay and a guided presentation. No backend, broker, database or paid API is required.
2. **Optional read-only integration path** (W2–W6) — an independent Go simulator publishes MQTT events to a Go twin backend with REST/WebSocket APIs, an in-memory projection, mock MES traceability, and optional PostgreSQL durability, replay, checkpoints and verified offline recovery.

## Project status

All planned implementation phases are implemented in the repository; the W1 audit recorded partial-verification items. Per-phase acceptance by the user remains a human gate; W6 offline recovery is measured and awaiting final acceptance. This is still a local synthetic concept demo, **not production infrastructure**.

| Track                  | What it adds                                                                           | Status                                                                                | Evidence                                                                                                               |
| ---------------------- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Browser demo v0.1–v1.0 | 7 stations, camera, production simulation, analytics, trace, fault/replay, guided tour | Complete; user-accepted                                                               | [`docs/ACCEPTANCE.md`](docs/ACCEPTANCE.md)                                                                             |
| W1 baseline            | Repository audit and gap report for the visual baseline                                | Audited; partial verification (e.g. five distinct complete machine models not proven) | [`docs/IMPLEMENTATION_BASELINE.md`](docs/IMPLEMENTATION_BASELINE.md), [`docs/W1_GAP_REPORT.md`](docs/W1_GAP_REPORT.md) |
| W2 simulator           | Independent Go simulator + MQTT/NDJSON publisher + scenarios                           | Implemented; awaiting formal acceptance                                               | [`docs/W2_REPORT.md`](docs/W2_REPORT.md)                                                                               |
| W3 backend             | MQTT ingestion, validation, deduplication, twin state, REST + WebSocket                | Implemented; experimental                                                             | [`docs/W3_REPORT.md`](docs/W3_REPORT.md)                                                                               |
| W4 remote view         | Read-only remote data source inside the 3D scene                                       | Implemented; experimental                                                             | [`docs/W4_REPORT.md`](docs/W4_REPORT.md)                                                                               |
| W5 mock MES            | Simulated board trace and work-order lookup                                            | Implemented; simulated only                                                           | [`docs/W5_REPORT.md`](docs/W5_REPORT.md)                                                                               |
| W6 persistence/replay  | PostgreSQL event log, checkpoints, history seek, metrics                               | Implemented; experimental                                                             | [`docs/W6_REPORT.md`](docs/W6_REPORT.md)                                                                               |
| W6 offline recovery    | Durable source, terminal verification, prefix recovery                                 | Implementation and measurements complete; awaiting user acceptance                    | [`docs/W6_RECOVERY_REPORT.md`](docs/W6_RECOVERY_REPORT.md)                                                             |

Latest verification on 2026-11-10: browser lint, type check, unit tests, build and Playwright smoke suite pass; both Go modules pass `go test -race` and `go vet`. See [Checks](#checks) for exact commands and results.

A fuller wrap-up, including evidence and handover notes, is in [`docs/PROJECT_SUMMARY.md`](docs/PROJECT_SUMMARY.md).

## Architecture

```text
                    Browser (React + React Three Fiber)
                     |                         |
        local virtual-time simulator     remote read-only client
                     |                    REST + WebSocket (W4)
                     |                         |
                     |                 Go twin backend (W3)
                     |                 /       |          \
                     |        asset registry  projection  PostgreSQL (W6)
                     |                         |          |
                     |                  mock MES (W5)   durable replay
                     |                         ^
                     |              MQTT QoS 1 (read-only)
                     |                         |
                     |            independent Go simulator (W2)
                     |                         |
                     |              optional SQLite outbox +
                     |              read-only source API (W6 recovery)
```

The local browser path and the remote path are deliberately independent. Switching to remote pauses the local simulation; switching back restores it. Remote controls never operate equipment or the simulator.

## Quick start

### Browser-only demo (default)

Requires a supported Node.js LTS and npm.

```sh
npm ci
npm run dev
```

Open the URL printed by Vite. Drag to orbit, scroll to zoom, and click a machine or use the keyboard-accessible station list to focus it. **Reset view** restores the overview. Use **Start/Pause**, **Reset**, and the 1×/2×/5× selector.

The scene uses original procedural geometry plus two locally packaged GLBs. WebGL is required; a fallback message is shown when it is unavailable.

#### Static deployment

Run `npm run build` and serve the contents of `dist/` at the web server's URL root. For a local production preview:

```sh
npx vite preview --host 127.0.0.1
```

The build is configured for root deployment; a subpath requires setting Vite's `base` and rebuilding. The demo loads only bundled/local assets and requires no external API.

### Optional remote stack (W2 + W3 + W4 + W5)

Build the UI first, then start the isolated local stack:

```sh
npm ci
npm run build
docker compose -p smt-demo \
  -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml up --build
```

Open `http://127.0.0.1:4174/` and select **Remote W3 · read only** under **Data source**. The default run ID is `demo-run`. Stop only that stack with:

```sh
docker compose -p smt-demo \
  -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml down
```

### Optional persistence and replay (adds W6 PostgreSQL)

```sh
npm run build
docker compose -p smt-w6 \
  -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml -f compose.w6.yml \
  up --build
```

PostgreSQL uses a named Docker volume and local-demo-only credentials; the database has no host port. To stop without deleting data, run the same command with `stop`. A later `up` reuses the run and its event history.

### Optional durable-source recovery (adds the W6 source API)

```sh
docker compose -p smt-w6 \
  -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml \
  -f compose.w6.yml -f compose.w6-recovery.yml up --build
```

This path commits every simulator boundary to SQLite **before** publishing, and the backend repairs missed MQTT events from the read-only source API. It is intended for evidence and resilience testing, not normal demo use. Add the recovery overlay only to a fresh stack; a retained run must keep its matching PostgreSQL prefix and outbox generation.

> **Run IDs and volumes.** Use a fresh run ID for each new production run. The Compose defaults use `demo-run`; a second publisher that reuses a retained run ID freezes that run as a conflict. To repeat a demo from scratch, deliberately remove the project volumes (`docker compose -p <project> ... down -v`), or use a new run ID with a fresh stack. Never re-create only the simulator outbox volume for a run whose PostgreSQL prefix is retained: prefix fingerprints will reject the mismatch.

## What each layer provides

### Browser demo (v0.1–v1.0)

- **v0.2 production model** — deterministic seeded 100-board work order, bounded FIFO input queues, single-slot stations. Configure seed, quantity, defect probability, processing times and capacities in `src/data/scenario.ts`. Inspection failures do not remove boards; transfers are instantaneous in the model.
- **v0.3 illustrative mechanisms** — loader/unloader lifts, printer squeegee, SPI/AOI scanner indicators, placement gantry/feeders, reflow conveyor indicator. Conceptual, not physics-accurate.
- **v0.4 analytics and trace** — board search by ID or scene selection, event trace, throughput, WIP, average cycle time, pass rate, completion histogram and simulated alerts. WIP and work-order progress are current values; the others use the selected window, with both endpoints included.
  - **Throughput/h** = completed boards in the window × 3,600,000 ÷ elapsed virtual milliseconds in the window.
  - **WIP** = created boards − completed boards at the current snapshot.
  - **Average cycle/s** = mean `(completion time − creation time)` for boards completed in the window, including queue and blocked time.
  - **Board pass rate** = completed boards in the window that passed all recorded inspections ÷ completed boards in the window with at least one inspection; uninspected boards are excluded.
- **v0.5 fault and replay** — inject/clear a mounter fault at the current virtual time, then inspect the recorded run in read-only replay. Events at the same timestamp apply in sequence. Logs are in memory only; export/import is not implemented.
- **v1.0 guided presentation** — a 3½-minute tour at 1× with scripted camera presets, a fault at 70 s, recovery at 145 s and a KPI summary. Manual simulation controls are disabled during the tour.
- **v0.6 model assets** — original Blender scenes `assets/blender/printer.blend` and `assets/blender/mounter.blend`; browser GLBs in `public/models/`. Recreate them with Blender 4.2 LTS:

```sh
blender -b --factory-startup --python assets/blender/generate_models.py
```

The GLBs expose named groups such as `Cabinet`, `Conveyor`, `PCB_Anchor`, `Gantry_X`, `Gantry_Y`, `Placement_Head`, `Feeders` and `Signal_Tower`. The motion anchors are empty placeholders: the procedural illustration still drives animation and selection. A failed GLB load keeps the procedural shell visible. Each model is capped at 2 MiB by a unit test; separate LOD variants are not implemented.

### W2 — independent simulator (`simulator/`)

- Go discrete-event engine with a virtual clock, seed, finite FIFO buffers, board and machine lifecycles, fault injection/recovery, and a versioned event envelope defined in `contracts/event.schema.json`.
- Publishes ordered NDJSON (`--dry-run`) or MQTT QoS 1. Publisher calls block on PUBACK, providing bounded backpressure rather than unbounded buffering.
- Scenarios: `normal.json` (100 boards), `fault.json` (mounter fault/recovery at 70/145 simulated seconds), `inspection.json` (all inspections fail), `reconnect.json` (normal data; disconnect injected externally), plus `w6-1000.json` for soak runs.
- Optional durable mode (`--outbox-path`) commits each virtual boundary and its engine checkpoint to SQLite before publishing; `--source-addr` exposes a read-only source API for W6 recovery. Without durable mode, duplicates and crash loss are possible.
- `SIGUSR1` toggles pause/resume on Linux; `SIGINT`/`SIGTERM` stop the process.

```sh
cd simulator
go run ./cmd/simulator run --scenario scenarios/normal.json --seed 2026 --speed 1000 --dry-run \
  > /tmp/opencode/w2-events.ndjson
go test -race -count=1 ./...
go vet ./...
```

### W3 — twin backend (`backend/`)

- Consumes MQTT read-only, validates against the canonical schema, deduplicates `(runId, eventId)`, enforces contiguous sequences and freezes the projection on gap/conflict.
- Exposes snapshot + ordered delta WebSocket updates, REST snapshot/assets endpoints, and health. Snapshot creation and subscription registration share a lock with event application.
- Keeps observed machine state separate from simulator-derived flow state and reason. Offline NDJSON import can rebuild a complete run without MQTT; health remains 503 because MQTT is intentionally disconnected.
- Compose starts the backend with `--expect-fresh-run`, so a fresh publisher establishes completeness from sequence 1. Without that assertion, integrity stays `uncertain` even after sequence 1 arrives. Restart changes the epoch and cannot recover missed events.
- The broker bounds each subscriber's QoS backlog to 10,000 messages and 16 MiB. Sustained overload can still drop messages; a missing tail with no later event or disconnect is not detectable.

```sh
cd backend
go run ./cmd/twin-backend --mqtt-url tcp://127.0.0.1:1883 --run-id demo-run
go test -race -count=1 ./...
go vet ./...
```

### W4 — remote 3D view (`src/twin/`, `compose.w4.yml`)

- Runtime data-source adapter (`local | remote`), snapshot-first WebSocket client with capped reconnect backoff, projection validation by epoch/revision/sequence, and visual-only interpolation of board transfers.
- Connection/activity state is displayed separately from backend integrity. A 15-second activity-stale label means **no messages**, not necessarily a broken connection.
- Remote snapshots and contiguous deltas drive the scene. Reconnect always replaces the view from a new snapshot; it cannot repair missing backend events. Local presentation, replay, controls and analytics are unavailable while remote.
- Vite and the optional nginx service proxy same-origin browser requests and verify allowed localhost origins. Vite accepts `localhost`/`127.0.0.1` on ports 4173 and 5173; nginx accepts `localhost`/`127.0.0.1` on 4174. The backend still enforces same-origin WebSocket checks. This is local-demo security, not a production or LAN deployment. See [`docs/W4_REPORT.md`](docs/W4_REPORT.md).

### W5 — simulated mock MES (`backend/internal/mes/`, `RemoteTrace.tsx`)

- Read-only point-in-time board trace and work-order lookup over W4. Search again to refresh.
- Board events are shown in sequence order with simulated, emission and ingestion times kept separate. Planned quantity comes from the start event; created counts observed board identities; in-progress means not yet completed. Completed boards are failed if any inspection failed, passed if inspections exist and all passed, and uninspected otherwise.
- History and duplicate tracking are backend-memory only. Restart loses history; disconnected or missing events cannot be recovered. Only linear station progression is supported; rework, skipped stations and repeated inspections are rejected.
- All data is synthetic, not certified KPI or real MES data. See [`docs/W5_REPORT.md`](docs/W5_REPORT.md).

```sh
curl http://127.0.0.1:4174/api/runs/demo-run/boards/board-1/trace
curl http://127.0.0.1:4174/api/runs/demo-run/workorders/work-order-1
```

### W6 — persistence, replay and recovery (`backend/internal/persistence`, `simulator/internal/outbox`)

- PostgreSQL event, run, checkpoint, asset and traceability tables; a per-run writer lease; transactional event/projection writes; ordered reads and restoration from the committed prefix.
- Checkpoints are written every 500 events and after a clean shutdown; only the three newest are retained. A checkpoint is verified against the committed prefix, trace rows and projections before use. Corrupt or incompatible checkpoints fall back to full replay. On the 1000-board run, verified restore took 0–2 s versus 58 s for full replay.
- Historical REST APIs reconstruct state at or before a simulated time without changing live state. The remote UI can seek history and return to live; live MES trace is hidden while viewing history.
- Durable-source recovery: the simulator commits exact envelopes, an engine checkpoint and an optional terminal digest to SQLite before publishing. The backend reconciles its committed prefix with the source, re-reads retained envelopes to recompute the terminal digest, verifies `workorder.completed` as the last event, then promotes completion.
- `integrity: "complete"` means the projection is contiguous. `terminalVerified: true` is the stronger claim that the source terminal boundary, ordered digest and final completion event were verified. MQTT QoS 1 and PUBACK never prove backend persistence.
- A verified run that later loses the broker is downgraded to `uncertain` and re-verified by the coordinator. Conflict-frozen runs are never thawed.
- A legacy MQTT-only load helper measures the 20× paced 1000-board path without the durable source: `bash scripts/w6-load.sh [project] [run-id]`. It requires free ports 1883/8080, stops only its own containers and retains its PostgreSQL volume. Use `tests/integration/w6-recovery.sh` for recovery/soak checks.

See [`docs/W6_RECOVERY_REPORT.md`](docs/W6_RECOVERY_REPORT.md) for the measured 100/1000-board and burst evidence, and [`docs/W6_RECOVERY_DESIGN.md`](docs/W6_RECOVERY_DESIGN.md) for the protocol design.

## API

The backend binds to `127.0.0.1:8080`; through the W4 nginx service the same paths are available at `http://127.0.0.1:4174/`.

| Method | Path                                                                               | Purpose                                                                                                 |
| ------ | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| GET    | `/api/health`                                                                      | MQTT connectivity/subscription readiness (503 until ready); also checks the database lease when durable |
| GET    | `/api/assets`                                                                      | Asset registry for the existing frontend GLBs                                                           |
| GET    | `/api/runs/{id}/snapshot`                                                          | Current authoritative projection, revision and integrity                                                |
| GET    | `/api/runs/{id}/boards/{board}/trace`                                              | Simulated board trace (point in time)                                                                   |
| GET    | `/api/runs/{id}/workorders/{order}`                                                | Simulated work-order progress and reconciliation                                                        |
| GET    | `/api/runs/{id}/replay?simTimeMs=N`                                                | Read-only state at the selected simulated time                                                          |
| GET    | `/api/runs/{id}/events?fromSimTimeMs=0&toSimTimeMs=5000&afterSequence=0&limit=100` | Inclusive ordered event range, limit 1–1000                                                             |
| GET    | `/api/metrics`                                                                     | Process-local accepted/duplicate/conflict/gap/storage counters and Go heap bytes                        |
| WS     | `/ws/runs/{id}`                                                                    | Snapshot first, then ordered deltas with epoch, revision, event and integrity                           |

A never-seen run is `uncertain`. Restart changes the epoch; reconnect always starts from a new snapshot, never a delta replay. Times beyond the last stored event return the last known state, not proof that the run ended.

## Checks

Run from the project root:

```sh
npm run lint
npm run typecheck
npm test
npm run build
npx playwright install chromium   # once, if Chromium is not installed
npm run test:e2e
(cd simulator && go test -race -count=1 ./... && go vet ./...)
(cd backend && go test -race -count=1 ./... && go vet ./...)
```

Actual results on 2026-11-10 (Node 24.16.0, Go 1.26.1, Linux):

- `npm run lint`, `npm run typecheck`: pass.
- `npm test`: **45 passed, 1 skipped** (the skipped test is the opt-in W4 parity gate).
- `npm run build`: pass; reports the existing >500 kB minified chunk warning (1,601.32 kB / 452.80 kB gzip).
- `npm run test:e2e`: **20 passed, 1 skipped** in 31.8 s. The skipped test needs `W4_PARITY=1` and a generated event fixture.
- Simulator and backend `go test -race -count=1 ./...` plus `go vet ./...`: pass in both modules.
- The E2E smoke test writes `test-results/factory-overview.png`; screenshots are smoke artifacts, not pixel baselines.

Optional integration checks:

```sh
(cd simulator && W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v)
(cd backend && W3_MQTT_INTEGRATION=1 go test -count=1 ./internal/ingest ./internal/api -run Integration -v)
W4_PARITY=1 npx vitest run tests/unit/twin-w3-parity.test.ts
W4_PARITY=1 npx playwright test tests/e2e/remote.spec.ts
cd tests/integration && W6_PROJECT=w6-e2e-$(date +%s) ./w6-recovery.sh
```

`npm ci` currently reports 8 moderate and 2 critical advisories in existing dependencies. Dependency remediation and code splitting were not part of this demo.

## Scope and limitations

**Data and claims**

- All telemetry, KPIs, traces, MES records and equipment states are synthetic. There is no connection to ASMPT or any real machine, PLC, OPC UA server or MES.
- Inspection failure is not physical scrap; boards still move through the simulated line. Flow status is derived from simulator events, not measured sensors.
- Performance numbers are diagnostics on specific hardware, not target-hardware acceptance. The recorded local software-rendered measurement (2026-11-08, WSL2, i7-8700K, headless Chromium 158, SwiftShader, 1366×768, DPR 1) was 21.1 fps; the remote-view GPU path was not benchmarked.

**Security and deployment**

- The stack is local and unauthenticated: fixed localhost ports, no TLS, no production authentication, no multi-tenant isolation. Do not expose it to a LAN or the public internet.
- nginx and Vite proxies rewrite verified localhost origins only. The backend still enforces same-origin WebSocket checks.
- The PostgreSQL credentials and all local-demo volumes are for local use only.

**Delivery and durability**

- MQTT QoS 1 is not exactly-once delivery. PUBACK proves broker receipt, not backend persistence. Without durable mode, crash loss and duplicates are possible.
- W3 is in-memory. Restart loses history and cannot recover missing events. W6 adds durability only when `DATABASE_URL` is set; a restored run remains `uncertain` unless the source terminal is verified.
- W6 recovery supports a single backend writer. Compose does not set `W6_SOURCE_GENERATION`/`W6_SCENARIO_DIGEST`, so that deployment accepts the manifest's identity claim; pass them explicitly for strict identity checking.
- `terminalVerified` is in-memory; a restart re-verifies the source and changes the epoch. Source events are retained and acknowledgements only move a cursor. Reaching the quota requires operator action. Disk-loss recovery, source-API TLS/authentication and multi-tenant security are explicit non-goals.

**Remote view and traceability**

- W4 remote motion is illustrative; W3 cannot supply process start, exact queues, capacity or precise machine motion. A nonstandard station order is not discoverable from W3 snapshots.
- W5 supports only linear station progression and a fixed route; rework, skipped stations and repeated inspections are rejected. Backend restart loses trace history.

**Browser demo and build**

- Station mechanisms are conceptual. Cross-station PCB jumps remain a known visual gap; transfers are instantaneous in the model. GLB motion anchors are placeholders and LOD variants are not implemented.
- Analytics are calculated from the in-memory simulated run, not certified KPIs, MES data or production telemetry. Replay logs are in memory; export/import is not implemented.
- The build targets root deployment; a subpath requires setting Vite's `base`. The bundle exceeds 500 kB and dependency advisories remain.

## Documentation

| Document                                                             | Contents                                                              |
| -------------------------------------------------------------------- | --------------------------------------------------------------------- |
| [`docs/PROJECT_SUMMARY.md`](docs/PROJECT_SUMMARY.md)                 | Final wrap-up: delivered scope, evidence, demo script, handover notes |
| [`docs/PROJECT_PLAN.md`](docs/PROJECT_PLAN.md)                       | Original v0.1–v1.0 browser-demo plan                                  |
| [`docs/W2_W6_ROADMAP.md`](docs/W2_W6_ROADMAP.md)                     | Industrial integration roadmap and acceptance criteria                |
| [`docs/IMPLEMENTATION_BASELINE.md`](docs/IMPLEMENTATION_BASELINE.md) | W1 baseline audit                                                     |
| [`docs/W1_GAP_REPORT.md`](docs/W1_GAP_REPORT.md)                     | W1 gaps                                                               |
| [`docs/W2_IMPLEMENTATION_PLAN.md`](docs/W2_IMPLEMENTATION_PLAN.md)   | W2 design plan                                                        |
| [`docs/W2_REPORT.md`](docs/W2_REPORT.md)                             | W2 simulator report                                                   |
| [`docs/W3_REPORT.md`](docs/W3_REPORT.md)                             | W3 backend report                                                     |
| [`docs/W4_REPORT.md`](docs/W4_REPORT.md)                             | W4 remote-view report                                                 |
| [`docs/W5_REPORT.md`](docs/W5_REPORT.md)                             | W5 mock-MES report                                                    |
| [`docs/W6_REPORT.md`](docs/W6_REPORT.md)                             | W6 persistence/replay report                                          |
| [`docs/W6_RECOVERY_REPORT.md`](docs/W6_RECOVERY_REPORT.md)           | W6 offline-recovery evidence and limitations                          |
| [`docs/W6_RECOVERY_DESIGN.md`](docs/W6_RECOVERY_DESIGN.md)           | W6 recovery protocol design                                           |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)                       | Component architecture and data flow                                  |
| [`docs/EVENT_CONTRACT.md`](docs/EVENT_CONTRACT.md)                   | Versioned event envelope and semantics                                |
| [`docs/DECISIONS.md`](docs/DECISIONS.md)                             | Engineering decisions and trade-offs                                  |
| [`docs/ACCEPTANCE.md`](docs/ACCEPTANCE.md)                           | Browser-demo acceptance checklists                                    |

## Repository layout

```text
src/                 React UI, R3F scene, local simulation, analytics, replay, remote client
contracts/           Versioned JSON Schema and fixtures for external events
simulator/           Go independent simulator, scenarios, MQTT publisher, durable outbox
backend/             Go twin backend, ingestion, projection, replay, persistence, recovery
tests/unit/          Vitest unit tests
tests/e2e/           Playwright browser tests
tests/integration/   W6 recovery/soak/burst orchestration
deploy/              Mosquitto and nginx local-demo configuration
assets/blender/      Original Blender source scenes and generator
public/models/       Browser GLB assets
docs/                Plans, reports, architecture, contracts and acceptance notes
scripts/             Performance diagnostic and W6 load helper
compose.*.yml        Optional W2–W6 local stacks
```

## Reproducing the measured recovery evidence

```sh
cd tests/integration
W6_PROJECT=w6-e2e-$(date +%s) ./w6-recovery.sh
```

The script builds the stack, starts the simulator without the backend, waits for the terminal boundary, starts the backend and asserts completeness, uniqueness and sequence equality. It stops the project at the end and retains its volumes.

The 1000-board soak and 100000× live-burst variants are documented in [`docs/W6_RECOVERY_REPORT.md`](docs/W6_RECOVERY_REPORT.md), along with exact project names, event counts and the single recorded deployment deviation (the 1000-board recovery run used speed 100, a harsher burst than the planned 20×).
