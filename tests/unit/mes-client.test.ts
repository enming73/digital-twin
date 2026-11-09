import { afterEach, describe, expect, it, vi } from 'vitest'
import { getBoardTrace } from '../../src/twin/mesClient'

describe('remote MES client', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('validates and returns a board trace for an encoded identifier', async () => {
    const body = { runId: 'r', epoch: 'e', revision: 1, integrity: 'complete', synthetic: true, board: { id: 'p x', workOrderId: 'wo', status: 'at_station', inspections: {}, processed: false }, route: ['spi'], entries: [] }
    vi.stubGlobal('fetch', async (url: string) => new Response(JSON.stringify(url.includes('p%20x') ? body : {}), { status: 200 }))
    await expect(getBoardTrace('r', 'p x', new AbortController().signal)).resolves.toMatchObject(body)
  })
  it('rejects malformed data and HTTP errors', async () => {
    vi.stubGlobal('fetch', async () => new Response('{}', { status: 200 }))
    await expect(getBoardTrace('r', 'p', new AbortController().signal)).rejects.toThrow()
    vi.stubGlobal('fetch', async () => new Response('', { status: 404 }))
    await expect(getBoardTrace('r', 'p', new AbortController().signal)).rejects.toThrow('404')
  })
  it('rejects a valid-looking trace for a different board', async () => {
    const body = { runId: 'r', epoch: 'e', revision: 1, integrity: 'complete', synthetic: true, board: { id: 'other', workOrderId: 'wo', status: 'at_station', inspections: {}, processed: false }, route: ['spi'], entries: [] }
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify(body)))
    await expect(getBoardTrace('r', 'p', new AbortController().signal)).rejects.toThrow()
  })
  it('propagates network errors and cancellation', async () => {
    vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
      if (options.signal?.aborted) throw new DOMException('Aborted', 'AbortError')
      throw new TypeError('Network unavailable')
    })
    await expect(getBoardTrace('r', 'p', new AbortController().signal)).rejects.toThrow('Network unavailable')
    const controller = new AbortController()
    controller.abort()
    await expect(getBoardTrace('r', 'p', controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
  })
})
