import { readFileSync, statSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { artifactNames, stationAnchors } from '../../src/data/anchors'

function glbJson(name: string) {
  const file = readFileSync(`public/models/${name}.glb`)
  expect(file.readUInt32LE(0)).toBe(0x46546c67)
  expect(file.readUInt32LE(4)).toBe(2)
  const jsonLength = file.readUInt32LE(12)
  return JSON.parse(file.subarray(20, 20 + jsonLength).toString()) as {
    nodes: Array<{ name?: string }>
    meshes: unknown[]
    materials?: Array<{ name?: string; alphaMode?: string; pbrMetallicRoughness?: { baseColorFactor?: number[] } }>
  }
}

describe('original Blender machine assets', () => {
  it('mounter exports inspection doors, operator hardware and transparent safety glazing', () => {
    const json = glbJson('mounter')
    const names = json.nodes.map(node => node.name)
    for (const name of ['Vision_Door_L', 'Vision_Door_R', 'Operator_Panel', 'HMI_Display', 'Emergency_Stop']) {
      expect(names.filter(node => node === name)).toHaveLength(1)
    }
    const glass = json.materials?.find(mat => mat.name === 'Mounter safety glass')
    expect(glass?.alphaMode).toBe('BLEND')
    const alpha = glass?.pbrMetallicRoughness?.baseColorFactor?.[3]
    expect(alpha).toBeGreaterThan(0)
    expect(alpha).toBeLessThan(1)
  })
  for (const name of artifactNames) {
    it(`${name} has a Blender source and a GLB carrying its anchor contract`, () => {
      const blend = readFileSync(`assets/blender/${name}.blend`)
      expect(blend.subarray(0, 7).toString()).toBe('BLENDER')

      const path = `public/models/${name}.glb`
      expect(statSync(path).size).toBeLessThan(2 * 1024 * 1024)

      const json = glbJson(name)
      const names = json.nodes.map(node => node.name).filter((nodeName): nodeName is string => Boolean(nodeName))
      for (const anchor of stationAnchors[name]) expect(names).toContain(anchor)
      expect(new Set(names).size).toBe(names.length)
      expect(json.meshes.length).toBeGreaterThan(5)
      expect(json.meshes.length).toBeLessThanOrEqual(40)
    })
  }
})
