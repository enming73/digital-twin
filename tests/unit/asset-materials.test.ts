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
    expect(castsShadow(['printer safety glass'])).toBe(false)
    expect(castsShadow(['spi safety glass', 'aoi safety glass'])).toBe(false)
    expect(castsShadow(['reflow safety glass'])).toBe(false)
  })

  it('treats every station glazing name as glazing, including Blender suffix deduplication', () => {
    expect(castsShadow(['aoi safety glass.001'])).toBe(false)
    expect(castsShadow(['Printer safety glass'])).toBe(false)
    expect(castsShadow(['Mounter safety glazing'])).toBe(true)
  })
})
