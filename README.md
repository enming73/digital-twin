# SMT Digital Twin Studio

## W6 persistence (in progress; not accepted)

The optional W6 overlay starts PostgreSQL and connects the backend to it:

```sh
npm ci
npm run build
docker compose -p smt-w6 -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml -f compose.w6.yml up --build
```

Open `http://127.0.0.1:4174/` and select the remote source. PostgreSQL uses a named Docker volume and local-demo-only credentials; the database has no host port. To stop the project without deleting its data, use the same `-p` and four `-f` arguments with `stop`. A subsequent `up` reuses the run and its event history. The simulator must use a **new run ID for each new run**; rerunning a completed publisher with the same ID is not a new production run. Deleting the named volume permanently deletes W6 data; do not remove it unless deliberately resetting the database.

The experimental read-only historical API is `GET /api/runs/{id}/replay?simTimeMs=5000`; it includes every committed event at or before the requested simulated time. `GET /api/runs/{id}/events?fromSimTimeMs=0&toSimTimeMs=5000&afterSequence=0&limit=100` returns an inclusive time range ordered by sequence, with limits from 1 to 1000. In the remote view, enter simulated milliseconds under **HISTORY / REMOTE**, select **Seek history**, and use **Return to live** to display the latest live snapshot. While viewing history, the live MES trace is hidden rather than presented as a historical trace. Checkpoints are written every 500 events and after the consumer stops during clean shutdown; only the three newest are retained. A valid checkpoint is checked against the stored prefix and used for restart and seek; an invalid or incompatible checkpoint falls back to full event replay. On the 1000-board test run, checkpoint restore took 0–2 s versus 58 s for full replay. Times beyond the last stored event return the last known state, not proof that the run ended.

This is incomplete experimental work, not W6 acceptance. The backend restores committed events and trace data after restart, but a disconnect or missing MQTT tail can leave events absent; `uncertain` must not be read as complete. The publisher has no durable outbox, and MQTT QoS 1 or publisher acknowledgements do not prove end-to-end delivery. Automated gap repair, high-load 1000-board acceptance, production security and representative performance measurements remain outstanding. A 100000× burst exceeded the bounded broker queue and failed the 1000-board test; see `docs/W6_REPORT.md`. PostgreSQL unavailability stops durable ingestion rather than acknowledging an uncommitted event. The default browser-only demo still works without this stack.

For a separately named 20× paced 1000-board measurement, free local ports 1883/8080 and run `bash scripts/w6-load.sh [project] [run-id]`. It defaults to `demo-run`; pass a fresh run ID for each new run, because a new publisher reusing a stored run ID freezes that run as a conflict. The script prints the work-order response, event count, latency percentiles and a single backend memory sample. It stops only its own containers and **retains** its named database volume. Its single memory sample is not peak RSS; a reconciled count does not prove that no source tail is missing.

## W5 mock MES traceability (SIMULATED, experimental)

With the optional W4 stack below running, select the remote source and use **Mock MES · read only** to look up a board (for example `board-1`) or work order (`work-order-1`). Select an active PCB in the scene to fill the board search. Results are point-in-time: search again to refresh. A completed PCB remains searchable until the backend restarts. The response revision and integrity belong to the query and can differ from the current WebSocket projection. For example:

```sh
curl http://127.0.0.1:4174/api/runs/demo-run/boards/board-1/trace
curl http://127.0.0.1:4174/api/runs/demo-run/workorders/work-order-1
```

The fixed route is configured in the backend and is not evidence of visitation; entries show observed board events in sequence order, with simulated time, emission wall time and ingestion wall time kept separate. Planned comes from the start event; created counts observed board identities; in-progress means not yet completed. Completed boards are classified as failed if any recorded inspection failed, passed if inspections exist and all passed, or uninspected otherwise. Failure does **not** mean physical scrap; boards still move through the simulated line. Reconciliation compares completion counts only when all completed boards have at least one inspection. Optional mock metadata does not create orders. All data is synthetic, not certified KPI or real MES data.

History and duplicate tracking exist only in backend memory. Restart loses history; disconnected or missing events cannot be recovered, and `uncertain`, `gap` and `conflict` indicate limited trust. Even `complete` cannot detect a missing tail event. Only linear station progression is supported; rework, skipped stations and repeated inspections are rejected. No persistence, real MES, equipment control or production authentication is included. See `docs/W5_REPORT.md` and `docs/EVENT_CONTRACT.md`.

## W4 remote 3D view (experimental, SIMULATED)

The default is still the independent browser-only local demo. To use the optional **read-only** W3 source, build the UI and start the isolated local stack from the repository root:

```sh
npm ci
npm run build
docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml up --build
```

