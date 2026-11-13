import { useEffect, useMemo } from 'react'
import { useFrame, useThree } from '@react-three/fiber'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { SMAAPass } from 'three/examples/jsm/postprocessing/SMAAPass.js'
import { N8AOPass } from 'n8ao'
import { aoSupported } from './ao'

/**
 * Screen-space ambient occlusion for the contact shadows under and inside the machines.
 *
 * N8AO draws the scene into its own float targets, so three's tone mapping and MSAA do not apply:
 * gamma correction is left to the OutputPass, and SMAA replaces the lost multisampling. The composer
 * owns rendering (priority 1) and is built once; resizes only call `setSize`.
 */
export default function AmbientOcclusion() {
  const { gl, scene, camera, size, viewport } = useThree()
  const supported = aoSupported(gl.extensions)
  const composer = useMemo(() => {
    if (!supported) return null
    const instance = new EffectComposer(gl)
    const ao = new N8AOPass(scene, camera, size.width, size.height)
    ao.configuration.aoRadius = 1.0
    ao.configuration.distanceFalloff = 1.0
    ao.configuration.intensity = 2.5
    ao.configuration.halfRes = true
    ao.configuration.gammaCorrection = false
    instance.addPass(ao)
    instance.addPass(new SMAAPass())
    instance.addPass(new OutputPass())
    return instance
    // Built once per renderer/scene/camera; size changes go through setSize below.
  }, [gl, scene, camera, supported])

  useEffect(() => {
    if (!composer) {
      console.warn('Ambient occlusion disabled: the WebGL context has no float render targets.')
      document.body.dataset.renderer = 'direct'
      return
    }
    document.body.dataset.renderer = 'composed'
    composer.setPixelRatio(viewport.dpr)
    composer.setSize(size.width, size.height)
  }, [composer, size.width, size.height, viewport.dpr])

  useEffect(() => () => composer?.dispose(), [composer])
  useFrame(() => {
    if (composer) composer.render()
    else gl.render(scene, camera)
  }, 1)
  return null
}
