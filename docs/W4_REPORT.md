# W4 report — experimental synthetic remote view

## Objective and changes

Provide an optional read-only W3 source for the existing 3D scene without replacing the local simulator, camera, analytics, replay or guided presentation. All data is **SIMULATED**. See `src/twin/` for source adapters, projection and connection lifecycle; `src/scene/remoteMotion.ts` and `src/scene/Factory.tsx` for illustrative visual motion/status; `src/app/App.tsx` for source controls and integrity display; `vite.config.ts`, `deploy/w4/nginx.conf`, `compose.w4.yml` for localhost-only same-origin proxies. Tests are in `tests/unit/{twin-projection,twin-w3-parity,remote-client,remote-motion,proxy-origin}.test.ts` and `tests/e2e/remote.spec.ts`.

## Commands and actual results

Executed 2026-11-09 in the W4 worktree:

- `npm ci`: completed; npm reported 10 vulnerabilities (8 moderate, 2 critical) and one blocked esbuild install script. No automatic dependency upgrade was attempted.
- `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`: passed; 34 unit tests passed, one opt-in parity test skipped in the default run. Build reports the existing >500 kB chunk warning (now 1,594.41 kB minified).
- `W4_PARITY=1 npx vitest run tests/unit/twin-w3-parity.test.ts`: 1 passed, comparing the full W2 stream against a W3 offline-import snapshot.
- `W4_PARITY=1 npm run test:e2e`: 12 passed, including 100-board event fixture, source switching and reconnect snapshot replacement.
- In each of `backend/` and `simulator/`: `go test -race -count=1 ./...` and `go vet ./...` passed. W3 MQTT/process integration command from the W3 README plus `W3_PROCESS_INTEGRATION=1` passed the offline restart, live-crash, ingestion disconnect and two-WebSocket tests. W2 `W2_MQTT_INTEGRATION=1 go test -count=1 ./internal/publisher -run Integration -v` passed.
- `docker compose -f compose.w2.yml -f compose.w3.yml -f compose.w4.yml config --quiet`: passed. A separate Compose project started W2, W3 and W4; nginx allowed a local-origin WebSocket (101), rejected a hostile origin (403), and W3 directly rejected that origin (403). The Vite proxy accepted local-origin handshake and closed the hostile-origin connection. These are targeted handshake checks, not a security audit.
- Real browser against the Compose nginx proxy, subscriber connected before publisher: normal 100-board run completed with simulator exit 0, backend revision 3411, integrity `complete`, 100 boards, completed work order and zero visible boards after completion. In a fresh backend run with `fault.json` at 30×, simulator exit 0, revision 3413 and integrity `complete`; the browser station list displayed `FAULT`, `BLOCKED` and `STARVED`, and no boards remained visible at the end. These are individual runs, not proof against all load or timing failures.
- During another real 20× run, the browser showed 10 visible boards before restarting the W3 backend. It reconnected to a new epoch with zero visible boards and `gap` integrity after the still-running publisher resumed. This verifies replacement and honest loss indication, **not** recovery of missing data.

To reproduce the opt-in parity fixture, generate the NDJSON from `simulator/` using `go run ./cmd/simulator run --scenario scenarios/normal.json --run-id demo-run --speed 100000 --dry-run > /tmp/opencode/w4-events.ndjson`. From `backend/`, start `go run ./cmd/twin-backend --import /tmp/opencode/w4-events.ndjson --run-id demo-run --listen 127.0.0.1:18080`; in another terminal run `curl --fail --silent http://127.0.0.1:18080/api/runs/demo-run/snapshot > /tmp/opencode/w4-snapshot.json`. Stop that backend with Ctrl-C, then run the two `W4_PARITY=1` commands above from the root. `/tmp/opencode` must exist.

## Acceptance and limitations

- Local/remote source switching, local camera and presentation smoke tests: exercised; no remote requests on local startup in the new E2E.
- 100-board simulated remote flow and fault propagation: exercised as described; board movement is illustrative and not measured physical motion.
- Reconnect snapshot replacement and no lingering visible boards: tested with both a controlled WebSocket fixture and a real backend restart; live backend crash recovery is not implemented by W3 and a new snapshot does not heal lost events.
- Backend integrity and WebSocket connectivity are separate; a 15-second activity timeout can also flag a healthy, quiet completed run. The backend may not detect a lost tail event; QoS acknowledgements do not establish end-to-end delivery.
- W3 cannot provide processing starts, exact queue positions/capacities, exact transfer timing or full remote history. Remote machine motion is static; local-only controls and analytics are hidden. W3 does not expose station topology order; only the default seven-station order is supported, and alternate order cannot be reliably detected from a snapshot.
- The frontend reducer and snapshot validation are a second implementation of the W3 contract; the 100-board parity test covers one complete normal fixture, not every malformed input or all simulator configurations. No formal schema generator or independent code review was performed.
- Browser performance diagnostic, WSL2 Linux 6.18, Intel i7-8700K, headless Chromium 158.0.8127.18, SwiftShader, 1366×768 DPR 1, five-second requestAnimationFrame sample: local actively running 22.2 fps; remote 23.3 fps with an already completed, idle fault-scenario snapshot. These workloads are **not equivalent**, the latter does not measure live event throughput, and software rendering is not representative GPU hardware. No 30 fps target claim is made.
- Localhost-only proxy and broker have no production authentication or TLS; no real equipment, MES or durable event store. Existing npm advisories remain unresolved. No screenshots or independent review are included.
