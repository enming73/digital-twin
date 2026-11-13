# Materials and ambient occlusion

Date: 2026-11-13. Branch: `v16-materials-ao`. **Awaiting user visual acceptance.**

## Delivered

- **Powder coat** changed from bright semi-metallic paint to matte paint: base colour `(.46, .49, .47)` (was `.68/.72/.69`), metallic `.04` (was `.27`), roughness `.62` (was `.48`). All seven GLBs were regenerated; geometry is byte-for-byte identical and only that one material changed in each file.
- **Ambient occlusion** via `n8ao`'s `N8AOPass` in a three `EffectComposer`: half resolution, `aoRadius 1.0`, `distanceFalloff 1.0`, `intensity 2.5`.
- The composer ends with `SMAAPass` and `OutputPass`. N8AO draws into its own float target, so three's tone mapping and MSAA would otherwise be lost; N8AO's own gamma correction is disabled and the `OutputPass` applies ACES and the existing `toneMappingExposure` exactly as the direct path does.
- The pass is skipped by `?quality=low` (with the environment bake) and by `?ao=off` (pass only, for comparison). It is also gated at runtime on `EXT_color_buffer_float`: without float render targets the demo renders directly and warns, instead of showing an empty stage.
- No geometry, camera, simulation, backend or copy changes. No downloaded textures.

## Inspect

```sh
npm run dev -- --host 127.0.0.1 --port 5188
```

| Base `6b16446` (old paint, no AO) | This branch (matte paint + AO) |
| --- | --- |
| ![Before](screenshots/materials-ao/before-overview.png) | ![After](screenshots/materials-ao/overview.png) |

Isolating the two levers on the same build:

- paint only: [overview without AO](screenshots/materials-ao/overview-no-ao.png) — the matte paint is the larger change; the line loses its plastic sheen.
- AO only (same paint, AO off → on): compare that image with [the overview](screenshots/materials-ao/overview.png).

Close-ups with AO: [mounter](screenshots/materials-ao/mounter.png) · [printer](screenshots/materials-ao/printer.png) · [SPI](screenshots/materials-ao/spi.png). Without AO/base: [mounter at base](screenshots/materials-ao/before-mounter.png), [low-quality overview](screenshots/materials-ao/overview-low.png), [low-quality mounter](screenshots/materials-ao/mounter-low.png).

Inspected at 1600×900 in headless Chromium/SwiftShader: the cabinets read as matte paint rather than plastic; AO darkens the recesses under frames, the plinth line and the door returns, and is subtle in the open. Glazed interiors stay readable, the planar floor reflection shows no doubled or leaking AO, and there are no console or page errors in either mode.

## Measurements

| Item | Value |
| --- | --- |
| Bundle | 1,679 kB → 1,909 kB minified (478 → 603 kB gzip); existing >500 kB warning remains |
| New packages | `n8ao@2.0.2`, `@react-three/postprocessing@3.2.2` (brings `postprocessing@6.39.6`) |
| Callback rate, AO on | 1.68 /s |
| Callback rate, `?ao=off` (same lighting) | 1.86 /s |
| Callback rate, `?quality=low` | 3.41 /s |
| `npm test` | 85 passed, 1 skipped |
| `npm run typecheck` / `lint` / `build` | exit 0 |
| `npm run test:e2e` | 31 passed, 1 skipped, 7.6 min |

Headless SwiftShader at 1366×768, 8-second samples. With the environment lighting held constant the AO pass costs roughly 10 %; most of the `?quality=low` gain comes from skipping the environment bake, not from AO. These are software-rasteriser numbers and say nothing about the demo machine.

## Failure and fallback behaviour

- `?quality=low` or `?ao=off`: no composer is constructed, no extra render targets exist.
- A context without `EXT_color_buffer_float`: the composer is not built, the scene renders directly, and the console warns once. Pinned by a browser test that stubs the extension and asserts the direct renderer.
- The browser guard asserts `body[data-renderer]` is `composed` with AO and `direct` with `?quality=low`, so a switch that silently stops working fails the suite.
- Resizes call `composer.setSize`/`setPixelRatio` on a single composer; they do not rebuild it, so GPU targets are not leaked per resize.

## Boundaries and decisions

- `@react-three/postprocessing` is installed but only its peer `postprocessing` is used: its composer accepts `Effect`s, while N8AO ships a `Pass`. The manual composer is N8AO's documented integration.
- The composer owns rendering (`useFrame` priority 1). Any later post effect belongs in that composer.
- The quality switch is read when the scene mounts; changing `?quality=low` requires a reload, because the composer takes over the frame loop.
- Material changes are independent of AO: reverting the generator commit restores the old paint without touching the pass, and `?ao=off` disables AO without touching the paint.
- AO softness is deliberate: `intensity 2.5` gives a legible contact band under the cabinets without dirtying the large flat panels.
