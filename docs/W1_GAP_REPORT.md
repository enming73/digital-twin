# W1 baseline gap report

Status: **partially verified, not an unconditional W1 pass**. W1 is an audit of existing functionality, not a new development phase.

| Criterion | Finding | Evidence / limitation |
|---|---|---|
| Seven conceptual stations | Present | `src/data/stations.ts`; seven list buttons verified by `tests/e2e/factory.spec.ts`. |
| At least five simplified machine models | Partially verified | Seven machine instances, station-specific procedural mechanisms and two distinct GLB shells exist; five distinct complete machine models are not proven because five stations share procedural shell geometry. Clarify whether station instances suffice before claiming this criterion. |
| GLB export if claimed | Present | Two `.blend` sources and corresponding printer/mounter `.glb` files; asset unit and browser loading tests passed. Provenance beyond repository source/generator was not independently audited. |
| Selection, orbit, focus, reset | Present | `Factory.tsx`, `App.tsx`; E2E tests select all list entries and reset. Automated test does not independently exercise every 3D mesh click/orbit gesture. |
| Responsive UI and disclaimer | Present, limited validation | CSS and disclaimer exist; browser smoke uses 1366×768 only, not a comprehensive viewport/accessibility audit. |
| Baseline checks | Pass with warning | See `IMPLEMENTATION_BASELINE.md`; build has a large-chunk warning. |

Other documented gaps: instantaneous visual station-to-station jumps; unanimated GLB anchors; no dedicated LOD and no representative GPU benchmark; full guided tour not browser-tested end-to-end. These are not blockers to W2 by themselves. No visual upgrade is included in W2.