Open `http://127.0.0.1:4174/`, then select **Remote W3 · read only** under **Data source**. The default run ID is `demo-run`. Switching to remote pauses the local simulator; switching back restores that paused local run. Camera selection and reset remain available. Remote controls do not operate equipment or the simulator; presentation, local replay, local controls and local analytics are unavailable in remote mode. Stop only this stack with `docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml down`.

For development use `npm run dev -- --host 127.0.0.1 --port 4173` while the W3 backend listens on `127.0.0.1:8080`. The Vite proxy accepts WebSocket origins only at `http://localhost:4173`, `http://127.0.0.1:4173`, and the equivalent port 5173 origins; the nginx proxy accepts only localhost/127.0.0.1 on port 4174. Each verifies the incoming browser Origin before rewriting it for the backend. These fixed origins, localhost port bindings, unauthenticated HTTP and lack of TLS are **local-demo security**, not a production or LAN deployment. Direct backend WebSocket cross-origin checks remain enabled. The nginx Compose service serves the previously built `dist/`; rebuild after changes.

Remote snapshots and contiguous deltas drive the scene. A reconnect always replaces the view from a new snapshot; it cannot repair missing backend events. Connection status and the backend's `complete`, `uncertain`, `gap`, or `conflict` integrity are separate. `gap`/`conflict` means the backend froze its projection. The 15-second activity-stale label means **no messages**, not necessarily a broken connection: a completed run may legitimately be quiet. A backend restart can expose an empty uncertain run; no persistence or automatic catch-up is claimed. Processing start, queue capacity, cycle time, exact board location and movement are not available in W3; remote board placement and 300 ms transfer motion are illustrative, and machine mechanisms remain still. Only the standard seven-station topology/order is supported; W3 snapshots do not convey topology order, so a differently ordered backend cannot be detected reliably. See `docs/W4_REPORT.md` for checks and limitations.

## W3 synthetic twin backend (experimental)

The Go backend consumes **SIMULATED** W2 MQTT events; it is read-only and is not connected to equipment or the browser demo. Start a local isolated stack from the repository root:

```sh
docker compose -f compose.w2.yml -f compose.w3.yml up --build
curl http://127.0.0.1:8080/api/health
curl http://127.0.0.1:8080/api/assets
curl http://127.0.0.1:8080/api/runs/demo-run/snapshot
docker compose -f compose.w2.yml -f compose.w3.yml down
```

The Compose backend waits for Mosquitto, and the simulator waits for backend subscription readiness. HTTP and MQTT host ports bind localhost only. `GET /api/health` reports MQTT connectivity (503 until subscribed), not run completeness. The snapshot's `integrity` is `complete`, `uncertain`, `gap`, or `conflict`. A gap/conflict freezes that run; no automatic missing-event recovery is claimed. A never-seen run is uncertain. Compose asserts a fresh publisher with `--expect-fresh-run`, so the first sequence-1 event can establish completeness. Do **not** use that assertion if another publisher may already have run; without it, integrity remains uncertain even if sequence 1 arrives. Restart changes `epoch` and cannot recover missed events.

The local broker bounds each subscriber's QoS backlog to 10,000 messages and 16 MiB. Sustained overload can still cause broker-side drops; a sequence gap is visible when a later event arrives, but a missing tail with no later event or disconnect is not detectable. MQTT publisher acknowledgements do not guarantee backend delivery.

WebSocket `ws://127.0.0.1:8080/ws/runs/demo-run` sends an atomic `snapshot` first, followed by ordered `delta` messages with epoch, revision, event, ingestion timestamp and integrity. Integrity-only changes use `status`. A slow subscriber is closed (a `resync_required` frame is attempted); reconnect always takes a new snapshot, not a delta replay. Unknown runs return 404. Cross-origin browser requests are not enabled. This localhost demo has no authentication or TLS and is not suitable for production or public exposure.

For a standalone backend, from `backend/` run `go run ./cmd/twin-backend --mqtt-url tcp://127.0.0.1:1883 --run-id demo-run`; `--stations` selects the ordered, unique station IDs. Set `TWIN_SCHEMA_PATH` when not starting from `backend/`; it must point to the canonical `contracts/event.schema.json`. To inspect a complete offline W2 log without MQTT, generate NDJSON as below and start the backend with `--import /tmp/opencode/w2-events.ndjson --run-id demo-run`. Offline mode validates the entire run before exposing it; health remains 503 because MQTT is intentionally disconnected. It is not durable storage or automatic broker recovery.

```sh
cd simulator
go run ./cmd/simulator run --scenario scenarios/normal.json --run-id demo-run --speed 100000 --dry-run > /tmp/opencode/w2-events.ndjson
cd ../backend
go run ./cmd/twin-backend --import /tmp/opencode/w2-events.ndjson --run-id demo-run
go test -race ./...
W3_MQTT_INTEGRATION=1 go test -count=1 ./internal/ingest ./internal/api -run Integration -v
```

