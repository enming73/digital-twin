import type { SceneBoard, SceneModel } from '../twin/types'
import { stations } from '../data/stations'
import { easeInOutCubic } from './cameraTween'

/** A visual-only transfer between two stations; `startMs` and `durationMs` are in simulated time. */
export type Transfer = { boardId: string; fromX: number; startMs: number; durationMs: number }

const MIN_DURATION = 350
const MAX_DURATION = 900
const MS_PER_METRE = 110

/** Illustrative travel time, not a belt specification. */
export function transferDuration(fromX: number, toX: number): number {
  return Math.min(MAX_DURATION, Math.max(MIN_DURATION, Math.abs(toX - fromX) * MS_PER_METRE))
}

/** Where a board belongs when it is not travelling: its queue slot or its place on the conveyor. */
export function boardTarget(board: SceneBoard, scene: SceneModel): { x: number; z: number } {
  const station = stations.find(item => item.id === board.stationId)
  const x = station?.position[0] ?? 0
  if (board.position === 'processing') {
    const machine = scene.stations[board.stationId]?.localMotion
    const progress = !machine || machine.startedAtMs === null
      ? 1
      : Math.max(0, Math.min(1, ((machine.faultAtMs ?? scene.simTimeMs) - machine.startedAtMs) / machine.processTimeMs))
    return { x: x - 0.55 + progress * 1.1, z: 0.12 }
  }
  if (board.position === 'in_transit') return { x, z: 0.12 }
  return { x: x - 1.2 + board.queueIndex * 0.5, z: -0.18 }
}

function xOf(stationId: string): number {
  return stations.find(item => item.id === stationId)?.position[0] ?? 0
}

/** One transfer per board whose station changed between two polls of the same run. */
export function startTransfers(previous: SceneModel | null, next: SceneModel): Transfer[] {
  if (!previous || next.simTimeMs <= previous.simTimeMs) return []
  const before = new Map(previous.boards.map(board => [board.id, board.stationId]))
  const transfers: Transfer[] = []
  for (const board of next.boards) {
    const from = before.get(board.id)
    if (!from || from === board.stationId) continue
    const fromX = xOf(from)
    transfers.push({ boardId: board.id, fromX, startMs: previous.simTimeMs, durationMs: transferDuration(fromX, xOf(board.stationId)) })
  }
  return transfers
}

/** Keeps the transfers still in flight, adds new ones, and drops everything when time runs backwards. */
export function updateTransfers(active: Transfer[], previous: SceneModel | null, next: SceneModel): Transfer[] {
  if (previous && next.simTimeMs < previous.simTimeMs) return []
  const present = new Set(next.boards.map(board => board.id))
  const running = active.filter(transfer => present.has(transfer.boardId) && next.simTimeMs - transfer.startMs < transfer.durationMs)
  return [...running, ...startTransfers(previous, next)]
}

/** Eased x between the old station and the board's current target. */
export function boardX(transfer: Transfer | undefined, targetX: number, simTimeMs: number): number {
  if (!transfer) return targetX
  const fraction = Math.max(0, Math.min(1, (simTimeMs - transfer.startMs) / transfer.durationMs))
  return transfer.fromX + (targetX - transfer.fromX) * easeInOutCubic(fraction)
}
