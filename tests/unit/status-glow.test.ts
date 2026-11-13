import { describe, expect, it } from 'vitest'
import { MAX_GLOW_SCALE, MIN_GLOW_SCALE, glowOpacity, glowScale } from '../../src/scene/statusGlow'

describe('status light glow', () => {
  it('follows the documented per-state table', () => {
    expect(glowOpacity('FAULT')).toBeCloseTo(0.6, 5)
    expect(glowOpacity('BLOCKED')).toBeCloseTo(0.52, 5)
    expect(glowOpacity('STARVED')).toBeCloseTo(0.52, 5)
    expect(glowOpacity('RUNNING')).toBeCloseTo(0.46, 5)
    expect(glowOpacity('IDLE')).toBeCloseTo(0.12, 5)
    expect(glowOpacity('UNKNOWN')).toBeCloseTo(0.12, 5)
    expect(glowOpacity('')).toBeCloseTo(0.12, 5)
    expect(glowScale('FAULT')).toBeCloseTo(0.46, 5)
    expect(glowScale('RUNNING')).toBeCloseTo(0.4, 5)
    expect(glowScale('IDLE')).toBeCloseTo(0.26, 5)
  })

  it('stays between the lens width and the no-neon budget', () => {
    for (const state of ['FAULT', 'BLOCKED', 'STARVED', 'RUNNING', 'IDLE', 'UNKNOWN', '']) {
      expect(glowScale(state)).toBeGreaterThanOrEqual(MIN_GLOW_SCALE)
      expect(glowScale(state)).toBeLessThanOrEqual(MAX_GLOW_SCALE)
      expect(glowOpacity(state)).toBeGreaterThanOrEqual(0)
      expect(glowOpacity(state)).toBeLessThanOrEqual(0.6)
    }
  })

  it('orders the states by urgency', () => {
    expect(glowOpacity('FAULT')).toBeGreaterThan(glowOpacity('STARVED'))
    expect(glowOpacity('STARVED')).toBeGreaterThan(glowOpacity('RUNNING'))
    expect(glowOpacity('RUNNING')).toBeGreaterThan(glowOpacity('IDLE'))
  })
})
