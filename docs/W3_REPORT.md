# W3 report — experimental implementation, review pending

## Follow-up verification — W3 remains open

- A real WebSocket slow-reader test now exercises a stalled socket, server disconnection and reconnect to a current snapshot. A real MQTT live-crash test kills the backend after ten events, verifies the restarted process exposes an empty `uncertain` state with a new epoch, and restores the complete 100-board run using offline NDJSON. This is not automatic recovery.
- A reproducible unit test found a stale subscription acknowledgement could set `subscribed=true` after disconnection. Connection generations now reject stale success and failure callbacks; race-enabled package tests passed.
- Under concurrent heavy Go, Docker and frontend checks, the W2 integration test observed `expected sequence 2886, got 2887`; a separate two-WebSocket integration test also observed a sequence gap. Both froze the run and reported the gap, as designed. Re-running the same integration command separately passed; a further run alongside race tests passed too. The source of the intermittent gap has **not** been established, so a normal-flow acceptance pass under load cannot be claimed. Do not interpret broker PUBACK as subscriber delivery acknowledgement.
- The slow-reader test initially timed out with an artificially tiny receive buffer; after using the normal OS receive buffer it passed. Broader network environments and timing remain unverified.
- These follow-up changes have not yet received independent review. W3 is not ready for acceptance until the intermittent missing-event behavior is investigated and the full acceptance checks are rerun.

## Objective and scope

Add a read-only synthetic Go twin backend in parallel with the unchanged browser demo and W2 publisher. No real equipment, MES, durable database, browser MQTT, or W4 UI integration. All represented telemetry is **SIMULATED**.

## Implementation

- `backend/internal/contract/`: v1 schema, duplicate JSON key, topic/envelope and completion-arithmetic validation; canonical schema at `contracts/event.schema.json`.
- `backend/internal/twin/`: per-run contiguous sequences, idempotency, conflict/gap freeze, in-memory machine, alarm, board, inspection and work-order projection; snapshot and atomic bounded subscriptions.
- `backend/internal/ingest/`: MQTT QoS 1 subscription, reconnect, health and permanent uncertainty on observed disconnect/invalid messages. `backend/internal/replay/` reconstructs complete NDJSON offline in a private store.
- `backend/internal/api/`: read-only health, assets, run snapshot, snapshot-first WebSocket deltas/status and slow-client resync. `backend/cmd/twin-backend/` configures and starts the service. `backend/Dockerfile` and `compose.w3.yml` add local deployment.
- `README.md`, `docs/ARCHITECTURE.md` and `docs/EVENT_CONTRACT.md` document configuration, endpoints, safety assumptions and recovery limits.

## Reproduction

From root, run `docker compose -f compose.w2.yml -f compose.w3.yml up --build`; query `http://127.0.0.1:8080/api/health`, `/api/assets`, and `/api/runs/demo-run/snapshot`. Use `docker compose -f compose.w2.yml -f compose.w3.yml down` to stop that stack. Other commands and the WS/NDJSON protocol are in `README.md`.

## Verification performed

- `cd backend && go test -race -count=1 ./...` and `go vet ./...`: passed (six packages); `go test -count=1 ./internal/replay -v` subsequently passed after adding offline/live projection equality and missing-event checks.
- `cd backend && W3_MQTT_INTEGRATION=1 go test -count=1 ./internal/ingest ./internal/api -run Integration -v`: passed; real local Mosquitto, W2 100-board run, duplicate publication, broker stop/restart, two real WS consumers and contiguous matching revisions.
- `cd backend && W3_PROCESS_INTEGRATION=1 go test -count=1 ./cmd/twin-backend -run TestOfflineImportAfterProcessRestart -v`: passed; two independent backend processes each loaded the full fixture with different epochs. This does not simulate a live backend crash followed by import.
- `docker compose -f compose.w2.yml -f compose.w3.yml config`: passed. Isolated Compose project `w3-validation-20261109` built and started; health/subscription ready before simulator; REST snapshot reached sequence 3411, 100 boards and completed work order. Only that project was shut down afterward. This Compose run preceded the later conservative empty-run integrity change; the updated change is covered by unit tests but was not re-run in Compose.
- `cd simulator && go test -race ./... && go vet ./...` passed; `W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v` passed.
- Root `npm run lint`, `npm run typecheck`, `npm test` (19), and `npm run build` passed. Build retains the existing >500 kB chunk warning. `npm run test:e2e` initially failed 3/9 with timeouts while heavy Go/Docker checks ran concurrently; a separate rerun passed 9/9. The reason for the first failures was not independently established.
- `npm ci` reported 10 dependency vulnerabilities (8 moderate, 2 critical); remediation was outside this phase.

## Acceptance and remaining risks

- Two WebSocket consumers receive the same actual W2 MQTT run in integration tests. Duplicate MQTT publication leaves revision unchanged. Gap and conflicts are unit-tested and visible, not silently applied.
- Restart produces a new random epoch and empty/uncertain snapshot. A full offline fixture can be re-imported; automatic broker catch-up and durable recovery are **not** implemented. The offline/live reducer equality test ignores epoch and backend ingestion time; the process test checks two offline starts, not a live backend crash followed by fixture import.
- Simulator flow state/reason is preserved rather than independently derived. Because W2 has no process-start or queue snapshot, exact processing slots, queue capacity and intermediate timing are not represented. `pcb.exited` followed by `pcb.entered` can temporarily expose `in_transit`.
- Bounded subscriber overflow closes the subscription, with best-effort `resync_required`; the store overflow behavior is unit-tested but a network-level slow-consumer test is not yet present. MQTT re-subscription failures, simultaneous loss/receive ordering and uncommon malformed/overflow inputs need broader adversarial coverage.
- Event fingerprints and board state grow in memory until process exit. The implementation clones the complete projection for each event; no throughput or memory budget is claimed. MQTT/HTTP security is localhost-only demo configuration, not production authentication or TLS.
- Implementation has not received an independent code review. These gaps mean this report does **not** assert all W3 acceptance criteria or authorize W4.
