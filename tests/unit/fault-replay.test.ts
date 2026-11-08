import { describe, expect, it } from 'vitest'
import { Simulation, type Scenario } from '../../src/simulation/engine'
import { replayAt } from '../../src/domain/replay'
import { getMetrics } from '../../src/domain/analytics'
import { machineMotion } from '../../src/scene/motion'

const config: Scenario = { seed: 1, quantity: 20, defectRate: 0, stations: [
  { id: 'upstream', processTimeMs: 20, inputCapacity: 1 },
  { id: 'mounter', processTimeMs: 40, inputCapacity: 1 },
  { id: 'downstream', processTimeMs: 30, inputCapacity: 1 },
] }

describe('fault and historical replay', () => {
  it('holds work, propagates only as buffers fill or drain, and resumes without loss', () => {
    const sim = new Simulation(config); sim.start(); sim.advance(85)
    const before = sim.snapshot().stations[1]; expect(before.boardId).not.toBeNull()
    sim.injectFault('mounter')
    const faulted = sim.snapshot().stations[1]; expect(faulted.state).toBe('FAULT')
    const pose = machineMotion(before, 85)
    expect(faulted.boardId).toBe(before.boardId)
    sim.advance(500)
    const halted = sim.snapshot(); expect(halted.stations[1].boardId).toBe(before.boardId)
    expect(machineMotion(halted.stations[1], halted.simTimeMs)).toEqual(pose)
    expect(halted.stations[0].state).toBe('BLOCKED'); expect(halted.stations[1].queue).toHaveLength(1)
    expect(halted.stations[2].state).toBe('STARVED'); expect(halted.stations[2].queue).toHaveLength(0); expect(halted.stations[2].boardId).toBeNull()
    const ids = halted.stations.flatMap(s => [...s.queue, ...(s.boardId ? [s.boardId] : [])]); expect(new Set(ids).size).toBe(ids.length)
    sim.clearFault('mounter'); expect(machineMotion(sim.snapshot().stations[1], sim.snapshot().simTimeMs)).toEqual(pose); sim.advance(10000)
    expect(sim.snapshot().completed).toBe(20)
    expect(new Set(sim.events().filter(e => e.type === 'BOARD_COMPLETED').map(e => e.payload.boardId)).size).toBe(20)
  })
  it('reconstructs the same snapshot and KPIs from ordered events at a historical time', () => {
    const sim = new Simulation(config); sim.start(); sim.advance(85); sim.injectFault('mounter'); sim.advance(200)
    const expected = sim.snapshot(); const expectedEvents = sim.events(); sim.advance(300); sim.clearFault('mounter'); sim.advance(500)
    const actual = replayAt(config, sim.events(), 285)
    expect(actual).toEqual(expected)
    expect(replayAt(config, sim.events(), 285)).toEqual(actual)
    expect(getMetrics(actual, sim.events().filter(e => e.simTimeMs <= 285), 300000)).toEqual(getMetrics(expected, expectedEvents, 300000))
    expect(replayAt(config, sim.events(), sim.snapshot().simTimeMs)).toEqual(sim.snapshot())
  })
})
