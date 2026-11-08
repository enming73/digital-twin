export type StationConfig = { id: string; processTimeMs: number; inputCapacity: number }
export type Scenario = { seed: number; quantity: number; defectRate: number; stations: StationConfig[] }
export type MachineState = 'IDLE' | 'RUNNING' | 'BLOCKED' | 'STARVED' | 'FAULT'
export type MachineSnapshot = StationConfig & { queue: string[]; boardId: string | null; state: MachineState; startedAtMs: number | null; finishAtMs: number | null; faultAtMs: number | null; remainingMs: number | null }
export type Board = { id: string; workOrderId: string; stationId: string | null; status: 'QUEUED' | 'PROCESSING' | 'BLOCKED' | 'COMPLETED'; createdAtMs: number; completedAtMs: number | null; inspections: { stationId: string; passed: boolean }[] }
export type ProductionEvent = Readonly<{ id: string; schemaVersion: 1; simTimeMs: number; sequence: number; type: string; payload: Readonly<Record<string, string | number | boolean>> }>
export type Snapshot = { simTimeMs: number; running: boolean; created: number; completed: number; quantity: number; stations: MachineSnapshot[]; boards: Board[] }

export class Simulation {
  private config: Scenario
  private time = 0
  private running = false
  private created = 0
  private completed = 0
  private randomState: number
  private machines: MachineSnapshot[]
  private boards: Board[] = []
  private log: ProductionEvent[] = []

