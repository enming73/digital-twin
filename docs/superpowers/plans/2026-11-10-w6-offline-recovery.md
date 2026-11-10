# W6 Offline Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task only after user approval. Checkboxes track work. Do not commit, push, or merge without separate authorization.

**Goal:** Recover an ordered synthetic run after the backend misses MQTT events, including a missing tail, using a persistent source log and a verified terminal boundary.

**Architecture:** A Go simulator stores each complete virtual-time boundary and its post-boundary engine state in a SQLite transaction before offering immutable envelopes over MQTT or a local read-only paginated source API. A backend recovery coordinator is the sole per-run writer: it catches up from the committed PostgreSQL prefix, verifies the source terminal boundary and identity, then atomically replaces the in-memory projection and resynchronizes WebSocket clients. MQTT remains a best-effort wake-up path, not proof of delivery.

**Tech Stack:** Existing Go 1.24 simulator/backend, PostgreSQL, Paho MQTT, Docker Compose, Vitest/Playwright; SQLite using `modernc.org/sqlite` (pure Go, pin a Go-1.24-compatible release only after verification).

**Spec:** `docs/W6_RECOVERY_DESIGN.md`; W6 contracts and boundaries in `docs/W2_W6_ROADMAP.md` and `docs/EVENT_CONTRACT.md`.

## Global Constraints

- Simulator, source API and recovery are optional; the browser-only demo remains the default, with no mandatory backend or paid service. Everything remains SIMULATED and read-only to external consumers; no real equipment, PLC or MES.
- User confirmed the capacity policy: pause generation on a full/unavailable durable store; never silently discard or overwrite events. Default storage limit is 256 MiB per source volume, configurable; retain acknowledged events in this phase, so reaching the limit requires operator action, not automatic deletion.
- Treat event identity as `(runId,eventId)` and sequence as a unique contiguous per-run integer starting at 1. Persist exact serialized envelope bytes, including first `emittedAt`; all retries use those bytes. Never equate MQTT PUBACK with backend commit.
- A new generation boundary is externally visible only after the **whole event batch and post-`Next()` engine state** commit atomically. No process may publish or serve a partial batch. Unknown versions, corruption, topology/config mismatch and source identity mismatch fail closed.
- Existing frozen `conflict` is never auto-cleared. A previously detected `gap` is repaired only through an explicit recovery transition after verification; clients see uncertain/recovering status and receive an atomic new epoch snapshot, never an unannounced status flip.
- Integrity `complete` requires a durable terminal control record with the final sequence and a verified contiguous committed prefix through that sequence; absent source, absent marker, queue/disk failure or missing tail remain uncertain/gap. This is a local synthetic evidence claim, not a cryptographic security or production guarantee.
- Source API is internal-only in Compose, binds `127.0.0.1` standalone, has no host port in Compose, and never exposes SQLite or write access to the browser. Compose credentials and run identity are local demo controls, not authentication or TLS.
- Preserve user data; new Compose projects use unique names and named volumes. No deletion/reset of existing volumes or automatic cleanup of durable logs. Keep explicit diagnostic projects and expected logs until the user chooses cleanup.

## Review Focus

1. Crash between `Engine.Next()` mutation and transaction commit: restart must replay the previous committed state without missing or duplicating an event (Task 2).
2. A stopped source after its final publish but before backend commit: missing tail must recover and completion may be declared only after verified terminal boundary (Tasks 3–5).
3. Duplicate, stale, conflicting and reordered live messages during catch-up: no concurrent writer, no extra board, no silent thaw of conflict (Tasks 4–5).
4. Disk full, quota reached, missing source or malformed/unknown-version responses: generation pauses, health explains why, backend stays uncertain and no uncommitted sequence is exposed (Tasks 1–5).
5. Wrong run, different scenario/seed, stale source generation, malformed checkpoint or terminal digest: reject recovery rather than combining streams; WS clients receive a fresh epoch and snapshot only after verified installation (Tasks 2–5).

## File map

