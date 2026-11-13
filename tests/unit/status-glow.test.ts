import { describe, expect, it } from 'vitest'
import { glowOpacity, glowScale } from '../../src/scene/statusGlow'

describe('status light glow', () => {
  it('glows brightest for a fault and dimmest for an idle machine', () => {
    expect(glowOpacity('FAULT')).toBeGreaterThan(glowOpacity('RUNNING'))
    expect(glowOpacity('RUNNING')).toBeGreaterThan(glowOpacity('IDLE'))
    expect(glowOpacity('IDLE')).toBeGreaterThan(0)
    expect(glowOpacity('FAULT')).toBeLessThanOrEqual(0.8)
  })

  it('keeps the halo small enough to read as a lamp, not a flare', () => {
    expect(glowScale('FAULT')).toBeGreaterThan(glowScale('RUNNING'))
    expect(glowScale('FAULT')).toBeLessThanOrEqual(0.7)
    expect(glowScale('IDLE')).toBeGreaterThan(0)
  })
})
