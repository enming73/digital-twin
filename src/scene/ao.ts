/**
 * `?quality=low` skips the ambient-occlusion pass along with the environment bake.
 * `?ao=off` skips only the pass, which is how its cost and its look are compared.
 */
export function aoEnabled(search: string): boolean {
  const params = new URLSearchParams(search)
  if (params.get('quality') === 'low') return false
  return params.get('ao') !== 'off'
}

/** N8AO renders into half-float colour and full-float depth targets; without them the stage goes blank. */
export function aoSupported(extensions: { has(name: string): boolean }): boolean {
  return extensions.has('EXT_color_buffer_float')
}
