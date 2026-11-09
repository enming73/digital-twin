import type { RemoteVisualState } from '../scene/remoteMotion'
import type { RemoteProjection, SceneModel } from './types'

export function remoteScene(projection: RemoteProjection, visual: RemoteVisualState): SceneModel {
  const stations: SceneModel['stations'] = {}
  for (const [id, machine] of Object.entries(projection.machines)) {
    const state = machine.observedState === 'FAULT' ? 'FAULT' : machine.flowState === 'BLOCKED' || machine.flowState === 'STARVED' ? machine.flowState : machine.observedState || 'UNKNOWN'
    stations[id] = { state, observedState: machine.observedState, flowState: machine.flowState, reason: machine.reason, localMotion: null }
  }
  return {
    simTimeMs: projection.simTimeMs,
    stations,
    boards: Object.entries(visual.boards).map(([id, board]) => ({ id, stationId: board.machineId, position: board.status === 'in_transit' ? 'in_transit' : 'processing', queueIndex: 0, visual: { fromX: board.fromX, toX: board.toX, startedAt: board.startedAt } })),
  }
}
