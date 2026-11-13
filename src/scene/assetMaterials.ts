/**
 * Blended glazing renders see-through but still writes an opaque silhouette into the shadow map,
 * which darkens the very cavity the observation windows exist to show. Station glazing materials
 * are named `<kind> safety glass`; the mounter's capitalised sample name stays supported.
 */
export function isGlazing(materialName: string): boolean {
  return materialName.toLowerCase().endsWith('safety glass')
}

/** Whether a loaded mesh should cast a shadow, judged by every material slot it uses. */
export function castsShadow(materialNames: readonly string[]): boolean {
  return !materialNames.some(isGlazing)
}