- `simulator/internal/outbox/{store,service,recovery}_*.go`: SQLite schema/migrations, transactional batches, reads, terminal metadata and quota.
- `simulator/internal/engine/{engine,checkpoint}.go`: versioned complete state export/import, including PRNG, failure map, fault index, queues and config identity.
- `simulator/internal/runner/runner.go`, `simulator/cmd/simulator/main.go`: commit-before-publish boundary, resume, source serving and lifecycle.
- `backend/internal/recovery/{client,coordinator}.go`, `backend/internal/twin/store.go`, `backend/internal/ingest/mqtt.go`, `backend/cmd/twin-backend/main.go`: verified catch-up, serialization, integrity transitions and client resync.
- `compose.w6-recovery.yml`, `simulator/Dockerfile`, `backend/Dockerfile`, `docs/EVENT_CONTRACT.md`, `README.md`, `docs/W6_RECOVERY_REPORT.md`: isolated deployment, versioned protocol and evidence.

## Task 1 — Durable source store and capacity policy

**Files:** Create `simulator/internal/outbox/store.go`, `simulator/internal/outbox/store_test.go`; modify `simulator/go.mod`, `simulator/go.sum`, `simulator/Dockerfile`.

**Interfaces:** `Open(path string, maxBytes int64) (*Store, error)`; `CommitBoundary(ctx context.Context, runID string, events [][]byte, state []byte, terminal *Terminal) error`; `ReadPage(ctx context.Context, runID string, after int64, limit int) (Page, error)`; `Close() error`. `Terminal` contains run ID, final sequence, event-count and SHA-256 of the ordered length-prefixed exact envelopes; `Page` returns at most 500 ordered envelopes plus durable head and optional terminal.

- [ ] Test fresh database migration and reopening; batch atomicity, duplicate same bytes, conflicting same ID/sequence, invalid sequence and crash rollback; assert no partial event batch or checkpoint is readable.
- [ ] Test 256 MiB default and an injected small quota: a boundary that would exceed it returns `ErrCapacity` without changing the prior durable head; inject write failure and disk-full errors and assert `ErrStorageUnavailable`. Verify closing does not delete data.
- [ ] Run `cd simulator && go test ./internal/outbox -run 'Test(Store|Quota|Atomic)' -v`; expect missing package/tests to fail before implementation.
- [ ] Implement SQLite transaction using a single writer, schema version table, unique run/ID and run/sequence, stored raw BLOBs, terminal in the same transaction and explicit quota accounting including state and metadata. Refuse unknown DB versions. Enable WAL and verify durability settings; checkpoint/flush before reporting a committed batch.
- [ ] Run the command again and `go test -race ./...` in `simulator/`; expect pass. Verify the Alpine `CGO_ENABLED=0` simulator image builds with the pinned SQLite dependency.

## Task 2 — Engine checkpoint and atomic virtual boundary

**Files:** Create `simulator/internal/engine/checkpoint.go`, `simulator/internal/engine/checkpoint_test.go`, `simulator/internal/runner/durable_test.go`; modify `simulator/internal/engine/engine.go`, `simulator/internal/runner/runner.go`, `simulator/cmd/simulator/main.go`.

**Interfaces:** `(*engine.Engine).ExportState() ([]byte, error)`; `engine.RestoreState(config scenario.Scenario, seed uint32, runID string, data []byte) (*engine.Engine, error)`; `runner.RunDurable(ctx context.Context, engine *engine.Engine, outbox *outbox.Store, sink publisher.Sink, speed float64, toggle <-chan struct{}) error`.

