# Synthetic SMT event contract v1

## W5 synthetic read-only query contract

`GET /api/runs/{runId}/boards/{boardId}/trace` returns `{runId, epoch, revision, integrity, reason?, synthetic: true, board, route, entries}`. Each entry contains `sequence`, `eventId`, `eventType`, `machineId`, `simTimeMs`, `emittedAt`, `ingestedAt` and, for inspection events, `passed`. Sequence is the ordering key, including when virtual timestamps coincide. Only successfully applied events are included. The route is configured intent, not proof of a visit.

`GET /api/runs/{runId}/workorders/{workOrderId}` returns the same envelope and route, plus `order`, nullable synthetic `metadata`, `progress` (`planned`, `created`, `inProgress`, `completed`, `passed`, `failed`, `uninspected`) and `reconciliation` (`pending`, `matched`, `mismatch`, or `unavailable`). Counts classify completed boards: failed means any recorded failed inspection; passed means one or more recorded inspections and none failed; uninspected means none recorded. Reconciliation is unavailable when any completed board is uninspected. A failed inspection does not imply scrap.

Unknown run or entity returns 404. Identifiers are scoped to their run. Query data and integrity are atomically captured at the returned revision; resubmit the query for updates. History is in-memory only and does not heal loss. Gaps/conflicts freeze application, and uncertain or incomplete history must not be treated as complete; even `complete` is not a delivery guarantee. No rework, skipped stations, or repeated inspections are supported.

This contract is for the independent simulator only. All events represent **SIMULATED** data; they do not indicate real equipment or production. The JSON Schema is `contracts/event.schema.json`; a sample for each of the nine event types is in `contracts/fixtures/`.

The W3 backend consumes this same v1 contract; see `README.md` for its separate snapshot/WS protocol and conservative completeness rules. It records ingestion wall time separately from `emittedAt` and `simTimeMs` and never interprets either as measured equipment time.

Each envelope contains `schemaVersion`, `eventId`, `runId`, `source`, nullable `machineId`, `eventType`, per-run `sequence`, virtual `simTimeMs`, UTC `emittedAt`, and `payload`. `source` is `simulator`, every payload includes `synthetic: true`, and `emittedAt` is wall time rather than virtual time. Work-order events have null machine IDs; other events have machine IDs. Inspection events originate at `spi` or `aoi`. Work-order completion additionally requires passed + failed = quantity; this arithmetic rule is enforced in Go because JSON Schema cannot express it.

Topics are `demo/smt/{runId}/machines/{machineId}/events` and `demo/smt/{runId}/line/events`; topic identifiers cannot contain `/`, `+` or `#`. Events are not retained. MQTT QoS 1 is at-least-once: consumers must deduplicate by `(runId,eventId)` and detect missing or out-of-order global sequence numbers. Retrying a publication must preserve the entire serialized event. The W6 durable source protocol below provides ordered catch-up; MQTT alone is never proof of persistence.

`pcb.entered` means a station takes ownership in its input queue. `pcb.process.completed` occurs before `pcb.exited`; exit means leaving the processing/holding slot, immediately before the next station's entered event. Inspection follows process completion and precedes exit. Work-order start precedes board events; completion follows final exit. Observed machine status (`IDLE`, `RUNNING`, `FAULT`) is distinct from derived flow status (`NORMAL`, `BLOCKED`, `STARVED`), which includes a reason.

## W6 durable source protocol v1 (internal)

When the simulator runs with `--outbox-path`, each virtual boundary is committed to SQLite before any MQTT publication: the exact serialized envelopes, the post-`Next()` engine checkpoint, and the optional terminal record. Envelopes are immutable once committed; retries must reuse the stored bytes.

`--source-addr` exposes a read-only HTTP API (disabled by default; Compose binds it inside the network with no host port). All responses are JSON and all data is **SIMULATED**. It never serves SQLite files and accepts no equipment commands.

`GET /v1/runs/{runId}/manifest` returns `protocolVersion` (1), `runId`, `sourceGenerationId`, `scenarioDigest`, durable `head` and `terminal` (or null). `sourceGenerationId` is derived from the run ID and canonical scenario digest, so a changed scenario yields a different generation. Unknown runs return 404.

`GET /v1/runs/{runId}/events?after=N&limit=L` returns `{protocolVersion, runId, head, terminal, events}` where `events` are the exact stored envelopes with `sequence > N`, in ascending order. `0 <= N`; `1 <= L <= 500`. A page is truncated before the 1 MiB response budget; a single envelope that cannot fit returns 413 rather than being split. Invalid bounds return 400 and unknown runs 404. Clients must not treat a non-terminal head as complete.

`POST /v1/runs/{runId}/ack` accepts `{protocolVersion:1, generation, sequence}` and returns `{acknowledged}`. It is a backend control-plane acknowledgement of a durably committed contiguous prefix, not an equipment command and not proof that source events were delivered. Acknowledgements must match the registered generation, must not exceed `head`, and must be monotonically non-decreasing; violations return 409. This phase persists the cursor and never deletes source events.

Protocol requests may include `?protocolVersion=1`; any other version returns 400. MQTT remains a best-effort notification path: broker delivery, QoS 1 and PUBACK are never treated as evidence of backend persistence.

## Browser event migration mapping

The browser remains unchanged. Its `BOARD_CREATED` corresponds to first `pcb.entered`; `BOARD_TRANSFERRED` corresponds to preceding `pcb.exited` and next `pcb.entered`; `PROCESS_COMPLETED` maps to `pcb.process.completed`; `INSPECTION_RECORDED` to `pcb.inspection.completed`; `MACHINE_STATE_CHANGED` maps to observed/flow status; `FAULT_INJECTED`/`FAULT_CLEARED` produce alarm and state transitions; work-order start/completion map to the corresponding work-order events. This is a semantic mapping, not a direct byte-for-byte conversion; browser events do not carry the full external envelope.
