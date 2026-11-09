# W4 Live 3D Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or, if explicitly authorized, superpowers:subagent-driven-development. Steps use checkboxes for tracking. Do not implement until the user approves W4.

**Goal:** Add an optional, read-only W3-backed 3D view with safe reconnect and resynchronization while preserving the existing offline local demonstration.

**Architecture:** Keep the W3 snapshot and contiguous WebSocket deltas as the authoritative remote projection, separate from the existing local `Simulation`. A typed scene view model represents only observable facts; a remote transport validates and reduces messages, while an independent visual layer interpolates transfers without mutating business state. Use same-origin reverse proxying for `/api` and `/ws`, retaining the backend's same-origin WebSocket check.

**Tech Stack:** Existing React, TypeScript strict, Vite, R3F/Three.js, Vitest, Playwright, Go W3 backend, Docker Compose/Mosquitto; no new service dependency in default local mode.

**Spec:** `docs/W2_W6_ROADMAP.md` §3, §6–7; existing W3 behavior in `docs/W3_REPORT.md`, `backend/internal/api/http.go`, `backend/internal/twin/store.go`, and `docs/EVENT_CONTRACT.md`. This plan is a proposal, not W3 acceptance or permission to implement.

## Global Constraints

- Only W4; no real equipment, browser MQTT, MES, durable event storage, remote replay or production controls.
- Local is the default and remains usable without backend or network; remote is an explicit, visibly SIMULATED, read-only opt-in.
- Never infer processing start, queue slot/capacity, cycle time, board history, inspection history, KPI or recovery guarantees not supplied by W3.
- Preserve original assets, scene, selection, camera and local simulation, analytics, replay and guided presentation.
- Maintain strict epoch/revision validation; server status messages have no revision and must not advance it.
- Protect the same-origin boundary; no wildcard CORS, authentication claims, public listeners or production deployment claims.
- Document decisions, limitations, commands and measured performance; stop at the W4 gate.

## Baseline and preconditions

- W3 is implemented but `docs/W3_REPORT.md` calls it experimental and awaiting user acceptance/independent review. Approval of this plan must explicitly also settle whether W3 is accepted as the W4 baseline.
- W3 snapshots contain board location/status, observed and flow machine state, work orders, sim time, epoch/revision/integrity; WS provides an atomic snapshot first, then event deltas or integrity-only status, and may close slow clients. Unknown run returns 404. `in_transit` retains the previous machine ID; completed boards must not render.
- W3 can remain `uncertain` after broker loss; `gap`/`conflict` freezes its projection. A new snapshot cannot repair an incomplete backend; restart creates a new epoch and may be empty. Missing tail events are not always detectable. Health 503 in offline import does not invalidate a valid snapshot by itself.
- Existing `Factory` consumes local `Snapshot`/`MachineSnapshot`, while dashboard and animations assume queues, process timings and local event history; do not fabricate these for remote. Printer and mounter have GLBs, remaining machines procedural.
- Vite currently has no API proxy; backend rejects cross-origin browser WebSockets. Development proxy alone is not a production deployment solution.

## Review Focus

1. Duplicate, missing, out-of-order or wrong-epoch deltas: discard unsafe stream and require a fresh snapshot (Task 2).
2. `status` without revision, gap/conflict, stale connection and backend restart: clearly separate connectivity from data integrity and never claim recovery (Tasks 2–3).
3. Snapshot during transit, exit/enter at same simulated time, duplicate IDs and completed boards: render each known board at most once; no ghosts after resync (Tasks 2 and 4).
4. Switching source or run during pending callbacks, offline backend and unmount: cancel stale requests, timers and sockets; never silently substitute local data as remote (Tasks 3 and 5).
5. Unsupported fields, unexpected machine IDs and high-speed bursts: show unavailable/unknown honestly, avoid fabricated positions or unbounded animation queues (Tasks 1, 4 and 6).

## File map