- [ ] Test interrupted runs at boundaries before/after durable commit and within batches containing multiple same-time events. Restart must produce the same remaining event IDs, sequences, payloads and virtual times as uninterrupted execution, including faults and inspection randomness; exact stored envelopes must survive retries unchanged.
- [ ] Test rejection of changed run ID, scenario/seed, topology, checkpoint version, checksum and corrupted queue/PRNG/fault state. Verify a quota failure does not publish the proposed batch and generation resumes from the previous committed boundary after storage is available.
- [ ] Run `cd simulator && go test ./internal/engine ./internal/runner -run 'Test(Durable|Checkpoint)' -v`; expect fail before implementation.
- [ ] Implement versioned checkpoint containing **all** mutable `Engine` fields (`snapshot`, `sequence`, `started`, `random`, `faultIndex`, `failed`) and canonical scenario/seed/version identity. Clone state before `Next()`, assign emission times once, validate and serialize every event in its returned batch, then commit batch + post-state + terminal atomically. On capacity/storage failure restore pre-boundary state, expose a paused/unavailable status and wait for operator resolution without generating further events; cancellation must stop cleanly.
- [ ] Add `--outbox-path`, `--outbox-max-bytes`, and `--resume` with explicit rejection of an existing run unless resume is requested. Preserve current broker-free `--dry-run` and non-durable mode. After the terminal batch, keep the source service available until explicitly stopped; publishing may be retried, never change stored bytes.
- [ ] Rerun targeted tests and `go test -race ./...`; expect pass. Inject SIGKILL in a subprocess before and after SQLite commit; compare durable run to the uninterrupted fixture excluding only independently generated wall-clock fields.

## Task 3 — Versioned source read and acknowledgement protocol

**Files:** Create `simulator/internal/outbox/service.go`, `simulator/internal/outbox/service_test.go`; modify `simulator/cmd/simulator/main.go`, `docs/EVENT_CONTRACT.md`, `compose.w6-recovery.yml`.

**Interfaces:** `GET /v1/runs/{runId}/manifest` returns `protocolVersion`, run ID, source generation ID, scenario digest, head, terminal or null; `GET /v1/runs/{runId}/events?after=N&limit=L` returns ordered exact envelopes, with `1 <= L <= 500`; `POST /v1/runs/{runId}/ack` accepts a monotonically increasing backend durable sequence and matching source generation ID. `POST` is backend-only control-plane acknowledgement, never an equipment command; this phase persists acknowledgements but does not delete events.

- [ ] Test paging, empty page, inclusive boundary behavior, 400 for invalid bounds, 404 for unknown run, unsupported version, stale generation, invalid and decreasing/beyond-head acknowledgements, concurrent append/read, source restart, and a terminal record readable after MQTT has gone quiet.
- [ ] Run `cd simulator && go test ./internal/outbox -run TestSourceAPI -v`; expect fail before implementation.
- [ ] Implement bounded HTTP server with request/read deadlines, 1 MiB response/body limits, localhost default and Compose-internal listen override; each page stops before either the event-count or byte limit, and an individual oversized event is an explicit error. Reject any unknown generation or malformed payload. Document the source API and threat model. Source loss returns unavailable, never an empty completed run.
- [ ] Add a named simulator outbox volume to `compose.w6-recovery.yml` with no host-published source port. Keep the old Compose files and defaults working without the overlay. Run targeted tests and Compose `config --quiet`.

## Task 4 — Backend recovery coordinator and freeze policy

**Files:** Create `backend/internal/recovery/client.go`, `backend/internal/recovery/coordinator.go`, `backend/internal/recovery/coordinator_test.go`; modify `backend/internal/twin/store.go`, `backend/internal/persistence/{events,restore}.go`, `backend/internal/ingest/mqtt.go`, `backend/cmd/twin-backend/main.go`.

**Interfaces:** `Source.Manifest(ctx, runID) (Manifest, error)`, `Source.ReadPage(ctx, runID string, after int64, limit int) (Page, error)`, `Source.Ack(ctx, runID, generation string, sequence int64) error`; `Coordinator.Recover(ctx context.Context, runID string) error`. The coordinator owns a per-run recovery lock shared with live application; source HTTP requests have a 5 s deadline and bounded retry (250 ms to 5 s, cancelable).

