# W2 Independent Simulator Implementation Plan

> Execution only after explicit user approval. Implement sequentially with test-first cycles; no subagents unless separately authorized.

**Goal:** deliver a headless, seeded Go SMT simulator publishing synthetic versioned events via MQTT, independent of the browser and backend.

**Architecture:** port documented browser domain semantics into a separate Go module, with a synchronous deterministic event engine and pluggable NDJSON/MQTT sinks. A paced CLI controls virtual-time advancement; the MQTT sink isolates transport, acknowledgments, bounded backpressure and reconnection from domain logic.

**Tech stack:** Go (target 1.24+), JSON Schema draft 2020-12, Eclipse Paho Go MQTT client, Mosquitto 2, Docker Compose; existing TypeScript/Vite app untouched. Pin exact Go dependency and container image versions during implementation and record them.

**Spec:** `docs/W2_W6_ROADMAP.md` §§2–5; audited semantics in `docs/IMPLEMENTATION_BASELINE.md`; existing engine `src/simulation/engine.ts`.

## Global constraints and decisions

- W2 only; no browser integration, backend, real MES/equipment, PLC commands, production credentials, UI redesign, or advanced assets. Existing local demo and deterministic replay remain unchanged.
- All events include `source: "simulator"` and payload `synthetic: true`; docs and CLI identify telemetry as SIMULATED. MQTT is at-least-once, never exactly-once.
- Default station order and IDs, cycle times, seed, rate and queue capacities match `src/data/scenario.ts`. One slot per station doubles as blocked output; no separate output buffer. FIFO; downstream-first settling; transfers instantaneous only when next input queue has capacity. Reflow remains single-slot. Failed inspection boards remain in flow, classified as failed, not physically removed.
- Domain event ordering: process completions in station order; then fault/recovery scheduled at the same instant in scenario order; then downstream-first settlement, repeating until stable. Define this explicitly where it differs from browser manual fault timing. No dependence on wall clock, scheduling chunk size or render frames.
- UUID-like IDs are not required: CLI `--run-id` defaults to `demo-run`; unique run ID per independent run is operator responsibility. IDs `event-00000001`, `board-1`, `work-order-1` stable within a run; dedup key `(runId,eventId)`. Sequence is global per run, starts at 1, with no gaps in generated stream. No collision across independent runs sharing runId is promised.
- `emittedAt` is actual UTC RFC3339Nano wall time created once at sink enqueue; retries reuse the same serialized bytes. Determinism comparisons omit only `emittedAt`. W2 does not introduce ingestion time: W3 consumer records it independently.
- MQTT topics `demo/smt/{runId}/machines/{machineId}/events` and `demo/smt/{runId}/line/events`; identifier components reject `/`, `+`, `#` and empty strings. No retained event publications. Consumers deduplicate `(runId,eventId)`, detect gaps via sequence; reordering/gaps are reported, not silently repaired. W3 owns consumer policy and recovery.
- Queue overflow policy is **block advancement with bounded context-aware backpressure**, never drop or reorder; prolonged outage pauses virtual time. Reconnect uses capped exponential backoff, retransmits unacknowledged messages with identical IDs/bytes, QoS 1; acknowledgments gate removal. In-memory queue is not crash durable. Ordered single in-flight publisher preserves publisher order but consumers must still tolerate duplicate/out-of-order messages. On SIGTERM/SIGINT stop generating, attempt bounded drain and return nonzero if not delivered; no claim of lossless crash recovery.
- Broker development listener is anonymous only on container network and host `127.0.0.1:1883`; never expose to public interfaces. Local demo only, not production security. Compose simulator connects to `tcp://mosquitto:1883`.

## Review focus

1. Equal-time faults/completions and multiple commands: task 2 tests exact ordering and recovery.
2. Full queues, empty downstream and permanently faulted work: task 2 tests capacity, no false states, and bounded termination/deadlock reporting.
3. Broker loss while publish is in flight, duplicate acknowledgments and shutdown: task 3 tests retry identity, bounded queue and explicit nonzero failure.
4. Invalid IDs, payload variants and malformed scenarios: tasks 1/2 test strict validation before emitting.
5. Pause, signals and high speed with slow publisher: task 4 tests clock freezing, drain deadline and backpressure without silent loss.

## Files and interfaces

Create `contracts/event.schema.json`, `contracts/fixtures/*.json`, `docs/EVENT_CONTRACT.md`; create `simulator/go.mod`, `simulator/go.sum`, `simulator/cmd/simulator/main.go`, `simulator/internal/{contract,scenario,engine,publisher,runner}/` and corresponding `_test.go` files; create `simulator/scenarios/{normal,fault,inspection,reconnect}.json`; create `simulator/Dockerfile`, `compose.w2.yml`, `deploy/mosquitto/mosquitto.conf`; update README and `docs/ARCHITECTURE.md` (create if absent), `docs/W2_REPORT.md` after implementation. Do not modify browser runtime files. Contract fixtures and scenarios are separate: fixtures are valid examples of every external event; scenarios are simulator input.

