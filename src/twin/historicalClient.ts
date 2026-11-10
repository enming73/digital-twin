import { parseSnapshot } from './projection'
import type { RemoteProjection } from './types'

export async function fetchHistoricalSnapshot(runId: string, simTimeMs: number, signal: AbortSignal): Promise<RemoteProjection> {
  if (!runId || !Number.isSafeInteger(simTimeMs) || simTimeMs < 0) throw new Error('Invalid historical query')
  const response = await fetch(`/api/runs/${encodeURIComponent(runId)}/replay?simTimeMs=${simTimeMs}`, { signal })
  if (!response.ok) throw new Error(`Historical query failed (${response.status})`)
  const value: unknown = await response.json()
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid historical response')
  const body = value as Record<string, unknown>
  if (body.mode !== 'historical' || body.synthetic !== true || body.requestedSimTimeMs !== simTimeMs) throw new Error('Invalid historical response')
  const snapshot = parseSnapshot(body.snapshot, runId)
  if (!snapshot || snapshot.simTimeMs > simTimeMs) throw new Error('Invalid historical snapshot')
  return snapshot
}
