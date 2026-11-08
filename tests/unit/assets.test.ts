import { readFileSync, statSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('original Blender machine assets', () => {
  for (const name of ['printer', 'mounter']) {
    it(`${name} has a Blender source and a compact GLB with named components`, () => {
      const blend = readFileSync(`assets/blender/${name}.blend`)
      expect(blend.subarray(0, 7).toString()).toBe('BLENDER')
      const path = `public/models/${name}.glb`
      expect(statSync(path).size).toBeLessThan(2 * 1024 * 1024)
      const file = readFileSync(path)
      expect(file.readUInt32LE(0)).toBe(0x46546c67)
      expect(file.readUInt32LE(4)).toBe(2)
      const jsonLength = file.readUInt32LE(12)
      const json = JSON.parse(file.subarray(20, 20 + jsonLength).toString())
      const names = json.nodes.map((node: { name?: string }) => node.name)
      for (const part of ['Cabinet', 'Conveyor', 'PCB_Anchor', 'Gantry_X', 'Gantry_Y', 'Placement_Head', 'Feeders', 'Signal_Tower']) expect(names).toContain(part)
      expect(json.meshes.length).toBeGreaterThan(5)
    })
  }
})