- Create `src/twin/types.ts`: remote protocol and neutral, nullable scene view-model types.
- Create `src/twin/projection.ts`: validation, snapshot conversion and pure W3 event reducer.
- Create `src/twin/remoteClient.ts`: same-origin WebSocket lifecycle, bounded retry and generation cancellation.
- Create `src/twin/localAdapter.ts`: translate current local snapshot into neutral scene model without changing local simulation semantics.
- Create `src/scene/remoteMotion.ts`: bounded, non-authoritative board visual transitions.
- Modify `src/scene/Factory.tsx`, `src/scene/motion.ts`, `src/app/App.tsx`, `src/styles/simulation.css`: consume source-aware scene data, status and mode controls.
- Modify `vite.config.ts`; create `deploy/w4/nginx.conf` and `compose.w4.yml`: optional same-origin dev and deployment proxy, localhost binding and static UI serving.
- Add `tests/unit/twin-projection.test.ts`, `tests/unit/remote-client.test.ts`, `tests/unit/remote-motion.test.ts`, `tests/e2e/remote.spec.ts`; add fixture/server support under `tests/fixtures/w4/` and `scripts/` if necessary.
- Update `README.md`, `docs/ARCHITECTURE.md`, `docs/DECISIONS.md`, `docs/ACCEPTANCE.md`; create `docs/W4_REPORT.md` with actual results.

## Task 1: Source-neutral display contract and local adapter

**Files:** `src/twin/types.ts`, `src/twin/localAdapter.ts`, `src/scene/Factory.tsx`, `tests/unit/twin-projection.test.ts`.

**Interfaces:** Export `type SourceMode = 'local' | 'remote'`, `type SceneModel`, and `localScene(snapshot: Snapshot): SceneModel`. `SceneModel` contains `simTimeMs`, station records keyed by stable ID with state/observedState/flowState/reason and nullable local-only motion details, plus unique visible board IDs and station IDs. Preserve enough local data for existing motion and selection, but do not reuse local `MachineSnapshot` as a remote contract. A remote board may be at a station or in transit; completed boards are absent from visible boards.

- [ ] Test all seven local station IDs, queue and processing board IDs, selection identity and the unchanged local visual motion inputs against a real `Simulation` snapshot.
- [ ] Run `npx vitest run tests/unit/twin-projection.test.ts`; expect failure before adapter exists.
- [ ] Implement the types/adapter and adapt `Factory` to the neutral model without changing local UI behavior or camera lifecycle. Keep the local dashboard on the original local snapshot/events.
- [ ] Run the targeted unit test and `npm run typecheck`; expect both to pass. Run `npm run test:e2e -- tests/e2e/factory.spec.ts tests/e2e/motion.spec.ts tests/e2e/presentation.spec.ts`; expect all existing tests to pass.

## Task 2: W3 snapshot and event projection

**Files:** `src/twin/types.ts`, `src/twin/projection.ts`, `tests/unit/twin-projection.test.ts`, fixtures derived from `contracts/fixtures/` and `backend/internal/twin/store.go`.

**Interfaces:** Export `parseSnapshot(value: unknown, runId: string): RemoteProjection | null` and `applyMessage(current: RemoteProjection, message: unknown): { kind: 'applied'; projection: RemoteProjection } | { kind: 'resync'; reason: string }`. The projection retains epoch, revision, lastSequence, simTimeMs, integrity/reason, W3 machine/board/work-order fields and transition metadata. Validate incoming messages at the boundary, including event type/payload and runId.

- [ ] Test snapshot with 7 machines, multiple boards, `in_transit`, completed, empty/unknown state, distinct observed/flow state and alarms; assert no unknown machine is silently mapped onto a different station.
- [ ] Test each of the nine event types against the backend reducer: workorder start/completion, machine state/alarm changes, board entry/exit/process/inspection; assert `lastSequence` and `revision` advance only for one valid delta, and inspection/processed fields follow W3 semantics.
- [ ] Test delta before snapshot, malformed messages, run mismatch, epoch change, missing/duplicate/reordered revisions or event sequence, invalid board transition, and a `status` without revision; invalid data requests resync, status changes only integrity/reason.
- [ ] Run `npx vitest run tests/unit/twin-projection.test.ts` and verify failures for missing behavior; implement pure immutable projection and strict validation, using `backend/internal/twin/store.go` as the semantic oracle. Do not claim independent derivation of BLOCKED/STARVED.
- [ ] Replay the same W2 fixture through the frontend reducer and W3 offline-import snapshot; compare machine/board/work-order fields, revision, sequence and simulated time, excluding epoch and ingestion timestamps. Run targeted tests and `npm run typecheck`; expect pass.

