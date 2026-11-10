# OpenCode Agent Instructions — SMT Digital Twin Studio

You are working on an **industrial-realistic, browser-first, ASMPT-inspired SMT digital twin concept demo**. Read `docs/PROJECT_PLAN.md`, `docs/W2_W6_ROADMAP.md`, and `docs/PROJECT_SUMMARY.md` before starting work. The browser-demo and W2–W6 integration tracks are implemented; no new phase is currently authorized.

## Operating rules

1. Work on **one phase only** (v0.1 initially). Never begin the next phase until the user explicitly approves.
2. Before coding, inspect the repository, summarize intended changes and assumptions; avoid unnecessary questions when the plan provides defaults.
3. Use React + TypeScript strict + Vite + React Three Fiber + Drei; keep simulation in pure TypeScript independent of React and Three.js.
4. Build original generic models. Never copy proprietary ASMPT CAD or claim official affiliation. Clearly label all telemetry SIMULATED.
5. For v0.1 use procedural geometry so the demo works before Blender assets exist. Preserve a path to swap in named GLB assets later.
6. Preserve deterministic virtual-time simulation and event-log replay requirements as phases are added. Never use render-frame delta as the business clock.
7. Keep the app runnable after every milestone. No mandatory backend, real equipment or paid API before v1.0.
8. Add automated tests and run all relevant checks; never claim tests passed unless actually run.
9. Document assumptions, decisions, known limitations and exact commands to reproduce.
10. At each phase gate, stop and request approval to continue.

## Initial task

Implement **v0.1 only** from `docs/PROJECT_PLAN.md`: 7 conceptual SMT stations, realistic industrial materials, conveyors, static PCBs, camera orbit/focus/reset, selection/details, responsive UI, disclaimer, smoke tests and README. Create the project skeleton if absent.

## Expected completion report

- Implemented features and file paths
- How to install/run/test
- Actual test results
- Known issues and screenshots if available
- Explicit statement: `v0.1 complete; awaiting approval for v0.2` (only if acceptance criteria pass)

## Post-v1.0 industrial integration track (W1–W6)

The browser-only v1.0 demo has been reported as user-accepted. W1–W6 are implemented in the repository: the independent simulator (W2), twin backend (W3), remote 3D view (W4), mock MES (W5), and persistence/replay/recovery (W6). W6 offline recovery is measured and awaiting final user acceptance. Do not confuse browser-demo version numbers with W-phase maturity.

The project is at handover and formal acceptance, not at the start of a new implementation phase. Before making changes, read `docs/PROJECT_SUMMARY.md`, [`README.md`](README.md), and the relevant phase report. Any further work (production hardening, real equipment/MES integration, frontend polish) requires a newly agreed scope and explicit user approval.
