# W6 Durable Replay and Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task after explicit user approval. Steps use checkbox (`- [ ]`) syntax for tracking. Do not commit unless the user authorizes it.

**Goal:** Persist the synthetic W2/W5 event history and projections, provide reproducible historical queries and restart recovery, and measure resilience and performance without claiming lossless transport.

**Architecture:** PostgreSQL is the durable source of accepted events; a single ordered per-run writer validates a candidate against a private copy of the W5 reducer, commits the event and projection checkpoint atomically, then updates the in-memory WebSocket view. Recovery and historical queries use the same reducer, stored run topology and versioned checkpoints; MQTT delivery and browser reconnect remain explicitly separate reliability boundaries.

**Tech Stack:** Go 1.24+, existing Paho/Gorilla backend, PostgreSQL 16, `pgx/v5`, SQL migrations, Docker Compose, React/TypeScript/Vitest/Playwright.

**Spec:** `docs/W2_W6_ROADMAP.md` §3, §9–10; `docs/EVENT_CONTRACT.md`; W5 behavior and limitations in `docs/W5_REPORT.md`. This plan proposes implementation decisions, not claims of existing capabilities.

## Global Constraints

- Read-only, localhost demo; every UI/API indication remains SIMULATED; no real MES, equipment, credentials, proprietary assets or equipment control.
- Preserve default browser-only demo, W4 local/remote switch, W5 trace and order semantics, strict station order, virtual time and existing REST/WS clients.
- Events are identified by `(runId,eventId)` and ordered by `(runId,sequence)`; preserve source `emittedAt`, virtual `simTimeMs` and first backend `ingestedAt` independently.
- Different payload for an existing ID or sequence freezes the run as `conflict`; a missing/out-of-order sequence freezes it as `gap`. Do not silently reorder, skip or assert completeness after an unverified disconnect.
- Postgres failure fails readiness and stops accepting/acknowledging new events; never publish an uncommitted delta. Unknown event or checkpoint versions fail closed.
- Restart restores the committed contiguous prefix, not events absent from PostgreSQL; a missing tail remains undetectable absent trusted source completion evidence.
- No automatic reconstruction from a non-durable W2 publisher: persistent MQTT sessions and PUBACK cannot ensure end-to-end crash recovery. Explicitly report uncertain coverage and fail the no-loss acceptance if actual losses occur.

## Baseline audit (2026-11-09)

- Git baseline `88147f6`; clean before planning. W5 is implemented in `backend/internal/twin/{store,trace}.go`, `backend/internal/mes/fixtures.go`, `backend/internal/api/http.go`, `src/twin/mesClient.ts`, `src/components/RemoteTrace.tsx`. A single store lock makes W5 projection, trace and WS registration consistent, but history and deduplication are RAM-only. Assets are hard-coded in `backend/internal/api/http.go`. `backend/internal/replay/import.go` only imports a complete NDJSON run into memory; it is not time travel or durable recovery.
- MQTT uses a fresh client ID and ordered Paho callbacks (`backend/internal/ingest/mqtt.go`); broker loss marks integrity uncertain. W2 publisher is QoS 1 with one in-flight event, not a durable outbox. Existing WS sends an atomic snapshot then deltas and forces snapshot resync after reconnect.
- Ran `npm run lint`, `npm run typecheck`, `npm test` (38 pass, 1 optional parity skipped), `npm run build` (pass; 1,598.91 kB chunk warning), `npm run test:e2e` (16 pass, 1 optional parity skipped). `go test -race -count=1 ./...` and `go vet ./...` passed in both `backend/` and `simulator/`. W2 publisher and W3 ingest/API opt-in MQTT integration tests passed; Compose W2/W3/W4 config validation passed. These results do not prove W6 or repeat the optional W4 parity run, nor a new full Compose 100-board run.
- W5 prior report records 100 completed, 98 passed, 2 failed for its own stack run; that is historical evidence, not a measurement made during this audit. README's W2-awaiting-acceptance and older W3 descriptions are stale and should not be mistaken for phase status.

## Review Focus

1. Crash after database commit but before RAM/WS publication: recovery yields the committed event exactly once and reconnect gets a fresh epoch/snapshot (Task 3).
2. A duplicate ID with changed content or reused sequence: freeze durably, never double-count, including across restart (Tasks 2–3).
3. Same simulated timestamp, empty range, invalid bounds, unknown run/version: deterministic boundary semantics and explicit errors (Tasks 4–5).
4. Broker loss, queue overflow, database outage or missing last event: status never silently returns to complete without evidence (Tasks 3, 6).
5. Corrupt/incompatible snapshot, changed topology, partial projection or simultaneous subscribers: reject incompatible checkpoint or rebuild safely; no mixed revision views (Tasks 3–4, 6).

