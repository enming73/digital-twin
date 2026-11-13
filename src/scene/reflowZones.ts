import type { MachineSnapshot } from '../simulation/engine'

/** Slat centres of the authored `Zone_Shutter` node; asserted against the GLB. */
export const ZONE_X = [-0.45, 0, 0.45] as const

/** The oven is shown as three conceptual zones; the active one follows the board's progress. */
export function activeZone(machine: MachineSnapshot | null, simTimeMs: number): number | null {
  if (!machine || machine.startedAtMs === null || machine.boardId === null) return null
  const elapsed = (machine.faultAtMs ?? simTimeMs) - machine.startedAtMs
  const progress = Math.max(0, Math.min(1, elapsed / machine.processTimeMs))
  return Math.min(2, Math.floor(progress * 3))
}
