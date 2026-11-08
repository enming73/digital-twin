import { describe, expect, it } from 'vitest'
import { stations, boardPositions, overviewCamera } from '../../src/data/stations'

describe('factory layout', () => {
  it('defines seven uniquely identified stations in production order', () => {
    expect(stations.map(s => s.id)).toEqual(['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader'])
    expect(new Set(stations.map(s => s.id)).size).toBe(7)
    expect(stations.every(s => s.bounds.every(n => n > 0) && s.camera.position.length === 3)).toBe(true)
  })
  it('defines twenty static boards and an overview preset', () => {
    expect(boardPositions).toHaveLength(20)
    expect(overviewCamera.position).toHaveLength(3)
  })
})
