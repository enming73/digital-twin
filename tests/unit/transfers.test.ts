import { describe, expect, it } from 'vitest'
import type { SceneBoard, SceneModel } from '../../src/twin/types'
import { boardTarget, boardX, boardZ, startTransfers, transferDuration, updateTransfers } from '../../src/scene/transfers'

const stationX: Record<string, number> = {
  loader: -9.45, printer: -6.3, spi: -3.15, mounter: 0, reflow: 3.15, aoi: 6.3, unloader: 9.45,
}

function board(id: string, stationId: string, position: SceneBoard['position'] = 'queued', queueIndex = 0): SceneBoard {
  return { id, stationId, position, queueIndex }
}

function model(simTimeMs: number, boards: SceneBoard[]): SceneModel {
  return {
    simTimeMs,
    stations: Object.fromEntries(Object.keys(stationX).map(id => [id, { state: 'IDLE', observedState: null, flowState: null, reason: null, localMotion: null }])),
    boards,
  }
}

describe('transfer duration', () => {
  it('scales with distance and clamps to a readable window', () => {
    expect(transferDuration(0, 0)).toBe(350)
    expect(transferDuration(0, 2)).toBe(350)
    expect(transferDuration(0, 6)).toBe(660)
    expect(transferDuration(0, 30)).toBe(900)
  })

  it('outlives two polls so a hop is never invisible at higher speeds', () => {
    expect(transferDuration(-9.45, -6.3, 0)).toBe(350)
    expect(transferDuration(-9.45, -6.3, 200)).toBe(400)
    expect(transferDuration(-9.45, -6.3, 500)).toBe(1000)
  })
})

describe('board target position', () => {
  it('places a queued board beside its station, offset by queue index', () => {
    expect(boardTarget(board('b1', 'printer', 'queued', 1), model(0, []))).toEqual({ x: -7, z: -0.18 })
  })

  it('follows the processing progress when the machine motion is known', () => {
    const scene = model(3500, [])
    scene.stations.printer.localMotion = {
      id: 'printer', state: 'RUNNING', boardId: 'b1', queue: [], processTimeMs: 5000, startedAtMs: 1000,
      faultAtMs: null, inputCapacity: 2, outputCapacity: 2,
    } as unknown as SceneModel['stations'][string]['localMotion']
    expect(boardTarget(board('b1', 'printer', 'processing'), scene)).toEqual({ x: -6.3, z: 0.12 })
  })

  it('does not place a board at the origin when its station is unknown', () => {
    expect(boardTarget(board('b1', 'unknown'), model(0, []))).toBeNull()
  })
})

describe('starting transfers', () => {
  it('starts one transfer per board that changed station, from where it was drawn', () => {
    const previous = model(1500, [board('b1', 'loader', 'processing')])
    const next = model(1600, [board('b1', 'printer')])
    expect(startTransfers(previous, next)).toEqual([
      { boardId: 'b1', fromX: -8.9, fromZ: 0.12, startMs: 1600, durationMs: 350 },
    ])
  })

  it('scales the hop to the observed poll gap', () => {
    const previous = model(1500, [board('b1', 'loader')])
    const next = model(2000, [board('b1', 'printer')])
    expect(startTransfers(previous, next)[0].durationMs).toBe(1000)
  })

  it('ignores boards that stayed put, boards that are new, and backwards time', () => {
    const previous = model(1500, [board('b1', 'loader')])
    expect(startTransfers(previous, model(1600, [board('b1', 'loader')]))).toEqual([])
    expect(startTransfers(previous, model(1600, [board('b2', 'printer')]))).toEqual([])
    expect(startTransfers(previous, model(1400, [board('b1', 'printer')]))).toEqual([])
    expect(startTransfers(null, model(1600, [board('b1', 'printer')]))).toEqual([])
  })

  it('still animates a transfer that happens between polls at the same simulated time', () => {
    const previous = model(1500, [board('b1', 'loader')])
    const next = model(1500, [board('b1', 'printer')])
    expect(startTransfers(previous, next)).toHaveLength(1)
  })
})

describe('active transfer list', () => {
  const transfer = { boardId: 'b1', fromX: -9.45, fromZ: 0.12, startMs: 1500, durationMs: 350 }

  it('keeps an existing transfer across a slow poll', () => {
    const previous = model(1600, [board('b1', 'printer')])
    const next = model(1700, [board('b1', 'printer')])
    expect(updateTransfers([transfer], previous, next)).toEqual([transfer])
  })

  it('drops a transfer once its duration has elapsed', () => {
    const previous = model(1800, [board('b1', 'printer')])
    const next = model(1900, [board('b1', 'printer')])
    expect(updateTransfers([transfer], previous, next)).toEqual([])
  })

  it('clears everything when time runs backwards', () => {
    const previous = model(5000, [board('b1', 'printer')])
    const next = model(1200, [board('b1', 'printer')])
    expect(updateTransfers([transfer], previous, next)).toEqual([])
  })

  it('forgets transfers for boards that disappeared', () => {
    expect(updateTransfers([transfer], model(1600, []), model(1700, []))).toEqual([])
  })

  it('never records transfers for remote boards, which interpolate server-side', () => {
    const previous = model(1500, [board('b1', 'loader')])
    const next = model(1600, [{ ...board('b1', 'printer'), visual: { fromX: -9.45, toX: -6.3, startedAt: 0 } }])
    expect(updateTransfers([], previous, next)).toEqual([])
  })
})

describe('interpolated position', () => {
  const transfer = { boardId: 'b1', fromX: -9.45, fromZ: 0.12, startMs: 1000, durationMs: 500 }

  it('eases from the old station to the target', () => {
    expect(boardX(transfer, -6.3, 1000)).toBeCloseTo(-9.45, 6)
    expect(boardX(transfer, -6.3, 1125)).toBeCloseTo(-9.253125, 6)
    expect(boardX(transfer, -6.3, 1250)).toBeCloseTo(-7.875, 6)
    expect(boardX(transfer, -6.3, 1500)).toBeCloseTo(-6.3, 6)
    expect(boardX(transfer, -6.3, 1800)).toBeCloseTo(-6.3, 6)
  })

  it('eases z as well, so a board does not snap between conveyor and queue', () => {
    expect(boardZ(transfer, -0.18, 1000)).toBeCloseTo(0.12, 6)
    expect(boardZ(transfer, -0.18, 1250)).toBeCloseTo(-0.03, 6)
    expect(boardZ(transfer, -0.18, 1500)).toBeCloseTo(-0.18, 6)
  })

  it('uses the target when no transfer is active', () => {
    expect(boardX(undefined, -6.3, 1000)).toBe(-6.3)
    expect(boardZ(undefined, -0.18, 1000)).toBe(-0.18)
  })

  it('keeps travelling when the destination queue index changes mid-hop', () => {
    const target = boardTarget(board('b1', 'printer', 'queued', 1), model(1200, []))
    expect(target).not.toBeNull()
    const x = boardX(transfer, target!.x, 1200)
    expect(x).toBeGreaterThan(-9.45)
    expect(x).toBeLessThan(-7)
  })
})