- [ ] Test the reproduced 536-of-3411-style tail: source has terminal N, database has committed prefix K, no new MQTT message arrives; recovery applies exactly K+1..N, preserves exact event identity and verifies the completion event. Test restart after backend commit but before source ack: replay duplicates are idempotent and acknowledgement advances only after commit.
- [ ] Test missing or unreachable source, incomplete terminal, wrong generation/scenario, malformed response, gap, duplicate with altered content, DB transaction failure, page overlap, out-of-order live messages and simultaneous recovery calls. Assert `complete` is never claimed without proof and `conflict` is never automatically thawed.
- [ ] Run `cd backend && go test ./internal/recovery ./internal/twin -run 'Test(Recover|Recovery)' -v`; expect fail before implementation.
- [ ] Implement cursor-based catch-up using source pages and existing transactional reducer. For gap repair, build a private verified projection from the persisted prefix plus fetched missing events; commit only the verified contiguous extension, record an explicit recovery transition, then install with a new epoch under the same lock used for snapshot/subscriber registration. While recovering, reject or boundedly buffer live messages; a full buffer fails closed without acknowledgement.
- [ ] Validate terminal event count, final sequence, full ordered byte digest and source generation before promoting completeness: recompute the byte digest from all retained source envelopes (PostgreSQL JSONB is not byte-preserving), and compare each decoded source envelope to its stored backend event fingerprint. An already stored conflict stays frozen. If source is unreachable, retain current projection and `uncertain`/`gap`, return a visible reason, and retry within the bounded backoff. Acknowledgement failures never roll back a committed backend event; repeated acknowledgements are safe.
- [ ] Run targeted tests and `go test -race ./...` in `backend/`; expect pass. Existing W3/W5 REST and WS protocol tests must remain green.

## Task 5 — End-to-end recovery and no-regression gate

**Files:** Create `backend/internal/recovery/integration_test.go`, `tests/e2e/w6-recovery.spec.ts`, `docs/W6_RECOVERY_REPORT.md`; modify `README.md`, `docs/ARCHITECTURE.md`, `compose.w6-recovery.yml`.

**Interfaces:** Existing `/api/runs/{id}/snapshot`, `/ws/runs/{id}`, trace and order endpoints retain their schemas; recovery status uses the existing `integrity`/`reason` fields and snapshot-first WS resync. All recovery-enabled data remains SIMULATED.

- [ ] Add opt-in isolated Compose tests for backend offline through source completion, broker restart with source available, consumer crash before/after DB commit, simulator crash before/after atomic boundary commit, and reconnecting clients. Assert 100 unique boards, no duplicates, terminal count and DB/source sequence/digest equality; show no ghost PCBs and a new WS epoch after catch-up.
- [ ] Test queue/disk exhaustion and source loss: source pauses, no silent discards, backend does not become complete. Test 1000 boards at the documented 20× workload and the previous 100000× burst; if a bounded backlog fills, prove backpressure or explicit non-completion rather than claiming universal losslessness.
- [ ] Run `cd backend && W6_RECOVERY_INTEGRATION=1 go test -race -count=1 ./internal/recovery -v`; run `npx playwright test tests/e2e/w6-recovery.spec.ts`. Expect new tests to fail before implementation and pass after it. Record exact project names, IDs, observed counts, dropped messages, interruption windows and cleanup commands. Do not delete test volumes automatically unless each test itself created and owns them and cleanup is explicitly scoped.
- [ ] Run `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`, `npm run test:e2e`; run `go test -race -count=1 ./...` and `go vet ./...` in both Go modules; run W2/W3/W6 opt-in integrations and `docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w6.yml -f compose.w6-recovery.yml config --quiet`. Record every failure/skip and the existing bundle warning.
- [ ] Write `docs/W6_RECOVERY_REPORT.md` with acceptance matrix, measured results, wall-clock/virtual-time distinction, capacity policy, known limitations, independent-review status and exact reproduction commands. Update README/architecture and document source volume retention, startup order, resume versus new run and read-only security boundaries. Stop for user review; do not claim acceptance if any criterion is unverified.

## Self-review and execution gate

- Coverage: source durability/quota → Tasks 1–2; versioned source and terminal → Task 3; catch-up, conflict and resync → Task 4; crash/reconnect/1000-board and documentation → Task 5.
- Design decision requiring verification before implementation: SQLite driver/version and compatibility with the existing Alpine/CGO-disabled image. If no suitable driver is available, stop rather than silently replacing the agreed stack or weakening atomicity.
- This plan proposes local internal HTTP for source access; it is not authenticated against a malicious local actor. Production security and disk-loss recovery remain explicit non-goals.
- **STOP: do not implement until the user approves this plan.**
