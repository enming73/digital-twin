import { describe, expect, it } from 'vitest'
import { aoEnabled } from '../../src/scene/ao'

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
})
