import { signalIntensity } from './status'

/**
 * Additive halo around the signal lens. It is a sprite, not a post-processing pass:
 * a neon bloom would fight the restrained look the demo is aiming for.
 */
export function glowOpacity(state: string): number {
  return Math.min(0.8, 0.18 + signalIntensity(state) * 0.5)
}

export function glowScale(state: string): number {
  return 0.24 + signalIntensity(state) * 0.4
}
