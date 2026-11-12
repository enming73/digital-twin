export type Vec3 = [number, number, number]
export type CameraPreset = { position: Vec3; target: Vec3 }
export type Station = { id: string; name: string; short: string; description: string; position: Vec3; bounds: Vec3; kind: string; camera: CameraPreset; mechanismCamera: CameraPreset }
const entries = [
  ['loader', 'PCB Loader', 'LOAD', 'Introduces bare boards to the line.', 'loader'],
  ['printer', 'Solder Paste Printer', 'PRINT', 'Conceptual stencil printing station, inspired by DEK equipment.', 'printer'],
  ['spi', 'Solder Paste Inspection', 'SPI', 'Inspects the deposited solder paste before placement.', 'inspection'],
  ['mounter', 'Pick & Place', 'PLACE', 'Conceptual component placement station, inspired by SIPLACE equipment.', 'mounter'],
  ['reflow', 'Reflow Oven', 'REFLOW', 'Generic thermal process for soldering placed components.', 'oven'],
  ['aoi', 'Automated Optical Inspection', 'AOI', 'Generic optical inspection of assembled boards.', 'inspection'],
  ['unloader', 'PCB Unloader', 'UNLOAD', 'Collects finished boards at the end of the line.', 'loader'],
]
// Each framing keeps the process window in the centre and stays outside OrbitControls' minDistance.
function mechanismFraming(x: number, kind: string): CameraPreset {
  if (kind === 'inspection') return { position: [x + 1.7, 2.15, 3.4], target: [x, 1.72, 0.9] }
  if (kind === 'oven') return { position: [x + 2.0, 2.05, 3.5], target: [x, 1.55, 0.95] }
  if (kind === 'loader') return { position: [x + 1.8, 1.9, 3.4], target: [x, 1.45, 0.95] }
  return { position: [x + 1.85, 1.95, 3.35], target: [x, 1.55, 0.9] }
}

export const stations: Station[] = entries.map(([id, name, short, description, kind], i) => {
  const x = (i - 3) * 3.15
  return {
    id, name, short, description, kind,
    position: [x, 0, 0],
    bounds: [2.6, 2.5, 2],
    camera: { position: [x + 3.6, 3.2, 5.3], target: [x, 1.05, 0] },
    mechanismCamera: mechanismFraming(x, kind),
  }
})
export const overviewCamera: CameraPreset = { position: [14, 12, 22], target: [0, 0.7, 0] }
export const boardPositions: Vec3[] = Array.from({ length: 20 }, (_, i) => [-10 + i * 1.05, 1.08, 0])
