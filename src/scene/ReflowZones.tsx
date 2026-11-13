import { useMemo } from 'react'
import * as THREE from 'three'
import type { MachineSnapshot } from '../simulation/engine'
import { activeZone } from './reflowZones'

const ZONES = [-0.9, -0.45, 0]
const ACTIVE = '#e1a545'
const IDLE = '#39443f'

let labels: THREE.CanvasTexture[] | null = null

function zoneLabels(): THREE.CanvasTexture[] {
  if (labels) return labels
  labels = ZONES.map((_, index) => {
    const canvas = document.createElement('canvas')
    canvas.width = 96
    canvas.height = 40
    const context = canvas.getContext('2d')!
    context.fillStyle = '#eef1eb'
    context.font = 'bold 28px monospace'
    context.textAlign = 'center'
    context.textBaseline = 'middle'
    context.fillText(`Z${index + 1}`, 48, 22)
    const texture = new THREE.CanvasTexture(canvas)
    texture.colorSpace = THREE.SRGBColorSpace
    return texture
  })
  return labels
}

/**
 * Three conceptual oven zones: the lamp for the zone holding the board is lit.
 * This is an illustration of process staging, not a thermal profile or a measured recipe.
 */
export default function ReflowZones({ machine, time }: { machine: MachineSnapshot | null; time: number }) {
  const textures = useMemo(zoneLabels, [])
  const active = activeZone(machine, time)
  return <group name="Reflow_Zones">
    {ZONES.map((x, index) => <group key={x}>
      <mesh name={`Zone_Lamp_${index + 1}`} position={[x, 1.905, 0.99]}>
        <boxGeometry args={[0.06, 0.045, 0.012]} />
        <meshBasicMaterial color={active === index ? ACTIVE : IDLE} />
      </mesh>
      <mesh position={[x, 1.855, 0.992]}>
        <planeGeometry args={[0.15, 0.055]} />
        <meshBasicMaterial map={textures[index]} transparent toneMapped={false} />
      </mesh>
    </group>)}
  </group>
}