## File map and dependency order

1. `compose.w6.yml`, `backend/migrations/`, `backend/internal/persistence/`: local database, versioned schema, transactional repository.
2. `backend/internal/twin/{store,trace}.go`, `backend/internal/ingest/mqtt.go`, `backend/cmd/twin-backend/main.go`: atomic writer, restart and connectivity policy; maintain existing interfaces where practical.
3. `backend/internal/replay/`, `backend/internal/api/http.go`: checkpoint-aware replay and bounded read-only APIs.
4. `src/twin/`, `src/components/`, `tests/e2e/`: historical controls without mutating live state, reconnect regression coverage.
5. `scripts/`, `simulator/scenarios/`, `docs/`, `README.md`: reproducible 1000-board test, metrics, operating and acceptance report.

## Task 1: Versioned PostgreSQL schema and local stack

**Files:** Create `compose.w6.yml`, `backend/migrations/001_w6.sql`, `backend/internal/persistence/{migrate.go,migrate_test.go}`; modify `backend/go.mod`, `backend/go.sum`, backend Dockerfile and README.

**Interfaces:** `persistence.Migrate(ctx context.Context, db *pgxpool.Pool) error`; migrations run at backend startup under an advisory lock, tracked in a schema-version table. Connection uses `DATABASE_URL`; absent URL preserves existing memory-only W3/W5 operation.

- [ ] Write DB integration tests for an empty DB, running migration twice, constraints and incompatible newer schema version; use an isolated temporary database/container, never an existing user database.
- [ ] Run `cd backend && W6_PG_INTEGRATION=1 go test ./internal/persistence -run TestMigration -v`; initially fails.
- [ ] Add tables: `runs` (run ID, immutable ordered topology JSON, integrity/reason and last sequence); `events` (run ID, event ID, sequence, schema version, sim time, source emission time, first ingest time, canonical envelope JSONB; unique run/event and run/sequence); `snapshots` (run, sequence, sim time, format/reducer versions, full checkpoint JSONB); `assets` (machine ID, representation and model URL); `board_trace` and `work_order_projection` (run-scoped revision and derived JSON). Add indexes on `(run_id,sim_time_ms,sequence)` and snapshot sequence. Foreign keys and atomic migration are required.
- [ ] Compose adds PostgreSQL 16 with named volume, health check, internal networking and no host database port by default; configure the W6 backend only when this overlay is used. Seed asset rows idempotently without changing the existing `/api/assets` response. Document local-only credentials, volume persistence, explicit data removal and image/dependency versions.
- [ ] Rerun migration tests and `docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml -f compose.w6.yml config --quiet`; expect pass.

## Task 2: Durable ordered event transaction

**Files:** Create `backend/internal/persistence/{events.go,events_test.go}`; modify `backend/internal/twin/{store,trace}.go` and tests.

**Interfaces:** `Append(ctx context.Context, event contract.Event, ingestedAt time.Time, projection Checkpoint) (AppendResult, error)` serializes per run, compares canonical decoded envelopes on duplicate, and atomically stores accepted event, run integrity and changed trace/order projection. `Checkpoint` includes the full reducer state, topology and trace/order data needed to resume validation, not merely the public `twin.Snapshot`.

- [ ] Test identical duplicates before/after restart, payload-changed event ID, reused sequence, out-of-order sequence, reducer-invalid event, rollback after injected SQL error and two concurrent writers for one run. Assert exactly one accepted event/projection update, durable gap/conflict status, and no published delta on failure.
- [ ] Run `cd backend && W6_PG_INTEGRATION=1 go test ./internal/persistence ./internal/twin -run 'Test(Append|Durable)' -v`; initially fails.
- [ ] Extract a pure candidate reduction from `Store.Apply`; under a per-run serialization boundary derive candidate without changing visible state, lock the run row, verify sequence and uniqueness, commit event plus projections/integrity, then install candidate and publish. For duplicate semantic equality compare canonical decoded event including emission time but excluding ingestion time; preserve the original ingest time. Persist gap/conflict separately in a transaction even when rejecting the offending event.
- [ ] Use one active backend writer per run via a lifetime PostgreSQL advisory lock; refuse startup for a competing writer. Readers never observe uncommitted projections. Explicitly serialize public snapshot registration with commit/publication so no subscriber misses a committed revision.
- [ ] Run targeted tests, `go test -race ./internal/twin ./internal/persistence`; expect pass. Verify W5 trace/order tests still pass unchanged.

## Task 3: Restore checkpoints and crash-safe startup

