# Synthetic SMT event contract v1

## W5 synthetic read-only query contract

`GET /api/runs/{runId}/boards/{boardId}/trace` returns `{runId, epoch, revision, integrity, reason?, synthetic: true, board, route, entries}`. Each entry contains `sequence`, `eventId`, `eventType`, `machineId`, `simTimeMs`, `emittedAt`, `ingestedAt` and, for inspection events, `passed`. Sequence is the ordering key, including when virtual timestamps coincide. Only successfully applied events are included. The route is configured intent, not proof of a visit.

`GET /api/runs/{runId}/workorders/{workOrderId}` returns the same envelope and route, plus `order`, nullable synthetic `metadata`, `progress` (`planned`, `created`, `inProgress`, `completed`, `passed`, `failed`, `uninspected`) and `reconciliation` (`pending`, `matched`, `mismatch`, or `unavailable`). Counts classify completed boards: failed means any recorded failed inspection; passed means one or more recorded inspections and none failed; uninspected means none recorded. Reconciliation is unavailable when any completed board is uninspected. A failed inspection does not imply scrap.

Unknown run or entity returns 404. Identifiers are scoped to their run. Query data and integrity are atomically captured at the returned revision; resubmit the query for updates. History is in-memory only and does not heal loss. Gaps/conflicts freeze application, and uncertain or incomplete history must not be treated as complete; even `complete` is not a delivery guarantee. No rework, skipped stations, or repeated inspections are supported.

This contract is for the independent simulator only. All events represent **SIMULATED** data; they do not indicate real equipment or production. The JSON Schema is `contracts/event.schema.json`; a sample for each of the nine event types is in `contracts/fixtures/`.

The W3 backend consumes this same v1 contract; see `README.md` for its separate snapshot/WS protocol and conservative completeness rules. It records ingestion wall time separately from `emittedAt` and `simTimeMs` and never interprets either as measured equipment time.

Each envelope contains `schemaVersion`, `eventId`, `runId`, `source`, nullable `machineId`, `eventType`, per-run `sequence`, virtual `simTimeMs`, UTC `emittedAt`, and `payload`. `source` is `simulator`, every payload includes `synthetic: true`, and `emittedAt` is wall time rather than virtual time. Work-order events have null machine IDs; other events have machine IDs. Inspection events originate at `spi` or `aoi`. Work-order completion additionally requires passed + failed = quantity; this arithmetic rule is enforced in Go because JSON Schema cannot express it.

Topics are `demo/smt/{runId}/machines/{machineId}/events` and `demo/smt/{runId}/line/events`; topic identifiers cannot contain `/`, `+` or `#`. Events are not retained. MQTT QoS 1 is at-least-once: consumers must deduplicate by `(runId,eventId)` and detect missing or out-of-order global sequence numbers. Retrying a publication must preserve the entire serialized event. Durable recovery and consumer handling are future phases, not implemented by this contract.

`pcb.entered` means a station takes ownership in its input queue. `pcb.process.completed` occurs before `pcb.exited`; exit means leaving the processing/holding slot, immediately before the next station's entered event. Inspection follows process completion and precedes exit. Work-order start precedes board events; completion follows final exit. Observed machine status (`IDLE`, `RUNNING`, `FAULT`) is distinct from derived flow status (`NORMAL`, `BLOCKED`, `STARVED`), which includes a reason.

## Browser event migration mapping

The browser remains unchanged. Its `BOARD_CREATED` corresponds to first `pcb.entered`; `BOARD_TRANSFERRED` corresponds to preceding `pcb.exited` and next `pcb.entered`; `PROCESS_COMPLETED` maps to `pcb.process.completed`; `INSPECTION_RECORDED` to `pcb.inspection.completed`; `MACHINE_STATE_CHANGED` maps to observed/flow status; `FAULT_INJECTED`/`FAULT_CLEARED` produce alarm and state transitions; work-order start/completion map to the corresponding work-order events. This is a semantic mapping, not a direct byte-for-byte conversion; browser events do not carry the full external envelope.
