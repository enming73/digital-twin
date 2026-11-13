/** `?quality=low` also skips the ambient-occlusion pass: its depth prepass is the expensive part. */
export function aoEnabled(search: string): boolean {
  return new URLSearchParams(search).get('quality') !== 'low'
}