**Files:** Create `backend/internal/persistence/{restore.go,restore_test.go}`; modify `backend/cmd/twin-backend/main.go`, `backend/internal/twin/store.go`, `backend/internal/ingest/mqtt.go`.

**Interfaces:** `Restore(ctx context.Context, runID string, stations []string, epoch string) (*twin.Store, error)` loads latest compatible checkpoint and applies ordered subsequent events using original ingest timestamps. Checkpoints use explicit format/reducer versions and are written every 500 accepted events and on clean shutdown; append transactions need not wait for periodic checkpoint creation.

- [ ] Test clean restart, forced process termination immediately before commit and immediately after commit/before publication, persisted frozen states, missing/corrupt checkpoint fallback to full log, unsupported event version, topology mismatch, and checkpoint/log mismatch. Assert restored trace/order and business state match committed prefix and new epoch forces WS resync.
- [ ] Run `cd backend && W6_PG_INTEGRATION=1 go test ./internal/persistence ./cmd/twin-backend -run 'Test(Restore|Crash)' -v`; initially fails.
- [ ] Restore before accepting subscriptions or starting ingestion; rebuild private state using stored times, validate contiguous sequence, version and checkpoint compatibility, and only expose it after success. If restored integrity was complete, downgrade to uncertain at startup because W2 cannot prove no missed tail; preserve gap/conflict. Compare topology exactly, refuse mismatch. Keep readiness false on DB loss and refuse uncommitted mutations.
- [ ] Verify Paho acknowledgment behavior experimentally and use manual acknowledgment only after durable commit where supported; on DB failure disconnect without acknowledgment and retry with bounded backoff. Do not equate broker redelivery with guaranteed end-to-end recovery. Persist uncertainty on disconnect/restart and test it. Run targeted crash tests and Go race suite.

## Task 4: Historical replay and range API

**Files:** Create `backend/internal/replay/{historical.go,historical_test.go}` and `backend/internal/api/replay_test.go`; modify `backend/internal/api/http.go`, `backend/internal/persistence/` and `docs/EVENT_CONTRACT.md`.

**Interfaces:** `ReplayAt(ctx context.Context, runID string, simTimeMs int64) (HistoricalSnapshot, error)`; `GET /api/runs/{id}/replay?simTimeMs=N`; `GET /api/runs/{id}/events?fromSimTimeMs=N&toSimTimeMs=N&afterSequence=N&limit=N`.

- [ ] Test replay against full reduction at start, middle and end including multiple events at same time; verify histories and order counts, restored ingest times, frozen/uncertain integrity, and snapshots with/without checkpoints. Assert live state and live WS revision unchanged by queries.
- [ ] Test inclusive time range, stable sequence ordering, pagination with `afterSequence`, max `limit=1000` (default 100), empty results, negative/noninteger/reversed bounds (400), unknown run (404), unsupported versions (422), DB unavailable (503). Require nondecreasing `simTimeMs` in accepted contiguous events so time-based checkpoints are valid; reject regressions as conflict.
- [ ] Run `cd backend && W6_PG_INTEGRATION=1 go test ./internal/replay ./internal/api -run 'Test(Historical|Replay|EventRange)' -v`; initially fails.
- [ ] Implement replay as a private reducer from latest compatible checkpoint whose simulated time is at or before the target, then apply every event at `simTimeMs <= N` in sequence order. Return explicit `mode: historical`, requested time, applied sequence/revision and stored integrity; timestamps beyond the last event return the last known state, never a claim that the source run has ended.
- [ ] Run targeted tests and existing `go test -race ./...`; expect pass. Document endpoint semantics, limits and separate time domains.

## Task 5: Read-only historical remote view

**Files:** Create `src/twin/historicalClient.ts`, `tests/unit/historical-client.test.ts`, `tests/e2e/remote-replay.spec.ts`; modify `src/components/` remote controls and `src/twin/` data-source state.

**Interfaces:** `fetchHistoricalSnapshot(runId: string, simTimeMs: number, signal: AbortSignal): Promise<HistoricalSnapshot>` consumes Task 4; live WS remains connected separately and entering historical mode does not write to the server or local simulation.

- [ ] Unit-test same-time result, invalid/server error, out-of-order response cancellation, run/source switching and return-to-live replacing historical view with latest live snapshot.
- [ ] E2E-test remote historical seek, visible SIMULATED/HISTORICAL and integrity indicators, explicit return to live, reconnect during seek and preservation of local presentation/camera; mock the APIs following existing `tests/e2e/remote-trace.spec.ts` patterns.
- [ ] Run `npx vitest run tests/unit/historical-client.test.ts` and `npx playwright test tests/e2e/remote-replay.spec.ts`; initially fail.
- [ ] Implement a nonnegative simulated-millisecond seek input and read-only seek/return buttons; use abort/generation guards, disable misleading live controls during historical display, and reset visual interpolation when replacing snapshots. Keep W5 trace query point-in-time behavior clearly labeled; do not show a current trace as historical.
- [ ] Rerun both tests plus existing remote and remote-trace E2E; expect pass.

