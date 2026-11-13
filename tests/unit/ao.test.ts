import { describe, expect, it } from 'vitest'
import { aoEnabled, aoSupported } from '../../src/scene/ao'

describe('ambient occlusion switch', () => {
  it('runs by default and in remote mode', () => {
    expect(aoEnabled('')).toBe(true)
    expect(aoEnabled('?source=remote')).toBe(true)
    expect(aoEnabled('?quality=high')).toBe(true)
  })

  it('is skipped with the existing low-quality switch', () => {
    expect(aoEnabled('?quality=low')).toBe(false)
    expect(aoEnabled('?source=remote&quality=low')).toBe(false)
  })

  it('can be disabled without skipping the environment, for comparison', () => {
    expect(aoEnabled('?ao=off')).toBe(false)
    expect(aoEnabled('?ao=on&quality=low')).toBe(false)
  })
})

describe('ambient occlusion capability', () => {
  it('requires float render targets', () => {
    expect(aoSupported({ has: name => name === 'EXT_color_buffer_float' })).toBe(true)
    expect(aoSupported({ has: () => false })).toBe(false)
  })
})
