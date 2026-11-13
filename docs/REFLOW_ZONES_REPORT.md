# Reflow zone indicators

Date: 2026-11-13. Branch: `v20-reflow-zones`. **Awaiting user visual acceptance.**

## What changed

The oven had three zone shutters and no way to read them. Three small lamps (Z1–Z3) and labels now sit on the insulated header, and the lamp for the zone holding the board is lit.

- Zone is derived from the existing process progress: `floor(progress × 3)`, clamped to 0–2, where progress is `(faultAtMs ?? simTime − startedAtMs) / processTimeMs`. No new state, no event change.
- Empty oven: all lamps dark. Faulted oven: the zone freezes at the moment of the fault, matching the mechanism.
- Indicators are drawn in the scene at the authored header position, so they do not need a GLB regeneration.

## Evidence

| Zone 1 active | Zone 3 active | Idle |
| --- | --- | --- |
| ![Zone 1](screenshots/reflow-zones/zone-1.png) | ![Zone 3](screenshots/reflow-zones/zone-3.png) | ![Idle](screenshots/reflow-zones/idle.png) |

The lamps and labels read at the existing mechanism camera. They sit on the left two-thirds of the header to stay clear of the operator panel, and are labelled `Z1`–`Z3` rather than given temperatures.

Checks actually run:

```text
npx vitest run tests/unit/reflow-zones.test.ts   4 passed
npm test                                        126 passed, 1 skipped
```

## Boundaries and decisions

- This is process staging, not a thermal profile: there is no temperature, dwell time or recipe, and the labels deliberately avoid numbers.
- The zone follows the same `processTimeMs` the animation already uses, so it can never disagree with the mechanism.
- Labels are runtime canvas textures; nothing is downloaded.
- A future oven model change moves the header, and the lamps would need the same treatment the signal lens got (an authored-position constant plus a GLB assertion); not done here because the header is a shared cladding box, not a named node.
