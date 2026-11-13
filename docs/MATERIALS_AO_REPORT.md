# Materials round (ambient occlusion evaluated and removed)

Date: 2026-11-13. Branch: `v16-materials-ao`. **Awaiting user visual acceptance.**

## Delivered

- **Powder coat** changed from bright semi-metallic paint to matte paint: base colour `(.46, .49, .47)` (was `.68/.72/.69`), metallic `.04` (was `.27`), roughness `.62` (was `.48`). All seven GLBs were regenerated; geometry is byte-for-byte identical and only that one material changed in each file.
- **No other rendering change.** The ambient-occlusion pass that was built earlier on this branch was measured, shown to the user and removed. Camera, simulation, backend, copy and screenshots are otherwise as before.

## Inspect

```sh
npm run dev -- --host 127.0.0.1 --port 5188
```

| Base `6b16446` (old paint) | This branch (matte paint) |
| --- | --- |
| ![Before](screenshots/materials-ao/before-overview.png) | ![After](screenshots/materials-ao/overview.png) |

Close-ups: [mounter](screenshots/materials-ao/mounter.png) · [printer](screenshots/materials-ao/printer.png) · [SPI](screenshots/materials-ao/spi.png) · [reflow](screenshots/materials-ao/reflow.png). Base mounter: [before](screenshots/materials-ao/before-mounter.png). Low quality (`?quality=low`): [overview](screenshots/materials-ao/overview-low.png) · [mounter](screenshots/materials-ao/mounter-low.png).

The cabinets now read as matte paint instead of plastic sheen; the metallic `0.27` on a non-metal surface was the main cause. Glazing, trim and the floor reflection are unchanged.

## Why ambient occlusion was removed

AO was implemented as `n8ao`'s `N8AOPass` in a three composer (with `SMAA` and `OutputPass` so tone mapping and antialiasing survived), then compared three ways at the same camera:

| Comparison (mean difference, 0–255 levels) | Overview | Mounter close-up |
| --- | --- | --- |
| AO off → intensity 2.5 | 2.2 | 8.5 |
| AO off → intensity 4.0 / radius 1.4 | — | 12.3 |

The contact shading the demo needs was already supplied by the key light's shadow map; AO only darkened the cabinet underside, door returns and cavity by a few levels. It cost 165 kB of bundle (478 → 603 kB gzip) and about a tenth of the frame time on headless SwiftShader. The user chose to drop it, so the pass, its two packages and its tests were removed.

That earlier work also found a real trap worth keeping in mind if AO is ever revisited: N8AO renders into float targets and bypasses three's tone mapping and MSAA, so it needs `gammaCorrection = false` plus `SMAAPass` and `OutputPass`, and a context without `EXT_color_buffer_float` renders a blank stage unless the pass is gated.

## Measurements

| Item | Value |
| --- | --- |
| Bundle | 1,679.32 kB minified / 478.31 kB gzip (unchanged from base; the AO packages are gone) |
| `npm test` | 81 passed, 1 skipped |
| `npm run typecheck` / `lint` / `build` | exit 0 (existing >500 kB warning) |
| `npm run test:e2e` | 29 passed, 1 skipped, 6.1 min |
| Callback rate, default | 1.86 /s |
| Callback rate, `?quality=low` | 3.41 /s |

Headless SwiftShader at 1366×768. The default-vs-low gap is the environment bake, not this round.

## Boundaries and decisions

- This round is deliberately one material value plus seven regenerated GLBs. Reverting the generator commit restores the previous paint exactly.
- AO was implemented, reviewed and then withdrawn on evidence rather than left in as dead weight; the branch history keeps that record.
- No bloom, depth of field, textures or geometry changes. LOD variants remain unimplemented.
