import { MeshReflectorMaterial } from '@react-three/drei'

/** Polished epoxy floor: a blurred planar reflection, tuned to read as sheen rather than a mirror. */
export default function Floor() {
  return <mesh rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
    <planeGeometry args={[100, 100]} />
    <MeshReflectorMaterial
      resolution={256}
      blur={[420, 120]}
      mixBlur={1}
      mixStrength={1.6}
      roughness={0.95}
      depthScale={1.4}
      minDepthThreshold={0.6}
      maxDepthThreshold={2.2}
      color="#b6bfb7"
      metalness={0}
      mirror={0.16}
    />
  </mesh>
}
