# W6 offline recovery report

Status: **implementation and measurements complete; awaiting user acceptance.** All
data below is SIMULATED; there is no real equipment, PLC or MES integration.
One deviation is recorded: the 1000-board run used speed 100 instead of 20x
(recovery is bound by PostgreSQL write throughput, so the faster run is the
harsher burst). The simulator does not republish committed-but-unpublished
boundaries on `--resume`; the backend recovers them from the source API.

## What was built

- Simulator SQLite durable source (`simulator/internal/outbox`): one transaction
  per virtual boundary stores the exact event envelopes, the post-`Next()`
  engine checkpoint and, for the final boundary, a terminal record with the
  ordered SHA-256 digest. Quota is enforced by `max_page_count`
  (`quota + 1 MiB` schema reserve) and `journal_size_limit`
  (`max(quota/8, 64 KiB)`); the WAL is truncated when it grows past half that
  limit. Capacity and storage failures pause generation, restore the last
  committed checkpoint and never publish an uncommitted batch.
- Engine checkpoint (`simulator/internal/engine/checkpoint.go`): versioned,
  checksummed, includes every mutable field (snapshot/queues, sequence, started,
  PRNG, fault index, failed boards) plus run/seed/scenario identity. Restore
  rejects mismatched identity, unknown versions and impossible states.
- Source API (`simulator/internal/outbox/service.go`, `handler.go`): versioned
  `manifest`, exact-byte pages (`after`/`limit`, 1 MiB budget) and a
  generation-scoped monotonic acknowledgement cursor. No host port in Compose;
  the server binds loopback by default.
- Backend recovery (`backend/internal/recovery`): verifies protocol version,
  run identity, source generation and scenario digest, catches up from the
  committed prefix, re-reads all retained source envelopes to recompute the
  terminal digest, compares the already-committed prefix against stored backend
  fingerprints, requires `workorder.completed` as the last event, then promotes
  completion and acknowledges. MQTT is treated only as a wake-up path.
- Live-stream policy: while recovery is active, events are rejected with
  `twin.ErrRecovering` and not acknowledged; with a durable source configured,
  gap/out-of-order live messages are deferred for source recovery instead of
  terminating ingestion. Conflicts stay frozen and are never thawed.
- WebSocket resync: promotion publishes a `snapshot` update with a new epoch and
  `terminalVerified`; the browser client installs a mid-stream snapshot.

## Acceptance matrix

| Criterion | Result | Evidence |
| --- | --- | --- |
| Crash before/after SQLite commit | Verified | `TestDurableSIGKILLRecoversFromCommittedBoundary` (SIGKILL subprocess, both modes) |
| Real disk full | Verified on isolated filesystem | `TestStoreDiskFullPreservesPrefix` on a 1 MiB tmpfs; prefix unchanged, `ErrStorageUnavailable` |
| Concurrent page/head consistency | Verified | `TestStoreConcurrentPageNeverExceedsReportedHead` |
| Backend offline through source completion, then restart | Verified | `tests/integration/w6-recovery.sh`, project `w6-e2e-script-20261110` |
| 100 unique boards, no duplicates | Verified | snapshot 100/100 completed; PostgreSQL 3411 rows, 3411 distinct event IDs |
| DB and source sequence equality | Verified | source head 3411; DB sequences 1..3411; `complete` + `terminalVerified` |
| Verified terminal digest | Verified | coordinator recomputes the digest and compares the committed prefix |
| Live messages during recovery | Verified by E2E | deferred without acknowledgement; run repaired from source |
| 1000 boards | Verified at 100x (harsher than 20x) | project `w6-e2e-1000-20261110`: 34911 source events, 1000/1000 boards, 34911 distinct DB rows, `complete` |
| 100000x burst with backend online | Verified | project `w6-e2e-burst-20261110`: 20 live events deferred while recovering, 100/100 boards, 3411 verified events |
| Playwright recovery scenario | Verified | `tests/e2e/w6-recovery.spec.ts` (mid-stream new-epoch snapshot, no reconnect) |
| Lint, typecheck, build, unit | Verified | `npm run lint`, `npm run typecheck`, `npm run build`, `npm test` (45 passed, 1 skipped) |
| Full Playwright suite | Verified | `npm run test:e2e` → 20 passed, 1 skipped (W4 fixture gate) |

## Measured evidence

Project `w6-e2e-script-20261110` (`docker compose ... up`, volumes retained):

- simulator source manifest: `head=3411`, terminal `eventCount=3411`,
  digest `05d7c409…8e1f39`, generation `f8e8c4e0…9bb2`.
- backend snapshot after restart: `integrity=complete`,
  `terminalVerified=true`, `lastSequence=3411`, epoch `…-r3411`,
  100 boards all `completed`, work order `completed` with 98 passed / 2 failed.
- PostgreSQL: `count(*)=3411`, `min(sequence)=1`, `max(sequence)=3411`,
  `count(distinct event_id)=3411`.
- After the independent review fixes (terminal-required mode, prefix-first
  verification, no live deltas during recovery), the same script passed again
  on project `w6-e2e-postreview-20261110`.

The first run of the same scenario (project `w6-e2e-20261110`) exposed two
defects that are now fixed: deferred live messages caused a fatal sequence gap,
and neither the in-memory nor the PostgreSQL writer would extend a frozen gap
during verified recovery.

## Known limitations

- Single backend writer: recovery serialization is in-process. Running two
  backend instances against one run is not supported.
- Compose does not set `W6_SOURCE_GENERATION` / `W6_SCENARIO_DIGEST`, so the
  backend accepts the manifest's own identity claim in that deployment. Pass
  the values explicitly for strict identity checking.
- `TerminalVerified` is in-memory only. After a restart the coordinator
  re-verifies the source before promoting again; the epoch changes each time.
- Source events are retained; acknowledgements move a cursor and never delete
  data. Reaching the quota requires operator action.
- Acknowledged MQTT delivery is never treated as backend persistence.
- Physical capacity is bounded per SQLite database, not per Docker volume; treat
  the quota as an application limit, not a filesystem guarantee.
- Disk-loss recovery, TLS/authentication of the source API and multi-tenant
  security are explicit non-goals for this local demo.

## Reproduce

```bash
cd tests/integration
W6_PROJECT=w6-e2e-$(date +%s) ./w6-recovery.sh
```

The script builds the Compose stack, starts the simulator with `--no-deps` so
the backend has never run, waits for the terminal boundary, starts the backend
and asserts completeness, uniqueness and sequence equality. It stops the
project at the end and retains its volumes.

1000-board soak (speed 100):

```bash
W6_PROJECT=w6-e2e-1000-$(date +%s) W6_BOARDS=1000 W6_TIMEOUT=1500 \
W6_COMPOSE_FILES="compose.w2.yml,compose.w3.yml,compose.w6.yml,compose.w6-recovery.yml,compose.w6-soak1000.yml" \
./w6-recovery.sh
```

100000x live burst (backend online throughout):

```bash
W6_LIVE=1 W6_PROJECT=w6-e2e-burst-$(date +%s) W6_TIMEOUT=600 \
W6_COMPOSE_FILES="compose.w2.yml,compose.w3.yml,compose.w6.yml,compose.w6-recovery.yml,compose.w6-burst.yml" \
./w6-recovery.sh
```

Unit and race checks:

```bash
(cd simulator && go test -race -count=1 ./... && go vet ./...)
(cd backend && go test -race -count=1 ./... && go vet ./...)
npm test
```
