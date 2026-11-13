# Cross-station board transfer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the instantaneous cross-station board jump with a short, deterministic visual transfer driven by simulated time.

**Architecture:** A pure module diffs successive scene snapshots, records a transfer per board with a start time in `simTimeMs`, and interpolates the board's x from its old station to its destination slot. The renderer only reads that interpolation; the simulator, event log and remote path are untouched.

**Tech Stack:** TypeScript strict, React Three Fiber (existing `Boards` component), Vitest, Playwright.

**Spec:** item 3 of `docs/FOLLOW_UPS.md`.

## Global Constraints

- The business clock stays `scene.simTimeMs`. No `performance.now()` or frame delta may decide where a board is.
- Pausing must freeze transfers; seeking backwards must discard them; a new run must start with none.
- Only the visual layer changes. Do not alter `Simulation`, event payloads, KPI logic or the W2–W6 contracts.
- Remote (`W4`) boards keep their existing server-driven `visual` interpolation and are out of scope.
- A transfer is illustrative travel, not a belt speed; document that with the same wording as the mechanism motion.

## Review Focus

1. **A transfer replayed by the UI poll rate.** The board must not depend on how often `setSnapshot` fires; two polls during one transfer must produce the same path, and a slow poll must not skip the animation's start.
2. **Pause and speed changes.** At 5× a 0.6 s transfer happens inside one poll; pausing mid-transfer must freeze the board, not finish it.
3. **Backwards seek and reset.** After dragging replay back, no board may sit between stations, and no stale transfer may fire when the same board later reaches the same station again.
4. **Boards queued behind the destination.** A board entering a full queue must land on its slot, not on the machine; queue index changes while travelling must not teleport it.
5. **Frame cost.** Transfer state must not re-render React per frame or allocate per board per frame in a way that shows in the existing software-renderer measurement.

## File Map

- `src/scene/transfers.ts`: pure transfer model — creation, expiry, position.
- `src/scene/boardPosition.ts` (or exported from transfers): target x/z for queued, processing and new arrivals.
- `tests/unit/transfers.test.ts`: all pure behaviour.
- `src/scene/Factory.tsx`: `Boards` uses the model; adds `data-transfers` for the browser check.
- `src/app/App.tsx`: forwards the count to the `.scene` attribute if that is where the test reads it.
- `tests/e2e/simulation.spec.ts`: assert transfers occur and clear during a run.
- `docs/TRANSFER_REPORT.md`, `docs/screenshots/transfers/`: evidence.

### Task 1: Pure transfer model

**Interfaces:**
- `type Transfer = { boardId: string; fromX: number; startMs: number; durationMs: number }`
- `transferDuration(fromX: number, toX: number): number` — `clamp(|dx| * 110, 350, 900)` ms.
- `boardTarget(board: SceneBoard, scene: SceneModel): { x: number; z: number }` — queued → `x - 1.2 + queueIndex * 0.5`, `z -0.18`; processing → `x - 0.55 + progress * 1.1`, `z 0.12`, where `progress` is the existing formula from `Factory.tsx` (`startedAtMs`/`processTimeMs`, clamped), defaulting to 1 when unknown.
- `startTransfers(previous: SceneModel | null, next: SceneModel): Transfer[]` — one per board whose `stationId` changed and whose `next.simTimeMs` is later than `previous.simTimeMs`; `startMs = previous.simTimeMs`.
- `boardX(transfer: Transfer | undefined, targetX: number, simTimeMs: number): number` — eased lerp from `fromX` to `targetX`; without a transfer, `targetX`.

- [ ] Write `tests/unit/transfers.test.ts` covering: distance-scaled duration clamped to 350–900 ms; a transfer starting at the previous snapshot's time; half-way position at half duration; completion at the duration; no transfer for a board that stayed put; no transfer for a brand-new board; no transfer when `simTimeMs` did not advance (pause).
- [ ] Run `npx vitest run tests/unit/transfers.test.ts`; expect failure because the module does not exist.
- [ ] Implement `src/scene/transfers.ts` with the signatures above; keep it free of React and Three.js.
- [ ] Run the test file until green; run `npm test`.

### Task 2: Wire it into the renderer

**Interfaces:** consumes Task 1. `Boards` keeps a `useRef<Transfer[]>`; on each `scene` change it calls `startTransfers` with the previous scene, drops expired entries, and computes each board's x through `boardX`.

- [ ] Extend the unit tests with a "slow poll" case: transferring at t=0 and polling again at t=800 must yield the target position, not a restart, and a poll during the transfer must keep `startMs` unchanged.
- [ ] Replace the position logic in `src/scene/Factory.tsx`'s `Boards` with `boardTarget` + `boardX`; keep the instanced mesh, colours and click handling unchanged. Store the previous `SceneModel` in a ref and reset both refs when `scene.simTimeMs` goes backwards (replay seek/reset).
- [ ] Add `data-transfers` (count of active transfers) to the `.scene` element so a browser test can observe it without pixel comparison.
- [ ] Run typecheck, lint, `npm test`; start the dev server and watch one full run at 1× and 5×: boards leave a station, travel, and land on the next slot; pause freezes them; dragging replay back clears them.
- [ ] Extend `tests/e2e/simulation.spec.ts`: start the simulation, poll `.scene[data-transfers]` until it is non-zero, and assert it returns to zero when the run pauses after the last board moves.

### Task 3: Evidence and docs

- [ ] Capture a short sequence (three screenshots ~150 ms apart) of one board between two stations, plus an overview.
- [ ] Write `docs/TRANSFER_REPORT.md`: duration rule, what is deterministic and what is illustrative, pause/seek behaviour, measurements.
- [ ] Update `README.md` (the "cross-station PCB jumps remain a known visual gap" sentence) and `docs/DECISIONS.md` (visual transfer, sim-time based).
- [ ] Run the full suite (typecheck, lint, unit, build, e2e) and record the software-renderer callback rate before/after.
- [ ] Obtain one fresh whole-branch review; fix Critical/Important findings with RED→GREEN tests.
- [ ] Present the close-up sequence for acceptance; do not merge or push without instruction.

## Self-review

The transfer model is pure and testable without a renderer, which is also how pause/seek behaviour is pinned. The visual stays on `simTimeMs`, so replay determinism is untouched. Remote boards are explicitly out of scope, and the README sentence is corrected only after the behaviour exists.