## Task 6: MQTT/WS recovery and explicit repair boundary

**Files:** Modify `backend/internal/ingest/mqtt.go`, `backend/internal/api/`, `src/twin/remoteClient.ts`; create `backend/internal/ingest/w6_integration_test.go` and `tests/e2e/w6-reconnect.spec.ts`; update README.

**Interfaces:** Existing snapshot-first WS protocol remains unchanged. Recovery repair is an operator-driven **offline** validated full-run NDJSON import into a new, unused run ID; never overwrite or silently heal the original frozen run. Use the existing W2 dry-run only as a separately generated authoritative fixture, not evidence that a live run was recovered.

- [ ] Test MQTT disconnect/reconnect with duplicates, broker restart, backend restart, DB outage while delivering, slow WS client and two clients reconnecting. Assert acknowledged persisted events appear once, subscriptions restart with snapshot, and no stale/ghost boards remain. If events are missed, assert uncertain/gap and no claim of recovery; explicitly test a missing tail with no subsequent sequence.
- [ ] Run `cd backend && W6_PG_INTEGRATION=1 W3_MQTT_INTEGRATION=1 go test ./internal/ingest ./internal/api -run TestW6 -v` and `npx playwright test tests/e2e/w6-reconnect.spec.ts`; initially fail.
- [ ] Implement reconnect/backoff and readiness/status handling; keep uncertain after reconnect unless a separately verified complete source import establishes coverage. Extend offline import to validate and atomically populate an unused run, with original event IDs preserved only within the new run envelope generated specifically for that run. Reject a mismatched run ID and partially imported run; do not mutate existing run. Document unavailable automatic gap repair and bounded broker queue limitations.
- [ ] Rerun integration and browser tests. Report exact fault injections and which messages were actually observed, not inferred.

## Task 7: 1000-board load, monitoring, documentation and gate

**Files:** Create `simulator/scenarios/w6-1000.json`, `scripts/w6-load.sh`, `docs/W6_REPORT.md`; modify `backend/internal/api/http.go`, README and `docs/ARCHITECTURE.md`.

**Interfaces:** `GET /api/health` retains existing compatibility and adds DB readiness; expose local-only `GET /api/metrics` as documented JSON counters/gauges: accepted, duplicates, conflicts, gaps, DB errors, last sequence and per-process Go memory. Load script uses a fresh run ID and records elapsed time, event/board counts, ingestion latency distribution, backend RSS and browser FPS; it never removes an existing volume.

- [ ] Test metrics increase exactly once for accepted/duplicate/conflict and health is 503 for DB failure; test a 1000-board seeded scenario with exactly 1000 unique created and terminal boards, no over-capacity, no missing sequence and reconciled MES counts. Run `cd simulator && go test ./internal/engine -run TestW6Thousand -v`; initially fails.
- [ ] Add the 1000-board fixture based on `normal.json`; create reproducible load and restart scripts using an isolated Compose project and unique run ID. Derive latency from backend ingest wall time minus emission wall time, disclose same-host clock assumptions and exclude sim time; report p50/p95/p99, event/s, board/s, peak RSS and measurement duration. Use `scripts/measure-performance.mjs` or a documented equivalent for remote FPS and report renderer, GPU/software, browser, viewport, DPR and hardware. Do not invent a threshold or result.
- [ ] Run the 1000-board live stack, query order/trace, restart backend and compare durable business state and traces, then seek selected timestamps and compare against a full reduction. Record exact commands, start/end times, resource conditions, accepted event counts, lost/duplicate boards and actual measurements in `docs/W6_REPORT.md`.
- [ ] Run `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`, `npm run test:e2e`; in both Go modules run `go test -race -count=1 ./...` and `go vet ./...`; run W2/W3/W6 opt-in integrations and four-file Compose config check. Include failures/skips and the existing bundle warning in the report.
- [ ] Update README/architecture with schema/migration, startup, recovery, API, backup/volume caution, experimental security, assumptions and limitations. Fill an explicit pass/fail acceptance matrix for each W6 criterion; if transport loss, unavailable metrics or environment prevents proof, mark the criterion unverified or failed, never passed. Stop and request user approval before any further phase.
