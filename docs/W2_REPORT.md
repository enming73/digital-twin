# W2 report — implemented, awaiting user acceptance

## Scope and implementation

W2 adds an independent synthetic Go simulator; it does not connect the existing browser to MQTT and does not add a backend or real equipment connection. `contracts/` contains the v1 schema and examples; `simulator/` contains the Go contract, scenarios, engine, publisher, runner and CLI; `compose.w2.yml` and `deploy/mosquitto/` define local services. Contract semantics are in `EVENT_CONTRACT.md`.

## Commands and observed results

In `/tmp/opencode/smt-w2` on 2026-11-08: `cd simulator && go test -race ./...` passed; `go vet ./...` passed. `W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v` passed with a Docker Mosquitto container. Normal `go run ./cmd/simulator run --scenario scenarios/normal.json --speed 100000 --dry-run` emitted 3,411 events including 700 entered, 700 processed, 700 exited and 200 inspections; completion reported 100 boards. Fault dry-run emitted alarm raise at 70000 ms and clear at 145000 ms, and completed 100. `docker compose -f compose.w2.yml up --build --abort-on-container-exit --exit-code-from simulator` exited 0; broker logged a read-only configuration `chown` warning. Services were removed with `docker compose -f compose.w2.yml down`.

Browser checks: `npm run lint`, `npm run typecheck`, `npm test` (19 passed), `npm run build` (passed with existing >500 kB chunk warning), and `npm run test:e2e` (9 passed). `npm ci` reported 10 dependency vulnerabilities, including 2 critical; dependency remediation was not attempted.

## Acceptance and limitations

- Headless dry-run and local broker operation: demonstrated; synthetic markers present.
- Determinism and 100-board lifecycle: covered by unit tests, including schema-validation of generated events. Further review of unusual scenarios is advisable.
- Fault propagation: bounded upstream/downstream, simultaneous completion/fault and permanent-fault deadlock tests pass; not every possible fault timing has been tested.
- MQTT reconnect: a gated Docker integration test verifies reconnection and no retained events. A separate TCP protocol test disconnects after QoS 1 PUBLISH and before PUBACK and verifies the retry uses identical topic and bytes, allowing downstream deduplication. Consumers themselves are W3 scope.
- SIGUSR1 pause/resume passes a process-level test; a SIGTERM test with an unavailable broker verifies nonzero exit for an interrupted publish. A unit test verifies the three-second shutdown window can finish the current in-flight event. The synchronous single-in-flight publisher has no durable queue or crash recovery.
- Automated checks above exercise the W2 acceptance scenarios, but are not an exhaustive proof across all possible schedules or network failures. This implementation was self-reviewed in the current session, not independently reviewed. W2 is presented for user acceptance; no W3 work is authorized by this report.
