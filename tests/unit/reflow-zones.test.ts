import { describe, expect, it } from 'vitest'
import { activeZone } from '../../src/scene/reflowZones'
import type { MachineSnapshot } from '../../src/simulation/engine'

function oven(overrides: Partial<MachineSnapshot>): MachineSnapshot {
  return {
    id: 'reflow', state: 'RUNNING', boardId: 'board-1', queue: [], processTimeMs: 7000,
    startedAtMs: 1000, faultAtMs: null, inputCapacity: 2, outputCapacity: 2,
    ...overrides,
  } as MachineSnapshot
}

describe('reflow zone indicator', () => {
  it('walks the board through three conceptual zones', () => {
    expect(activeZone(oven({}), 1000)).toBe(0)
    expect(activeZone(oven({}), 3400)).toBe(1)
    expect(activeZone(oven({}), 6800)).toBe(2)
  })

  it('holds the last zone once processing completes', () => {
    expect(activeZone(oven({}), 8000)).toBe(2)
  })

  it('lights nothing when the oven is empty, halted or faulted at an unknown time', () => {
    expect(activeZone(null, 4000)).toBeNull()
    expect(activeZone(oven({ boardId: null, startedAtMs: null }), 4000)).toBeNull()
    expect(activeZone(oven({ startedAtMs: null }), 4000)).toBeNull()
  })

  it('freezes the zone while the machine is faulted', () => {
    expect(activeZone(oven({ faultAtMs: 2600 }), 9000)).toBe(0)
  })
})
