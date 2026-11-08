# SMT Digital Twin Studio — W2–W6 Industrial Integration Roadmap

Status: **W1 reported complete; W2 proposed; repository audit required**  
Date: 2026-11-08  
Audience: OpenCode coding agent  
Project: industrial-realistic, ASMPT-inspired **concept demo** (not official or connected to real machines)

## 1. Scope and relationship to v1.0

The original v0.1–v1.0 roadmap delivered a **browser-only presentation demo**. The W1–W6 roadmap describes a **separate architectural maturity track**. A completed v1.0 does **not** imply that W2–W6 are implemented. W1 means the 3D scene, camera controls, selection, and simplified machine assets are present; Blender-authored GLB files are only confirmed if repository inspection verifies them. Do not claim a W1 pass based on version numbers alone.

**First action:** inspect repository and produce `docs/IMPLEMENTATION_BASELINE.md`: implemented features, files, current test results, which models are Blender/GLB versus procedural, current simulator/replay behavior, actual dependency versions, and gap matrix for W1–W6. Preserve all existing v1.0 presentation features. No rewrites without evidence of need.

## 2. Overall architecture and migration strategy

```text
                  [Existing React / R3F / Three.js UI]
                         | WebSocket + REST (W3/W4)
                  [Twin Backend (Go)]
                  /       |          \
         Asset Registry  State Store   Event Store (W6)
                         ^
                  MQTT normalized events
                         ^
              [Independent Simulator (W2)]
                         |
            [Mock MES adapter (W5)]
```

During W2, **do not** replace the browser simulator or wire it to the UI. Develop and test an independent publisher in parallel. During W3, establish a contract and backend; W4 adds UI integration via a feature flag and fallback to existing local demo. W5 adds mock MES data; W6 adds persistence/replay and resilience. Avoid real production equipment and real MES access.

Defaults: TypeScript existing UI, **Go** for independent simulator and backend (reuse existing Go code if present), Mosquitto MQTT, PostgreSQL for W6 (optional TimescaleDB only when justified), Docker Compose for local services. If repository already contains a suitable alternative, document tradeoff before changing stack. All data **SIMULATED**.

## 3. Cross-cutting contracts and engineering rules

- Versioned event envelope: `schemaVersion`, `eventId`, `runId`, `source`, `machineId` (nullable for line-wide events), `eventType`, `sequence`, `simTimeMs`, `emittedAt`, `payload`.
- Event types: `machine.state.changed`, `machine.alarm.raised`, `machine.alarm.cleared`, `pcb.entered`, `pcb.exited`, `pcb.process.completed`, `pcb.inspection.completed`, `workorder.started`, `workorder.completed`.
- Use a seeded discrete-event simulation with virtual time. No business-state updates from rendering frame delta. Board transfer is capacity-aware and never teleports.
- Exactly-once delivery is **not** assumed for MQTT. Consumers deduplicate by `(runId,eventId)` and use sequence for gap detection; handle replay/out-of-order policy explicitly.
- Topic pattern: `demo/smt/{runId}/machines/{machineId}/events` and `demo/smt/{runId}/line/events`. Retained state snapshots, if used, must be clearly distinguished from append-only events.
- No browser connection to MQTT broker in W2; never expose production credentials. Backend/API is the trust boundary.
- Separate **machine observed status** from **derived BLOCKED/STARVED**, and retain derivation reasons.
- Stable machine IDs, PCB IDs, work-order IDs, and explicit station ordering; panel/unit tracking is a future extension.
- Persist event time and ingestion time separately; distinguish simulated clock from real wall time.
- Maintain read-only integration and a visible demo disclaimer. No ASMPT logos or proprietary geometry without permission.
- Every phase requires tests, documentation, repeatable commands, and a STOP gate for user approval.

## 4. W1 — Baseline audit (not a new implementation phase)

**Objective:** verify what is already done and establish a no-regression baseline.

**Checklist:** 7 conceptual stations; at least five simplified machine models; GLB export if claimed; selectable machines; orbit/focus/reset camera; responsive UI; demo disclaimer; build/test baseline; list gaps. If GLB assets are missing, mark W1 **partially complete** rather than silently inventing completion. Do not block W2 solely because advanced visual polish is absent; record the gap.

**Deliverable:** `docs/IMPLEMENTATION_BASELINE.md`, `docs/W1_GAP_REPORT.md`.

## 5. W2 — Independent SMT simulator + MQTT publisher (NEXT)

**Goal:** an independent process emits realistic, deterministic SMT production events over MQTT without any browser or Twin Backend dependency.

### W2 tasks (in order)
1. Inspect existing simulator and reuse its domain semantics. Record differences from planned event contracts.
2. Define JSON Schema (or equivalent versioned contracts) and representative fixtures for all event types. Publish sample payloads in `docs/EVENT_CONTRACT.md`.
3. Implement Go discrete-event engine with virtual clock, seed, configurable station cycle times, finite buffers, FIFO PCB flow, fault injection/recovery, board and machine lifecycles.
4. Implement MQTT publisher with reconnect/backoff, bounded queue, configurable QoS 1, delivery acknowledgments, and explicit overflow behavior; avoid claiming exactly-once semantics.
5. CLI: `simulator run --scenario ... --seed ... --speed ... --mqtt-url ...`; `--dry-run` emits NDJSON to stdout; support pause/stop via clean signal handling (pause control may be CLI or scenario command).
6. Docker Compose: Mosquitto + simulator; broker configuration scoped to localhost by default; no open anonymous public listener.
7. Provide example scenarios: normal 100-PCB run, SIPLACE fault with upstream blocking/downstream starvation, inspection rejects, broker disconnect/reconnect.
8. Unit, integration, and determinism tests; record exact reproducible commands.