## Contract decisions (task 1)

Envelope schema requires exactly `schemaVersion: 1`, `eventId`, `runId`, `source: "simulator"`, `machineId` (string or null), `eventType`, positive integer `sequence`, nonnegative integer `simTimeMs`, UTC RFC3339 `emittedAt`, and `payload`; disallow unknown envelope/payload fields. Payload always includes `synthetic: true`. Conditional schemas bind these payloads:

| eventType | machineId | additional required payload |
|---|---|---|
| `machine.state.changed` | machine | `observedState` (`IDLE`, `RUNNING`, `FAULT`), `flowState` (`NORMAL`, `BLOCKED`, `STARVED`), `reason` string |
| `machine.alarm.raised`, `.cleared` | machine | `alarmId`, `reason` |
| `pcb.entered`, `.exited`, `.process.completed` | machine | `boardId`, `workOrderId` |
| `pcb.inspection.completed` | SPI/AOI | `boardId`, `workOrderId`, `passed` boolean |
| `workorder.started` | null | `workOrderId`, `quantity` positive integer |
| `workorder.completed` | null | `workOrderId`, `quantity`, `passed`, `failed` nonnegative integers, passed + failed = quantity (enforce in Go; JSON Schema alone cannot express addition) |

`pcb.entered` means queue ownership acquired; `pcb.exited` means board leaves the processing/output-hold slot, emitted immediately before the next station's entered event, or final exit. `pcb.process.completed` precedes exit. Inspection follows process completion, before exit. `workorder.started` precedes board events; completion follows final exit. First-station creation is represented by first entered. Machine state is emitted whenever either observed or derived flow state/reason changes: `RUNNING` while processing or holding completed work, `IDLE` without a board, `FAULT` while faulted; derive BLOCKED only when completed board cannot enter full downstream queue, STARVED only when idle with no queue and work remains. Reason identifies blocking downstream station or empty input; fault is observed, not a derived flow condition. Both fault alarm transitions and state transitions are published.

## Task 1: Schema and fixtures

**Consumes:** roadmap envelope and event definitions. **Produces:** schema above, fixture for each of nine event types, Go `contract.Event` and `Validate(Event) error`, serialization with exact JSON names.

- [ ] Write contract tests: valid fixture for each event; reject missing fields, unknown type, wrong/null machine ID, malformed time, invalid identifier and wrong payload; assert completed counts reconcile.
- [ ] Run `cd simulator && go test ./internal/contract -v`; confirm failures are missing contract/validation, not setup errors.
- [ ] Implement schema, fixtures, contract Go types and validator; document all samples, event ordering, synthetic data, duplicate/gap policy and migration mapping from `BOARD_CREATED`, `BOARD_TRANSFERRED`, `PROCESS_COMPLETED`, `INSPECTION_RECORDED`, `MACHINE_STATE_CHANGED`, fault and work-order events. Existing local events are not altered. Use a pinned schema validator in tests to validate fixtures and Go-generated JSON against the actual schema.
- [ ] Run `cd simulator && go test ./internal/contract -v`; all pass. Verify schema and Go validation both reject malformed examples.

## Task 2: Scenario and deterministic engine

**Consumes:** `contract.Event`. **Produces:** `scenario.Load(path string) (Scenario, error)`, `engine.New(Scenario, uint32, string) (*Engine, error)`, `(*Engine).Next() ([]contract.Event, bool, error)` where each call advances to the next virtual boundary, returns ordered events and `done`, with events' `emittedAt` filled by the sink; `(*Engine).Snapshot() Snapshot` for tests. A scheduled fault command has integer `simTimeMs`, `machineId`, action `raise|clear`, and `reason`.

- [ ] Write tests asserting same seed/scenario and different call pacing give identical events except `emittedAt`; 100 boards have exactly seven entered/process/exit events per board in order, single terminal exit, no duplicates and no queue beyond capacity; FIFO and ownership hold after every boundary.
- [ ] Add fault test: at 70000 ms raise mounter fault, clear at 145000 ms; active work preserves remaining duration, input fills before upstream BLOCKED, downstream drains before STARVED, recovery completes all boards. Also test fault at completion timestamp and multiple same-time commands, empty machine, blocked machine and unrecovered-fault deadlock; engine reports explicit deadlock rather than hangs.
- [ ] Add inspection tests with 0 and 1 defect rate and fixed-seed reproducibility, failed boards still exiting and completion totals reconciling. Reject duplicate stations, unknown fault IDs, invalid cycle/capacity/quantity/rate/times and duplicate or impossible fault transitions.
- [ ] Run `cd simulator && go test ./internal/{scenario,engine} -v`; verify expected failures. Implement scenario validation and engine with explicit virtual-time event scheduling, seeded 32-bit LCG matching browser `(1664525*x + 1013904223) mod 2^32`, downstream-first settling and event ordering above. Provide normal, fault and inspection scenario JSON; reconnect scenario uses normal semantics and transport test orchestration rather than fabricated machine faults.
- [ ] Run the same command; all pass. Compare a small browser fixture for queue, inspection and fault semantics; document intentional contract/tie-order differences instead of claiming byte-for-byte parity.

