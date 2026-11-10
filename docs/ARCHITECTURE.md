# Architecture

## W6 experimental persistence (in progress)

When `DATABASE_URL` is set, the backend migrates PostgreSQL, obtains a session-owned per-run writer lock, restores the committed prefix, then subscribes to MQTT. The store serializes reducer validation, transactional event/projection persistence and WebSocket publication; durable mode manually acknowledges a received MQTT message only after commit. `GET /api/health` additionally checks the lock-owning database connection. A durable ingestion error stops the backend; recovery of events that never reached the database is not guaranteed. Disconnect/restart makes source coverage uncertain, while stored gaps and conflicts stay frozen.

Read-only `/api/runs/{id}/replay` reconstructs an independent state from the event prefix and `/api/runs/{id}/events` returns a bounded time range. The remote UI renders historical state independently of the still-running live WebSocket and hides live MES trace details during history. A versioned checkpoint is written atomically every 500 events and once more after a clean consumer shutdown; only the three newest per run are retained. Checkpoints index each event by SHA-256 fingerprint and are verified in one ordered scan against the committed prefix, trace entries and persisted board/work-order projections before the suffix is reduced. Corrupt or incompatible checkpoints fall back to full replay, and a damaged checkpoint at the committed cursor is replaced on the next clean shutdown. Durable ingestion ignores events for unregistered runs and exits non-zero on storage failure. The 100000× 1000-board burst test exceeded the broker queue and failed; see `docs/W6_REPORT.md`. This stack remains local, synthetic and unauthenticated.

## W5 mock MES

`backend/internal/twin/` updates a per-board in-memory event trace inside the same lock as the W3 snapshot, after reducer validation and deduplication. The `backend/internal/mes/` fixture supplies optional descriptive metadata only; the simulator event stream remains authoritative. Read-only same-origin REST queries expose point-in-time board traces and order counts with epoch, revision and integrity. `src/components/RemoteTrace.tsx` cancels obsolete requests and keeps the W4 connection/integrity display separate. There is no durable history, automatic repair or real MES.

## W4 optional remote display

The local simulator remains the default. `src/twin/` translates local snapshots to a neutral scene model and separately validates/reduces W3 snapshot-first WebSocket messages by epoch, revision and sequence. `remoteClient.ts` reconnects with capped backoff and replaces the projection on a new snapshot; its connection/activity state is separate from backend integrity. `src/scene/remoteMotion.ts` uses wall time only to interpolate observed board transfers and never changes the projection. W3's lack of process-start and queue data prevents reconstructing precise equipment motion. The scene uses the standard configured station order, not JSON map order; a nonstandard backend topology is unsupported and not discoverable from W3 snapshots.

Vite and the optional nginx service reverse-proxy same-origin browser requests. Both validate allowed browser WebSocket origins before changing the upstream Origin; W3's own same-origin check remains in place. This is an unauthenticated localhost concept demo, not a production trust or security model.

## Browser demo

The independent React/R3F browser demo owns its existing TypeScript virtual-time simulator, in-memory event log, replay reducer and presentation UI. It does not depend on the W2 process or a broker. Its known visual and replay limitations are in `README.md` and `docs/ACCEPTANCE.md`.

## W2 independent simulator

```
scenario + seed -> Go virtual-time engine -> ordered events -> NDJSON OR MQTT QoS 1 -> subscriber
```

`simulator/internal/scenario` validates line configuration; `engine` controls board ownership and virtual boundaries independently of wall time; `runner` paces those boundaries; `publisher` stamps wall-clock emission time and serializes the versioned synthetic envelope. `contracts/event.schema.json` and `docs/EVENT_CONTRACT.md` describe the external format. Publisher calls block until PUBACK: this is a bounded one-event in-flight queue and applies backpressure rather than dropping events. A failed acknowledgement retries the same serialized bytes and ID. Duplicates are possible; events and pending work are not durable across process crashes. On shutdown, the runner attempts to finish the current publish for up to three seconds and reports an error if the run is interrupted; remaining generated events in the batch are not claimed delivered.

For local development Compose exposes Mosquitto only at `127.0.0.1:1883`; anonymous clients inside its network are allowed. This is not production authentication, encryption or network security. W3 will own consumer validation/deduplication, gap detection and ingestion timestamps; W4 may add an optional UI data source, while the browser local demo remains available. No backend, browser/MQTT integration or live equipment is implemented in W2.
# W3 experimental backend

`backend/` is a separate Go module. The canonical JSON schema remains `contracts/event.schema.json`; Docker copies it into the image, and standalone execution resolves it relative to `backend/` or via `TWIN_SCHEMA_PATH`. MQTT is read-only, non-retained and QoS 1. Validated events enter a per-run in-memory reducer with `(runId,eventId)` deduplication and strict contiguous sequence. The observed machine state and simulator-derived flow state/reason are separate. The backend projects board station ownership and transit, not unreported process starts or queue details.

Snapshot creation and WebSocket registration share a lock with event application. Deltas carry epoch/revision; reconnect always starts with a new snapshot. Gap and conflicting events freeze projection. Broker loss and invalid input mark completeness uncertain; loss cannot be repaired automatically because W2 does not retain history. Initial completeness is unknown until sequence 1 and is only established when a fresh publisher is explicitly asserted. A full ordered offline NDJSON import builds a fresh in-memory store before exposing it. `emittedAt`, virtual `simTimeMs`, and backend `ingestedAt` are distinct. No database, durable event history, remote browser integration, real equipment, or MES is provided.
