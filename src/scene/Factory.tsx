import { useEffect, useMemo, useRef, useState } from 'react'
import { Canvas, useThree } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { overviewCamera, stations, type Station } from '../data/stations'
import type { MachineSnapshot, Snapshot } from '../simulation/engine'
import { machineMotion } from './motion'

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

function ProceduralShell({ selected, oven }: { selected: boolean; oven: boolean }) {
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
    <group name="Signal_Tower"><Box position={[1.01, 1.94, -0.65]} size={[0.07, 0.38, 0.07]} color="#63706b" /><Box position={[1.01, 2.17, -0.65]} size={[0.12, 0.16, 0.12]} color="#d6a35a" /></group>
  </>
}

function AssetShell({ kind, selected }: { kind: 'printer' | 'mounter'; selected: boolean }) {
  const [asset, setAsset] = useState<THREE.Group | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    let loaded: THREE.Group | null = null
    fetch(`${import.meta.env.BASE_URL}models/${kind}.glb`, { signal: controller.signal })
      .then(response => { if (!response.ok) throw new Error('Model unavailable'); return response.arrayBuffer() })
      .then(data => new GLTFLoader().parseAsync(data, ''))
      .then(gltf => {
        if (controller.signal.aborted) return
        loaded = gltf.scene
        loaded.traverse(child => { if (child instanceof THREE.Mesh) { child.castShadow = true; child.receiveShadow = true } })
        setAsset(loaded)
      }).catch(() => { /* Procedural shell stays visible if a local asset fails. */ })
    return () => { controller.abort(); loaded?.traverse(child => { if (child instanceof THREE.Mesh) { child.geometry.dispose(); const materials = Array.isArray(child.material) ? child.material : [child.material]; materials.forEach(material => material.dispose()) } }) }
  }, [kind])
  if (!asset) return <ProceduralShell selected={selected} oven={false} />
  return <><primitive object={asset} />{selected && <Box position={[0, .03, 0]} size={[2.76, .05, 2.05]} color="#92ae96" />}</>
}

function Machine({ station, machine, time, selected, onSelect }: { station: Station; machine: MachineSnapshot; time: number; selected: boolean; onSelect: () => void }) {
  const label = useMemo(() => {
    const canvas = document.createElement('canvas'); canvas.width = 256; canvas.height = 64
    const ctx = canvas.getContext('2d')!
    ctx.fillStyle = '#eef1eb'; ctx.fillRect(0, 0, 256, 64)
    ctx.fillStyle = '#40574a'; ctx.font = 'bold 30px monospace'; ctx.textAlign = 'center'; ctx.textBaseline = 'middle'; ctx.fillText(station.short, 128, 32)
    return new THREE.CanvasTexture(canvas)
  }, [station.short])
  useEffect(() => () => label.dispose(), [label])
  return <group position={station.position} name={station.id} onClick={e => { e.stopPropagation(); onSelect() }}>
    {station.id === 'printer' || station.id === 'mounter' ? <AssetShell kind={station.id} selected={selected} /> : <ProceduralShell selected={selected} oven={station.kind === 'oven'} />}
    <ProcessAssembly station={station} machine={machine} time={time} />
    <sprite position={[0, 2.28, 0]} scale={[1.1, 0.275, 1]}><spriteMaterial map={label} /></sprite>
  </group>
}

function CameraRig({ station, resetKey }: { station: Station | null; resetKey: number }) {
  const { camera, controls } = useThree()
  useEffect(() => {
    const preset = station?.camera ?? overviewCamera
    camera.position.set(...preset.position)
    if (controls && 'target' in controls) {
      const orbit = controls as unknown as { target: THREE.Vector3; update: () => void }
      orbit.target.set(...preset.target); orbit.update()
    } else camera.lookAt(...preset.target)
  }, [camera, controls, station, resetKey])
  return <OrbitControls makeDefault minDistance={3} maxDistance={42} maxPolarAngle={Math.PI / 2.05} />
}

function Boards({ snapshot, selectedBoard, onSelectBoard }: { snapshot: Snapshot; selectedBoard: string | null; onSelectBoard: (id: string) => void }) {
  const ref = useRef<THREE.InstancedMesh>(null)
  const ids = useRef<string[]>([])
  useEffect(() => {
    if (!ref.current) return
    const mesh = ref.current; const matrix = new THREE.Matrix4(); let count = 0
    ids.current = []
    const add = (id: string, x: number, z: number) => { matrix.makeTranslation(x, 1.08, z); mesh.setMatrixAt(count, matrix); mesh.setColorAt(count, new THREE.Color(id === selectedBoard ? '#e3b85a' : '#32715b')); ids.current.push(id); count++ }
    snapshot.stations.forEach((machine, index) => {
      const x = stations[index].position[0]
      machine.queue.forEach((id, i) => add(id, x - 1.2 + i * 0.5, -0.18))
      if (machine.boardId) {
        const progress = machine.startedAtMs === null ? 1 : Math.max(0, Math.min(1, ((machine.faultAtMs ?? snapshot.simTimeMs) - machine.startedAtMs) / machine.processTimeMs))
        add(machine.boardId, x - 0.55 + progress * 1.1, 0.12)
      }
    })
    mesh.count = count; mesh.instanceMatrix.needsUpdate = true; if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
  }, [snapshot, selectedBoard])
  return <instancedMesh ref={ref} args={[undefined, undefined, snapshot.quantity]} name="PCB_Anchors" castShadow frustumCulled={false} onClick={e => { e.stopPropagation(); if (e.instanceId !== undefined && ids.current[e.instanceId]) onSelectBoard(ids.current[e.instanceId]) }}><boxGeometry args={[0.4, 0.045, 0.32]} /><meshStandardMaterial color="#ffffff" metalness={0.2} roughness={0.6} /></instancedMesh>
}

export default function Factory({ selected, onSelect, resetKey, snapshot, selectedBoard, onSelectBoard }: { selected: Station | null; onSelect: (station: Station) => void; resetKey: number; snapshot: Snapshot; selectedBoard: string | null; onSelectBoard: (id: string) => void }) {
  const boardStation = snapshot.boards.find(board => board.id === selectedBoard)?.stationId
  return <Canvas shadows dpr={[1, 1.5]} camera={{ position: overviewCamera.position, fov: 40 }} fallback={<div className="canvas-fallback">3D view unavailable. WebGL is required to view the factory.</div>}>
    <color attach="background" args={['#d9ded9']} /><fog attach="fog" args={['#d9ded9', 35, 80]} />
    <ambientLight intensity={1.25} /><directionalLight position={[4, 12, 8]} intensity={2} castShadow shadow-mapSize={[1024, 1024]} shadow-camera-left={-18} shadow-camera-right={18} shadow-camera-top={15} shadow-camera-bottom={-15} />
    <mesh rotation={[-Math.PI / 2, 0, 0]} receiveShadow><planeGeometry args={[100, 100]} /><meshStandardMaterial color="#b8c1ba" roughness={0.95} /></mesh>
    <gridHelper args={[60, 60, '#9fa9a1', '#aab3ab']} position={[0, 0.005, 0]} />
    {stations.map((station, index) => <Machine key={station.id} station={station} machine={snapshot.stations[index]} time={snapshot.simTimeMs} selected={selected?.id === station.id || boardStation === station.id} onSelect={() => onSelect(station)} />)}
    <Boards snapshot={snapshot} selectedBoard={selectedBoard} onSelectBoard={onSelectBoard} /><CameraRig station={selected} resetKey={resetKey} />
  </Canvas>
}
