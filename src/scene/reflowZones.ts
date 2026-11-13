import type { MachineSnapshot } from '../simulation/engine'

/** The oven is shown as three conceptual zones; the active one follows the board's progress. */
export function activeZone(machine: MachineSnapshot | null, simTimeMs: number): number | null {
  if (!machine || machine.startedAtMs === null || machine.boardId === null) return null
  const elapsed = (machine.faultAtMs ?? simTimeMs) - machine.startedAtMs
  const progress = Math.max(0, Math.min(1, elapsed / machine.processTimeMs))
  return Math.min(2, Math.floor(progress * 3))
}
