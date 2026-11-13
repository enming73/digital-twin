import type { SceneBoard, SceneModel } from '../twin/types'
import { stations } from '../data/stations'
import { easeInOutCubic } from './cameraTween'

/** A visual-only transfer between two stations; all times are simulated milliseconds. */
export type Transfer = { boardId: string; fromX: number; fromZ: number; startMs: number; durationMs: number }

const MIN_DURATION = 350
const MAX_DURATION = 900
const MS_PER_METRE = 110

/**
 * Illustrative travel time, not a belt specification.
 * A hop must outlive at least two polls, otherwise 2× and 5× would never show a frame in flight.
 */
export function transferDuration(fromX: number, toX: number, pollGapMs = 0): number {
  const natural = Math.min(MAX_DURATION, Math.max(MIN_DURATION, Math.abs(toX - fromX) * MS_PER_METRE))
  return Math.max(natural, pollGapMs * 2)
}

/** Where a board belongs when it is not travelling: its queue slot or its place on the conveyor. */
export function boardTarget(board: SceneBoard, scene: SceneModel): { x: number; z: number } | null {
  const station = stations.find(item => item.id === board.stationId)
  if (!station) return null
  const x = station.position[0]
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

/**
 * One transfer per board whose station changed between two polls of the same run.
 * The hop starts where the board was last drawn and at the poll that discovered it.
 */
export function startTransfers(previous: SceneModel | null, next: SceneModel): Transfer[] {
  if (!previous || next.simTimeMs < previous.simTimeMs) return []
  const gap = next.simTimeMs - previous.simTimeMs
  const before = new Map(previous.boards.map(board => [board.id, board]))
  const transfers: Transfer[] = []
  for (const board of next.boards) {
    const was = before.get(board.id)
    if (!was || was.stationId === board.stationId) continue
    const origin = boardTarget(was, previous)
    if (!origin) continue
    const toX = xOf(board.stationId)
    transfers.push({
      boardId: board.id,
      fromX: origin.x,
      fromZ: origin.z,
      startMs: next.simTimeMs,
      durationMs: transferDuration(origin.x, toX, gap),
    })
  }
  return transfers
}

/** Keeps the transfers still in flight, adds new ones, and drops everything when time runs backwards. */
export function updateTransfers(active: Transfer[], previous: SceneModel | null, next: SceneModel): Transfer[] {
  if (previous && next.simTimeMs < previous.simTimeMs) return []
  // Remote boards carry their own server-side interpolation; the local transfer model must not count them.
  if (next.boards.some(board => board.visual)) return []
  const present = new Set(next.boards.map(board => board.id))
  const running = active.filter(transfer => present.has(transfer.boardId) && next.simTimeMs - transfer.startMs < transfer.durationMs)
  const created = startTransfers(previous, next)
    .filter(transfer => next.simTimeMs - transfer.startMs < transfer.durationMs)
  return [...running, ...created]
}

/** Eased x between where the board was drawn and its current target. */
export function boardX(transfer: Transfer | undefined, targetX: number, simTimeMs: number): number {
  if (!transfer) return targetX
  const fraction = Math.max(0, Math.min(1, (simTimeMs - transfer.startMs) / transfer.durationMs))
  return transfer.fromX + (targetX - transfer.fromX) * easeInOutCubic(fraction)
}

/** Eased z, so a board leaving the conveyor for a queue slot does not snap sideways. */
export function boardZ(transfer: Transfer | undefined, targetZ: number, simTimeMs: number): number {
  if (!transfer) return targetZ
  const fraction = Math.max(0, Math.min(1, (simTimeMs - transfer.startMs) / transfer.durationMs))
  return transfer.fromZ + (targetZ - transfer.fromZ) * easeInOutCubic(fraction)
}
