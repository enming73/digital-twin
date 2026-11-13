import { describe, expect, it } from 'vitest'
import { activeZone } from '../../src/scene/reflowZones'
import { boardTarget } from '../../src/scene/transfers'
import type { SceneModel } from '../../src/twin/types'
import { stations } from '../../src/data/stations'
import type { MachineSnapshot } from '../../src/simulation/engine'

function oven(overrides: Partial<MachineSnapshot>): MachineSnapshot {
  return {
    id: 'reflow', processTimeMs: 7000, inputCapacity: 2,
    state: 'RUNNING', boardId: 'board-1', queue: [], startedAtMs: 1000,
    finishAtMs: 8000, faultAtMs: null, remainingMs: null,
    ...overrides,
  }
}

describe('reflow zone indicator', () => {
  it('walks the board through three conceptual zones at exact boundaries', () => {
    const machine = oven({ processTimeMs: 6000, startedAtMs: 0, finishAtMs: 6000 })
    expect(activeZone(machine, 0)).toBe(0)
    expect(activeZone(machine, 1999)).toBe(0)
    expect(activeZone(machine, 2000)).toBe(1)
    expect(activeZone(machine, 3999)).toBe(1)
    expect(activeZone(machine, 4000)).toBe(2)
    expect(activeZone(machine, 6000)).toBe(2)
  })

  it('keeps the last zone while a finished board is held back by a full downstream buffer', () => {
    expect(activeZone(oven({ state: 'BLOCKED', finishAtMs: null, remainingMs: 0 }), 12000)).toBe(2)
  })

  it('lights nothing when the oven is empty or has not started', () => {
    expect(activeZone(null, 4000)).toBeNull()
    expect(activeZone(oven({ boardId: null, startedAtMs: null, finishAtMs: null }), 4000)).toBeNull()
    expect(activeZone(oven({ startedAtMs: null }), 4000)).toBeNull()
  })

  it('freezes the zone at the moment of a late fault', () => {
    expect(activeZone(oven({ faultAtMs: 6000 }), 20000)).toBe(2)
    expect(activeZone(oven({ faultAtMs: 2500 }), 20000)).toBe(0)
  })

  it('agrees with the board position used by the renderer', () => {
    const machine = oven({ processTimeMs: 6000, startedAtMs: 0, finishAtMs: 6000 })
    for (const elapsed of [0, 1000, 3000, 3500, 5000, 5500, 5999]) {
      const scene = {
        simTimeMs: elapsed,
        stations: { reflow: { state: machine.state, observedState: null, flowState: null, reason: null, localMotion: machine } },
        boards: [],
      } as SceneModel
      const target = boardTarget({ id: 'board-1', stationId: 'reflow', position: 'processing', queueIndex: 0 }, scene)!
      const stationX = stations.find(station => station.id === 'reflow')!.position[0]
      const third = Math.min(2, Math.max(0, Math.floor((target.x - stationX + 0.55) / (1.1 / 3))))
      expect(activeZone(machine, elapsed)).toBe(third)
    }
  })
})
