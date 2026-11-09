import type { Integrity, RemoteBoard, RemoteWorkOrder } from './types'

type Envelope = { runId: string; epoch: string; revision: number; integrity: Integrity; reason?: string; synthetic: true; route: string[] }
export type TraceEntry = { sequence: number; eventId: string; eventType: string; machineId: string | null; simTimeMs: number; emittedAt: string; ingestedAt: string; passed?: boolean }
export type BoardTrace = Envelope & { board: RemoteBoard; entries: TraceEntry[] }
export type WorkOrderDetail = Envelope & { order: RemoteWorkOrder; metadata: { description: string; synthetic: true } | null; progress: { planned: number; created: number; inProgress: number; completed: number; passed: number; failed: number; uninspected: number }; reconciliation: string }

const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
const str = (v: unknown) => typeof v === 'string'
const num = (v: unknown) => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0
function envelope(v: unknown, runId: string): v is Record<string, unknown> {
  return object(v) && v.runId === runId && str(v.epoch) && num(v.revision) && ['complete', 'uncertain', 'gap', 'conflict'].includes(String(v.integrity)) && v.synthetic === true && Array.isArray(v.route) && v.route.every(str)
}
async function request(path: string, signal: AbortSignal): Promise<unknown> {
  const response = await fetch(path, { signal })
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  return response.json()
}
export async function getBoardTrace(runId: string, boardId: string, signal: AbortSignal): Promise<BoardTrace> {
  const v = await request(`/api/runs/${encodeURIComponent(runId)}/boards/${encodeURIComponent(boardId)}/trace`, signal)
  if (!envelope(v, runId) || !object(v.board) || v.board.id !== boardId || !str(v.board.workOrderId) || !str(v.board.status) || !object(v.board.inspections) || typeof v.board.processed !== 'boolean' || !Array.isArray(v.entries) || !v.entries.every(e => object(e) && num(e.sequence) && str(e.eventId) && str(e.eventType) && (e.machineId === null || str(e.machineId)) && num(e.simTimeMs) && str(e.emittedAt) && str(e.ingestedAt) && (e.passed === undefined || typeof e.passed === 'boolean'))) throw new Error('Invalid board trace')
  return v as BoardTrace
}
export async function getWorkOrder(runId: string, workOrderId: string, signal: AbortSignal): Promise<WorkOrderDetail> {
  const v = await request(`/api/runs/${encodeURIComponent(runId)}/workorders/${encodeURIComponent(workOrderId)}`, signal)
  if (!envelope(v, runId) || !object(v.order) || v.order.id !== workOrderId || !object(v.progress) || !['planned', 'created', 'inProgress', 'completed', 'passed', 'failed', 'uninspected'].every(k => num((v.progress as Record<string, unknown>)[k])) || !str(v.reconciliation) || !(v.metadata === null || (object(v.metadata) && str(v.metadata.description) && v.metadata.synthetic === true))) throw new Error('Invalid work order')
  return v as WorkOrderDetail
}