See `docs/W3_REPORT.md` for verification and limitations. The registry points at existing frontend GLBs; the backend does not serve these assets. Processing-start, exact internal queues and capacities cannot be reconstructed from the W2 contract; observed and simulator-derived flow status remain separate. Duplicate tracking is memory-only and grows with events; there is no persistence, authentication, MES or equipment integration.

## W2 independent simulator

The separate Go simulator generates **SIMULATED** data and does not connect to the browser or real equipment. It is awaiting user acceptance. See `docs/W2_REPORT.md` for test results and known gaps, and `docs/EVENT_CONTRACT.md` for the external event contract.

From the project root, run a broker-free example (Go 1.24+):

```sh
cd simulator
go run ./cmd/simulator run --scenario scenarios/normal.json --seed 2026 --speed 1000 --dry-run > /tmp/opencode/w2-events.ndjson
go test -race ./...
W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v
```

`--dry-run` writes NDJSON only to stdout; errors go to stderr. Omit it to publish to `--mqtt-url tcp://127.0.0.1:1883`; use a unique `--run-id` for each separate run. From the project root, `docker compose -f compose.w2.yml up --build` starts a local Mosquitto broker and simulator. The anonymous development broker is exposed only on `127.0.0.1`, and is not production security. SIGUSR1 toggles pause/resume on Linux; SIGINT/SIGTERM stop the process. See the report for limitations of shutdown, integration testing and currently incomplete acceptance evidence.

Available scenarios: `normal.json`, `fault.json` (mounter fault/recovery at 70/145 simulated seconds), `inspection.json` (all inspections fail), and `reconnect.json` (normal production data; disconnect is injected externally). For a manual reconnect exercise, run `docker compose -f compose.w2.yml up --build` in one terminal, then in a second run `docker compose -f compose.w2.yml stop mosquitto` followed by `docker compose -f compose.w2.yml start mosquitto`; the simulator waits/retries. Use `docker compose -f compose.w2.yml down` to remove only that Compose stack. The automated integration test uses its own container and dynamic local port.

An independently modeled, browser-only SMT line concept. **Concept demo · Synthetic data · Not affiliated with ASMPT.** Station geometry is original, generic and conceptual; no proprietary CAD, branding or third-party model downloads are used. There is no equipment connection or live telemetry.

## Run

Requires a supported Node.js LTS and npm. From the project root:

```sh
npm install
npm run dev
```

Open the URL printed by Vite. Drag to orbit, scroll to zoom, and click a machine or select a station from the keyboard-accessible list to focus it. **Reset view** restores the overview. Use **Start/Pause**, **Reset**, and the 1×/2×/5× selector in the simulation panel. Time and counts are SIMULATED. WebGL is required for the scene; a fallback message is shown when unavailable.

Selecting a station opens its close-up camera preset; **Refocus close-up** returns to that preset after orbiting. Procedural station mechanisms illustrate the simulated process only: they are not measured equipment positions or real machine motion. Animation updates follow the virtual clock and stop when it is paused.

## Checks

```sh
npm run lint
npm run typecheck
npm test
npm run build
npx playwright install chromium # once, if Chromium is not installed
npm run test:e2e
```

The E2E smoke test writes `test-results/factory-overview.png`. Screenshot output may vary by GPU/browser; it is a smoke artifact, not a pixel-baseline comparison.

## Scope and limitations

v0.2 adds a deterministic, synthetic 100-board work order and bounded FIFO queues. Configure the seed, quantity, defect probability, station processing times and input capacities in `src/data/scenario.ts`. Inspection outcomes do not remove boards from the line. Stations have one processing slot; after processing, the slot holds a blocked board until the next input buffer has capacity. Reflow uses the same simplified single-slot model. Transfers are instantaneous in the model; the display interpolates within a station but does not animate the transfer between stations. The amber tower lights remain decorative. Event records are kept in memory during a run, and reset clears them.

v0.3 adds simplified loader/unloader lifts, printer squeegee, SPI/AOI scanner indicators, placement gantry/feeders and a reflow conveyor indicator. These are conceptual, not physics-accurate. Cross-station PCB jumps remain a known acceptance gap.

## v0.4 analytics and trace

Scroll the right-hand panel to **Production insights**. Enter an existing board ID (for example `board-1` after starting) to see its recorded event history and current station; selecting a visible PCB in the 3D scene also opens its trace. The selected board is amber and its current station is highlighted. Completed boards remain searchable after they leave the scene. Simulated inspection failures and injected equipment faults appear in the alert list.

The window selector applies to completions, throughput, average cycle time, board pass rate, completion chart and inspection alerts. Both window endpoints are included. WIP and the work-order progress are current values, not windowed. Definitions:

