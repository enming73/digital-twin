import { useEffect, useMemo, useRef, useState } from 'react'
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { overviewCamera, stations, type CameraPreset, type Station } from '../data/stations'
import { commonAnchors, stationAnchors, type StationId } from '../data/anchors'
import type { MachineSnapshot } from '../simulation/engine'
import type { SceneModel } from '../twin/types'
import { machineMotion } from './motion'
import { anchorOffsets, applyAnchorOffsets, captureAnchors } from './anchors'
import { easeInOutCubic, lerpPreset, tweenProgress } from './cameraTween'
import { signalColor, signalIntensity } from './status'
import Floor from './Floor'
import LightRig, { EXPOSURE } from './lighting'
import { castsShadow } from './assetMaterials'

function Box({ position, size, color, metal = 0.25, name }: { position: [number, number, number]; size: [number, number, number]; color: string; metal?: number; name?: string }) {
  return <mesh name={name} position={position} castShadow receiveShadow><boxGeometry args={size} /><meshStandardMaterial color={color} metalness={metal} roughness={0.57} /></mesh>
}

function ProcessAssembly({ station, machine, time }: { station: Station; machine: MachineSnapshot; time: number }) {
  const { sweep, lift, cycle } = machineMotion(machine, time)
  switch (station.id) {
    case 'loader':
    case 'unloader':
      return <group name="Board_Lift"><Box position={[0, 1.32 + lift * 0.18, 0.98]} size={[0.75, 0.08, 0.12]} color="#b4c6ba" /><Box position={[0, 1.41 + lift * 0.18, 1.04]} size={[0.48, 0.035, 0.14]} color="#39715c" /></group>
    case 'printer':
      return <group name="Stencil_Squeegee"><Box position={[sweep * 0.48, 1.59, 0.99]} size={[0.1, 0.29, 0.12]} color="#b7ccc0" metal={0.75} /><Box position={[0, 1.76, 0.98]} size={[1.45, 0.04, 0.08]} color="#93a59b" metal={0.7} /></group>
    case 'spi':
    case 'aoi':
      return <group name={station.id === 'spi' ? 'SPI_Scanner' : 'AOI_Scanner'}><Box position={[sweep * 0.57, 1.59, 0.99]} size={[0.08, 0.27, 0.08]} color="#b8d8cb" /><Box position={[sweep * 0.57, 1.73, 1.02]} size={[0.19, 0.08, 0.12]} color="#819a8c" /></group>
    case 'mounter':
      return <group name="Gantry_X"><Box position={[0, 1.73, 0.98]} size={[1.5, 0.08, 0.1]} color="#a1b4a8" metal={0.7} /><group name="Placement_Head"><Box position={[sweep * 0.52, 1.56 - lift * 0.07, 1.02]} size={[0.2, 0.26, 0.13]} color="#b6c8bd" /><Box position={[sweep * 0.52, 1.38 - lift * 0.07, 1.03]} size={[0.06, 0.12, 0.08]} color="#83978a" /></group><group name="Feeders"><Box position={[-0.38 + lift * 0.08, 1.31, 1.01]} size={[0.85, 0.06, 0.15]} color="#748d7d" /></group></group>
    case 'reflow':
      return <group name="Reflow_Conveyor"><Box position={[-0.65 + cycle * 1.3, 1.56, 0.99]} size={[0.24, 0.08, 0.08]} color="#a0b5a7" /></group>
    default: return null
  }
}

function ProceduralShell({ selected, oven, statusColor = '#d6a35a' }: { selected: boolean; oven: boolean; statusColor?: string }) {
  return <>
    <group name="Cabinet">
      <Box position={[0, 0.52, 0]} size={[2.55, 1.04, 1.85]} color={selected ? '#c4d1ca' : '#d8ddda'} />
      <Box position={[0, 0.12, 0]} size={[2.62, 0.22, 1.92]} color="#626c69" />
      <Box position={[0, 1.45, 0]} size={[2.52, oven ? 0.85 : 0.72, 1.78]} color="#d4dad7" />
      <Box position={[0, 1.56, 0.91]} size={[1.65, 0.37, 0.035]} color={oven ? '#858f89' : '#405755'} metal={0.4} />
      <Box position={[0, 0.55, 0.94]} size={[2.22, 0.57, 0.025]} color="#c1c9c5" />
      <Box position={[0, 0.58, 0.96]} size={[0.025, 0.48, 0.03]} color="#8e9994" />
      <Box position={[0.9, 1.6, 0.95]} size={[0.25, 0.28, 0.06]} color="#526461" />
    </group>
    <group name="Conveyor"><Box position={[0, 1.01, 0]} size={[3.13, 0.12, 0.72]} color="#596660" metal={0.65} /><Box position={[0, 1.12, 0.43]} size={[3.13, 0.07, 0.07]} color="#9ba9a3" metal={0.8} /><Box position={[0, 1.12, -0.43]} size={[3.13, 0.07, 0.07]} color="#9ba9a3" metal={0.8} /></group>
    <group name="Signal_Tower"><Box position={[1.01, 1.94, -0.65]} size={[0.07, 0.38, 0.07]} color="#63706b" /><Box position={[1.01, 2.17, -0.65]} size={[0.12, 0.16, 0.12]} color={statusColor} /></group>
  </>
}

