import { stations } from '../data/stations'
import type { RemoteProjection } from '../twin/types'

type VisualBoard = { machineId: string; status: string; fromX: number; toX: number; startedAt: number }
export type RemoteVisualState = { epoch: string; revision: number; boards: Record<string, VisualBoard> }
const xFor = (machineId: string, status: string): number | null => {
  const station = stations.find(item => item.id === machineId)
  return station ? station.position[0] + (status === 'in_transit' ? 1.2 : 0) : null
}
export function resetRemoteVisual(projection: RemoteProjection): RemoteVisualState {
  const boards: Record<string, VisualBoard> = {}
  for (const board of Object.values(projection.boards)) {
    if (board.status === 'completed' || !board.machineId) continue
    const x = xFor(board.machineId, board.status)
    if (x !== null) boards[board.id] = { machineId: board.machineId, status: board.status, fromX: x, toX: x, startedAt: 0 }
  }
  return { epoch: projection.epoch, revision: projection.revision, boards }
}
export function nextRemoteVisual(previous: RemoteVisualState, projection: RemoteProjection, nowMs: number): RemoteVisualState {
  if (previous.epoch !== projection.epoch || projection.revision < previous.revision || projection.revision > previous.revision + 1) return resetRemoteVisual(projection)
  if (projection.revision === previous.revision) return previous
  const next = resetRemoteVisual(projection)
  for (const [id, board] of Object.entries(next.boards)) {
    const prior = previous.boards[id]
    if (!prior) continue
    const fromIndex = stations.findIndex(station => station.id === prior.machineId)
    const toIndex = stations.findIndex(station => station.id === board.machineId)
    if (prior.status === 'in_transit' && board.status === 'at_station' && toIndex === fromIndex + 1) {
      board.fromX = prior.toX; board.startedAt = nowMs
    } else if (prior.machineId === board.machineId && prior.status === board.status) {
      next.boards[id] = prior
    }
  }
  return next
}