**W2 acceptance criteria**
- Runs headless with no browser, Go process + broker only.
- Fixed scenario + seed yields identical ordered domain event stream when non-deterministic envelope fields are excluded.
- 100-PCB normal scenario terminates with no duplicated/lost boards and no buffer overflow; every board has a valid lifecycle.
- Fault scenario produces plausible downstream STARVED and upstream BLOCKED **after** queues/buffers evolve, not instant global red state.
- MQTT subscriber receives schema-valid events in local integration test; reconnect is tested; duplicates are tolerated by contract.
- Dry-run can be tested without Docker/broker.
- All telemetry marked synthetic; no existing v1.0 functionality regresses.

**W2 explicit non-goals:** WebSocket API, Twin Backend, real MES, actual PLC/OPC UA, browser integration, advanced Blender assets, certified production KPIs.

**W2 gate:** stop after W2 implementation, publish tests/results, wait for user acceptance before W3.

## 6. W3 — Twin Backend

**Goal:** consume MQTT events, normalize/deduplicate them, maintain an authoritative in-memory twin state, and expose REST snapshot + WebSocket delta APIs.

**Tasks:** Go backend; schema validation; topic subscriptions; idempotency; per-run sequence tracking and gap detection; machine/PCB state reducers; asset registry; REST `/api/runs/{id}/snapshot`, `/api/assets`, `/api/health`; WS `/ws/runs/{id}`; snapshot-then-delta subscription protocol with revision numbers; reconnect/resync; backend integration tests; docker-compose integration.

**Acceptance:** two subscribers receive consistent updates; duplicate events do not double-count boards; restart/reconnect is explicit (W3 may reconstruct from W2 replay fixtures; durable recovery deferred W6); browser not yet required to consume backend. No silent sequence gaps.

**Non-goals:** production DB persistence, real equipment, MES.

## 7. W4 — Live 3D line integration

**Goal:** integrate existing R3F scene with W3 APIs without discarding local presentation mode.

**Tasks:** runtime data-source adapter `local | remote`; snapshot + WS subscription; map machine IDs to named scene objects; PCB enter/exit animation driven by events; fault/highlight/status light/details panel; stale/offline indicator; interpolation strictly visual, not business state; E2E reconnect/resync test; performance profiling.

**Acceptance:** switching local/remote preserves scene; remote 100-PCB scenario animates correctly; fault propagation matches backend state; no ghost PCBs after reconnect; meaningful offline display; existing camera and presentation mode remain functional.

## 8. W5 — Mock MES integration

**Goal:** enrich the twin with work orders, routing, PCB identity, inspection results and traceability using a **synthetic MES**, not real vendor integration.

**Tasks:** mock MES service/adapter; work order API; PCB-to-order mapping; station routing; event correlation; board trace page; work-order progress; inspection outcome and reject tracking; seeded fixtures; tests for duplicate IDs, missing associations, rework/skip policy (document unsupported cases).

**Acceptance:** query a PCB and see chronological processing history; query a work order and reconcile counts with simulator; IDs stable across simulator, backend, and UI; synthetic data disclaimer present.

## 9. W6 — Durable events, replay, resilience and performance

**Goal:** durable event storage, reproducible time travel, recovery, and measurable system behavior.

**Tasks:** PostgreSQL schema/migrations for events, snapshots, assets and traceability projections; event ingestion idempotency; replay reducer with versioned schemas; seek/time range; snapshots for efficient restore; MQTT disconnect recovery; WS reconnect + gap/resync; load tests; profiling and basic monitoring; docker-compose and README.

**Acceptance:** replay to selected timestamp reproduces original state; restart reconstructs latest state; duplicate and out-of-order events are handled according to documented policy; simulated 1000-PCB run finishes without board loss; measured latency, throughput, memory and FPS reported with hardware/browser conditions (no invented performance numbers).

## 10. Phase delivery and reporting format

For each phase create `docs/W{N}_REPORT.md` with: objective; implemented changes and file paths; commands to run; test commands and actual outputs; acceptance checklist pass/fail; architecture decisions; limitations and next-step risks. Update `README.md` and `docs/ARCHITECTURE.md` as needed. Do not report success unless tests actually ran. Do not proceed beyond approved phase.

## 11. Immediate OpenCode task

1. Read `AGENTS.md`, `docs/PROJECT_PLAN.md`, and this roadmap.
2. Audit actual repository; produce W1 baseline and gap report.
3. Produce `docs/W2_IMPLEMENTATION_PLAN.md` with file-level changes, schemas, test plan, dependency/port choices and explicit migration strategy.
4. Run existing baseline tests; report actual results.
5. **STOP. Do not implement W2 until the user approves the plan.**
