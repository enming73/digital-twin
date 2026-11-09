import { describe, expect, it } from 'vitest'
import { nextRemoteVisual, resetRemoteVisual } from '../../src/scene/remoteMotion'
import type { RemoteProjection } from '../../src/twin/types'
import { remoteScene } from '../../src/twin/remoteAdapter'

const state: RemoteProjection = { runId: 'demo-run', epoch: 'a', revision: 1, lastSequence: 1, simTimeMs: 0, integrity: 'complete', machines: {}, workOrders: {}, boards: { b1: { id: 'b1', workOrderId: 'wo', machineId: 'loader', status: 'at_station', inspections: {}, processed: false } } }
const update = (machineId: string, status: string, revision: number): RemoteProjection => ({ ...state, revision, lastSequence: revision, boards: { b1: { ...state.boards.b1, machineId, status } } })

describe('remote visual state', () => {
  it('animates only an observed exit followed by entry and resets on replacement', () => {
    const initial = resetRemoteVisual(state)
    const exited = nextRemoteVisual(initial, update('loader', 'in_transit', 2), 10)
    const entered = nextRemoteVisual(exited, update('printer', 'at_station', 3), 20)
    expect(entered.boards.b1.fromX).not.toBe(entered.boards.b1.toX)
    expect(entered.boards.b1.startedAt).toBe(20)
    const reset = resetRemoteVisual({ ...state, epoch: 'b', boards: {} })
    expect(Object.keys(reset.boards)).toEqual([])
  })
  it('removes completed boards and does not invent an unseen transfer', () => {
    const initial = resetRemoteVisual(state)
    const entered = nextRemoteVisual(initial, update('printer', 'at_station', 2), 20)
    expect(entered.boards.b1.fromX).toBe(entered.boards.b1.toX)
    const completed = nextRemoteVisual(entered, update('printer', 'completed', 3), 30)
    expect(Object.keys(completed.boards)).toEqual([])
  })
  it('maps observed fault and flow state without inventing local processing information', () => {
    const projection: RemoteProjection = { ...state, machines: { loader: { observedState: 'FAULT', flowState: 'BLOCKED', reason: 'motor', alarms: {} } } }
    const scene = remoteScene(projection, resetRemoteVisual(projection))
    expect(scene.stations.loader.state).toBe('FAULT')
    expect(scene.stations.loader.localMotion).toBeNull()
    expect(scene.boards.map(board => board.id)).toEqual(['b1'])
  })
})