- **Throughput / h** = completed boards in the window × 3,600,000 / elapsed virtual milliseconds in the window. Before any virtual time has elapsed it is unavailable.
- **WIP** = created boards − completed boards, at the current snapshot.
- **Average cycle / s** = mean `(completion time − creation time)` for boards completed in the window, in seconds. This includes input queue and blocked time.
- **Board pass rate** = boards completed in the window that passed all their recorded inspections ÷ completed boards in the window with at least one inspection. Boards with no recorded inspection are excluded; no inspected boards means unavailable. Defective boards are not scrapped in this concept.
- **Completion chart** groups actual `BOARD_COMPLETED` events into 12 equal virtual-time bins; alerts are simulated failed inspection and fault events, not real equipment alarms.

Values are calculated from the current in-memory simulated run, not live data, certified KPIs, real MES records or production telemetry. There is one mock work order; the event log is cleared by simulation reset.

## v0.5 simulated fault and replay

Use **Inject fault** to fault the mounter at the current virtual time; **Clear fault** recovers it. This is only a simulation, not an equipment command. A processing board stays in the machine, preserving its remaining processing duration and illustrative pose. The mounter input queue can still receive boards up to its capacity. Upstream blocks only after its output cannot transfer; downstream runs until its available boards are exhausted. Clearing the fault resumes the remaining duration; a previously completed but blocked board instead waits for downstream capacity. A faulted empty machine resumes normal queue handling. The simulation clock can be paused independently.

**Open replay** pauses the live simulation and enters a read-only historical view. Drag **Replay time**, select a fault event in the timeline, or use **Play replay** and the speed selector. The scene and dashboard use events at or before the chosen time; the live simulation is unchanged while viewing. **Return to live** returns to the paused live state; press Resume to continue. All events sharing a timestamp are applied in sequence, so timestamp seek represents the state after all events at that time. Logs are in memory only; reset discards them. Export/import is not implemented.

## v1.0 guided presentation

Select **Start guided presentation** in the right panel. The 3½-minute tour at 1× resets the simulated run, starts the work order, moves through overview, printer and mounter camera presets, injects a simulated mounter fault at 70 seconds, clears it at 145 seconds and shows a KPI summary. **Pause**, **Resume**, **Restart** and **Exit** control the tour; manual simulation controls are disabled during it. Exit leaves the simulated run paused so **Open replay** can inspect the recorded incident. Restart discards the current simulated run. Camera cuts are immediate; captions and metrics are illustrative and SIMULATED.

Scripted events are scheduled at exact virtual timestamps, independent of timer update size. The normal speed selector is disabled during the guided tour, which starts at 1×. Pausing and resuming adds pause/resume events to the log, while the fault and recovery times remain fixed.

### Static deployment

Run `npm install`, `npm run build`, and serve the contents of `dist/` at a web server's URL root. For a local production preview run `npx vite preview --host 127.0.0.1`. This build is configured for root deployment; a subpath deployment requires setting Vite's `base` to that subpath and rebuilding. The demo loads only bundled/local assets and requires no external API. WebGL and JavaScript must be enabled. Smoke-test screenshots are written under `test-results/` by Playwright and are not pixel baselines.

The printer and mounter use locally packaged GLBs with procedural fallbacks; other stations remain procedural. The floor and cabinets are simplified, not dimensional representations of real equipment. Performance depends on WebGL hardware; desktop 1366×768 is the target, with a stacked smaller-screen layout. No external services are needed at runtime.

## v0.6 model assets and performance

Original generic source scenes are `assets/blender/printer.blend` and `assets/blender/mounter.blend`; browser assets are `public/models/printer.glb` and `public/models/mounter.glb`. Recreate them with Blender 4.2 LTS from the project root:

```sh
blender -b --factory-startup --python assets/blender/generate_models.py
```

The GLBs use PBR materials and expose named groups including `Cabinet`, `Conveyor`, `PCB_Anchor`, `Gantry_X`, `Gantry_Y`, `Placement_Head`, `Feeders` and `Signal_Tower`. The named motion anchors are currently empty placeholders: the existing procedural illustration remains responsible for animation and selection, not GLB-node animation. The per-model size budget is 2 MiB and is checked by `npm test`. If a GLB fails to load, its procedural shell remains visible. Meshes are low-poly, but separate level-of-detail variants are not yet implemented.

To reproduce a **software-rendered diagnostic**, start `npm run dev -- --host 127.0.0.1 --port 4173` and run `node scripts/measure-performance.mjs`. On 2026-11-08, WSL2 Linux 6.18, Intel i7-8700K, headless Chromium 158, SwiftShader, 1366×768, DPR 1, 5-second browser `requestAnimationFrame` cadence was **21.1 fps** (106 intervals / 5.033 s). This is not a GPU frame-time benchmark and is not representative mid-range desktop hardware; no target-hardware performance acceptance is claimed. Cross-station board jumps and earlier verification gaps remain.
