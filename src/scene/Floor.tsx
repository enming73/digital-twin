import { MeshReflectorMaterial } from '@react-three/drei'

/** Polished epoxy floor: a blurred planar reflection, tuned to read as sheen rather than a mirror. */
export default function Floor() {
  return <mesh rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
    <planeGeometry args={[400, 400]} />
    <MeshReflectorMaterial
      resolution={512}
      blur={[700, 200]}
      mixBlur={1}
      mixStrength={1.1}
      roughness={0.95}
      depthScale={0.9}
      minDepthThreshold={0.2}
      maxDepthThreshold={1}
      color="#8f9a92"
      metalness={0}
      mirror={0.12}
    />
  </mesh>
}
