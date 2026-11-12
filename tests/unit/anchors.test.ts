import * as THREE from 'three'
import { describe, expect, it } from 'vitest'
import { anchorOffsets, applyAnchorOffsets, captureAnchors } from '../../src/scene/anchors'

function mounterTree() {
  const root = new THREE.Group()
  const gantry = new THREE.Group(); gantry.name = 'Gantry_X'; gantry.position.set(0.1, 1.66, 1.0)
  const head = new THREE.Group(); head.name = 'Placement_Head'; head.position.set(0, 1.54, 1.0)
  const feeders = new THREE.Group(); feeders.name = 'Feeders'; feeders.position.set(0, 1.28, 1.02)
  root.add(gantry, head, feeders)
  return root
}

describe('anchor poses', () => {
  it('captures only anchors that exist in the loaded model', () => {
    const root = mounterTree()
    const rest = captureAnchors(root, ['Gantry_X', 'Placement_Head', 'Not_There'])
    expect([...rest.keys()]).toEqual(['Gantry_X', 'Placement_Head'])
    expect(rest.get('Gantry_X')?.x).toBe(0.1)
  })

  it('keeps mounter offsets inside the authored bounds for a whole cycle', () => {
    for (const cycle of [0, 0.25, 0.5, 0.75, 1]) {
      const pose = { cycle, sweep: Math.sin(cycle * Math.PI * 2), lift: (1 - Math.cos(cycle * Math.PI * 2)) / 2 }
      const offsets = anchorOffsets('mounter', pose)
      for (const axis of ['x', 'y', 'z'] as const) expect(Math.abs(offsets.Gantry_X[axis] ?? 0)).toBeLessThanOrEqual(0.45)
      expect(Math.abs(offsets.Placement_Head.y ?? 0)).toBeLessThanOrEqual(0.12)
      expect(Math.abs(offsets.Feeders.y ?? 0)).toBeLessThanOrEqual(0.01)
    }
  })

  it('keeps the reflow belt inside half a metre of travel', () => {
    for (const cycle of [0, 0.5, 1]) {
      const offsets = anchorOffsets('reflow', { cycle, sweep: 0, lift: 0 })
      expect(offsets.Belt.x).toBeGreaterThanOrEqual(0)
      expect(offsets.Belt.x).toBeLessThanOrEqual(0.5)
    }
  })

  it('returns no offsets for an unknown kind', () => {
    expect(anchorOffsets('mystery', { cycle: 0.5, sweep: 1, lift: 1 })).toEqual({})
  })

  it('restores rest positions exactly instead of accumulating', () => {
    const root = mounterTree()
    const rest = captureAnchors(root, ['Gantry_X', 'Placement_Head'])
    const offsets = anchorOffsets('mounter', { cycle: 0.5, sweep: 1, lift: 1 })
    applyAnchorOffsets(root, rest, offsets)
    expect(root.getObjectByName('Gantry_X')?.position.x).toBeCloseTo(0.1 + 0.45, 6)
    applyAnchorOffsets(root, rest, {})
    expect(root.getObjectByName('Gantry_X')?.position.x).toBeCloseTo(0.1, 6)
    expect(root.getObjectByName('Placement_Head')?.position.y).toBeCloseTo(1.54, 6)
  })
})