  constructor(config: Scenario) {
    if (!Number.isInteger(config.seed) || !Number.isInteger(config.quantity) || config.quantity < 1 || !Number.isFinite(config.defectRate) || config.defectRate < 0 || config.defectRate > 1 || !config.stations.length || new Set(config.stations.map(s => s.id)).size !== config.stations.length || config.stations.some(s => !s.id || !Number.isFinite(s.processTimeMs) || s.processTimeMs <= 0 || !Number.isInteger(s.inputCapacity) || s.inputCapacity < 1)) throw new Error('Invalid scenario configuration')
    this.config = structuredClone(config)
    this.randomState = config.seed >>> 0
    this.machines = this.newMachines()
  }
  private newMachines(): MachineSnapshot[] { return this.config.stations.map(s => ({ ...s, queue: [], boardId: null, state: 'IDLE', startedAtMs: null, finishAtMs: null, faultAtMs: null, remainingMs: null })) }
  private emit(type: string, payload: Record<string, string | number | boolean> = {}) {
    const sequence = this.log.length + 1
    this.log.push(Object.freeze({ id: `event-${sequence}`, schemaVersion: 1, simTimeMs: this.time, sequence, type, payload: Object.freeze({ ...payload }) }))
  }
  private state(machine: MachineSnapshot, state: MachineState) {
    if (machine.state !== state) { machine.state = state; this.emit('MACHINE_STATE_CHANGED', { stationId: machine.id, state }) }
  }
  private random(): number { this.randomState = (Math.imul(this.randomState, 1664525) + 1013904223) >>> 0; return this.randomState / 4294967296 }
  private board(id: string): Board { return this.boards[Number(id.slice(6)) - 1] }
  private settle() {
    // Downstream-first transfers free capacity before upstream decisions at the same time.
    for (let i = this.machines.length - 1; i >= 0; i--) {
      const machine = this.machines[i]
      if (machine.state === 'FAULT') continue
      if (machine.boardId && machine.finishAtMs === null) {
        const id = machine.boardId
        const board = this.board(id)
        const next = this.machines[i + 1]
        if (!next) {
          board.stationId = null; board.status = 'COMPLETED'; board.completedAtMs = this.time; this.completed++
          this.emit('BOARD_COMPLETED', { boardId: id, stationId: machine.id }); machine.boardId = null; machine.startedAtMs = null
          if (this.completed === this.config.quantity) this.emit('WORK_ORDER_COMPLETED', { quantity: this.completed })
        } else if (next.queue.length < next.inputCapacity) {
          next.queue.push(id); board.stationId = next.id; board.status = 'QUEUED'
          this.emit('BOARD_TRANSFERRED', { boardId: id, from: machine.id, to: next.id }); machine.boardId = null; machine.startedAtMs = null
        } else { board.status = 'BLOCKED'; this.state(machine, 'BLOCKED') }
      }
      if (!machine.boardId && machine.queue.length) {
        const id = machine.queue.shift()!
        machine.boardId = id; machine.startedAtMs = this.time; machine.finishAtMs = this.time + machine.processTimeMs
        this.board(id).status = 'PROCESSING'; this.state(machine, 'RUNNING'); this.emit('PROCESS_STARTED', { stationId: machine.id, boardId: id })
      } else if (!machine.boardId) this.state(machine, this.created === this.config.quantity && this.completed === this.config.quantity ? 'IDLE' : 'STARVED')
    }
    const first = this.machines[0]
    while (this.created < this.config.quantity && first.queue.length < first.inputCapacity) {
      const id = `board-${++this.created}`
      this.boards.push({ id, workOrderId: 'work-order-1', stationId: first.id, status: 'QUEUED', createdAtMs: this.time, completedAtMs: null, inspections: [] })
      first.queue.push(id); this.emit('BOARD_CREATED', { boardId: id, stationId: first.id })
    }
    if (!first.boardId && first.queue.length) {
      const id = first.queue.shift()!; first.boardId = id; first.startedAtMs = this.time; first.finishAtMs = this.time + first.processTimeMs
      this.board(id).status = 'PROCESSING'; this.state(first, 'RUNNING'); this.emit('PROCESS_STARTED', { stationId: first.id, boardId: id })
      // Refill the queue after taking the first board.
      while (this.created < this.config.quantity && first.queue.length < first.inputCapacity) {
        const nextId = `board-${++this.created}`
        this.boards.push({ id: nextId, workOrderId: 'work-order-1', stationId: first.id, status: 'QUEUED', createdAtMs: this.time, completedAtMs: null, inspections: [] })
        first.queue.push(nextId); this.emit('BOARD_CREATED', { boardId: nextId, stationId: first.id })
      }
    }
    if (this.machines.some(machine => machine.state !== 'FAULT' && !machine.boardId && machine.queue.length > 0)) this.settle()
  }
  start() { if (!this.running && this.completed < this.config.quantity) { this.running = true; this.emit(this.time === 0 && this.created === 0 ? 'RUN_STARTED' : 'RUN_RESUMED'); this.settle() } }
  pause() { if (this.running) { this.running = false; this.emit('RUN_PAUSED') } }
  injectFault(id: string) {
    const machine = this.machines.find(m => m.id === id)
    if (!machine) throw new Error('Unknown station')
    if (machine.state === 'FAULT') return
    machine.faultAtMs = this.time; machine.remainingMs = machine.finishAtMs === null ? null : Math.max(0, machine.finishAtMs - this.time); machine.finishAtMs = null
    this.emit('FAULT_INJECTED', { stationId: id, remainingMs: machine.remainingMs ?? -1 }); this.state(machine, 'FAULT')
  }
  clearFault(id: string) {
    const machine = this.machines.find(m => m.id === id)
    if (!machine) throw new Error('Unknown station')
    if (machine.state !== 'FAULT') return
    if (machine.remainingMs !== null) {
      machine.finishAtMs = this.time + machine.remainingMs
      if (machine.startedAtMs !== null && machine.faultAtMs !== null) machine.startedAtMs += this.time - machine.faultAtMs
    }
    machine.faultAtMs = null; machine.remainingMs = null
    this.emit('FAULT_CLEARED', { stationId: id, startedAtMs: machine.startedAtMs ?? -1, finishAtMs: machine.finishAtMs ?? -1 })
    this.state(machine, machine.finishAtMs !== null ? 'RUNNING' : machine.boardId ? 'BLOCKED' : 'STARVED')
    this.settle()
  }
  reset() { this.time = 0; this.running = false; this.created = 0; this.completed = 0; this.randomState = this.config.seed >>> 0; this.machines = this.newMachines(); this.boards = []; this.log = [] }
  advanceWall(wallMs: number, speed: number) { if (!Number.isFinite(speed) || speed <= 0) throw new Error('Invalid speed'); this.advance(wallMs * speed) }
  advance(ms: number) {
    if (!Number.isFinite(ms) || ms < 0) throw new Error('Invalid time advance')
    if (!this.running) return
    const target = this.time + ms
    while (true) {
      const next = Math.min(...this.machines.map(m => m.finishAtMs ?? Infinity))
      if (next > target || !Number.isFinite(next)) break
      this.time = next
      for (const machine of this.machines) if (machine.finishAtMs === next && machine.boardId) {
        const board = this.board(machine.boardId); machine.finishAtMs = null
        this.emit('PROCESS_COMPLETED', { stationId: machine.id, boardId: board.id })
        if (machine.id === 'spi' || machine.id === 'aoi') { const passed = this.random() >= this.config.defectRate; board.inspections.push({ stationId: machine.id, passed }); this.emit('INSPECTION_RECORDED', { stationId: machine.id, boardId: board.id, passed }) }
      }
      this.settle()
      if (this.completed === this.config.quantity) { this.running = false; return }
    }
    this.time = target
  }
  snapshot(): Snapshot { return structuredClone({ simTimeMs: this.time, running: this.running, created: this.created, completed: this.completed, quantity: this.config.quantity, stations: this.machines, boards: this.boards }) }
  events(): ProductionEvent[] { return this.log.slice() }
}