function SelectionRing() {
  return <mesh position={[0, 0.02, 0]} rotation={[-Math.PI / 2, 0, 0]}>
    <ringGeometry args={[1.12, 1.34, 48]} />
    <meshBasicMaterial color="#7fb08b" transparent opacity={0.6} side={THREE.DoubleSide} />
  </mesh>
}

function AssetShell({ station, machine, time, selected, state }: { station: Station; machine: MachineSnapshot | null; time: number; selected: boolean; state: string }) {
  const kind = station.id
  const [asset, setAsset] = useState<THREE.Group | null>(null)
  const rest = useRef<Map<string, THREE.Vector3>>(new Map())
  const lens = useRef<THREE.MeshStandardMaterial | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    let loaded: THREE.Group | null = null
    fetch(`${import.meta.env.BASE_URL}models/${kind}.glb`, { signal: controller.signal })
      .then(response => { if (!response.ok) throw new Error('Model unavailable'); return response.arrayBuffer() })
      .then(data => new GLTFLoader().parseAsync(data, ''))
      .then(gltf => {
        if (controller.signal.aborted) return
        loaded = gltf.scene
        loaded.traverse(child => {
          if (child instanceof THREE.Mesh) {
            const materials = Array.isArray(child.material) ? child.material : [child.material]
            child.castShadow = castsShadow(materials.map(material => material.name))
            child.receiveShadow = true
          }
        })
        loaded.getObjectByName('Signal_Lens')?.traverse(child => {
          if (child instanceof THREE.Mesh && child.material instanceof THREE.MeshStandardMaterial) {
            child.material = child.material.clone()
            lens.current = child.material
          }
        })
        rest.current = captureAnchors(loaded, stationAnchors[kind as StationId] ?? commonAnchors)
        setAsset(loaded)
      }).catch(() => { /* Procedural shell stays visible if a local asset fails. */ })
    return () => { controller.abort(); rest.current = new Map(); loaded?.traverse(child => { if (child instanceof THREE.Mesh) { child.geometry.dispose(); const materials = Array.isArray(child.material) ? child.material : [child.material]; materials.forEach(material => material.dispose()) } }) }
  }, [kind])
  useFrame(() => {
    if (!asset || rest.current.size === 0) return
    applyAnchorOffsets(asset, rest.current, machine ? anchorOffsets(kind, machineMotion(machine, time)) : {})
  })
  useEffect(() => {
    const material = lens.current
    if (!material) return
    const colour = new THREE.Color(signalColor(state))
    material.color.copy(colour)
    material.emissive.copy(colour)
    material.emissiveIntensity = signalIntensity(state)
  }, [state, asset])
  if (!asset) return <><ProceduralShell selected={selected} oven={kind === 'reflow'} statusColor={signalColor(state)} />{machine && <ProcessAssembly station={station} machine={machine} time={time} />}</>
  return <><primitive object={asset} />{selected && <SelectionRing />}</>
}

function Machine({ station, machine, time, selected, onSelect, status }: { station: Station; machine: MachineSnapshot | null; time: number; selected: boolean; onSelect: () => void; status: string }) {
  const label = useMemo(() => {
    const canvas = document.createElement('canvas'); canvas.width = 256; canvas.height = 64
    const ctx = canvas.getContext('2d')!
    ctx.fillStyle = '#eef1eb'; ctx.fillRect(0, 0, 256, 64)
    ctx.fillStyle = '#40574a'; ctx.font = 'bold 30px monospace'; ctx.textAlign = 'center'; ctx.textBaseline = 'middle'; ctx.fillText(station.short, 128, 32)
    return new THREE.CanvasTexture(canvas)
  }, [station.short])
  useEffect(() => () => label.dispose(), [label])
  return <group position={station.position} name={station.id} onClick={e => { e.stopPropagation(); onSelect() }}>
    <AssetShell station={station} machine={machine} time={time} selected={selected} state={status} />
    {!machine && <Box position={[1.01, 2.3, -0.65]} size={[0.14, 0.14, 0.14]} color={signalColor(status)} />}
    {!selected && <sprite position={[0, 2.28, 0]} scale={[1.1, 0.275, 1]}><spriteMaterial map={label} /></sprite>}
  </group>
}

