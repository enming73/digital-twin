import { describe, expect, it } from 'vitest'
import { castsShadow } from '../../src/scene/assetMaterials'

describe('loaded asset shadow casting', () => {
  it('keeps opaque machine parts in the shadow map', () => {
    expect(castsShadow(['Cabinet geometry', 'Satin aluminium'])).toBe(true)
    expect(castsShadow(['Mounter HMI ink'])).toBe(true)
  })

  it('keeps blended glazing out of the shadow map', () => {
    expect(castsShadow(['Mounter safety glass'])).toBe(false)
    expect(castsShadow(['Door upright', 'Mounter safety glass'])).toBe(false)
  })
})
