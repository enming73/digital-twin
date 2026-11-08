import { describe, expect, it } from 'vitest'
import { Presentation } from '../../src/presentation/presentation'
import { Simulation } from '../../src/simulation/engine'
import { defaultScenario } from '../../src/data/scenario'

describe('guided presentation', () => {
  it('runs the identical fault and recovery scenario regardless of advancement chunks', () => {
    const run = (step: number) => {
      const sim = new Simulation(defaultScenario); const tour = new Presentation(sim)
      tour.start()
      while (!tour.finished) tour.advance(Math.min(step, 210000 - sim.snapshot().simTimeMs))
      expect(sim.events().find(e => e.type === 'FAULT_INJECTED')?.simTimeMs).toBe(70000)
      expect(sim.events().find(e => e.type === 'FAULT_CLEARED')?.simTimeMs).toBe(145000)
      expect(sim.snapshot().simTimeMs).toBe(210000)
      return sim.events()
    }
    expect(run(5000)).toEqual(run(17000))
  })
  it('pauses the simulation and restarts from the same initial state', () => {
    const sim = new Simulation(defaultScenario); const tour = new Presentation(sim)
    tour.start(); tour.advance(25000); tour.pause(); const frozen = sim.snapshot()
    tour.advance(20000); expect(sim.snapshot()).toEqual(frozen)
    tour.resume(); tour.advance(1000); expect(sim.snapshot().simTimeMs).toBe(26000)
    tour.start(); expect(sim.snapshot().simTimeMs).toBe(0); expect(tour.chapter.id).toBe('overview')
  })
  it('shows capacity-driven propagation during the scripted fault', () => {
    const sim = new Simulation(defaultScenario); const tour = new Presentation(sim)
    tour.start(); tour.advance(120000)
    const stations = sim.snapshot().stations
    expect(stations.find(s => s.id === 'mounter')?.state).toBe('FAULT')
    expect(stations.slice(0, 3).some(s => s.state === 'BLOCKED')).toBe(true)
    expect(stations.slice(4).some(s => s.state === 'STARVED')).toBe(true)
    tour.advance(30000)
    expect(sim.snapshot().stations.find(s => s.id === 'mounter')?.state).not.toBe('FAULT')
  })
})
