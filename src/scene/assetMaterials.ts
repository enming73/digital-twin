/** Materials that are blended, not solid: writing them into the shadow map would occlude the cavity. */
const nonCastingMaterials: readonly string[] = ['Mounter safety glass']

/** Whether a loaded mesh should cast a shadow, judged by every material slot it uses. */
export function castsShadow(materialNames: readonly string[]): boolean {
  return !materialNames.some(name => nonCastingMaterials.includes(name))
}
