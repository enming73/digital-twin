import { useMemo } from 'react'
import * as THREE from 'three'
import type { MachineSnapshot } from '../simulation/engine'
import { ZONE_X, activeZone } from './reflowZones'

const ACTIVE = '#e1a545'
const IDLE = '#39443f'
const INK = '#40574a'

// Shared for the page's lifetime: disposing per unmount would blank the other two labels.
let labels: THREE.CanvasTexture[] | null = null

function zoneLabels(): THREE.CanvasTexture[] {
  if (labels) return labels
  labels = ZONE_X.map((_, index) => {
    const canvas = document.createElement('canvas')
    canvas.width = 96
    canvas.height = 40
    const context = canvas.getContext('2d')!
    context.fillStyle = INK
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
    {ZONE_X.map((x, index) => <group key={x}>
      <mesh name={`Zone_Lamp_${index + 1}`} position={[x, 1.905, 0.985]}>
        <boxGeometry args={[0.06, 0.045, 0.012]} />
        <meshBasicMaterial color={active === index ? ACTIVE : IDLE} />
      </mesh>
      <mesh position={[x, 1.852, 0.985]}>
        <planeGeometry args={[0.15, 0.0625]} />
        <meshBasicMaterial map={textures[index]} transparent />
      </mesh>
    </group>)}
  </group>
}
