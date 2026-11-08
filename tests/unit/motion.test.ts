import { describe, expect, it } from 'vitest'
import { machineMotion } from '../../src/scene/motion'
import { Simulation } from '../../src/simulation/engine'
import { defaultScenario } from '../../src/data/scenario'

describe('illustrative machine motion', () => {
  it('uses bounded deterministic virtual-time poses and freezes without virtual-time advancement', () => {
    const sim = new Simulation(defaultScenario); sim.start(); sim.advance(750)
    const state = sim.snapshot(); const machine = state.stations[0]
    const pose = machineMotion(machine, state.simTimeMs)
    expect(pose.cycle).toBeGreaterThanOrEqual(0); expect(pose.cycle).toBeLessThanOrEqual(1)
    expect(pose).not.toEqual(machineMotion(machine, state.simTimeMs + 500))
    expect(pose).toEqual(machineMotion(machine, state.simTimeMs))
    sim.pause(); sim.advance(1000)
    expect(machineMotion(sim.snapshot().stations[0], sim.snapshot().simTimeMs)).toEqual(pose)
  })
  it('holds stopped machines at a fixed pose even as other stations advance', () => {
    const sim = new Simulation(defaultScenario)
    const machine = sim.snapshot().stations[0]
    expect(machineMotion(machine, 0)).toEqual(machineMotion(machine, 10000))
    expect(machineMotion({ ...machine, state: 'BLOCKED', startedAtMs: 0 }, 5000)).toEqual(machineMotion({ ...machine, state: 'BLOCKED', startedAtMs: 0 }, 15000))
  })
  it('produces the same pose at the same virtual time at different speeds', () => {
    const a = new Simulation(defaultScenario); const b = new Simulation(defaultScenario)
    a.start(); b.start(); a.advanceWall(1000, 1); b.advanceWall(200, 5)
    expect(machineMotion(a.snapshot().stations[0], a.snapshot().simTimeMs)).toEqual(machineMotion(b.snapshot().stations[0], b.snapshot().simTimeMs))
  })
})
