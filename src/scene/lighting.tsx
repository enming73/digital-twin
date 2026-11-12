import { memo } from 'react'
import { Environment, Lightformer } from '@react-three/drei'

/** ACES exposure for the showroom rig; the single number to tune if the line reads too bright. */
export const EXPOSURE = 0.85

/** `?quality=low` skips the environment bake: the IBL is the dominant frame cost on weak hardware. */
export function iblEnabled(search: string): boolean {
  return new URLSearchParams(search).get('quality') !== 'low'
}

const softboxes = <>
  <color attach="background" args={['#2a3230']} />
  {/* overhead softbox, front fill, rear rim, plus one small hard source for highlights */}
  <Lightformer form="rect" intensity={2.4} position={[0, 10, 0]} scale={[14, 6, 1]} rotation={[Math.PI / 2, 0, 0]} color="#f2f6ef" />
  <Lightformer form="rect" intensity={0.9} position={[9, 4, 8]} scale={[8, 8, 1]} rotation={[0, -Math.PI / 4, 0]} color="#e8efe9" />
  <Lightformer form="rect" intensity={1.2} position={[-8, 5, -9]} scale={[10, 6, 1]} rotation={[0, Math.PI / 5, 0]} color="#dfe8e2" />
  <Lightformer form="rect" intensity={7} position={[-2.5, 3.2, 6]} scale={[2.4, 1.2, 1]} rotation={[0, 0.45, 0]} color="#ffffff" />
</>

/**
 * Authored environment baked once into a PMREM, plus a key that casts shadows.
 * No downloaded HDRI — the demo stays offline and every light is original.
 */
function LightRig() {
  const useIbl = iblEnabled(typeof window === 'undefined' ? '' : window.location.search)
  return <>
    {useIbl && <Environment resolution={256} frames={1} environmentIntensity={0.65}>{softboxes}</Environment>}
    <ambientLight intensity={0.12} />
    <directionalLight
      position={[6, 12, 8]}
      intensity={2.6}
      castShadow
      shadow-mapSize={[2048, 2048]}
      shadow-bias={-0.0004}
      shadow-normalBias={0.02}
      shadow-camera-left={-12}
      shadow-camera-right={12}
      shadow-camera-top={10}
      shadow-camera-bottom={-10}
    />
    <directionalLight position={[-7, 5, 6]} intensity={0.35} color="#d9e6da" />
    <directionalLight position={[0, 6, -10]} intensity={0.6} color="#eef3ea" />
  </>
}

export default memo(LightRig)
