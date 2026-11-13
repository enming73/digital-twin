import { readFileSync, statSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { artifactNames, stationAnchors } from '../../src/data/anchors'
import { anchorOffsets } from '../../src/scene/anchors'
import { SIGNAL_LENS } from '../../src/scene/statusGlow'

type GlbNode = { name?: string; mesh?: number; children?: number[]; translation?: number[] }
type GlbMaterial = {
  name?: string
  alphaMode?: string
  pbrMetallicRoughness?: { baseColorFactor?: number[]; metallicFactor?: number; roughnessFactor?: number }
}
type GlbJson = {
  nodes: GlbNode[]
  meshes: Array<{ primitives: Array<{ material?: number; attributes: { POSITION: number } }> }>
  materials?: GlbMaterial[]
  accessors: Array<{ min?: number[]; max?: number[] }>
}

/** Vertical extent of a node's meshes, seals included: must equal the specified leaf height. */
function nodeHeight(json: GlbJson, nodeName: string): number {
  const root = json.nodes.findIndex(node => node.name === nodeName)
  let low = Infinity
  let high = -Infinity
  const walk = (index: number) => {
    const node = json.nodes[index]
    if (node.mesh !== undefined) {
      for (const primitive of json.meshes[node.mesh].primitives) {
        const accessor = json.accessors[primitive.attributes.POSITION]
        low = Math.min(low, accessor.min?.[1] ?? low)
        high = Math.max(high, accessor.max?.[1] ?? high)
      }
    }
    for (const child of node.children ?? []) walk(child)
  }
  walk(root)
  return high - low
}

function glbJson(name: string): GlbJson {
  const file = readFileSync(`public/models/${name}.glb`)
  expect(file.readUInt32LE(0)).toBe(0x46546c67)
  expect(file.readUInt32LE(4)).toBe(2)
  const jsonLength = file.readUInt32LE(12)
  return JSON.parse(file.subarray(20, 20 + jsonLength).toString()) as GlbJson
}

function nodeNames(json: GlbJson) {
  return json.nodes.map(node => node.name)
}

/** Material indices used by a node and every descendant mesh. */
function usedMaterials(json: GlbJson, nodeName: string): number[] {
  const root = json.nodes.findIndex(node => node.name === nodeName)
  const used = new Set<number>()
  const walk = (index: number) => {
    const node = json.nodes[index]
    if (node.mesh !== undefined) {
      for (const primitive of json.meshes[node.mesh].primitives) {
        if (primitive.material !== undefined) used.add(primitive.material)
      }
    }
    for (const child of node.children ?? []) walk(child)
  }
  walk(root)
  return [...used]
}

function glazingIndex(json: GlbJson, materialName: string): number {
  const index = json.materials?.findIndex(material => material.name === materialName) ?? -1
  expect(index, `${materialName} is exported`).toBeGreaterThanOrEqual(0)
  const glass = json.materials?.[index]
  expect(glass?.alphaMode).toBe('BLEND')
  const alpha = glass?.pbrMetallicRoughness?.baseColorFactor?.[3]
  expect(alpha).toBeGreaterThan(0)
  expect(alpha).toBeLessThan(1)
  return index
}

describe('original Blender machine assets', () => {
  it('mounter exports inspection doors, operator hardware and transparent safety glazing', () => {
    const json = glbJson('mounter')
    for (const name of ['Vision_Door_L', 'Vision_Door_R', 'Operator_Panel', 'HMI_Display', 'Emergency_Stop']) {
      expect(nodeNames(json).filter(nodeName => nodeName === name)).toHaveLength(1)
    }
    const glass = glazingIndex(json, 'Mounter safety glass')
    for (const door of ['Vision_Door_L', 'Vision_Door_R']) expect(usedMaterials(json, door)).toContain(glass)
  })

  it('printer exports two framed observation doors with the process in view', () => {
    const json = glbJson('printer')
    for (const name of ['Vision_Door_L', 'Vision_Door_R', 'Operator_Panel', 'HMI_Display', 'Emergency_Stop']) {
      expect(nodeNames(json).filter(nodeName => nodeName === name)).toHaveLength(1)
    }
    const glass = glazingIndex(json, 'printer safety glass')
    for (const door of ['Vision_Door_L', 'Vision_Door_R']) expect(usedMaterials(json, door)).toContain(glass)
  })

  for (const kind of artifactNames) {
    it(`${kind} powder coat is a darker, near-dielectric paint rather than a bright metal`, () => {
      const paint = glbJson(kind).materials?.find(material => material.name === 'Powder coated warm grey')
      const [red, green, blue] = paint?.pbrMetallicRoughness?.baseColorFactor ?? [1, 1, 1]
      const luminance = 0.2126 * red + 0.7152 * green + 0.0722 * blue
      expect(luminance).toBeLessThan(0.5)
      expect(paint?.pbrMetallicRoughness?.metallicFactor).toBeLessThanOrEqual(0.1)
      const roughness = paint?.pbrMetallicRoughness?.roughnessFactor ?? 0
      expect(roughness).toBeGreaterThan(0.5)
      expect(roughness).toBeLessThan(0.7)
    })
  }

  for (const [kind, height] of [['mounter', 0.66], ['printer', 0.62], ['spi', 0.62], ['aoi', 0.62]] as const) {
    it(`${kind} builds each door leaf to its specified ${height} m height`, () => {
      const json = glbJson(kind)
      for (const door of nodeNames(json).filter((nodeName): nodeName is string => /^Vision_Door(_[LR])?$/.test(nodeName ?? ''))) {
        expect(Math.abs(nodeHeight(json, door) - height)).toBeLessThan(0.001)
      }
    })
  }

  it('reflow exports insulated maintenance covers with small inspection windows', () => {
    const json = glbJson('reflow')
    for (const name of ['Maintenance_Cover_A', 'Maintenance_Cover_B', 'Operator_Panel', 'HMI_Display', 'Emergency_Stop']) {
      expect(nodeNames(json).filter(nodeName => nodeName === name)).toHaveLength(1)
    }
    const glass = glazingIndex(json, 'reflow safety glass')
    for (const cover of ['Maintenance_Cover_A', 'Maintenance_Cover_B']) {
      expect(usedMaterials(json, cover)).toContain(glass)
      expect(Math.abs(nodeHeight(json, cover) - 0.58)).toBeLessThan(0.001)
    }
  })

  for (const kind of ['loader', 'unloader'] as const) {
    it(`${kind} exports a magazine guard door and its own labelled panel`, () => {
      const json = glbJson(kind)
      for (const name of ['Magazine_Door', 'Operator_Panel', 'HMI_Display', 'Emergency_Stop']) {
        expect(nodeNames(json).filter(nodeName => nodeName === name)).toHaveLength(1)
      }
      const glass = glazingIndex(json, `${kind} safety glass`)
      expect(usedMaterials(json, 'Magazine_Door')).toContain(glass)
      expect(Math.abs(nodeHeight(json, 'Magazine_Door') - 0.52)).toBeLessThan(0.001)
    })
  }

  for (const kind of ['spi', 'aoi']) {
    it(`${kind} exports a framed inspection door and its own labelled panel`, () => {
      const json = glbJson(kind)
      for (const name of ['Vision_Door', 'Operator_Panel', 'HMI_Display', 'Emergency_Stop']) {
        expect(nodeNames(json).filter(nodeName => nodeName === name)).toHaveLength(1)
      }
      const glass = glazingIndex(json, `${kind} safety glass`)
      expect(usedMaterials(json, 'Vision_Door')).toContain(glass)
    })
  }

  const enclosure = { x: 1.17, yMax: 1.82, zMin: -.83, zMax: .95 }
  const floors: Record<string, number> = { loader: .7, unloader: .7 }

  for (const kind of ['mounter', 'printer', 'spi', 'aoi', 'reflow', 'loader', 'unloader']) {
    it(`${kind} keeps every moving anchor inside the enclosure at both travel extremes`, () => {
      const json = glbJson(kind)
      const floor = floors[kind] ?? 1.09
      const boundsOf = (nodeName: string) => {
        const root = json.nodes.findIndex(node => node.name === nodeName)
        const low = [Infinity, Infinity, Infinity]
        const high = [-Infinity, -Infinity, -Infinity]
        const walk = (index: number) => {
          const node = json.nodes[index]
          const offset = node.translation ?? [0, 0, 0]
          if (node.mesh !== undefined) {
            for (const primitive of json.meshes[node.mesh].primitives) {
              const accessor = json.accessors[primitive.attributes.POSITION]
              for (let axis = 0; axis < 3; axis++) {
                low[axis] = Math.min(low[axis], (accessor.min?.[axis] ?? 0) + offset[axis])
                high[axis] = Math.max(high[axis], (accessor.max?.[axis] ?? 0) + offset[axis])
              }
            }
          }
          for (const child of node.children ?? []) walk(child)
        }
        walk(root)
        return { low, high }
      }
      for (const pose of [{ cycle: 0, sweep: -1, lift: 0 }, { cycle: 1, sweep: 1, lift: 1 }]) {
        for (const [anchorName, offset] of Object.entries(anchorOffsets(kind, pose))) {
          const { low, high } = boundsOf(anchorName)
          const dx = offset.x ?? 0
          const dy = offset.y ?? 0
          const dz = offset.z ?? 0
          expect(low[0] + dx, `${kind} ${anchorName} crosses a side wall`).toBeGreaterThan(-enclosure.x)
          expect(high[0] + dx, `${kind} ${anchorName} crosses a side wall`).toBeLessThan(enclosure.x)
          expect(low[1] + dy, `${kind} ${anchorName} crosses the conveyor`).toBeGreaterThan(floor)
          expect(high[1] + dy, `${kind} ${anchorName} crosses the roof`).toBeLessThan(enclosure.yMax)
          expect(low[2] + dz, `${kind} ${anchorName} crosses the back`).toBeGreaterThan(enclosure.zMin)
          expect(high[2] + dz, `${kind} ${anchorName} crosses the door`).toBeLessThan(enclosure.zMax)
        }
      }
    })
  }

  for (const name of artifactNames) {
    it(`${name} places its signal lens where the status glow is drawn`, () => {
      const json = glbJson(name)
      const lens = json.nodes.find(node => node.name === 'Signal_Lens')
      const mesh = lens?.children?.map(index => json.nodes[index]).find(node => node.mesh !== undefined)
      const translation = mesh?.translation ?? []
      for (let axis = 0; axis < 3; axis++) expect(translation[axis]).toBeCloseTo(SIGNAL_LENS[axis], 5)
    })
  }

  for (const name of artifactNames) {
    it(`${name} has a Blender source and a GLB carrying its anchor contract`, () => {
      const blend = readFileSync(`assets/blender/${name}.blend`)
      expect(blend.subarray(0, 7).toString()).toBe('BLENDER')

      const path = `public/models/${name}.glb`
      expect(statSync(path).size).toBeLessThan(2 * 1024 * 1024)

      const json = glbJson(name)
      const names = nodeNames(json).filter((nodeName): nodeName is string => Boolean(nodeName))
      for (const anchor of stationAnchors[name]) expect(names).toContain(anchor)
      expect(new Set(names).size).toBe(names.length)
      expect(json.meshes.length).toBeGreaterThan(5)
      expect(json.meshes.length).toBeLessThanOrEqual(40)
    })
  }
})
