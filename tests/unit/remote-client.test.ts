import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRemoteClient, type RemoteClientState } from '../../src/twin/remoteClient'

class Socket {
  static sockets: Socket[] = []
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  constructor(public url: string) { Socket.sockets.push(this) }
  close() { this.onclose?.() }
  message(data: unknown) { this.onmessage?.({ data: JSON.stringify(data) }) }
}
const snap = { runId: 'demo-run', epoch: 'epoch', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} }

describe('remote client', () => {
  beforeEach(() => { vi.useFakeTimers(); Socket.sockets = []; vi.stubGlobal('WebSocket', Socket); vi.stubGlobal('window', { location: { protocol: 'http:', host: 'localhost:4173' }, setTimeout, clearTimeout, setInterval, clearInterval }) })
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })
  it('requires a snapshot first and replaces state after reconnect', () => {
    const states: RemoteClientState[] = []
    const client = createRemoteClient({ runId: 'demo-run', onState: state => states.push(state) })
    client.start()
    expect(Socket.sockets[0].url).toBe('ws://localhost:4173/ws/runs/demo-run')
    Socket.sockets[0].onopen?.()
    Socket.sockets[0].message({ type: 'snapshot', epoch: 'epoch', revision: 0, snapshot: snap })
    expect(states.at(-1)?.projection?.epoch).toBe('epoch')
    Socket.sockets[0].message({ type: 'resync_required' })
    vi.advanceTimersByTime(250)
    Socket.sockets[1].onopen?.()
    Socket.sockets[1].message({ type: 'snapshot', epoch: 'new', revision: 0, snapshot: { ...snap, epoch: 'new' } })
    expect(states.at(-1)?.projection?.epoch).toBe('new')
    client.stop()
  })
  it('marks an inactive connection stale without changing integrity and cancels retries on stop', () => {
    const states: RemoteClientState[] = []
    const client = createRemoteClient({ runId: 'demo-run', onState: state => states.push(state) })
    client.start(); Socket.sockets[0].onopen?.(); Socket.sockets[0].message({ type: 'snapshot', epoch: 'epoch', revision: 0, snapshot: snap })
    vi.advanceTimersByTime(15000)
    expect(states.at(-1)?.stale).toBe(true)
    expect(states.at(-1)?.projection?.integrity).toBe('uncertain')
    client.stop(); vi.advanceTimersByTime(30000)
    expect(Socket.sockets).toHaveLength(1)
  })
  it('discards callbacks from an old socket and rejects deltas before a snapshot', () => {
    const states: RemoteClientState[] = []
    const client = createRemoteClient({ runId: 'demo-run', onState: state => states.push(state) })
    client.start()
    const first = Socket.sockets[0]
    first.message({ type: 'delta', epoch: 'epoch', revision: 1 })
    expect(states.at(-1)?.connection).toBe('disconnected')
    vi.advanceTimersByTime(250)
    const second = Socket.sockets[1]
    second.message({ type: 'snapshot', epoch: 'new', revision: 0, snapshot: { ...snap, epoch: 'new' } })
    first.message({ type: 'snapshot', epoch: 'epoch', revision: 0, snapshot: snap })
    expect(states.at(-1)?.projection?.epoch).toBe('new')
    client.stop()
  })
})
