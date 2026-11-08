import type { ProductionEvent, Snapshot } from '../simulation/engine'

export function getMetrics(snapshot: Snapshot, events: readonly ProductionEvent[], windowMs: number) {
  const start = Math.max(0, snapshot.simTimeMs - windowMs)
  const completedIds = new Set(events.filter(e => e.type === 'BOARD_COMPLETED' && e.simTimeMs >= start && e.simTimeMs <= snapshot.simTimeMs).map(e => e.payload.boardId))
  const completed = snapshot.boards.filter(b => completedIds.has(b.id) && b.completedAtMs !== null)
  const inspected = completed.filter(b => b.inspections.length > 0)
  const elapsed = snapshot.simTimeMs - start
  return {
    completed: completed.length,
    wip: snapshot.created - snapshot.completed,
    throughputPerHour: elapsed > 0 ? completed.length * 3600000 / elapsed : null,
    avgCycleMs: completed.length ? completed.reduce((sum, b) => sum + b.completedAtMs! - b.createdAtMs, 0) / completed.length : null,
    qualityPassRate: inspected.length ? inspected.filter(b => b.inspections.every(i => i.passed)).length / inspected.length : null,
  }
}

export function boardHistory(events: readonly ProductionEvent[], boardId: string): ProductionEvent[] {
  return events.filter(event => event.payload.boardId === boardId)
}

export function alarmEvents(events: readonly ProductionEvent[]): ProductionEvent[] {
  return events.filter(event => event.type === 'FAULT_INJECTED' || event.type === 'INSPECTION_RECORDED' && event.payload.passed === false)
}

export function throughputSeries(events: readonly ProductionEvent[], nowMs: number, windowMs: number, bins = 12) {
  const start = Math.max(0, nowMs - windowMs)
  const span = nowMs - start
  const points = Array.from({ length: bins }, (_, i) => ({ timeMs: start + span * (i + 1) / bins, count: 0 }))
  if (span <= 0) return points
  for (const e of events) if (e.type === 'BOARD_COMPLETED' && e.simTimeMs >= start && e.simTimeMs <= nowMs) {
    const index = Math.min(bins - 1, Math.floor((e.simTimeMs - start) / span * bins))
    points[index].count++
  }
  return points
}
