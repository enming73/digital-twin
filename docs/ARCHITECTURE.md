# Architecture

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
