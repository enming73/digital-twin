import { describe, expect, it } from 'vitest'
import { EASE_DURATION_MS, easeInOutCubic, lerpPreset, tweenProgress } from '../../src/scene/cameraTween'

const from = { position: [10, 5, 20] as [number, number, number], target: [0, 1, 0] as [number, number, number] }
const to = { position: [2, 3, 4] as [number, number, number], target: [2, 1.5, 1] as [number, number, number] }

describe('camera tween', () => {
  it('eases from zero to one with a symmetric midpoint', () => {
    expect(easeInOutCubic(0)).toBe(0)
    expect(easeInOutCubic(0.5)).toBe(0.5)
    expect(easeInOutCubic(1)).toBe(1)
    expect(easeInOutCubic(-1)).toBe(0)
    expect(easeInOutCubic(2)).toBe(1)
  })

  it('never moves backwards', () => {
    let previous = -1
    for (let i = 0; i <= 10; i++) {
      const value = easeInOutCubic(i / 10)
      expect(value).toBeGreaterThanOrEqual(previous)
      previous = value
    }
  })

  it('reports completion at the duration and clamps negative elapsed time', () => {
    expect(tweenProgress(0)).toEqual({ t: 0, done: false })
    expect(tweenProgress(EASE_DURATION_MS / 2)).toEqual({ t: 0.5, done: false })
    expect(tweenProgress(EASE_DURATION_MS)).toEqual({ t: 1, done: true })
    expect(tweenProgress(EASE_DURATION_MS * 3)).toEqual({ t: 1, done: true })
    expect(tweenProgress(-50)).toEqual({ t: 0, done: false })
  })

  it('interpolates endpoints exactly and the midpoint componentwise', () => {
    expect(lerpPreset(from, to, 0)).toEqual(from)
    expect(lerpPreset(from, to, 1)).toEqual(to)
    expect(lerpPreset(from, to, 0.5)).toEqual({ position: [6, 4, 12], target: [1, 1.25, 0.5] })
  })
})
