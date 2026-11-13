import { describe, expect, it } from 'vitest'
import { MACHINE_HALF_DEPTH, TALLEST_MACHINE, cableTray, floorGuides, trayPosts } from '../../src/scene/setDressing'

describe('pilot set dressing', () => {
  it('keeps the floor guides clear of the machine footprints', () => {
    for (const guide of floorGuides()) {
      expect(Math.abs(guide.position[2])).toBeGreaterThan(MACHINE_HALF_DEPTH)
      expect(guide.position[1]).toBeLessThan(0.05)
    }
  })

  it('runs the cable tray above and behind the machines', () => {
    for (const segment of cableTray()) {
      expect(segment.position[1]).toBeGreaterThan(TALLEST_MACHINE)
      expect(segment.position[2]).toBeLessThan(-MACHINE_HALF_DEPTH)
    }
  })

  it('stands the posts on the floor, outside the machine footprint', () => {
    for (const post of trayPosts()) {
      expect(post.position[1] - post.size[1] / 2).toBeCloseTo(0, 5)
      expect(post.position[2]).toBeLessThan(-MACHINE_HALF_DEPTH)
    }
  })
})