## Task 3: MQTT transport

**Consumes:** ordered `contract.Event`. **Produces:** `publisher.Sink` with `Publish(context.Context, contract.Event) error` and `Close(context.Context) error`; `publisher.NewMQTT(Config) (Sink, error)` and `publisher.NewNDJSON(io.Writer) Sink`. NDJSON uses same envelope with UTC timestamp; output contains events only, diagnostics go to stderr.

- [ ] Write fake-client tests for QoS 1 acknowledgment before dequeue, retry with identical topic/bytes/ID, reconnect, duplicate callback, FIFO, bounded full queue blocking, context cancellation, capped backoff and drain timeout; test publisher with `go test -race`.
- [ ] Run `cd simulator && go test -race ./internal/publisher -v`; verify expected failure; implement single-in-flight queue, serialized bytes per event and context-aware retry. Transport failure propagates; never regenerate a domain event or advance after queue saturation.
- [ ] Add Mosquitto local integration test using an independent subscriber and schema validation: receive all events for short scenario, interrupt and restart broker while an event is pending, verify eventual delivery, sequence/unique-ID accounting and duplicate tolerance. Explicitly assert no retained event. Use dynamic test port or isolated Compose network, bounded deadlines, deterministic cleanup; test is opt-in `W2_MQTT_INTEGRATION=1` and fails rather than silently skips when requested.
- [ ] Run `cd simulator && go test -race ./internal/publisher -v`, then `W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v` from `simulator/` with local broker test prerequisites documented; both pass.

## Task 4: CLI, pacing and local deployment

**Consumes:** engine and sink interfaces. **Produces:** `simulator run --scenario PATH --seed N --speed N --mqtt-url URL [--run-id ID] [--dry-run]`; speed positive finite, dry-run has no broker dependency. SIGUSR1 toggles pause/resume on Linux; SIGINT/SIGTERM initiate bounded drain. Default seed 2026, speed 1, run ID `demo-run`, MQTT `tcp://127.0.0.1:1883`.

- [ ] Add runner/CLI tests using fake clock/sink: pause freezes virtual clock, resume continues; speed affects wall pacing only; invalid flags fail; slow sink blocks simulation; SIGINT/SIGTERM stop generation and failed drain returns nonzero. Assert NDJSON stdout parses line-by-line and stderr contains diagnostics only.
- [ ] Run `cd simulator && go test ./internal/runner ./cmd/simulator -v`, confirm expected failures; implement CLI, paced runner, signal handling and sinks; never use wall time as business clock. Provide Dockerfile and `compose.w2.yml` with localhost-only broker port, broker healthcheck, dependent simulator, local synthetic configuration and no public anonymous binding. No backend container.
- [ ] Run `cd simulator && go test ./...`, `go test -race ./...`, `go vet ./...`; all pass. Run `go run ./cmd/simulator run --scenario scenarios/normal.json --seed 2026 --speed 1000 --dry-run > /tmp/opencode/w2-events.ndjson`, parse and validate output; run `docker compose -f compose.w2.yml config` then `docker compose -f compose.w2.yml up --build` with subscriber and verify clean completion. Document exact broker disconnect/reconnect commands and observed outputs; clean up only services created for this test.

## Task 5: Regression, documentation and W2 gate

- [ ] Document exact versions, CLI options, scenario format, event semantics, QoS/backpressure/duplicate and crash-loss limitations, security boundary, pause and stop commands, normal/fault/inspection/reconnect commands and W3 migration in README, `docs/EVENT_CONTRACT.md`, `docs/ARCHITECTURE.md`.
- [ ] Run all W2 unit/race/integration/Compose checks above; run `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`, `npm run test:e2e`. Record actual results, warnings and environment in `docs/W2_REPORT.md`; no claims based on planned tests.
- [ ] Check acceptance: headless broker and dry-run; deterministic events excluding `emittedAt`; 100 board ownership/lifecycle; delayed fault propagation; subscriber schema validity and reconnect/duplicate behavior; bounded queue and explicit shutdown; synthetic labeling; no browser regression. Mark each pass/fail with evidence and report any unresolved item.
- [ ] STOP. Ask user to accept W2 before starting W3; do not implement later phases.
