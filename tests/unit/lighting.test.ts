import { describe, expect, it } from 'vitest'
import { iblEnabled } from '../../src/scene/lighting'

describe('quality switch', () => {
  it('keeps the environment for the default and high quality', () => {
    expect(iblEnabled('')).toBe(true)
    expect(iblEnabled('?quality=high')).toBe(true)
    expect(iblEnabled('?source=remote')).toBe(true)
  })

  it('drops the environment when the demo machine cannot afford it', () => {
    expect(iblEnabled('?quality=low')).toBe(false)
    expect(iblEnabled('?quality=low&source=remote')).toBe(false)
  })
})
