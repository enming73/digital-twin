/** Authored lens centre in station space; every GLB places `Signal_Lens` here. */
export const SIGNAL_LENS: [number, number, number] = [1.01, 2.17, -0.65]

/** The halo must stay wider than the 0.12 m lens, and narrow enough to read as a lamp, not a flare. */
export const MIN_GLOW_SCALE = 0.16
export const MAX_GLOW_SCALE = 0.5

/**
 * Additive halos wash out to white on this pale backdrop once a channel clips, so the glow is a
 * softly tinted disc instead. A sprite, not a post-processing pass: a neon bloom would fight the
 * restrained look the demo is aiming for, and would need the render targets this project rejected
 * with ambient occlusion.
 */
const GLOW: Record<string, { opacity: number; scale: number }> = {
  FAULT: { opacity: 0.6, scale: 0.46 },
  BLOCKED: { opacity: 0.52, scale: 0.42 },
  STARVED: { opacity: 0.52, scale: 0.42 },
  RUNNING: { opacity: 0.46, scale: 0.4 },
}

const IDLE = { opacity: 0.12, scale: 0.26 }

export function glowOpacity(state: string): number {
  return GLOW[state]?.opacity ?? IDLE.opacity
}

export function glowScale(state: string): number {
  return GLOW[state]?.scale ?? IDLE.scale
}
