# Project summary and handover

Status: **all planned implementation phases are implemented; the W1 audit recorded partial-verification items. Per-phase user acceptance remains a human gate.**
Date: 2026-11-10
Project: SMT Digital Twin Studio — synthetic, browser-first SMT line concept demo.

> All telemetry, KPIs, traces and equipment states are SIMULATED. There is no real equipment, PLC, OPC UA server or MES connection. This is not affiliated with ASMPT and is not production infrastructure.

## 1. What was delivered

### Browser demo (v0.1–v1.0)

- Seven conceptual SMT stations in an original React + React Three Fiber scene: loader, printer, SPI, mounter, reflow, AOI and unloader.
- Orbit/zoom/focus/reset camera, keyboard-accessible station selection, responsive layout, persistent demo disclaimer and WebGL fallback.
- Deterministic virtual-time production simulation with a seeded 100-board work order, bounded FIFO queues, configurable processing times and inspection outcomes.
- Production insights: throughput, WIP, average cycle time, pass rate, completion histogram, alerts and board trace.
- Simulated mounter fault injection/recovery and read-only event-log replay.
- Guided 3½-minute presentation with scripted camera presets and a KPI summary.
- Two original Blender scenes and browser GLBs with procedural fallbacks; procedural mechanisms remain the animation/selection layer.
- Vitest unit tests, Playwright smoke/E2E tests and a software-rendered performance diagnostic.

### Industrial integration track (W1–W6)

- **W1** — repository baseline audit and gap report (partial verification recorded: five distinct complete machine models are not proven, and several visual checks are limited).
- **W2** — independent Go discrete-event simulator; versioned event schema and fixtures; NDJSON and MQTT QoS 1 publishers; normal, fault, inspection, reconnect and 1000-board scenarios; optional durable SQLite outbox.
- **W3** — Go twin backend; MQTT ingestion, schema validation, `(runId,eventId)` deduplication, contiguous-sequence enforcement, in-memory projection, REST snapshot/assets/health APIs, snapshot-first WebSocket deltas, offline NDJSON import.
- **W4** — runtime `local | remote` data source; remote snapshot/delta client with reconnect backoff; projection validation; visual-only board motion; same-origin localhost proxies.
- **W5** — simulated work-order and board-trace lookup over the backend, with point-in-time revisions and integrity.
- **W6** — optional PostgreSQL durability, writer lease, transactional projection, replay/history APIs, verified checkpoints, metrics; simulator durable source API; backend terminal-digest verification and offline recovery of missed MQTT events.

## 2. Verification snapshot

Commands and results actually run on 2026-11-10 in this repository:

| Check                                                          | Result                                                             |
| -------------------------------------------------------------- | ------------------------------------------------------------------ |
| `npm run lint`                                                 | Pass                                                               |
| `npm run typecheck`                                            | Pass                                                               |
| `npm test`                                                     | 45 passed, 1 skipped (opt-in W4 parity)                            |
| `npm run build`                                                | Pass; 1,601.32 kB minified / 452.80 kB gzip, >500 kB chunk warning |
| `npm run test:e2e`                                             | 20 passed, 1 skipped (opt-in W4 parity), 31.8 s                    |
| `cd simulator && go test -race -count=1 ./... && go vet ./...` | Pass                                                               |
| `cd backend && go test -race -count=1 ./... && go vet ./...`   | Pass                                                               |

Environment: Node 24.16.0, npm 12.2.1, Go 1.26.1, Linux.

Previously measured integration evidence (see reports for exact commands and caveats):

| Scenario                          | Evidence                                                                                                                                                       |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 100-board durable run             | 3,411 events; 100 completed boards; 98 passed / 2 failed; 3,411 distinct PostgreSQL rows; source head and DB sequence 1..3411; `complete` + `terminalVerified` |
| 1000-board soak, speed 100        | 34,911 source events; 1000/1000 boards; 34,911 distinct DB rows; `complete`; 0–2 s checkpoint restore vs 58 s full replay                                      |
| 100000× burst with backend online | 20 live messages deferred during recovery; 100/100 boards; 3,411 verified events                                                                               |
| Playwright recovery test          | New-epoch snapshot installed mid-stream without reconnecting                                                                                                   |
| MQTT-only 20× 1000-board pacing   | Reconciled 34,911 events, but integrity remained `uncertain` because a missing tail cannot be proven absent without the durable source                         |

The W6 offline-recovery report records one deviation: the 1000-board recovery run used speed 100 instead of the planned 20×. This is harsher for PostgreSQL ingestion and is documented rather than hidden.

## 3. Recommended demo script

### Five-minute local demo

```sh
npm ci
npm run dev
```

1. Orbit, zoom and select the printer and mounter; show **Refocus close-up** and **Reset view**.
2. Start the simulation at 2× or 5×; show queues, station states and moving boards.
3. Open **Production insights**; select a visible PCB to show its trace.
4. Inject a mounter fault, show upstream blocking/downstream starvation, then clear it.
5. Open **Replay**, drag time, show the incident event, then return to live.
6. Start the guided presentation to show the scripted tour and KPI summary.

### Optional remote stack demo

