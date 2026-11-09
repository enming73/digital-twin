import { describe, expect, it } from 'vitest'
import { defaultScenario } from '../../src/data/scenario'
import { Simulation } from '../../src/simulation/engine'
import { localScene } from '../../src/twin/localAdapter'
import { applyMessage, parseSnapshot } from '../../src/twin/projection'

describe('local scene adapter', () => {
  it('preserves station motion inputs and all visible board identities', () => {
    const simulation = new Simulation(defaultScenario)
    simulation.start()
    const snapshot = simulation.snapshot()
    const scene = localScene(snapshot)
    expect(Object.keys(scene.stations)).toEqual(snapshot.stations.map(station => station.id))
    for (const station of snapshot.stations) {
      expect(scene.stations[station.id].localMotion).toEqual(station)
      expect(scene.stations[station.id].state).toBe(station.state)
    }
    expect(scene.boards.map(board => board.id).sort()).toEqual(snapshot.stations.flatMap(station => [...station.queue, ...(station.boardId ? [station.boardId] : [])]).sort())
    expect(new Set(scene.boards.map(board => board.id)).size).toBe(scene.boards.length)
    expect(scene.simTimeMs).toBe(snapshot.simTimeMs)
  })
})

describe('W3 projection', () => {
  const snapshot = { runId: 'demo-run', epoch: 'epoch-1', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: { loader: { observedState: '', flowState: '', reason: '', alarms: {} } }, boards: {}, workOrders: {}, lastIngestedAt: '0001-01-01T00:00:00Z', emittedAt: '' }
  const event = { schemaVersion: 1, eventId: 'e1', runId: 'demo-run', source: 'simulator', machineId: null, eventType: 'workorder.started', sequence: 1, simTimeMs: 0, emittedAt: '2026-11-08T12:00:00Z', payload: { synthetic: true, workOrderId: 'wo-1', quantity: 1 } }
  it('validates snapshots and applies one contiguous event', () => {
    const state = parseSnapshot({ ...snapshot, reason: 'awaiting first event' }, 'demo-run')!
    expect(state).not.toBeNull()
    expect(parseSnapshot(snapshot, 'other')).toBeNull()
    const result = applyMessage(state, { type: 'delta', epoch: 'epoch-1', revision: 1, integrity: 'complete', event })
    expect(result.kind).toBe('applied')
    if (result.kind === 'applied') {
      expect(result.projection.workOrders['wo-1'].quantity).toBe(1)
      expect(result.projection.lastSequence).toBe(1)
      expect(result.projection.revision).toBe(1)
      expect(result.projection.reason).toBeUndefined()
    }
  })
  it('rejects gaps and epoch changes and applies status without advancing revision', () => {
    const state = parseSnapshot(snapshot, 'demo-run')!
    expect(applyMessage(state, { type: 'delta', epoch: 'epoch-1', revision: 2, integrity: 'complete', event }).kind).toBe('resync')
    expect(applyMessage(state, { type: 'delta', epoch: 'other', revision: 1, integrity: 'complete', event }).kind).toBe('resync')
    const status = applyMessage(state, { type: 'status', epoch: 'epoch-1', integrity: 'gap', reason: 'missing' })
    expect(status.kind).toBe('applied')
    if (status.kind === 'applied') expect([status.projection.revision, status.projection.integrity]).toEqual([0, 'gap'])
  })
  it('projects a board lifecycle, machine status, alarms and work order completion', () => {
    let state = parseSnapshot(snapshot, 'demo-run')!
    const send = (eventType: string, machineId: string | null, payload: Record<string, unknown>) => {
      const sequence = state.lastSequence + 1
      const result = applyMessage(state, { type: 'delta', epoch: state.epoch, revision: state.revision + 1, integrity: 'complete', event: { ...event, eventId: `e${sequence}`, sequence, eventType, machineId, payload: { synthetic: true, ...payload } } })
      expect(result.kind).toBe('applied')
      if (result.kind === 'applied') state = result.projection
    }
    send('workorder.started', null, { workOrderId: 'wo-1', quantity: 1 })
    send('machine.state.changed', 'loader', { observedState: 'FAULT', flowState: 'BLOCKED', reason: 'test' })
    send('machine.alarm.raised', 'loader', { alarmId: 'a1', reason: 'fault' })
    expect(state.machines.loader.alarms.a1).toBe('fault')
    send('machine.alarm.cleared', 'loader', { alarmId: 'a1', reason: 'fault' })
    expect(state.machines.loader.alarms.a1).toBeUndefined()
    send('pcb.entered', 'loader', { boardId: 'b1', workOrderId: 'wo-1' })
    expect(state.boards.b1.status).toBe('at_station')
    send('pcb.process.completed', 'loader', { boardId: 'b1', workOrderId: 'wo-1' })
    send('pcb.inspection.completed', 'loader', { boardId: 'b1', workOrderId: 'wo-1', passed: true })
    expect(state.boards.b1.inspections.loader).toBe(true)
    send('pcb.exited', 'loader', { boardId: 'b1', workOrderId: 'wo-1' })
    expect(state.boards.b1.status).toBe('completed')
    send('workorder.completed', null, { workOrderId: 'wo-1', quantity: 1, passed: 1, failed: 0 })
    expect(state.workOrders['wo-1'].status).toBe('completed')
  })
  it('uses explicit station order rather than alphabetical snapshot map order', () => {
    const machine = snapshot.machines.loader
    const state = parseSnapshot({ ...snapshot, machines: { aoi: machine, loader: machine, printer: machine, spi: machine, mounter: machine, reflow: machine, unloader: machine }, workOrders: { 'wo-1': { id: 'wo-1', quantity: 1, passed: 0, failed: 0, status: 'running' } } }, 'demo-run')!
    const result = applyMessage(state, { type: 'delta', epoch: state.epoch, revision: 1, integrity: 'complete', event: { ...event, eventType: 'pcb.entered', machineId: 'loader', payload: { synthetic: true, workOrderId: 'wo-1', boardId: 'b1' } } })
    expect(result.kind).toBe('applied')
  })
  it('rejects malformed envelopes and impossible board events', () => {
    const state = parseSnapshot(snapshot, 'demo-run')!
    const delta = (changes: Record<string, unknown>) => ({ type: 'delta', epoch: state.epoch, revision: 1, integrity: 'complete', event: { ...event, ...changes } })
    expect(applyMessage(state, delta({ eventId: '' })).kind).toBe('resync')
    expect(applyMessage(state, delta({ source: 'unknown' })).kind).toBe('resync')
    expect(applyMessage(state, delta({ eventType: 'pcb.exited', machineId: 'loader', payload: { synthetic: true, workOrderId: 'missing', boardId: 'b1' } })).kind).toBe('resync')
    expect(applyMessage({ ...state, integrity: 'gap' }, delta({})).kind).toBe('resync')
  })
  it('rejects snapshots with unknown station identities', () => {
    expect(parseSnapshot({ ...snapshot, machines: { stranger: snapshot.machines.loader } }, 'demo-run')).toBeNull()
    expect(parseSnapshot({ ...snapshot, boards: { b1: { id: 'b1', workOrderId: 'wo', machineId: 'stranger', status: 'at_station', inspections: {}, processed: false } } }, 'demo-run')).toBeNull()
  })
  it('rejects snapshots whose revision and sequence disagree', () => {
    expect(parseSnapshot({ ...snapshot, revision: 3, lastSequence: 2 }, 'demo-run')).toBeNull()
  })
})
