# W5 report — simulated mock MES

## Objective and implementation

Provide a read-only synthetic work-order, fixed-route and PCB trace view over W4, without a real MES or equipment connection. `backend/internal/twin/{store,trace}.go` maintains in-memory traces and work-order progress atomically with the W3 projection. `backend/internal/mes/fixtures.go` provides optional synthetic metadata; `backend/internal/api/http.go` exposes read-only board trace and work-order endpoints. `src/twin/mesClient.ts` and `src/components/RemoteTrace.tsx` add remote lookup without changing the W3 WebSocket protocol or local simulator.

## Commands and observed results

Executed on 2026-11-09 in the isolated W5 worktree:

- `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`: passed; 38 unit tests passed, one opt-in parity test skipped. Build warns about a 1,598.91 kB minified chunk.
- `npm run test:e2e`: 15 passed, one opt-in parity test skipped; after that full run, the added run-ID change test passed with all five `npx playwright test tests/e2e/remote-trace.spec.ts` tests. Tests include stale epoch, unknown ID, superseded query, run-ID changes and source-switch cancellation.
- With fresh W2 NDJSON and a W3 offline import as described in `docs/W4_REPORT.md`, `W4_PARITY=1 npx vitest run tests/unit/twin-w3-parity.test.ts`: 1 passed; `W4_PARITY=1 npx playwright test tests/e2e/remote.spec.ts`: 3 passed.
- `go test -race -count=1 ./...` and `go vet ./...` in both `backend/` and `simulator/`: passed. `W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v`: reconnect passed. `W3_MQTT_INTEGRATION=1 go test -count=1 ./internal/ingest ./internal/api -run Integration -v`: disconnect and two-subscriber tests passed, the latter now queries the 100-board MES API.
- `docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml config --quiet`: passed. `docker compose -p w5-audit-20261109 -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml up -d --build` started the backend before the simulator. Via `http://127.0.0.1:4174`, the running order reported created 94, completed 78, in progress 16; board-1 exposed 23 chronological entries and SPI fail. After `docker wait w5-audit-20261109-simulator-1` returned 0, the order reported revision 3411, integrity complete, created/completed 100, passed 98, failed 2, reconciliation matched. `docker compose -p w5-audit-20261109 -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml down` removed that project only.

## Acceptance and limitations

- Board history, order identity, inspection outcomes and progress have been exercised with the real W2 stream; all displays and responses identify the data as synthetic.
- Duplicate/conflicting events, skipped station, unknown order, repeated process/inspection, restart with empty history, all-failed inspection fixture, query supersession, run-ID changes and source-switch cancellation have automated coverage. The old-epoch response and gap response are explicitly checked. Tests do not exhaust every input, configuration, browser/network interleaving or a real external MES; selecting a PCB via the WebGL canvas was not separately exercised for this feature.
- The implementation was reviewed by its author; no independent code review was performed. No screenshots or target-hardware performance claims are included.
- History and deduplication are process-memory-only, grow with events, and cannot recover missed events or survive backend restart. Integrity `complete` does not guarantee a missing tail was detected. No real MES, equipment, production security, durable storage, rework, skipped stations, repeated inspections, or certified KPI is supported. The UI is a localhost demo; it is not a production deployment.