## Task 3: WebSocket lifecycle and safe same-origin access

**Files:** `src/twin/remoteClient.ts`, `vite.config.ts`, `deploy/w4/nginx.conf`, `compose.w4.yml`, `tests/unit/remote-client.test.ts`, `README.md`.

**Interfaces:** Export `createRemoteClient(options: { runId: string; onState: (state: RemoteClientState) => void }): { start(): void; stop(): void }`. `RemoteClientState` reports connection (`connecting | connected | disconnected`), last valid projection or null, last received wall time and retry/error reason; integrity remains a separate projection property. Resolve `/ws/runs/${encodeURIComponent(runId)}` from `window.location` with ws/wss; snapshot is the first WS frame, not a REST/WS race.

- [ ] With fake WebSocket and fake timers, test snapshot-first, incremental messages, `resync_required`, malformed/invalid sequence, closed socket, bounded exponential retries (250 ms doubling to 10 s), and new snapshot replacing rather than merging old state. A stale callback from an earlier connection/run cannot update the active state; `stop()` cancels retry and closes socket.
- [ ] Test no message for 15 s marks connection display stale without changing backend integrity or advancing sim time; receipt clears staleness. Explicitly label this an activity timeout, since a legitimately quiet/completed run can also be stale. Use injected clock for deterministic tests.
- [ ] Implement a Vite `/api` and `/ws` reverse proxy to `http://127.0.0.1:8080`, WebSocket upgrade enabled and Host rewritten to backend origin; do not weaken Go `CheckOrigin` or add wildcard CORS. Confirm that rewritten Host and preserved Origin actually satisfy the backend check; configure the proxy to set the upstream Origin consistently with its rewritten Host, only for this local development proxy.
- [ ] Add optional same-origin nginx static/proxy Compose service, host-bound `127.0.0.1:4174`, routing `/api/` and `/ws/` to backend with upgrade headers and backend-matching Host/Origin. Document explicit local-only trust assumptions and static build placement; `docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml config` must validate without replacing existing services.
- [ ] Run `npx vitest run tests/unit/remote-client.test.ts`, `npm run typecheck`, Compose config, and a real browser WS handshake through each proxy; expect pass. Verify a direct cross-origin handshake remains rejected.

## Task 4: Event-driven scene and status lights

**Files:** `src/scene/Factory.tsx`, `src/scene/remoteMotion.ts`, `src/scene/motion.ts`, `tests/unit/remote-motion.test.ts`.

**Interfaces:** Export `nextRemoteVisual(previous: RemoteVisualState, projection: RemoteProjection, nowMs: number): RemoteVisualState` and `resetRemoteVisual(projection: RemoteProjection): RemoteVisualState`. Visual state is keyed by board ID and bounded by currently known non-completed boards; wall time controls only interpolation, never the projection or simulated clock.

- [ ] Test `pcb.exited`/`pcb.entered` pairing as a single continuous visual transfer, including same-sim-time consecutive events, a snapshot already in transit, completion, selection, high-speed burst, epoch change and reconnect. Reset cancels interpolation and rebuilds only from the replacement snapshot; assert no duplicate or ghost IDs.
- [ ] Render `at_station` boards at clearly illustrative stable positions based on board ID; show in-transit boards at the previous station exit when destination is unknown, animate only after an actual adjacent `entered`. Bound each transition to 300 ms wall time and snap to newest authoritative location under burst/replacement; do not invent queue positions, processing progress or exact timing. Ensure instance capacity is at least visible-board count, including zero and quantity absent.
- [ ] Map `FAULT` first, then reported `BLOCKED`/`STARVED`, then reported `RUNNING`/`IDLE`; display unknown when absent. Color tower/status indicator from reported state and retain reason and alarms; do not convert uncertain data to a fabricated OFFLINE machine state. Remote process mechanisms stay still when timing is unavailable.
- [ ] Run `npx vitest run tests/unit/remote-motion.test.ts` and the local motion E2E; expect pass. Inspect scene in browser for seven selectable named station groups and fault/status readability.

