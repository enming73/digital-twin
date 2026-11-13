import { useEffect, useMemo } from 'react'
import { useFrame, useThree } from '@react-three/fiber'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { N8AOPass } from 'n8ao'

/**
 * Screen-space ambient occlusion for the contact shadows under and inside the machines.
 * Renders through a composer itself, so this component takes over rendering (useFrame priority 1).
 */
export default function AmbientOcclusion() {
  const { gl, scene, camera, size, viewport } = useThree()
  const composer = useMemo(() => {
    const instance = new EffectComposer(gl)
    const pass = new N8AOPass(scene, camera, size.width, size.height)
    pass.configuration.aoRadius = 0.6
    pass.configuration.distanceFalloff = 0.8
    pass.configuration.intensity = 1.5
    pass.configuration.halfRes = true
    instance.addPass(pass)
    return instance
  }, [gl, scene, camera, size.width, size.height])

  useEffect(() => {
    composer.setPixelRatio(viewport.dpr)
    composer.setSize(size.width, size.height)
  }, [composer, size.width, size.height, viewport.dpr])

  useEffect(() => () => composer.dispose(), [composer])
  useFrame(() => composer.render(), 1)
  return null
}