function CameraRig({ station, resetKey }: { station: Station | null; resetKey: number }) {
  const { camera, controls } = useThree()
  const tween = useRef<{ from: CameraPreset; to: CameraPreset; elapsed: number; active: boolean } | null>(null)

  useEffect(() => {
    const orbit = controls && 'target' in controls ? (controls as unknown as { target: THREE.Vector3 }) : null
    tween.current = {
      from: {
        position: [camera.position.x, camera.position.y, camera.position.z],
        target: orbit ? [orbit.target.x, orbit.target.y, orbit.target.z] : [0, 0, 0],
      },
      to: station?.mechanismCamera ?? overviewCamera,
      elapsed: 0,
      active: true,
    }
  }, [camera, controls, station, resetKey])

  useEffect(() => {
    const orbit = controls as unknown as { addEventListener?: (name: string, handler: () => void) => void; removeEventListener?: (name: string, handler: () => void) => void } | null
    if (!orbit?.addEventListener || !orbit.removeEventListener) return
    const cancel = () => { if (tween.current) tween.current.active = false }
    orbit.addEventListener('start', cancel)
    return () => orbit.removeEventListener?.('start', cancel)
  }, [controls])

  useFrame((_, delta) => {
    const current = tween.current
    if (!current?.active) return
    current.elapsed += delta * 1000
    const { t, done } = tweenProgress(current.elapsed)
    const pose = lerpPreset(current.from, current.to, easeInOutCubic(t))
    camera.position.set(...pose.position)
    if (controls && 'target' in controls) {
      const orbit = controls as unknown as { target: THREE.Vector3; update: () => void }
      orbit.target.set(...pose.target); orbit.update()
    } else camera.lookAt(...pose.target)
    if (done) current.active = false
  })

  return <OrbitControls makeDefault minDistance={3} maxDistance={42} maxPolarAngle={Math.PI / 2.05} />
}

function Boards({ scene, selectedBoard, onSelectBoard }: { scene: SceneModel; selectedBoard: string | null; onSelectBoard: (id: string) => void }) {
  const ref = useRef<THREE.InstancedMesh>(null)
  const ids = useRef<string[]>([])
  useFrame(() => {
    if (!ref.current || !scene.boards.some(board => board.visual && board.visual.fromX !== board.visual.toX)) return
    const matrix = new THREE.Matrix4()
    scene.boards.forEach((board, index) => {
      if (!board.visual) return
      const fraction = Math.max(0, Math.min(1, (Date.now() - board.visual.startedAt) / 300))
      matrix.makeTranslation(board.visual.fromX + (board.visual.toX - board.visual.fromX) * fraction, 1.08, 0.12)
      ref.current!.setMatrixAt(index, matrix)
    })
    ref.current.instanceMatrix.needsUpdate = true
  })
  useEffect(() => {
    if (!ref.current) return
    const mesh = ref.current; const matrix = new THREE.Matrix4(); let count = 0
    ids.current = []
    const add = (id: string, x: number, z: number) => { matrix.makeTranslation(x, 1.08, z); mesh.setMatrixAt(count, matrix); mesh.setColorAt(count, new THREE.Color(id === selectedBoard ? '#e3b85a' : '#32715b')); ids.current.push(id); count++ }
    scene.boards.forEach(board => {
      const station = stations.find(item => item.id === board.stationId)
      if (!station) return
      const x = station.position[0]
      if (board.visual) add(board.id, board.visual.fromX, 0.12)
      else if (board.position === 'queued') add(board.id, x - 1.2 + board.queueIndex * 0.5, -0.18)
      else {
        const machine = scene.stations[board.stationId]?.localMotion
        const progress = !machine || machine.startedAtMs === null ? 1 : Math.max(0, Math.min(1, ((machine.faultAtMs ?? scene.simTimeMs) - machine.startedAtMs) / machine.processTimeMs))
        add(board.id, x - 0.55 + progress * 1.1, 0.12)
      }
    })
    mesh.count = count; mesh.instanceMatrix.needsUpdate = true; if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
  }, [scene, selectedBoard])
  return <instancedMesh ref={ref} args={[undefined, undefined, Math.max(1, scene.boards.length)]} name="PCB_Anchors" castShadow frustumCulled={false} onClick={e => { e.stopPropagation(); if (e.instanceId !== undefined && ids.current[e.instanceId]) onSelectBoard(ids.current[e.instanceId]) }}><boxGeometry args={[0.4, 0.045, 0.32]} /><meshStandardMaterial color="#ffffff" metalness={0.2} roughness={0.6} /></instancedMesh>
}

export default function Factory({ selected, onSelect, resetKey, scene, selectedBoard, onSelectBoard }: { selected: Station | null; onSelect: (station: Station) => void; resetKey: number; scene: SceneModel; selectedBoard: string | null; onSelectBoard: (id: string) => void }) {
  const boardStation = scene.boards.find(board => board.id === selectedBoard)?.stationId
  return <Canvas shadows dpr={[1, 1.5]} gl={{ toneMappingExposure: EXPOSURE }} camera={{ position: overviewCamera.position, fov: 40 }} fallback={<div className="canvas-fallback">3D view unavailable. WebGL is required to view the factory.</div>}>
    <color attach="background" args={['#dde4dd']} /><fog attach="fog" args={['#dde4dd', 45, 110]} />
    <LightRig />
    <Floor />
    {stations.map(station => <Machine key={station.id} station={station} machine={scene.stations[station.id]?.localMotion ?? null} status={scene.stations[station.id]?.state ?? 'UNKNOWN'} time={scene.simTimeMs} selected={selected?.id === station.id || boardStation === station.id} onSelect={() => onSelect(station)} />)}
    <Boards scene={scene} selectedBoard={selectedBoard} onSelectBoard={onSelectBoard} /><CameraRig station={selected} resetKey={resetKey} />
  </Canvas>
}
