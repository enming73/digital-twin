import * as THREE from 'three'
import type { MechanismPose } from './motion'

type Offset = Partial<Record<'x' | 'y' | 'z', number>>
export type AnchorOffsets = Record<string, Offset>

/** Blender cleat pitch; one pitch of travel per process cycle keeps the wrap seam invisible. */
export const BELT_PITCH = 0.36

/**
 * Illustrative anchor travel in metres, derived from the normalized process pose.
 * These are conceptual movements, not machine coordinates.
 */
export function anchorOffsets(kind: string, pose: MechanismPose): AnchorOffsets {
  const { cycle, sweep, lift } = pose
  switch (kind) {
    case 'mounter':
      return {
        Gantry_X: { x: sweep * 0.45 },
        Gantry_Y: { x: sweep * 0.45 },
        Placement_Head: { x: sweep * 0.45, y: -lift * 0.12 },
        Feeders: { y: lift * 0.01 },
      }
    case 'printer':
      return { Squeegee: { x: sweep * 0.48 }, Clamp: { y: lift * 0.02 } }
    case 'spi':
    case 'aoi':
      return { Camera_Bridge: { x: sweep * 0.57 }, Camera_Head: { y: -lift * 0.05 } }
    case 'reflow':
      return { Belt: { x: Math.min(1, Math.max(0, cycle)) * BELT_PITCH }, Zone_Shutter: { y: lift * 0.03 } }
    case 'loader':
    case 'unloader':
      return { Lift: { y: lift * 0.18 }, Pusher: { z: sweep * 0.1 } }
    default:
      return {}
  }
}

/** Records each anchor's authored position so every frame can be applied from rest. */
export function captureAnchors(root: THREE.Object3D, names: readonly string[]): Map<string, THREE.Vector3> {
  const rest = new Map<string, THREE.Vector3>()
  for (const name of names) {
    const node = root.getObjectByName(name)
    if (node) rest.set(name, node.position.clone())
  }
  return rest
}

/** Writes rest + offset every call, so repeated frames cannot drift. */
export function applyAnchorOffsets(root: THREE.Object3D, rest: Map<string, THREE.Vector3>, offsets: AnchorOffsets): void {
  for (const [name, base] of rest) {
    const node = root.getObjectByName(name)
    if (!node) continue
    const offset = offsets[name]
    node.position.set(base.x + (offset?.x ?? 0), base.y + (offset?.y ?? 0), base.z + (offset?.z ?? 0))
  }
}
