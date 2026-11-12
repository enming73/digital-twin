export const artifactNames = ['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader'] as const

export type StationId = (typeof artifactNames)[number]

export const commonAnchors = ['Cabinet', 'Conveyor', 'PCB_Anchor', 'Signal_Tower', 'Signal_Lens'] as const

/** Anchor node names every station's GLB must export. See docs/superpowers/plans/2026-11-12-visual-polish-v11.md. */
export const stationAnchors: Readonly<Record<StationId, readonly string[]>> = {
  loader: [...commonAnchors, 'Magazine', 'Lift', 'Pusher'],
  printer: [...commonAnchors, 'Stencil', 'Squeegee', 'Clamp'],
  spi: [...commonAnchors, 'Camera_Bridge', 'Camera_Head'],
  mounter: [...commonAnchors, 'Gantry_X', 'Gantry_Y', 'Placement_Head', 'Feeders'],
  reflow: [...commonAnchors, 'Tunnel', 'Belt', 'Zone_Shutter'],
  aoi: [...commonAnchors, 'Camera_Bridge', 'Camera_Head'],
  unloader: [...commonAnchors, 'Magazine', 'Lift', 'Pusher'],
}
