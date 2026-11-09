# Synthetic SMT event contract v1

This contract is for the independent simulator only. All events represent **SIMULATED** data; they do not indicate real equipment or production. The JSON Schema is `contracts/event.schema.json`; a sample for each of the nine event types is in `contracts/fixtures/`.

The W3 backend consumes this same v1 contract; see `README.md` for its separate snapshot/WS protocol and conservative completeness rules. It records ingestion wall time separately from `emittedAt` and `simTimeMs` and never interprets either as measured equipment time.

Each envelope contains `schemaVersion`, `eventId`, `runId`, `source`, nullable `machineId`, `eventType`, per-run `sequence`, virtual `simTimeMs`, UTC `emittedAt`, and `payload`. `source` is `simulator`, every payload includes `synthetic: true`, and `emittedAt` is wall time rather than virtual time. Work-order events have null machine IDs; other events have machine IDs. Inspection events originate at `spi` or `aoi`. Work-order completion additionally requires passed + failed = quantity; this arithmetic rule is enforced in Go because JSON Schema cannot express it.

Topics are `demo/smt/{runId}/machines/{machineId}/events` and `demo/smt/{runId}/line/events`; topic identifiers cannot contain `/`, `+` or `#`. Events are not retained. MQTT QoS 1 is at-least-once: consumers must deduplicate by `(runId,eventId)` and detect missing or out-of-order global sequence numbers. Retrying a publication must preserve the entire serialized event. Durable recovery and consumer handling are future phases, not implemented by this contract.

`pcb.entered` means a station takes ownership in its input queue. `pcb.process.completed` occurs before `pcb.exited`; exit means leaving the processing/holding slot, immediately before the next station's entered event. Inspection follows process completion and precedes exit. Work-order start precedes board events; completion follows final exit. Observed machine status (`IDLE`, `RUNNING`, `FAULT`) is distinct from derived flow status (`NORMAL`, `BLOCKED`, `STARVED`), which includes a reason.

## Browser event migration mapping

The browser remains unchanged. Its `BOARD_CREATED` corresponds to first `pcb.entered`; `BOARD_TRANSFERRED` corresponds to preceding `pcb.exited` and next `pcb.entered`; `PROCESS_COMPLETED` maps to `pcb.process.completed`; `INSPECTION_RECORDED` to `pcb.inspection.completed`; `MACHINE_STATE_CHANGED` maps to observed/flow status; `FAULT_INJECTED`/`FAULT_CLEARED` produce alarm and state transitions; work-order start/completion map to the corresponding work-order events. This is a semantic mapping, not a direct byte-for-byte conversion; browser events do not carry the full external envelope.
