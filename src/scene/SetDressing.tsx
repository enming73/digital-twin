import { cableTray, floorGuides, trayPosts } from './setDressing'

/** Pilot set dressing: muted floor guides and one cable tray. Procedural, so it is cheap to reject. */
export default function SetDressing() {
  return <group name="Set_Dressing">
    {floorGuides().map((segment, index) => <mesh key={`guide-${index}`} position={segment.position}>
      <boxGeometry args={segment.size} />
      <meshStandardMaterial color="#b08a4f" roughness={0.85} metalness={0} />
    </mesh>)}
    {cableTray().map((segment, index) => <mesh key={`tray-${index}`} position={segment.position} castShadow>
      <boxGeometry args={segment.size} />
      <meshStandardMaterial color="#5b665f" roughness={0.6} metalness={0.4} />
    </mesh>)}
    {trayPosts().map((segment, index) => <mesh key={`post-${index}`} position={segment.position} castShadow>
      <boxGeometry args={segment.size} />
      <meshStandardMaterial color="#6d7770" roughness={0.55} metalness={0.5} />
    </mesh>)}
  </group>
}
