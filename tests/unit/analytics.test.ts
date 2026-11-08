import { describe, expect, it } from 'vitest'
import { getMetrics, boardHistory, alarmEvents, throughputSeries } from '../../src/domain/analytics'
import { Simulation } from '../../src/simulation/engine'
import { defaultScenario } from '../../src/data/scenario'

describe('simulated analytics', () => {
  it('calculates throughput, WIP, cycle and board quality from actual events', () => {
    const sim = new Simulation({ ...defaultScenario, quantity: 3, defectRate: 0 })
    sim.start(); sim.advance(200000)
    const snapshot = sim.snapshot(); const events = sim.events()
    const metrics = getMetrics(snapshot, events, 200000)
    expect(metrics.completed).toBe(3)
    expect(metrics.wip).toBe(0)
    expect(metrics.throughputPerHour).toBeCloseTo(3 * 3600000 / snapshot.simTimeMs)
    expect(metrics.avgCycleMs).toBeGreaterThan(0)
    expect(metrics.qualityPassRate).toBe(1)
    expect(boardHistory(events, 'board-1').filter(e => e.type === 'PROCESS_STARTED')).toHaveLength(7)
    expect(boardHistory(events, 'board-unknown')).toEqual([])
    expect(alarmEvents(events)).toEqual([])
    expect(throughputSeries(events, snapshot.simTimeMs, 200000).reduce((sum, p) => sum + p.count, 0)).toBe(3)
  })
  it('represents unavailable data as null and confines completed events to window', () => {
    const sim = new Simulation({ ...defaultScenario, quantity: 3, defectRate: 1 })
    expect(getMetrics(sim.snapshot(), sim.events(), 60000)).toMatchObject({ throughputPerHour: null, avgCycleMs: null, qualityPassRate: null, wip: 0 })
    sim.start(); sim.advance(200000)
    const snap = sim.snapshot(); const events = sim.events()
    const metrics = getMetrics(snap, events, 1)
    expect(metrics.completed).toBe(1)
    expect(metrics.qualityPassRate).toBe(0)
    expect(alarmEvents(events).length).toBe(6)
  })
  it('uses inclusive boundaries and the documented formulas on a known fixture', () => {
    const sim = new Simulation({ seed: 1, quantity: 2, defectRate: 0, stations: [{ id: 'spi', processTimeMs: 10, inputCapacity: 1 }] })
    sim.start(); sim.advance(20)
    const metrics = getMetrics(sim.snapshot(), sim.events(), 10)
    expect(metrics).toEqual({ completed: 2, wip: 0, throughputPerHour: 720000, avgCycleMs: 15, qualityPassRate: 1 })
    expect(throughputSeries(sim.events(), 20, 10).reduce((total, p) => total + p.count, 0)).toBe(2)
  })
})
