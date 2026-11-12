import { describe, expect, it } from 'vitest'
import { signalColor, signalIntensity } from '../../src/scene/status'

describe('signal tower state', () => {
  it('maps station state to the tower colour', () => {
    expect(signalColor('FAULT')).toBe('#e14e4e')
    expect(signalColor('BLOCKED')).toBe('#e1a545')
    expect(signalColor('STARVED')).toBe('#e1a545')
    expect(signalColor('RUNNING')).toBe('#53b879')
    expect(signalColor('UNKNOWN')).toBe('#87928c')
    expect(signalColor('IDLE')).toBe('#87928c')
    expect(signalColor('')).toBe('#87928c')
  })

  it('keeps emissive intensity inside zero and one for every state', () => {
    for (const state of ['FAULT', 'BLOCKED', 'STARVED', 'RUNNING', 'IDLE', 'UNKNOWN', '']) {
      const intensity = signalIntensity(state)
      expect(intensity).toBeGreaterThanOrEqual(0)
      expect(intensity).toBeLessThanOrEqual(1)
    }
    expect(signalIntensity('FAULT')).toBeGreaterThan(signalIntensity('IDLE'))
  })
})
