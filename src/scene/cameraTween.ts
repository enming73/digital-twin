import type { CameraPreset } from '../data/stations'

export const EASE_DURATION_MS = 1200

/** Symmetric ease used for the overview → mechanism push-in. */
export function easeInOutCubic(t: number): number {
  const clamped = Math.min(1, Math.max(0, t))
  return clamped < 0.5 ? 4 * clamped ** 3 : 1 - (-2 * clamped + 2) ** 3 / 2
}

export function tweenProgress(elapsedMs: number, durationMs: number = EASE_DURATION_MS): { t: number; done: boolean } {
  const ratio = durationMs <= 0 ? 1 : Math.min(1, Math.max(0, elapsedMs / durationMs))
  return { t: ratio, done: elapsedMs >= durationMs }
}

export function lerpPreset(from: CameraPreset, to: CameraPreset, t: number): CameraPreset {
  const mix = (a: number, b: number) => a + (b - a) * t
  return {
    position: [mix(from.position[0], to.position[0]), mix(from.position[1], to.position[1]), mix(from.position[2], to.position[2])],
    target: [mix(from.target[0], to.target[0]), mix(from.target[1], to.target[1]), mix(from.target[2], to.target[2])],
  }
}
