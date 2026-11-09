import type { Integrity, RemoteProjection } from './types'
import { stations } from '../data/stations'

const object = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v)
const integer = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0
const integrity = (v: unknown): v is Integrity => v === 'complete' || v === 'uncertain' || v === 'gap' || v === 'conflict'

export function parseSnapshot(value: unknown, runId: string): RemoteProjection | null {
  if (!object(value) || value.runId !== runId || typeof value.epoch !== 'string' || !value.epoch || !integer(value.revision) || !integer(value.lastSequence) || value.revision !== value.lastSequence || !integer(value.simTimeMs) || !integrity(value.integrity) || !object(value.machines) || !object(value.boards) || !object(value.workOrders)) return null
  for (const [id, machine] of Object.entries(value.machines)) {
    if (!stations.some(station => station.id === id)) return null
    if (!object(machine) || typeof machine.observedState !== 'string' || typeof machine.flowState !== 'string' || typeof machine.reason !== 'string' || !object(machine.alarms) || Object.values(machine.alarms).some(v => typeof v !== 'string')) return null
  }
  for (const [id, board] of Object.entries(value.boards)) {
    if (!object(board) || board.id !== id || typeof board.workOrderId !== 'string' || !['at_station', 'in_transit', 'completed'].includes(String(board.status)) || typeof board.machineId !== 'string' || !Object.hasOwn(value.machines, board.machineId) || !object(board.inspections) || Object.values(board.inspections).some(v => typeof v !== 'boolean') || typeof board.processed !== 'boolean') return null
  }
  for (const [id, order] of Object.entries(value.workOrders)) {
    if (!object(order) || order.id !== id || typeof order.quantity !== 'number' || typeof order.passed !== 'number' || typeof order.failed !== 'number' || typeof order.status !== 'string') return null
  }
  return structuredClone(value) as RemoteProjection
}

export function applyMessage(current: RemoteProjection, message: unknown): { kind: 'applied'; projection: RemoteProjection } | { kind: 'resync'; reason: string } {
  const fail = (reason: string) => ({ kind: 'resync' as const, reason })
  if (!object(message) || message.epoch !== current.epoch || !integrity(message.integrity)) return fail('invalid epoch or integrity')
  if (message.type === 'status') return { kind: 'applied', projection: { ...current, integrity: message.integrity, reason: typeof message.reason === 'string' ? message.reason : undefined } }
  if (current.integrity === 'gap' || current.integrity === 'conflict') return fail('backend projection frozen')
  if (message.type !== 'delta' || message.revision !== current.revision + 1 || !object(message.event)) return fail('invalid revision')
  const e = message.event
  if (e.schemaVersion !== 1 || e.source !== 'simulator' || typeof e.eventId !== 'string' || !e.eventId || typeof e.emittedAt !== 'string' || !Number.isFinite(Date.parse(e.emittedAt)) || e.runId !== current.runId || e.sequence !== current.lastSequence + 1 || !integer(e.simTimeMs) || e.simTimeMs < current.simTimeMs || !object(e.payload) || e.payload.synthetic !== true) return fail('invalid event')
  const next = structuredClone(current)
  const p = e.payload
  const machineId = typeof e.machineId === 'string' ? e.machineId : ''
  const machine = next.machines[machineId]
  if (e.eventType === 'workorder.started' && typeof e.payload.workOrderId === 'string' && typeof e.payload.quantity === 'number' && !next.workOrders[e.payload.workOrderId]) {
    next.workOrders[e.payload.workOrderId] = { id: e.payload.workOrderId, quantity: e.payload.quantity, passed: 0, failed: 0, status: 'running' }
  } else if (e.eventType === 'workorder.completed' && typeof p.workOrderId === 'string' && typeof p.quantity === 'number' && typeof p.passed === 'number' && typeof p.failed === 'number' && next.workOrders[p.workOrderId]?.status === 'running' && next.workOrders[p.workOrderId].quantity === p.quantity && p.passed + p.failed === p.quantity && Object.values(next.boards).filter(b => b.workOrderId === p.workOrderId && b.status === 'completed').length === p.quantity) {
    next.workOrders[p.workOrderId] = { id: p.workOrderId, quantity: p.quantity, passed: p.passed, failed: p.failed, status: 'completed' }
  } else if (e.eventType === 'machine.state.changed' && machine && typeof p.observedState === 'string' && typeof p.flowState === 'string' && typeof p.reason === 'string') {
    machine.observedState = p.observedState; machine.flowState = p.flowState; machine.reason = p.reason
  } else if ((e.eventType === 'machine.alarm.raised' || e.eventType === 'machine.alarm.cleared') && machine && typeof p.alarmId === 'string') {
    if (e.eventType === 'machine.alarm.raised' && typeof p.reason === 'string') machine.alarms[p.alarmId] = p.reason
    else if (e.eventType === 'machine.alarm.cleared') delete machine.alarms[p.alarmId]
    else return fail('invalid alarm')
  } else if (typeof e.eventType === 'string' && e.eventType.startsWith('pcb.') && machine && typeof p.boardId === 'string' && typeof p.workOrderId === 'string') {
    const id = p.boardId
    const workOrderId = p.workOrderId
    if (next.workOrders[workOrderId]?.status !== 'running') return fail('invalid work order')
    let board = next.boards[id]
    const stationIds = stations.map(station => station.id).filter(id => id in next.machines)
    if (e.eventType === 'pcb.entered') {
      if (!board) {
        if (stationIds[0] !== machineId) return fail('invalid first station')
        board = { id, workOrderId, inspections: {}, processed: false, status: 'at_station', machineId }
        next.boards[id] = board
      } else {
        if (board.workOrderId !== workOrderId || board.status !== 'in_transit' || stationIds.indexOf(machineId) !== stationIds.indexOf(board.machineId ?? '') + 1) return fail('invalid transfer')
        board.machineId = machineId; board.status = 'at_station'; board.processed = false
      }
    } else {
      if (!board || board.workOrderId !== workOrderId || board.status !== 'at_station' || board.machineId !== machineId) return fail('invalid board')
      if (e.eventType === 'pcb.process.completed' && !board.processed) board.processed = true
      else if (e.eventType === 'pcb.inspection.completed' && board.processed && typeof p.passed === 'boolean') board.inspections[machineId] = p.passed
      else if (e.eventType === 'pcb.exited' && board.processed) board.status = stationIds[stationIds.length - 1] === machineId ? 'completed' : 'in_transit'
      else return fail('invalid board event')
    }
  } else return fail('unsupported event')
  next.revision++
  next.lastSequence++
  next.simTimeMs = e.simTimeMs
  next.integrity = message.integrity
  if (next.integrity === 'complete') next.reason = undefined
  return { kind: 'applied', projection: next }
}
