import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchHistoricalSnapshot } from '../../src/twin/historicalClient'

const snapshot = { runId: 'r', epoch: 'historical', revision: 1, lastSequence: 1, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} }

describe('historical snapshot client', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('validates a matching simulated-time snapshot', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ mode: 'historical', synthetic: true, requestedSimTimeMs: 5, snapshot })))
    await expect(fetchHistoricalSnapshot('r', 5, new AbortController().signal)).resolves.toMatchObject({ runId: 'r', revision: 1 })
  })
  it('rejects wrong run, wrong timestamp, and HTTP errors', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ mode: 'historical', requestedSimTimeMs: 5, snapshot })))
    await expect(fetchHistoricalSnapshot('r', 5, new AbortController().signal)).rejects.toThrow()
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ mode: 'historical', requestedSimTimeMs: 6, snapshot })))
    await expect(fetchHistoricalSnapshot('r', 5, new AbortController().signal)).rejects.toThrow()
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ mode: 'historical', requestedSimTimeMs: 5, snapshot: { ...snapshot, runId: 'other' } })))
    await expect(fetchHistoricalSnapshot('r', 5, new AbortController().signal)).rejects.toThrow()
    vi.stubGlobal('fetch', async () => new Response('', { status: 404 }))
    await expect(fetchHistoricalSnapshot('r', 5, new AbortController().signal)).rejects.toThrow('404')
  })
  it('rejects invalid time and respects abort signals', async () => {
    await expect(fetchHistoricalSnapshot('r', -1, new AbortController().signal)).rejects.toThrow()
    vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
      if (options.signal?.aborted) throw new DOMException('Aborted', 'AbortError')
      return new Response('{}')
    })
    const controller = new AbortController(); controller.abort()
    await expect(fetchHistoricalSnapshot('r', 0, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
  })
})
