import { useMemo } from 'react'
import * as THREE from 'three'
import { signalColor } from './status'
import { glowOpacity, glowScale } from './statusGlow'

let cached: THREE.CanvasTexture | null = null

/** A radial falloff generated at runtime: no texture is downloaded. */
function glowTexture(): THREE.CanvasTexture {
  if (cached) return cached
  const size = 128
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const context = canvas.getContext('2d')!
  const gradient = context.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  gradient.addColorStop(0, 'rgba(255,255,255,1)')
  gradient.addColorStop(0.35, 'rgba(255,255,255,0.55)')
  gradient.addColorStop(1, 'rgba(255,255,255,0)')
  context.fillStyle = gradient
  context.fillRect(0, 0, size, size)
  cached = new THREE.CanvasTexture(canvas)
  cached.colorSpace = THREE.SRGBColorSpace
  return cached
}

/** Additive halo over the tower lens; depth-tested so a machine in front still occludes it. */
export default function StatusGlow({ position, state }: { position: [number, number, number]; state: string }) {
  const texture = useMemo(glowTexture, [])
  const scale = glowScale(state)
  return <sprite position={position} scale={[scale, scale, 1]}>
    <spriteMaterial
      map={texture}
      color={signalColor(state)}
      transparent
      opacity={glowOpacity(state)}
      blending={THREE.AdditiveBlending}
      depthWrite={false}
      toneMapped={false}
    />
  </sprite>
}
