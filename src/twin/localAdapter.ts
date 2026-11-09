import type { Snapshot } from '../simulation/engine'
import type { SceneBoard, SceneModel } from './types'

export function localScene(snapshot: Snapshot): SceneModel {
  const boards: SceneBoard[] = []
  const stations: SceneModel['stations'] = {}
  for (const machine of snapshot.stations) {
    stations[machine.id] = { state: machine.state, observedState: null, flowState: null, reason: null, localMotion: machine }
    machine.queue.forEach((id, queueIndex) => boards.push({ id, stationId: machine.id, position: 'queued', queueIndex }))
    if (machine.boardId) boards.push({ id: machine.boardId, stationId: machine.id, position: 'processing', queueIndex: 0 })
  }
  return { simTimeMs: snapshot.simTimeMs, stations, boards }
}
