import { describe, expect, it } from 'vitest'
import { Simulation, type Scenario } from '../../src/simulation/engine'
import { defaultScenario } from '../../src/data/scenario'

const scenario: Scenario = { seed: 42, quantity: 100, defectRate: 0.2, stations: [
  { id: 'a', processTimeMs: 50, inputCapacity: 2 },
  { id: 'b', processTimeMs: 300, inputCapacity: 1 },
  { id: 'c', processTimeMs: 80, inputCapacity: 2 },
] }

describe('deterministic production simulation', () => {
  it('completes 100 unique boards with bounded queues and deterministic logs', () => {
    const run = () => {
      const sim = new Simulation(scenario)
      sim.start()
      for (let i = 0; i < 400 && sim.snapshot().completed < 100; i++) {
        sim.advance(100)
        const state = sim.snapshot()
        for (const station of state.stations) expect(station.queue.length).toBeLessThanOrEqual(station.inputCapacity)
        const locations = state.stations.flatMap(s => [...s.queue, ...(s.boardId ? [s.boardId] : [])])
        expect(new Set(locations).size).toBe(locations.length)
      }
      expect(sim.snapshot().completed).toBe(100)
      const completed = sim.events().filter(e => e.type === 'BOARD_COMPLETED')
      expect(new Set(completed.map(e => e.payload.boardId)).size).toBe(100)
      expect(sim.events().some(e => e.type === 'MACHINE_STATE_CHANGED' && e.payload.state === 'BLOCKED')).toBe(true)
      return sim.events()
    }
    expect(run()).toEqual(run())
  })
  it('freezes while paused, resets, and scales only wall time', () => {
    const a = new Simulation(scenario); a.start(); a.advanceWall(200, 5); a.pause()
    const paused = a.snapshot(); a.advanceWall(1000, 5)
    expect(a.snapshot()).toEqual(paused)
    const b = new Simulation(scenario); b.start(); b.advanceWall(1000, 1)
    expect(b.snapshot().simTimeMs).toBe(paused.simTimeMs)
    expect(b.events().filter(e => e.type !== 'RUN_PAUSED')).toEqual(a.events().filter(e => e.type !== 'RUN_PAUSED'))
    a.reset(); expect(a.snapshot().simTimeMs).toBe(0); expect(a.snapshot().created).toBe(0)
    a.start(); a.advance(1000); expect(a.snapshot().created).toBeGreaterThan(0)
  })
  it('rejects invalid configurations and invalid clock advances', () => {
    expect(() => new Simulation({ ...scenario, stations: [] })).toThrow()
    expect(() => new Simulation({ ...scenario, quantity: -1 })).toThrow()
    expect(() => new Simulation({ ...scenario, defectRate: 2 })).toThrow()
    expect(() => new Simulation({ ...scenario, stations: [{ id: 'a', processTimeMs: 0, inputCapacity: 1 }] })).toThrow()
    const sim = new Simulation(scenario); expect(() => sim.advance(-1)).toThrow()
  })
  it('finishes the seven-station scenario at the same virtual instant regardless of advance size', () => {
    const large = new Simulation(defaultScenario); large.start(); large.advance(2_000_000)
    const small = new Simulation(defaultScenario); small.start()
    for (let i = 0; i < 2000 && small.snapshot().running; i++) small.advance(1000)
    expect(large.snapshot().completed).toBe(100)
    expect(large.snapshot()).toEqual(small.snapshot())
    expect(large.events()).toEqual(small.events())
    expect(large.events().filter(e => e.type === 'INSPECTION_RECORDED')).toHaveLength(200)
    const differentSeed = new Simulation({ ...defaultScenario, seed: 7 }); differentSeed.start(); differentSeed.advance(2_000_000)
    expect(differentSeed.events().filter(e => e.type === 'INSPECTION_RECORDED')).not.toEqual(large.events().filter(e => e.type === 'INSPECTION_RECORDED'))
  })
})
