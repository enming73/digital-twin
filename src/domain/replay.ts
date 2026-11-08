import type { Board, MachineSnapshot, ProductionEvent, Scenario, Snapshot } from '../simulation/engine'

/** Rebuild a read-only state by applying all recorded events at or before target time. */
export function replayAt(config: Scenario, events: readonly ProductionEvent[], targetMs: number): Snapshot {
  if (!Number.isFinite(targetMs) || targetMs < 0) throw new Error('Invalid replay time')
  const stations: MachineSnapshot[] = config.stations.map(s => ({ ...s, queue: [], boardId: null, state: 'IDLE', startedAtMs: null, finishAtMs: null, faultAtMs: null, remainingMs: null }))
  const boards: Board[] = []
  let running = false; let completed = 0
  for (const event of events) {
    if (event.simTimeMs > targetMs) break
    const payload = event.payload
    const id = String(payload.boardId ?? '')
    const board = boards.find(b => b.id === id)
    const machine = stations.find(s => s.id === payload.stationId)
    switch (event.type) {
      case 'RUN_STARTED': case 'RUN_RESUMED': running = true; break
      case 'RUN_PAUSED': case 'WORK_ORDER_COMPLETED': running = false; break
      case 'BOARD_CREATED': {
        const stationId = String(payload.stationId)
        boards.push({ id, workOrderId: 'work-order-1', stationId, status: 'QUEUED', createdAtMs: event.simTimeMs, completedAtMs: null, inspections: [] })
        stations.find(s => s.id === stationId)!.queue.push(id); break
      }
      case 'PROCESS_STARTED': if (machine && board) { machine.queue.splice(machine.queue.indexOf(id), 1); machine.boardId = id; machine.startedAtMs = event.simTimeMs; machine.finishAtMs = event.simTimeMs + machine.processTimeMs; board.status = 'PROCESSING' } break
      case 'PROCESS_COMPLETED': if (machine) machine.finishAtMs = null; break
      case 'BOARD_TRANSFERRED': if (board) { const from = stations.find(s => s.id === payload.from)!; const to = stations.find(s => s.id === payload.to)!; from.boardId = null; from.startedAtMs = null; to.queue.push(id); board.stationId = to.id; board.status = 'QUEUED' } break
      case 'BOARD_COMPLETED': if (board && machine) { machine.boardId = null; machine.startedAtMs = null; board.stationId = null; board.status = 'COMPLETED'; board.completedAtMs = event.simTimeMs; completed++ } break
      case 'INSPECTION_RECORDED': if (board && machine) board.inspections.push({ stationId: machine.id, passed: payload.passed === true }); break
      case 'MACHINE_STATE_CHANGED': if (machine) { machine.state = payload.state as MachineSnapshot['state']; if (machine.state === 'BLOCKED' && machine.boardId) boards.find(b => b.id === machine.boardId)!.status = 'BLOCKED' } break
      case 'FAULT_INJECTED': if (machine) { machine.faultAtMs = event.simTimeMs; machine.remainingMs = payload.remainingMs === -1 ? null : Number(payload.remainingMs); machine.finishAtMs = null } break
      case 'FAULT_CLEARED': if (machine) { machine.faultAtMs = null; machine.remainingMs = null; machine.startedAtMs = payload.startedAtMs === -1 ? null : Number(payload.startedAtMs); machine.finishAtMs = payload.finishAtMs === -1 ? null : Number(payload.finishAtMs) } break
    }
  }
  return { simTimeMs: targetMs, running, created: boards.length, completed, quantity: config.quantity, stations, boards }
}
