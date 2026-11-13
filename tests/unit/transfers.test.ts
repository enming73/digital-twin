import { describe, expect, it } from 'vitest'
import type { SceneBoard, SceneModel } from '../../src/twin/types'
import { boardTarget, boardX, startTransfers, transferDuration, updateTransfers } from '../../src/scene/transfers'

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
})

describe('starting transfers', () => {
  it('starts one transfer per board that changed station', () => {
    const previous = model(1500, [board('b1', 'loader')])
    const next = model(2000, [board('b1', 'printer')])
    expect(startTransfers(previous, next)).toEqual([
      { boardId: 'b1', fromX: stationX.loader, startMs: 1500, durationMs: 350 },
    ])
  })

  it('ignores boards that stayed put, boards that are new, and paused time', () => {
    const previous = model(1500, [board('b1', 'loader')])
    expect(startTransfers(previous, model(2000, [board('b1', 'loader')]))).toEqual([])
    expect(startTransfers(previous, model(2000, [board('b2', 'printer')]))).toEqual([])
    expect(startTransfers(previous, model(1500, [board('b1', 'printer')]))).toEqual([])
    expect(startTransfers(null, model(2000, [board('b1', 'printer')]))).toEqual([])
  })
})

describe('active transfer list', () => {
  const transfer = { boardId: 'b1', fromX: -9.45, startMs: 1500, durationMs: 350 }

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
})

describe('interpolated x', () => {
  const transfer = { boardId: 'b1', fromX: -9.45, startMs: 1000, durationMs: 500 }

  it('eases from the old station to the target', () => {
    expect(boardX(transfer, -6.3, 1000)).toBeCloseTo(-9.45, 6)
    expect(boardX(transfer, -6.3, 1250)).toBeCloseTo(-7.875, 6)
    expect(boardX(transfer, -6.3, 1500)).toBeCloseTo(-6.3, 6)
    expect(boardX(transfer, -6.3, 1800)).toBeCloseTo(-6.3, 6)
  })

  it('uses the target when no transfer is active', () => {
    expect(boardX(undefined, -6.3, 1000)).toBe(-6.3)
  })

  it('lands on the target when the next poll arrives after the transfer ended', () => {
    expect(boardX(transfer, -6.3, 1800)).toBeCloseTo(-6.3, 6)
  })
})
