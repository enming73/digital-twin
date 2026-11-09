import type { MachineSnapshot } from '../simulation/engine'

export type SourceMode = 'local' | 'remote'
export type SceneStation = {
  state: string
  observedState: string | null
  flowState: string | null
  reason: string | null
  localMotion: MachineSnapshot | null
}
export type SceneBoard = { id: string; stationId: string; position: 'queued' | 'processing' | 'in_transit'; queueIndex: number; visual?: { fromX: number; toX: number; startedAt: number } }
export type SceneModel = { simTimeMs: number; stations: Record<string, SceneStation>; boards: SceneBoard[] }

export type RemoteMachine = { observedState: string; flowState: string; reason: string; alarms: Record<string, string> }
export type RemoteBoard = { id: string; workOrderId: string; machineId?: string; status: string; inspections: Record<string, boolean>; processed: boolean }
export type RemoteWorkOrder = { id: string; quantity: number; passed: number; failed: number; status: string }
export type Integrity = 'complete' | 'uncertain' | 'gap' | 'conflict'
export type RemoteProjection = { runId: string; epoch: string; revision: number; lastSequence: number; simTimeMs: number; integrity: Integrity; reason?: string; machines: Record<string, RemoteMachine>; boards: Record<string, RemoteBoard>; workOrders: Record<string, RemoteWorkOrder> }
