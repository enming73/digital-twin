import type { MachineSnapshot } from '../simulation/engine'

/** Illustrative normalized process pose, never machine coordinates. */
export function machineMotion(machine: MachineSnapshot, simTimeMs: number) {
  const elapsed = machine.startedAtMs === null ? 0 : machine.state === 'RUNNING' || machine.state === 'FAULT' && machine.remainingMs !== null
    ? Math.max(0, Math.min(machine.processTimeMs, (machine.faultAtMs ?? simTimeMs) - machine.startedAtMs))
    : machine.processTimeMs
  const cycle = machine.startedAtMs === null ? 0 : elapsed / machine.processTimeMs
  const sweep = Math.sin(cycle * Math.PI * 2)
  const lift = (1 - Math.cos(cycle * Math.PI * 2)) / 2
  return { cycle, sweep, lift }
}
