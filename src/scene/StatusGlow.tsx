import { useMemo } from 'react'
import * as THREE from 'three'
import { signalColor } from './status'
import { glowOpacity, glowScale } from './statusGlow'

let cached: THREE.CanvasTexture | null = null

/**
 * A radial falloff generated at runtime: no texture is downloaded.
 * Cached for the lifetime of the page on purpose — each sprite shares it, so disposing on unmount
 * would blank the other stations.
 */
function glowTexture(): THREE.CanvasTexture {
  if (cached) return cached
  const size = 128
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const context = canvas.getContext('2d')!
  const gradient = context.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  gradient.addColorStop(0, 'rgba(255,255,255,0.95)')
  gradient.addColorStop(0.4, 'rgba(255,255,255,0.45)')
  gradient.addColorStop(1, 'rgba(255,255,255,0)')
  context.fillStyle = gradient
  context.fillRect(0, 0, size, size)
  cached = new THREE.CanvasTexture(canvas)
  cached.colorSpace = THREE.SRGBColorSpace
  return cached
}

/**
 * Soft halo over the tower lens. Normal blending keeps the hue on the pale backdrop (additive turns
 * the fault halo white), and depth testing lets a machine in front occlude it.
 */
export default function StatusGlow({ position, state }: { position: [number, number, number]; state: string }) {
  const texture = useMemo(glowTexture, [])
  const scale = glowScale(state)
  return <sprite name="Signal_Glow" position={position} scale={[scale, scale, 1]} raycast={() => undefined}>
    <spriteMaterial
      map={texture}
      color={signalColor(state)}
      transparent
      opacity={glowOpacity(state)}
      depthWrite={false}
    />
  </sprite>
}
