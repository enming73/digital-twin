/**
 * Pilot set dressing: painted floor guides and one cable tray behind the line.
 * Kept as data so the placement can be checked without a renderer.
 */
export const MACHINE_HALF_DEPTH = 0.97
export const TALLEST_MACHINE = 2.35

export type Segment = { position: [number, number, number]; size: [number, number, number] }

export function floorGuides(halfLength = 11.5): Segment[] {
  return [-1.15, 1.15].map(z => ({
    position: [0, 0.012, z],
    size: [halfLength * 2, 0.004, 0.06],
  }))
}

export function cableTray(halfLength = 10.5): Segment[] {
  const z = -1.4
  const y = 2.55
  return [
    { position: [0, y, z], size: [halfLength * 2, 0.05, 0.36] },
    { position: [0, y + 0.05, z - 0.18], size: [halfLength * 2, 0.06, 0.03] },
    { position: [0, y + 0.05, z + 0.18], size: [halfLength * 2, 0.06, 0.03] },
  ]
}

export function trayPosts(halfLength = 10.5, spacing = 5.25): Segment[] {
  const posts: Segment[] = []
  for (let x = -halfLength; x <= halfLength + 0.001; x += spacing) {
    posts.push({ position: [x, 2.55 / 2, -1.4], size: [0.08, 2.55, 0.08] })
  }
  return posts
}