## Task 5: Source selection and honest remote UI

**Files:** `src/app/App.tsx`, `src/components/Dashboard.tsx`, `src/styles/simulation.css`, `tests/e2e/remote.spec.ts`.

**Interfaces:** Default mode `local`; explicit accessible source selector offers `local` and `remote`, with a run ID input defaulting to `demo-run`. Source changes unsubscribe old transport; switching remote pauses the local simulation and exits local tour/replay without resetting the local run. Returning local restores its paused state and existing controls; selection and camera remain usable.

- [ ] Add E2E assertions that local cold start makes no remote requests, presentation still works, remote switch makes a WS request, and switching back does not reset local state or create a second scene/camera.
- [ ] Remote mode displays SIMULATED, run ID, connection/stale activity, epoch/revision, integrity/reason, last-received time and observed/flow states separately. Render last valid data as stale on disconnect; `gap`/`conflict` clearly mark frozen projection, `uncertain` is never relabeled complete. A new epoch/empty snapshot replaces old boards.
- [ ] Disable or hide local start/pause/reset/fault/speed/replay/tour and local-only dashboard in remote mode; explain they control only the local demo. Do not show guessed cycle, queue, progress history or KPIs; present W3-reported counts/work orders only where their semantics are explicit. Remote has no implicit local fallback: user may choose local explicitly.
- [ ] Test offline startup, 404/unknown run, timeout, source/run change while connecting, selected board disappearing, keyboard operation and source-specific controls. Run `npm run test:e2e -- tests/e2e/remote.spec.ts` plus all existing E2E; expect pass.

## Task 6: End-to-end acceptance, performance and phase gate

**Files:** `tests/e2e/remote.spec.ts`, `tests/fixtures/w4/`, `docs/W4_REPORT.md`, `README.md`, `docs/ARCHITECTURE.md`, `docs/DECISIONS.md`, `docs/ACCEPTANCE.md`.

- [ ] Use a deterministic local WS fixture server through the same-origin proxy for E2E: exercise a complete 100-board event stream, unique board identity and final zero visible boards; verify fault propagation follows reported states, disconnect/reconnect with different snapshot/epoch removes old boards, and `status` gap/uncertainty remains visible. Include burst and no-ghost assertions; mock time, not render-frame business time.
- [ ] Also run a real integration with the W2 simulator, Mosquitto, W3 backend and browser proxy. Start subscribers before publishing, use a unique run ID and bounded realistic speed; capture final backend integrity, count 100 completed boards, compare displayed machine/board state with snapshot, and test browser-side disconnect/reconnect. If broker drops or backend becomes uncertain, record failure rather than calling it recovered. Exercise the fault scenario with a second unique run.
- [ ] Run `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`, `npm run test:e2e`, `cd backend && go test -race -count=1 ./... && go vet ./...`, `cd simulator && go test -race -count=1 ./... && go vet ./...`, and opt-in W2/W3 MQTT integration tests from README. Record actual outcomes, warnings and any skipped prerequisites.
- [ ] Profile local and remote modes at 1366×768 with the same browser/hardware and a repeatable 100-board workload; report FPS/measurement method, CPU/GPU/browser, network and event rate, plus limitations. No invented 30 FPS pass or target-hardware conclusion from SwiftShader.
- [ ] Update documentation with start/stop commands, same-origin deployment and security limits, source switching, unsupported remote fields, stale semantics and W3 recovery limits. Create `docs/W4_REPORT.md` with each acceptance item and evidence. Stop and request approval for W5; never claim W4 complete if any required acceptance check remains unverified.