```sh
npm run build
docker compose -p smt-demo \
  -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml up --build
```

Open `http://127.0.0.1:4174/`, select **Remote W3 · read only**, look up `board-1` or `work-order-1`, and switch back to local. Add `compose.w6.yml` for PostgreSQL replay/history; add `compose.w6-recovery.yml` for durable-source recovery.

## 4. Handover notes

- **Ports:** Vite dev `4173`, nginx UI `4174`, backend `8080`, MQTT `1883`; all bound to localhost. PostgreSQL has no host port; the simulator source API is container-internal on `8081`.
- **Compose project names:** examples use `smt-demo` and `smt-w6`. Stop with the same `-p` and `-f` arguments.
- **Volumes:** PostgreSQL `w6_postgres` and simulator `simulator_outbox`. They are retained across `stop`/`down`; `down -v` permanently deletes them.
- **Run IDs:** use a fresh run ID for each new run. Reusing a retained `demo-run` with a new publisher freezes the run as a conflict. Do not recreate only the outbox volume for a run whose PostgreSQL prefix is retained.
- **Rebuild the UI:** the nginx service serves the existing `dist/`; run `npm run build` after frontend changes.
- **Strict recovery identity:** Compose does not set `W6_SOURCE_GENERATION` or `W6_SCENARIO_DIGEST`, so the deployment trusts the source manifest's own identity claim. Set both explicitly when strict checking is required.
- **Dependency state:** `npm ci` reports 8 moderate and 2 critical advisories in existing dependencies. No remediation or major upgrade was attempted.
- **Known build issue:** the single JS chunk exceeds 500 kB; no code splitting was implemented.

## 5. Known limitations and explicit non-goals

- No real equipment, PLC, OPC UA, fieldbus, MES or production authentication.
- No production security: localhost-only, unauthenticated HTTP/WebSocket, no TLS, no multi-tenant isolation, no LAN/public deployment guidance.
- MQTT QoS 1 is not exactly-once. PUBACK proves broker receipt, not backend persistence. The MQTT-only path cannot detect a missing tail event.
- W3 state and W5 trace history are in-memory. Restart loses them; W6 durability is optional and single-writer.
- W6 `terminalVerified` is in-memory and re-verified after restart with a new epoch. Source data is retained until quota limits; recovery is not disk-loss recovery.
- Remote W4 motion, queue state and machine mechanisms are illustrative. A nonstandard station order is not discoverable from W3 snapshots.
- W5 supports only linear routing; rework, skipped stations and repeated inspections are rejected.
- Browser-demo transfers are instantaneous and cross-station PCB jumps remain a visual gap. The W1 audit did not prove five distinct complete machine models; five stations share procedural shells. GLB motion anchors are placeholders; no LOD variants.
- Performance figures are software-rendered diagnostics on one host, not GPU or target-hardware acceptance.
- Replay logs are in memory; export/import is not implemented.
- The bundle size warning and dependency advisories remain.

## 6. If work continues

The roadmap is complete, so further work needs a new agreed scope. Likely candidates, in rough order of value:

1. Formal acceptance sign-off for W2–W6 and correction of any acceptance gaps found.
2. Production-hardening track: authentication/TLS, deployment topology, secret management, HA persistence, multi-run and multi-writer policy.
3. Real integration track: OPC UA/PLC adapters, real MES APIs, equipment-specific mappings, certified KPI definitions.
4. Frontend polish: cross-station transfer animation, GLB node animation, LOD variants, code splitting and dependency upgrades.
5. Operational tooling: retention/compaction policy, backup/restore runbook, monitoring dashboards built on `/api/metrics`.

## 7. Evidence index

- [`PROJECT_PLAN.md`](PROJECT_PLAN.md) — browser-demo plan.
- [`W2_W6_ROADMAP.md`](W2_W6_ROADMAP.md) — integration roadmap and acceptance criteria.
- [`IMPLEMENTATION_BASELINE.md`](IMPLEMENTATION_BASELINE.md), [`W1_GAP_REPORT.md`](W1_GAP_REPORT.md) — baseline audit.
- [`W2_REPORT.md`](W2_REPORT.md), [`W3_REPORT.md`](W3_REPORT.md), [`W4_REPORT.md`](W4_REPORT.md), [`W5_REPORT.md`](W5_REPORT.md) — phase reports.
- [`W6_REPORT.md`](W6_REPORT.md) — persistence/replay implementation and earlier open items.
- [`W6_RECOVERY_REPORT.md`](W6_RECOVERY_REPORT.md) — verified recovery evidence, measurements and limitations.
- [`W6_RECOVERY_DESIGN.md`](W6_RECOVERY_DESIGN.md) — source protocol and verification design.
- [`ARCHITECTURE.md`](ARCHITECTURE.md), [`EVENT_CONTRACT.md`](EVENT_CONTRACT.md), [`DECISIONS.md`](DECISIONS.md) — architecture, contract and decisions.
- [`ACCEPTANCE.md`](ACCEPTANCE.md) — browser-demo acceptance checklists.
- [`../README.md`](../README.md) — current setup, commands, API and limitations.
