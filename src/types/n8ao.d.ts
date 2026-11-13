declare module 'n8ao' {
  import type { Camera, Scene } from 'three'
  import type { Pass } from 'three/examples/jsm/postprocessing/Pass.js'

  export class N8AOPass extends Pass {
    constructor(scene: Scene, camera: Camera, width?: number, height?: number)
    configuration: {
      aoSamples: number
      aoRadius: number
      distanceFalloff: number
      intensity: number
      halfRes: boolean
      depthAwareUpsampling: boolean
      transparencyAware: boolean
    }
  }
}
