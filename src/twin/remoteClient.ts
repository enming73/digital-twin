import { applyMessage, parseSnapshot } from './projection'
import type { RemoteProjection } from './types'

export type RemoteClientState = { connection: 'connecting' | 'connected' | 'disconnected'; projection: RemoteProjection | null; lastReceivedAt: number | null; stale: boolean; reason: string | null; snapshotVersion: number }

export function createRemoteClient(options: { runId: string; onState: (state: RemoteClientState) => void }): { start(): void; stop(): void } {
  let state: RemoteClientState = { connection: 'disconnected', projection: null, lastReceivedAt: null, stale: false, reason: null, snapshotVersion: 0 }
  let socket: WebSocket | null = null
  let retry: number | undefined
  let monitor: number | undefined
  let generation = 0
  let active = false
  let delay = 250
  const publish = (change: Partial<RemoteClientState>) => { state = { ...state, ...change }; options.onState(state) }
  const connect = () => {
    if (!active) return
    const token = ++generation
    let snapshotReceived = false
    publish({ connection: 'connecting', stale: true, reason: null })
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws/runs/${encodeURIComponent(options.runId)}`)
    socket = ws
    const disconnect = (reason: string) => {
      if (!active || token !== generation) return
      ++generation
      ws.close()
      socket = null
      publish({ connection: 'disconnected', stale: true, reason })
      retry = window.setTimeout(connect, delay)
      delay = Math.min(delay * 2, 10000)
    }
    ws.onopen = () => { if (active && token === generation) publish({ connection: 'connected', reason: null }) }
    ws.onerror = () => disconnect('WebSocket error')
    ws.onclose = () => disconnect('WebSocket closed')
    ws.onmessage = event => {
      if (!active || token !== generation) return
      try {
        const message: unknown = JSON.parse(String(event.data))
        if (!message || typeof message !== 'object' || Array.isArray(message)) return disconnect('invalid message')
        const value = message as Record<string, unknown>
        if (!snapshotReceived) {
          const projection = value.type === 'snapshot' ? parseSnapshot(value.snapshot, options.runId) : null
          if (!projection || value.epoch !== projection.epoch || value.revision !== projection.revision) return disconnect('snapshot required')
          snapshotReceived = true
          delay = 250
          publish({ projection, lastReceivedAt: Date.now(), stale: false, reason: null, snapshotVersion: state.snapshotVersion + 1 })
        } else {
          if (value.type === 'resync_required' || !state.projection) return disconnect('resync required')
          const result = applyMessage(state.projection, value)
          if (result.kind === 'resync') return disconnect(result.reason)
          publish({ projection: result.projection, lastReceivedAt: Date.now(), stale: false, reason: null })
        }
      } catch { disconnect('invalid message') }
    }
  }
  return {
    start() {
      if (active) return
      active = true
      monitor = window.setInterval(() => { if (state.lastReceivedAt !== null && Date.now() - state.lastReceivedAt >= 15000 && !state.stale) publish({ stale: true }) }, 1000)
      connect()
    },
    stop() {
      active = false; ++generation
      if (retry !== undefined) window.clearTimeout(retry)
      if (monitor !== undefined) window.clearInterval(monitor)
      socket?.close(); socket = null
    },
  }
}
